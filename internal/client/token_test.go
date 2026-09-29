package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAuthController issues numbered tokens and serves /api, which accepts only
// the most recent token and answers any other with errorCode -44112.
type fakeAuthController struct {
	mu        sync.Mutex
	issued    int
	expiresIn int64
	bodies    []string // the body each /api request arrived with
}

func (f *fakeAuthController) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/openapi/authorize/token") {
			f.issued++
			_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "msg": "Success.", "result": map[string]any{
				"accessToken": "tok-" + string(rune('0'+f.issued)), "expiresIn": f.expiresIn,
			}})
			return
		}
		b, _ := io.ReadAll(r.Body)
		f.bodies = append(f.bodies, string(b))
		if r.Header.Get("Authorization") != "AccessToken=tok-"+string(rune('0'+f.issued)) {
			_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": -44112, "msg": "The access token has expired."})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "msg": "Success."})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeAuthController) revoke() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issued++ // the controller moved on; the client's token is now stale
}

func call(t *testing.T, c *http.Client, url, body string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPatch, url, strings.NewReader(body))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var env map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&env)
	return env
}

// A token the controller rejects mid-run is renewed and the request retried
// with its body intact. On 0.14.0 the token was fetched once at configure time,
// so a run longer than its lifetime failed half way.
func TestAuthTransportRenewsARejectedToken(t *testing.T) {
	f := &fakeAuthController{expiresIn: 7200}
	srv := f.server(t)
	meta, err := New(context.Background(), Config{Host: srv.URL, ControllerID: "c", ClientID: "id", ClientSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	c := meta.Client.GetConfig().HTTPClient

	if env := call(t, c, srv.URL+"/api", `{"n":1}`); env["errorCode"] != float64(0) {
		t.Fatalf("first call: %v", env)
	}
	f.revoke()
	if env := call(t, c, srv.URL+"/api", `{"n":2}`); env["errorCode"] != float64(0) {
		t.Fatalf("call after the token was revoked: %v", env)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.issued != 3 {
		t.Fatalf("issued %d tokens; want the initial, the revocation and one renewal", f.issued)
	}
	if last := f.bodies[len(f.bodies)-1]; last != `{"n":2}` {
		t.Fatalf("the retried request carried body %q", last)
	}
}

// A token near its stated expiry is renewed before it is used.
func TestTokenSourceRenewsBeforeExpiry(t *testing.T) {
	f := &fakeAuthController{expiresIn: 600}
	srv := f.server(t)
	now := time.Unix(0, 0)
	s := &tokenSource{host: srv.URL, controllerID: "c", clientID: "id", clientSecret: "s", http: srv.Client(),
		now: func() time.Time { return now }}

	first, err := s.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(4 * time.Minute) // 6 minutes left: still usable
	if again, _ := s.token(context.Background()); again != first {
		t.Fatalf("renewed too early: %q -> %q", first, again)
	}
	now = now.Add(2 * time.Minute) // 4 minutes left: inside renewBefore
	if renewed, _ := s.token(context.Background()); renewed == first {
		t.Fatal("did not renew a token about to expire")
	}
}
