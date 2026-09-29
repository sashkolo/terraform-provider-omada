package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Token lifetime handling. The controller states the lifetime in expiresIn
// (7200 s on 6.2.10); the token is renewed renewBefore ahead of that.
const (
	defaultTokenLifetime = 7200 * time.Second
	renewBefore          = 5 * time.Minute
)

// Open API error codes for a token the controller no longer accepts: -44112
// "The access token has expired" and -44113 "The access token is invalid".
var tokenRejectedCodes = map[int32]bool{-44112: true, -44113: true}

// tokenSource holds the current access token and renews it through the Open
// API's client-credentials grant.
type tokenSource struct {
	host, controllerID, clientID, clientSecret string
	http                                       *http.Client

	mu      sync.Mutex
	current string
	expiry  time.Time
	now     func() time.Time // for tests; time.Now when nil
}

func (s *tokenSource) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// token returns a token that is valid for at least renewBefore, fetching a new
// one when needed.
func (s *tokenSource) token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != "" && s.clock().Before(s.expiry.Add(-renewBefore)) {
		return s.current, nil
	}
	return s.fetchLocked(ctx)
}

// invalidate drops tok if it is still the current token, so the next call to
// token fetches a new one. Comparing first means concurrent requests that all
// saw the same rejected token renew it once.
func (s *tokenSource) invalidate(tok string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == tok {
		s.current = ""
	}
}

func (s *tokenSource) fetchLocked(ctx context.Context) (string, error) {
	body, err := json.Marshal(map[string]string{
		"omadacId":      s.controllerID,
		"client_id":     s.clientID,
		"client_secret": s.clientSecret,
	})
	if err != nil {
		return "", err
	}
	url := strings.TrimRight(s.host, "/") + "/openapi/authorize/token?grant_type=client_credentials"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting an access token: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading the access token response (HTTP %d): %w", resp.StatusCode, err)
	}

	var env struct {
		ErrorCode *int32 `json:"errorCode"`
		Msg       string `json:"msg"`
		Result    *struct {
			AccessToken *string `json:"accessToken"`
			ExpiresIn   *int64  `json:"expiresIn"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", fmt.Errorf("the access token response is not JSON (HTTP %d): %w", resp.StatusCode, err)
	}
	// Surface the controller's own reason; the SDK call this replaces reported
	// only the HTTP status.
	if env.ErrorCode != nil && *env.ErrorCode != 0 {
		return "", fmt.Errorf("the controller refused the access token request, error code %d: %s", *env.ErrorCode, env.Msg)
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("the access token request failed with HTTP %d: %s", resp.StatusCode, env.Msg)
	}
	if env.Result == nil {
		return "", fmt.Errorf("token response missing result")
	}
	if env.Result.AccessToken == nil || *env.Result.AccessToken == "" {
		return "", fmt.Errorf("token response missing access token")
	}

	lifetime := defaultTokenLifetime
	if env.Result.ExpiresIn != nil && *env.Result.ExpiresIn > 0 {
		lifetime = time.Duration(*env.Result.ExpiresIn) * time.Second
	}
	s.current = *env.Result.AccessToken
	s.expiry = s.clock().Add(lifetime)
	return s.current, nil
}

// authTransport adds the access token to every request. When the controller
// rejects the token (HTTP 401, or errorCode -44112/-44113), it renews the
// token and retries the request once.
type authTransport struct {
	base   http.RoundTripper
	tokens *tokenSource
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tok, err := t.tokens.token(req.Context())
	if err != nil {
		return nil, err
	}
	resp, err := t.base.RoundTrip(withToken(req, tok))
	if err != nil || !tokenRejected(resp) {
		return resp, err
	}

	// The body can only be replayed when the request can produce it again.
	if req.Body != nil && req.GetBody == nil {
		return resp, nil
	}
	retry := withToken(req, "")
	if req.GetBody != nil {
		b, err := req.GetBody()
		if err != nil {
			// Can't replay the body: hand back the controller's rejection
			// itself, which the caller reports, rather than an unrelated error.
			return resp, nil //nolint:nilerr // the rejection is the result
		}
		retry.Body = b
	}
	_ = resp.Body.Close()

	t.tokens.invalidate(tok)
	fresh, err := t.tokens.token(req.Context())
	if err != nil {
		return nil, err
	}
	retry.Header.Set("Authorization", "AccessToken="+fresh)
	return t.base.RoundTrip(retry)
}

func withToken(req *http.Request, tok string) *http.Request {
	r := req.Clone(req.Context())
	if tok != "" {
		r.Header.Set("Authorization", "AccessToken="+tok)
	}
	return r
}

// tokenRejected reports a response that rejects the access token. It reads the
// body to find the errorCode and puts it back for the caller.
func tokenRejected(resp *http.Response) bool {
	if resp.StatusCode == http.StatusUnauthorized {
		return true
	}
	if resp.Body == nil || !strings.Contains(resp.Header.Get("Content-Type"), "json") {
		return false
	}
	raw, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil {
		return false
	}
	var env struct {
		ErrorCode *int32 `json:"errorCode"`
	}
	if json.Unmarshal(raw, &env) != nil || env.ErrorCode == nil {
		return false
	}
	return tokenRejectedCodes[*env.ErrorCode]
}
