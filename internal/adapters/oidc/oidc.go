package oidc

import (
	"fmt"
	"net/url"

	"backend-challenge/internal/config"
)

type Config struct {
	Issuer   *url.URL
	Audience string
}

func NewConfig(cfg config.Config) (Config, error) {
	issuer, err := url.Parse(cfg.OIDCIssuer)
	if err != nil || issuer.Scheme == "" || issuer.Host == "" {
		return Config{}, fmt.Errorf("invalid OIDC_ISSUER")
	}
	if cfg.OIDCAudience == "" {
		return Config{}, fmt.Errorf("OIDC_AUDIENCE must not be empty")
	}
	return Config{Issuer: issuer, Audience: cfg.OIDCAudience}, nil
}
