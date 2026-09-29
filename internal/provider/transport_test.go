package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func tlsServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(srv.Certificate().Raw)
	return srv, hex.EncodeToString(sum[:])
}

// A pinned controller certificate is accepted without tls_skip_verify, in the
// colon-separated upper-case form openssl prints (homelab #516).
func TestBuildHTTPClientAcceptsThePinnedCertificate(t *testing.T) {
	srv, pin := tlsServer(t, func(w http.ResponseWriter, _ *http.Request) {})
	var colons []string
	for i := 0; i < len(pin); i += 2 {
		colons = append(colons, strings.ToUpper(pin[i:i+2]))
	}

	c, err := buildHTTPClient(tlsOptions{serverSHA256: strings.Join(colons, ":")}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("pinned certificate rejected: %v", err)
	}
	_ = resp.Body.Close()
}

// Any other certificate is refused, even with tls_skip_verify set: the pin
// replaces it.
func TestBuildHTTPClientRefusesAnotherCertificate(t *testing.T) {
	srv, _ := tlsServer(t, func(w http.ResponseWriter, _ *http.Request) {})
	other := strings.Repeat("ab", 32)

	c, err := buildHTTPClient(tlsOptions{skipVerify: true, serverSHA256: other}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(srv.URL); err == nil || !strings.Contains(err.Error(), "does not match tls_server_sha256") {
		t.Fatalf("err = %v, want a pin mismatch", err)
	}
}

func TestBuildHTTPClientRejectsAMalformedPin(t *testing.T) {
	if _, err := buildHTTPClient(tlsOptions{serverSHA256: "not-a-fingerprint"}, time.Second); err == nil {
		t.Fatal("accepted a malformed fingerprint")
	}
}

// Without a pin, CA or tls_skip_verify, the self-signed certificate is refused.
func TestBuildHTTPClientVerifiesByDefault(t *testing.T) {
	srv, _ := tlsServer(t, func(w http.ResponseWriter, _ *http.Request) {})
	c, err := buildHTTPClient(tlsOptions{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(srv.URL); err == nil {
		t.Fatal("a self-signed certificate was accepted without a pin")
	}
}

// A controller that stops answering fails the request at the timeout instead
// of hanging the run (and the state lock) forever. On 0.14.0 there was no
// timeout at all.
func TestBuildHTTPClientTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv, pin := tlsServer(t, func(w http.ResponseWriter, _ *http.Request) { <-release })
	defer close(release)

	c, err := buildHTTPClient(tlsOptions{serverSHA256: pin}, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := c.Get(srv.URL); err == nil {
		t.Fatal("request to a hanging controller succeeded")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timed out after %s, want about 200ms", elapsed)
	}
}

func TestBuildHTTPClientDefaultsTheTimeout(t *testing.T) {
	c, err := buildHTTPClient(tlsOptions{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Timeout != defaultRequestTimeout {
		t.Fatalf("timeout = %s, want %s", c.Timeout, defaultRequestTimeout)
	}
}
