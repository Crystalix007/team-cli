package gql

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var ErrUnexpected = errors.New("unexpected error")

type wsMessage struct {
	Type    string   `json:"type"`
	Payload *Payload `json:"payload,omitempty"`
	ID      string   `json:"id,omitempty"`
}

type Payload struct {
	Data       json.RawMessage    `json:"data,omitempty"`
	Extensions *PayloadExtensions `json:"extensions,omitempty"`
	Errors     []*wsError         `json:"errors,omitempty"`
}

func (p *Payload) UnmarshalData(tgt any) error {
	return json.Unmarshal(p.Data, tgt)
}

type PayloadExtensions struct {
	Authorization map[string]string `json:"authorization"`
}

type wsError struct {
	ErrorType string `json:"errorType"`
}

type Request struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

func Execute(
	ctx context.Context,
	endpoint string,
	accessToken string,
	req *Request,
) (*Payload, error) {
	ctx, cancelTimeout := context.WithTimeout(ctx, time.Second*30)
	defer cancelTimeout()

	enc, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("could not marshal request: %w", err)
	}

	r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(enc))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	r.Header.Add("Content-Type", "application/json")
	r.Header.Add("Authorization", accessToken)

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	defer resp.Body.Close()

	rawEnc, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: unexpected status code: %d %q", ErrUnexpected, resp.StatusCode, string(rawEnc))
	}

	var payload *Payload

	if err := json.Unmarshal(rawEnc, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload body: %w", err)
	}

	return payload, nil
}

type wsSubscriber struct {
	ws      *websocket.Conn
	authExt map[string]string
	reqID   uuid.UUID
}

func Subscribe(
	ctx context.Context,
	endpoint string,
	accessToken string,
	subscription *Request,
	onReady func(ctx context.Context) error,
	onData func(ctx context.Context, payload *Payload) (bool, error),
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("unable to parse endpoint %s: %w", endpoint, err)
	}

	authExt := map[string]string{
		"host":          u.Hostname(),
		"Authorization": accessToken,
	}

	endpoint = GenerateWSAddr(u)

	slog.Debug("Connecting to websocket", "endpoint", endpoint)

	encAuth, err := json.Marshal(authExt)
	if err != nil {
		return fmt.Errorf("failed to marshal auth data: %w", err)
	}

	subprotocol := `header-` + strings.ReplaceAll(base64.URLEncoding.EncodeToString(encAuth), "=", "")

	ws, _, err := websocket.DefaultDialer.DialContext(
		ctx,
		endpoint,
		http.Header{"sec-websocket-protocol": []string{"graphql-ws", subprotocol}},
	)
	if err != nil {
		return fmt.Errorf("failed to dial websocket: %w", err)
	}

	defer ws.Close()

	go func() {
		select {
		case <-ctx.Done():
			_ = ws.Close()
		}
	}()

	wss := &wsSubscriber{
		ws:      ws,
		authExt: authExt,
		reqID:   uuid.New(),
	}

	if err := wss.initConnection(); err != nil {
		return fmt.Errorf("failed to init connection: %w", err)
	}

	slog.Debug("Websocket initialized")

	if err := wss.start(subscription); err != nil {
		return fmt.Errorf("failed to start subscription: %w", err)
	}

	slog.Debug("Websocket subscription ready")

	if err := onReady(ctx); err != nil {
		return fmt.Errorf("onReady error: %w", err)
	}

	if err := wss.process(onData); err != nil {
		return fmt.Errorf("failed to process subscription: %w", err)
	}

	return nil
}

func GenerateWSAddr(u *url.URL) string {
	if strings.Contains(u.Host, ".appsync-api.") && strings.Contains(u.Host, ".amazonaws.") {
		u.Host = strings.Replace(u.Host, ".appsync-api.", ".appsync-realtime-api.", 1)
	} else {
		u.Path += "/realtime"
	}

	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}

	return u.String()
}

func (s *wsSubscriber) initConnection() error {
	if err := s.send(&wsMessage{Type: "connection_init"}); err != nil {
		return fmt.Errorf("failed to send connection_init: %w", err)
	}

	for {
		pkt, err := s.read()
		if err != nil {
			return fmt.Errorf("failed to read packet: %w", err)
		}

		switch pkt.Type {
		case "connection_ack":
			return nil
		case "connection_error":
			return fmt.Errorf("%w: connection error: %q", ErrUnexpected, pkt.Payload)
		default:
			slog.Warn("Received unexpected packet", "type", pkt.Type)
		}
	}
}

func (s *wsSubscriber) start(subscription *Request) error {
	encSubscription, err := json.Marshal(subscription)
	if err != nil {
		return fmt.Errorf("failed to marshal subscription: %w", err)
	}

	wrappedSubscription, err := json.Marshal(string(encSubscription))
	if err != nil {
		return fmt.Errorf("failed to marshal wrapped subscription: %w", err)
	}

	if err := s.send(&wsMessage{
		Type: "start",
		ID:   s.reqID.String(),
		Payload: &Payload{
			Data: wrappedSubscription,
			Extensions: &PayloadExtensions{
				Authorization: s.authExt,
			},
		},
	}); err != nil {
		return fmt.Errorf("failed to send connection_init: %w", err)
	}

	for {
		pkt, err := s.read()
		if err != nil {
			return fmt.Errorf("failed to read packet: %w", err)
		}

		switch pkt.Type {
		case "ka":
		// Ignore keep-alives
		case "error":
			for _, err := range pkt.Payload.Errors {
				slog.Warn("Received websocket error", "error", err)
			}

			return fmt.Errorf("%w: websocket error", ErrUnexpected)
		case "start_ack":
			if pkt.ID != s.reqID.String() {
				slog.Warn("Received unexpected start_ack", "got", pkt.ID, "expected", s.reqID.String())

				continue
			}

			return nil
		default:
			slog.Warn("Received unexpected packet", "type", pkt.Type)
		}
	}
}

func (s *wsSubscriber) process(onData func(ctx context.Context, payload *Payload) (bool, error)) error {
	for {
		pkt, err := s.read()
		if err != nil {
			return fmt.Errorf("failed to read packet: %w", err)
		}

		switch pkt.Type {
		case "ka":
		// Ignore keep-alives
		case "error":
			for _, err := range pkt.Payload.Errors {
				slog.Warn("Received websocket error", "error", err)
			}

			return fmt.Errorf("%w: websocket error", ErrUnexpected)
		case "data":
			if pkt.ID != s.reqID.String() {
				slog.Warn("Received unexpected data packet", "got", pkt.ID, "expected", s.reqID.String())

				continue
			}

			slog.Debug("Received data packet", "data", string(pkt.Payload.Data))

			cont, err := onData(context.Background(), pkt.Payload)
			if err != nil {
				return fmt.Errorf("failed to process data packet: %w", err)
			}

			if !cont {
				slog.Debug("Data handler requested exit")

				return nil
			}
		default:
			slog.Warn("Received unexpected packet", "type", pkt.Type)
		}
	}
}

func (s *wsSubscriber) read() (*wsMessage, error) {
	if err := s.ws.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		return nil, fmt.Errorf("failed to set read deadline: %w", err)
	}

	var res *wsMessage

	if err := s.ws.ReadJSON(&res); err != nil {
		return res, fmt.Errorf("failed to read JSON: %w", err)
	}

	return res, nil
}

func (s *wsSubscriber) send(msg *wsMessage) error {
	if err := s.ws.SetWriteDeadline(time.Now().Add(time.Second * 10)); err != nil {
		return fmt.Errorf("failed to set write deadline: %w", err)
	}

	if err := s.ws.WriteJSON(msg); err != nil {
		return fmt.Errorf("failed to write JSON: %w", err)
	}

	return nil
}
