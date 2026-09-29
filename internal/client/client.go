package client

import (
	"context"
	"net/http"

	"github.com/Tohaker/omada-go-sdk/omada"
)

type Config struct {
	Host, ControllerID, ClientID, ClientSecret string
	// HTTPClient carries the TLS settings and timeout. Its transport is wrapped
	// with one that adds (and renews) the access token; it is not modified.
	HTTPClient *http.Client
}

type Meta struct {
	Client   *omada.APIClient
	OmadacId string
}

// New fetches a first access token, so bad credentials fail at configure time,
// and returns an SDK client whose every request carries a current token. The
// token is renewed before it expires and once more if the controller reports it
// expired or invalid, so a plan and apply longer than the token's lifetime
// (7200 s on 6.2.10) no longer fails half way.
func New(ctx context.Context, cfg Config) (*Meta, error) {
	base := cfg.HTTPClient
	if base == nil {
		base = &http.Client{}
	}

	tokens := &tokenSource{
		host:         cfg.Host,
		controllerID: cfg.ControllerID,
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		http:         base,
	}
	if _, err := tokens.token(ctx); err != nil {
		return nil, err
	}

	authed := *base
	authed.Transport = &authTransport{base: transportOf(base), tokens: tokens}

	config := omada.NewConfiguration()
	config.Servers = omada.ServerConfigurations{
		{URL: cfg.Host},
	}
	config.HTTPClient = &authed

	return &Meta{
		Client:   omada.NewAPIClient(config),
		OmadacId: cfg.ControllerID,
	}, nil
}

func transportOf(c *http.Client) http.RoundTripper {
	if c.Transport != nil {
		return c.Transport
	}
	return http.DefaultTransport
}
