package team

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

//go:embed auth.html
var closePageSrc string

const localhostRedir = "http://localhost:43672/"

type AuthToken struct {
	IdToken      string    `json:"id_token"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
}

type IDToken struct {
	UserID   string `json:"userId"`
	GroupIDs string `json:"groupIds"`
	Email    any    `json:"email"`
}

func (t *AuthToken) ParseIDToken() (*IDToken, error) {
	parts := strings.Split(t.IdToken, ".")

	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: invalid format", ErrUnexpected)
	}

	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode: %w", err)
	}

	var out *IDToken

	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	return out, nil
}

type rawAuthToken struct {
	IdToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func FetchTokenViaDeviceCode(
	ctx context.Context,
	cfg *RemoteConfig,
	readCode func(context.Context) (string, error),
) (*AuthToken, error) {
	slog.Info("Fetching authentication token")

	state := randomCharacters(32)
	pkceKey, challenge := generateChallenge()

	redirUri := cfg.RedirectSignIn + "device_code/"

	params := url.Values{
		"redirect_uri":  {redirUri},
		"response_type": {cfg.OAuthResponseType},
		"client_id":     {cfg.UserPoolClientID},
		"scope":         {strings.Join(cfg.OAuthScopes, " ")},
		"state":         {state},
	}

	if cfg.OAuthResponseType == "code" {
		params.Add("code_challenge", challenge)
		params.Add("code_challenge_method", "S256")
	}

	u := url.URL{
		Scheme:   "https",
		Host:     cfg.OAuthDomain,
		Path:     "/oauth2/authorize",
		RawQuery: params.Encode(),
	}

	fmt.Println("\nPlease visit the following URL in your browser to authenticate:")
	fmt.Println(u.String())

	code, err := readCode(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not read code: %w", err)
	}

	u = url.URL{
		Scheme: "https",
		Host:   cfg.OAuthDomain,
		Path:   "/oauth2/token",
	}

	data := make(url.Values)
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("client_id", cfg.UserPoolClientID)
	data.Set("redirect_uri", redirUri)
	data.Set("code_verifier", pkceKey)

	return fetchToken(ctx, u, data)
}

func FetchToken(ctx context.Context, cfg *RemoteConfig, noBrowser bool) (*AuthToken, error) {
	slog.Info("Fetching authentication token")

	codeChan := make(chan string, 1)

	hs := &http.Server{
		Addr: ":43672",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			params := r.URL.Query()

			code := params.Get("code")
			if code != "" {
				slog.Debug("Got code from challenge", "code", code)

				select {
				case codeChan <- code:
				default:
					slog.Warn("Failed to send code")
				}
			}

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(closePageSrc))
		}),
	}

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()

		if err := hs.Shutdown(ctx); err != nil {
			slog.Warn("failed to shutdown http server", "err", err)
		}
	}()

	ctx, cancel := context.WithCancelCause(ctx)

	go func() {
		cancel(hs.ListenAndServe())
	}()

	state := randomCharacters(32)
	pkceKey, challenge := generateChallenge()

	redirUri := localhostRedir

	params := url.Values{
		"redirect_uri":  {redirUri},
		"response_type": {cfg.OAuthResponseType},
		"client_id":     {cfg.UserPoolClientID},
		"scope":         {strings.Join(cfg.OAuthScopes, " ")},
		"state":         {state},
	}

	if cfg.OAuthResponseType == "code" {
		params.Add("code_challenge", challenge)
		params.Add("code_challenge_method", "S256")
	}

	u := url.URL{
		Scheme:   "https",
		Host:     cfg.OAuthDomain,
		Path:     "/oauth2/authorize",
		RawQuery: params.Encode(),
	}

	fmt.Println("\nPlease visit the following URL in your browser to authenticate:")
	fmt.Println(u.String())

	if !noBrowser {
		if err := openBrowser(u.String()); err != nil {
			slog.Warn("failed to open browser", "err", err)
		}
	}

	var code string

	select {
	case code = <-codeChan:
		// ok
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(time.Minute * 5):
		slog.Info("Timeout waiting for challenge")

		return nil, errors.New("timeout waiting for challenge")
	}

	u = url.URL{
		Scheme: "https",
		Host:   cfg.OAuthDomain,
		Path:   "/oauth2/token",
	}

	data := make(url.Values)
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("client_id", cfg.UserPoolClientID)
	data.Set("redirect_uri", redirUri)
	data.Set("code_verifier", pkceKey)

	return fetchToken(ctx, u, data)
}

func RefreshToken(ctx context.Context, remote *RemoteConfig, old *AuthToken) (*AuthToken, error) {
	u := url.URL{
		Scheme: "https",
		Host:   remote.OAuthDomain,
		Path:   "/oauth2/token",
	}

	data := make(url.Values)
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", remote.UserPoolClientID)
	data.Set("refresh_token", old.RefreshToken)

	return fetchToken(ctx, u, data)
}

func fetchToken(ctx context.Context, u url.URL, data url.Values) (*AuthToken, error) {
	now := time.Now()

	ctx, cancelTimeout := context.WithTimeout(ctx, time.Second*30)
	defer cancelTimeout()

	r, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, fmt.Errorf("failed to send token request: %w", err)
	}

	defer resp.Body.Close()

	rawEnc, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: unexpected token status code: %d %q", ErrUnexpected, resp.StatusCode, string(rawEnc))
	}

	var token *rawAuthToken

	if err := json.Unmarshal(rawEnc, &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token body: %w", err)
	}

	return &AuthToken{
		IdToken:      token.IdToken,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    now.Add(time.Duration(token.ExpiresIn) * time.Second),
		TokenType:    token.TokenType,
	}, nil
}

var randChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randomCharacters(l int) string {
	out := make([]byte, l)

	for i := 0; i < l; i++ {
		out[i] = randChars[rand.IntN(len(randChars))]
	}

	return string(out)
}

func generateChallenge() (string, string) {
	chars := randomCharacters(32)

	challenge := base64.RawURLEncoding.EncodeToString([]byte(chars))

	hash := sha256.Sum256([]byte(challenge))

	encoded := base64.RawURLEncoding.EncodeToString(hash[:])

	return challenge, encoded
}

func openBrowser(url string) error {
	var (
		cmd  string
		args []string
	)

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}
