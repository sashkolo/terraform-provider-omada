package provider

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// defaultRequestTimeout bounds each controller request. Without a timeout, a
// controller that stops answering held the run, and the state lock, forever.
const defaultRequestTimeout = 60 * time.Second

// tlsOptions is how the provider trusts the controller's certificate.
type tlsOptions struct {
	// skipVerify accepts any certificate. Superseded by serverSHA256.
	skipVerify bool
	// serverSHA256 pins the controller's leaf certificate by the SHA-256 of
	// its DER encoding (hex, colons and case ignored). It suits the
	// controller's self-signed certificate, which a CA check can't verify for
	// an IP address, and it replaces skipVerify: the chain isn't checked, but
	// only that one certificate is accepted.
	serverSHA256 string
	// caCertPEM trusts these CA certificates instead of the system roots,
	// with normal chain and host-name verification.
	caCertPEM string
}

func buildHTTPClient(opts tlsOptions, timeout time.Duration) (*http.Client, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}

	if opts.caCertPEM != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(opts.caCertPEM)) {
			return nil, errors.New("ca_cert_pem holds no PEM certificate")
		}
		cfg.RootCAs = pool
	}

	switch {
	case opts.serverSHA256 != "":
		pin, err := parseFingerprint(opts.serverSHA256)
		if err != nil {
			return nil, err
		}
		// Chain verification is replaced by the pin, which VerifyConnection
		// checks on every handshake.
		cfg.InsecureSkipVerify = true
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("controller presented no certificate")
			}
			got := sha256.Sum256(cs.PeerCertificates[0].Raw)
			if subtle.ConstantTimeCompare(got[:], pin) != 1 {
				return fmt.Errorf("controller certificate SHA-256 %s does not match tls_server_sha256",
					hex.EncodeToString(got[:]))
			}
			return nil
		}
	case opts.skipVerify:
		cfg.InsecureSkipVerify = true
	}

	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{TLSClientConfig: cfg, Proxy: http.ProxyFromEnvironment},
	}, nil
}

// parseFingerprint accepts a SHA-256 fingerprint as 64 hex digits, with or
// without colons, in either case (the forms openssl and browsers print).
func parseFingerprint(s string) ([]byte, error) {
	clean := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), ":", ""))
	b, err := hex.DecodeString(clean)
	if err != nil || len(b) != sha256.Size {
		return nil, fmt.Errorf("tls_server_sha256 must be a SHA-256 fingerprint (64 hex digits), got %q", s)
	}
	return b, nil
}
