package oidc

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Identity struct {
	Subject          string
	ProviderID       string
	Issuer           string
	AuthorizedClient string
}
type identityContextKey struct{}

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityContextKey{}, identity)
}
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	value, ok := ctx.Value(identityContextKey{}).(Identity)
	return value, ok
}

type Verifier struct {
	config  Config
	client  *http.Client
	mu      sync.RWMutex
	jwksURL string
	keys    map[string]*rsa.PublicKey
}

func NewVerifier(config Config) *Verifier {
	return &Verifier{config: config, client: &http.Client{Timeout: 5 * time.Second}, keys: map[string]*rsa.PublicKey{}}
}

func (v *Verifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, err := v.Verify(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			writeAuthError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), identity)))
	})
}
func (v *Verifier) Verify(ctx context.Context, authorization string) (Identity, error) {
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return Identity{}, errors.New("bearer token required")
	}
	segments := strings.Split(parts[1], ".")
	if len(segments) != 3 {
		return Identity{}, errors.New("invalid JWT")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodeJSON(segments[0], &header); err != nil || header.Kid == "" || header.Alg != "RS256" {
		return Identity{}, errors.New("unsupported JWT header")
	}
	var claims struct {
		Iss string   `json:"iss"`
		Sub string   `json:"sub"`
		Aud audience `json:"aud"`
		Exp int64    `json:"exp"`
		Azp string   `json:"azp"`
	}
	if err := decodeJSON(segments[1], &claims); err != nil {
		return Identity{}, errors.New("invalid JWT claims")
	}
	if claims.Iss != v.config.Issuer.String() || claims.Sub == "" || (!claims.Aud.Contains(v.config.Audience) && claims.Azp != "wallet-internal") || claims.Exp <= time.Now().Unix() {
		return Identity{}, errors.New("JWT claims rejected")
	}
	key, err := v.key(ctx, header.Kid)
	if err != nil {
		return Identity{}, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(segments[2])
	if err != nil {
		return Identity{}, errors.New("invalid JWT signature")
	}
	digest := sha256.Sum256([]byte(segments[0] + "." + segments[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return Identity{}, errors.New("invalid JWT signature")
	}
	return Identity{Subject: claims.Sub, ProviderID: claims.Sub, Issuer: claims.Iss, AuthorizedClient: claims.Azp}, nil
}

type audience []string

func (a *audience) UnmarshalJSON(data []byte) error {
	var one string
	if json.Unmarshal(data, &one) == nil {
		*a = []string{one}
		return nil
	}
	return json.Unmarshal(data, (*[]string)(a))
}
func (a audience) Contains(value string) bool {
	for _, item := range a {
		if item == value {
			return true
		}
	}
	return false
}
func decodeJSON(segment string, target any) error {
	data, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

type discoveryDocument struct {
	JWKSURI string `json:"jwks_uri"`
}
type jwksDocument struct {
	Keys []jwk `json:"keys"`
}
type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
	Alg string `json:"alg"`
}

func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key := v.keys[kid]
	v.mu.RUnlock()
	if key != nil {
		return key, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(v.config.Issuer.String(), "/")+"/.well-known/openid-configuration", nil)
	if err != nil {
		return nil, err
	}
	response, err := v.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("OIDC discovery rejected")
	}
	var discovery discoveryDocument
	if err := json.NewDecoder(response.Body).Decode(&discovery); err != nil || discovery.JWKSURI == "" {
		return nil, errors.New("OIDC discovery has no jwks_uri")
	}
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, discovery.JWKSURI, nil)
	if err != nil {
		return nil, err
	}
	response, err = v.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("OIDC JWKS: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("OIDC JWKS rejected")
	}
	var document jwksDocument
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		return nil, errors.New("invalid OIDC JWKS")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, item := range document.Keys {
		if item.Kid == "" || item.Kty != "RSA" || item.Alg != "RS256" {
			continue
		}
		modulus, err := base64.RawURLEncoding.DecodeString(item.N)
		if err != nil {
			continue
		}
		exponent, err := base64.RawURLEncoding.DecodeString(item.E)
		if err != nil {
			continue
		}
		e := new(big.Int).SetBytes(exponent)
		if !e.IsInt64() {
			continue
		}
		parsed := &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: int(e.Int64())}
		if parsed.N.Sign() > 0 && parsed.E > 0 {
			v.keys[item.Kid] = parsed
		}
	}
	if key := v.keys[kid]; key != nil {
		return key, nil
	}
	return nil, errors.New("OIDC signing key not found")
}
func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
