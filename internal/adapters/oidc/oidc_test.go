package oidc

import (
	"testing"

	"backend-challenge/internal/config"
)

func TestNewConfig(t *testing.T) {
	cfg := config.Config{OIDCIssuer: "http://keycloak:8080/realms/wallet", OIDCAudience: "wallet-api"}
	got, err := NewConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Audience != "wallet-api" || got.Issuer.Host != "keycloak:8080" {
		t.Fatalf("unexpected config: %+v", got)
	}
}

func TestNewConfigRejectsInvalidIssuer(t *testing.T) {
	_, err := NewConfig(config.Config{OIDCIssuer: "not-an-url", OIDCAudience: "wallet-api"})
	if err == nil {
		t.Fatal("expected invalid issuer error")
	}
}
