package oidc

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestVerifierValidTokenAndSubjectProvider(t *testing.T) {
	verifier, token, server := testVerifier(t, "provider-sub", "wallet-api", time.Now().Add(time.Hour))
	defer server.Close()
	identity, err := verifier.Verify(serverContext(), "Bearer "+token)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Subject != "provider-sub" || identity.ProviderID != "provider-sub" || identity.AuthorizedClient != "wallet-api" {
		t.Fatalf("identity=%+v", identity)
	}
}
func TestVerifierRejectsInvalidIssuerAudienceExpirationAndSignature(t *testing.T) {
	cases := []struct {
		name, issuer, audience string
		expiry                 time.Time
	}{
		{"issuer", "http://wrong", "wallet-api", time.Now().Add(time.Hour)},
		{"audience", "", "wrong", time.Now().Add(time.Hour)},
		{"expired", "", "wallet-api", time.Now().Add(-time.Hour)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			verifier, token, server := testVerifier(t, "sub", test.audience, test.expiry)
			defer server.Close()
			if test.issuer != "" {
				token = signedToken(t, test.issuer, "sub", test.audience, test.expiry, testKey)
			}
			if _, err := verifier.Verify(serverContext(), "Bearer "+token); err == nil {
				t.Fatal("expected token rejection")
			}
		})
	}
	verifier, token, server := testVerifier(t, "sub", "wallet-api", time.Now().Add(time.Hour))
	defer server.Close()
	parts := strings.Split(token, ".")
	parts[2] = base64.RawURLEncoding.EncodeToString([]byte("bad"))
	if _, err := verifier.Verify(serverContext(), "Bearer "+strings.Join(parts, ".")); err == nil {
		t.Fatal("expected invalid signature")
	}
}

var testKey *rsa.PrivateKey

func testVerifier(t *testing.T, subject, audience string, expiry time.Time) (*Verifier, string, *httptest.Server) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	testKey = key
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			writeTestJSON(w, map[string]string{"jwks_uri": server.URL + "/keys"})
			return
		}
		writeTestJSON(w, map[string]any{"keys": []any{map[string]any{"kid": "test", "kty": "RSA", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1})}}})
	}))
	return NewVerifier(Config{Issuer: mustURL(server.URL), Audience: "wallet-api"}), signedToken(t, server.URL, subject, audience, expiry, key), server
}
func signedToken(t *testing.T, issuer, subject, audience string, expiry time.Time, key *rsa.PrivateKey) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString(mustJSON(map[string]any{"alg": "RS256", "kid": "test", "typ": "JWT"}))
	claims := base64.RawURLEncoding.EncodeToString(mustJSON(map[string]any{"iss": issuer, "sub": subject, "aud": audience, "azp": audience, "exp": expiry.Unix()}))
	input := header + "." + claims
	digest := sha256.Sum256([]byte(input))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(signature)
}
func mustJSON(value any) []byte { data, _ := json.Marshal(value); return data }
func writeTestJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func mustURL(value string) *url.URL  { parsed, _ := url.Parse(value); return parsed }
func serverContext() context.Context { return context.Background() }
