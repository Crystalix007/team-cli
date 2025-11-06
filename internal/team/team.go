package team

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

var (
	jsRegex    = regexp.MustCompile(`src="([\w./:_-]+\.js)"`)
	scopeRegex = regexp.MustCompile(`"([\w:/._-]+)"`)
)

var configExtractors = map[string]*regexp.Regexp{
	"aws_appsync_graphqlEndpoint":  regexp.MustCompile(`\Waws_appsync_graphqlEndpoint\W*:\W*"([\w:/._-]+)"`),
	"aws_user_pools_web_client_id": regexp.MustCompile(`\Waws_user_pools_web_client_id\W*:\W*"([\w:/._-]+)"`),
	"oauth_domain":                 regexp.MustCompile(`\Woauth\W*:.{0,999}.{0,999}.{0,999}.{0,999}\Wdomain\W*:\W*"([\w:/._-]+)"`),
	"oauth_responseType":           regexp.MustCompile(`\Woauth\W*:.{0,999}.{0,999}.{0,999}.{0,999}\WresponseType\W*:\W*"([\w:/._-]+)"`),
	"oauth_scope":                  regexp.MustCompile(`\Woauth\W*:.{0,999}.{0,999}.{0,999}.{0,999}\Wscope\W*:\W*\[(\W*(?:"[\w:/._-]+"\W*,?\W*)+)]`),
}

type RemoteConfig struct {
	GraphQLEndpoint   string   `json:"graphql_endpoint"`
	UserPoolClientID  string   `json:"user_pool_client_id"`
	OAuthDomain       string   `json:"oauth_domain"`
	OAuthResponseType string   `json:"oauth_response_type"`
	OAuthScopes       []string `json:"oauth_scopes"`
}

var ErrUnexpected = errors.New("unexpected error")

func ExtractConfig(ctx context.Context, addr string) (*RemoteConfig, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	server, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("could not parse server URL: %w", err)
	}

	if server.Scheme == "" {
		server.Scheme = "http"
	}

	slog.Info("Fetching homepage", "server", server)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: could not fetch homepage: %v", ErrUnexpected, resp.Status)
	}

	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response body: %w", err)
	}

	slog.Debug("Extracting homepage matches", "body", string(rawBody))

	matches := jsRegex.FindAllStringSubmatch(string(rawBody), -1)

	paths := make([]string, 0, len(matches))

	for _, match := range matches {
		slog.Debug("Found match", "match", match)

		if len(match) != 2 {
			continue
		}

		paths = append(paths, match[1])
	}

	if len(paths) != 1 {
		return nil, fmt.Errorf("%w: could find main JS file", ErrUnexpected)
	}

	jsURL, err := url.JoinPath(server.String(), paths[0])
	if err != nil {
		return nil, fmt.Errorf("could not combine path: %w", err)
	}

	slog.Info("Fetching main JS file", "file", jsURL)

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, jsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create js request: %w", err)
	}

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not send js request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: could not fetch js: %v", ErrUnexpected, resp.Status)
	}

	defer resp.Body.Close()

	rawBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response body: %w", err)
	}

	raw := make(map[string]string)

	for name, reg := range configExtractors {
		matches := reg.FindAllStringSubmatch(string(rawBody), -1)

		slog.Debug("Found matches", "name", name, "matches", matches)

		if len(matches) != 1 {
			return nil, fmt.Errorf("%w: could find extract %q (count=%v)", ErrUnexpected, name, len(matches))
		}

		raw[name] = matches[0][1]
	}

	slog.Debug("Extracted raw config", "raw", raw)

	matches = scopeRegex.FindAllStringSubmatch(raw["oauth_scope"], -1)

	scopes := make([]string, 0, len(matches))

	for _, match := range matches {
		slog.Debug("Found scope match", "match", match)

		if len(match) != 2 {
			return nil, fmt.Errorf("%w: invalid scope %q", ErrUnexpected, match[0])
		}

		scopes = append(scopes, match[1])
	}

	return &RemoteConfig{
		GraphQLEndpoint:   raw["aws_appsync_graphqlEndpoint"],
		UserPoolClientID:  raw["aws_user_pools_web_client_id"],
		OAuthDomain:       raw["oauth_domain"],
		OAuthResponseType: raw["oauth_responseType"],
		OAuthScopes:       scopes,
	}, nil
}
