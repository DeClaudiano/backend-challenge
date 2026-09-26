package httpadapter

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"backend-challenge/internal/adapters/oidc"
)

func TestBusinessHandlersRejectProviderIsolationViolation(t *testing.T) {
	handler := &BusinessHandler{}
	request := httptest.NewRequest(http.MethodGet, "/providers/provider-b/wagering/transactions/external", nil)
	request.SetPathValue("providerId", "provider-b")
	request.SetPathValue("externalTransactionId", "external")
	request = request.WithContext(oidc.WithIdentity(request.Context(), oidc.Identity{ProviderID: "provider-a", AuthorizedClient: "wallet-api"}))
	response := httptest.NewRecorder()
	handler.GetExternalTransaction(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
func TestBusinessHandlersValidateIdempotencyHeaderBeforeService(t *testing.T) {
	handler := &BusinessHandler{}
	request := httptest.NewRequest(http.MethodPost, "/wagering/transactions", strings.NewReader(`{"providerId":"client-value","externalTransactionId":"external","playerId":"player","walletId":"wallet","roundId":"round","gameId":"game","kind":"BET","money":{"amount":"1.00","currency":"BRL"}}`))
	request = request.WithContext(oidc.WithIdentity(request.Context(), oidc.Identity{ProviderID: "provider-a", AuthorizedClient: "wallet-api"}))
	response := httptest.NewRecorder()
	handler.ProcessTransaction(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
func TestBusinessHandlersRequireInternalClientForWalletCreation(t *testing.T) {
	handler := &BusinessHandler{}
	request := httptest.NewRequest(http.MethodPost, "/wallets", strings.NewReader(`{"playerId":"player","initialBalance":{"amount":"0.00","currency":"BRL"}}`))
	request = request.WithContext(oidc.WithIdentity(request.Context(), oidc.Identity{ProviderID: "provider-a", AuthorizedClient: "wallet-api"}))
	response := httptest.NewRecorder()
	handler.CreateWallet(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
