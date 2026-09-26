package httpadapter

import (
	"backend-challenge/internal/observability"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"backend-challenge/internal/adapters/oidc"
	"backend-challenge/internal/application/services"
	"backend-challenge/internal/domain"
	"backend-challenge/internal/domain/ledger"
	"backend-challenge/internal/domain/money"
	domainwagering "backend-challenge/internal/domain/wagering"
	domainwallet "backend-challenge/internal/domain/wallet"
)

type BusinessHandler struct {
	wallets      *services.WalletService
	transactions *services.TransactionService
	queries      *services.QueryService
	log          *slog.Logger
}

func NewBusinessHandler(wallets *services.WalletService, transactions *services.TransactionService, queries *services.QueryService, log *slog.Logger) *BusinessHandler {
	return &BusinessHandler{wallets: wallets, transactions: transactions, queries: queries, log: log}
}

type moneyDTO struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}
type openWalletRequest struct {
	PlayerID       string   `json:"playerId"`
	InitialBalance moneyDTO `json:"initialBalance"`
}
type wagerRequest struct {
	ProviderID                     string   `json:"providerId"`
	ExternalTransactionID          string   `json:"externalTransactionId"`
	PlayerID                       string   `json:"playerId"`
	WalletID                       string   `json:"walletId"`
	RoundID                        string   `json:"roundId"`
	GameID                         string   `json:"gameId"`
	Kind                           string   `json:"kind"`
	Money                          moneyDTO `json:"money"`
	ReferenceExternalTransactionID string   `json:"referenceExternalTransactionId,omitempty"`
}
type walletResponse struct {
	ID       string   `json:"id"`
	PlayerID string   `json:"playerId"`
	Balance  moneyDTO `json:"balance"`
	Version  int64    `json:"version"`
}
type transactionResponse struct {
	TransactionID    string    `json:"transactionId"`
	Status           string    `json:"status"`
	Balance          *moneyDTO `json:"balance,omitempty"`
	IdempotentReplay bool      `json:"idempotentReplay"`
	FailureCode      string    `json:"failureCode,omitempty"`
}
type transactionDetailResponse struct {
	TransactionID         string    `json:"transactionId"`
	ProviderID            string    `json:"providerId,omitempty"`
	ExternalTransactionID string    `json:"externalTransactionId,omitempty"`
	Status                string    `json:"status"`
	FailureCode           string    `json:"failureCode,omitempty"`
	Balance               *moneyDTO `json:"balance,omitempty"`
}
type ledgerResponse struct {
	WalletID   string                `json:"walletId"`
	Entries    []ledgerEntryResponse `json:"entries"`
	NextCursor string                `json:"nextCursor,omitempty"`
}
type ledgerEntryResponse struct {
	ID            string   `json:"id"`
	TransactionID string   `json:"transactionId"`
	Direction     string   `json:"direction"`
	Money         moneyDTO `json:"money"`
	BalanceBefore moneyDTO `json:"balanceBefore"`
	BalanceAfter  moneyDTO `json:"balanceAfter"`
	CreatedAt     string   `json:"createdAt"`
}
type reconciliationResponse struct {
	WalletID          string   `json:"walletId"`
	StoredBalance     moneyDTO `json:"storedBalance"`
	CalculatedBalance moneyDTO `json:"calculatedBalance"`
	Difference        moneyDTO `json:"difference"`
	Consistent        bool     `json:"consistent"`
	CheckedEntries    int      `json:"checkedEntries"`
}

func (h *BusinessHandler) CreateWallet(w http.ResponseWriter, r *http.Request) {
	identity, ok := identity(r)
	if !ok || !internalClient(identity) {
		writeJSON(w, http.StatusForbidden, errorBody("forbidden"))
		return
	}
	var request openWalletRequest
	if err := decodeBody(r, &request); err != nil {
		writeDomainError(w, err)
		return
	}
	initial, err := parseMoney(request.InitialBalance)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	result, err := h.wallets.Open(r.Context(), services.OpenWalletInput{PlayerID: request.PlayerID, Currency: initial.Currency(), InitialBalance: initial})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, walletDTO(result.Wallet))
}
func (h *BusinessHandler) ProcessTransaction(w http.ResponseWriter, r *http.Request) {
	identity, ok := identity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorBody("unauthorized"))
		return
	}
	var request wagerRequest
	if err := decodeBody(r, &request); err != nil {
		writeDomainError(w, err)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, errorBody("Idempotency-Key is required"))
		return
	}
	value, err := parseMoney(request.Money)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	result, processErr := h.transactions.Process(r.Context(), services.ProcessTransactionInput{ProviderID: identity.ProviderID, ExternalTransactionID: request.ExternalTransactionID, IdempotencyKey: key, PlayerID: request.PlayerID, WalletID: request.WalletID, RoundID: request.RoundID, GameID: request.GameID, Kind: domainwagering.Kind(request.Kind), Money: value, ReferenceExternalTransactionID: request.ReferenceExternalTransactionID})
	if processErr != nil && result.Transaction.ID() == "" {
		writeDomainError(w, processErr)
		return
	}
	response := transactionResultDTO(result)
	status := http.StatusOK
	if result.Transaction.Status() == domainwagering.PendingReference {
		status = http.StatusAccepted
	}
	if processErr != nil {
		status = statusForError(processErr)
	}
	if h.log != nil {
		observability.MessageLogger(r.Context(), h.log, "", result.Transaction.ID(), result.Transaction.WalletID(), identity.ProviderID).Info("wager transaction processed", "status", result.Transaction.Status(), "idempotentReplay", result.IdempotentReplay)
	}
	writeJSON(w, status, response)
}
func (h *BusinessHandler) GetWallet(w http.ResponseWriter, r *http.Request) {
	identity, ok := identity(r)
	if !ok || !internalClient(identity) {
		writeJSON(w, http.StatusForbidden, errorBody("forbidden"))
		return
	}
	value, err := h.queries.GetWallet(r.Context(), r.PathValue("walletId"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, walletDTO(value))
}
func (h *BusinessHandler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	identity, ok := identity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorBody("unauthorized"))
		return
	}
	value, err := h.queries.GetTransaction(r.Context(), r.PathValue("transactionId"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if value.Source() != domainwagering.External || value.ProviderID() != identity.ProviderID {
		writeJSON(w, http.StatusForbidden, errorBody("forbidden"))
		return
	}
	writeJSON(w, http.StatusOK, transactionDetailDTO(value))
}
func (h *BusinessHandler) GetExternalTransaction(w http.ResponseWriter, r *http.Request) {
	identity, ok := identity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorBody("unauthorized"))
		return
	}
	if r.PathValue("providerId") != identity.ProviderID {
		writeJSON(w, http.StatusForbidden, errorBody("forbidden"))
		return
	}
	value, err := h.queries.GetExternalTransaction(r.Context(), identity.ProviderID, r.PathValue("externalTransactionId"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, transactionDetailDTO(value))
}
func (h *BusinessHandler) ListLedger(w http.ResponseWriter, r *http.Request) {
	identity, ok := identity(r)
	if !ok || !internalClient(identity) {
		writeJSON(w, http.StatusForbidden, errorBody("forbidden"))
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody("invalid limit"))
			return
		}
		limit = parsed
	}
	page, err := h.queries.ListLedger(r.Context(), r.PathValue("walletId"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	entries := make([]ledgerEntryResponse, 0, len(page.Entries))
	for _, entry := range page.Entries {
		entries = append(entries, ledgerDTO(entry))
	}
	writeJSON(w, http.StatusOK, ledgerResponse{WalletID: r.PathValue("walletId"), Entries: entries, NextCursor: page.NextCursor})
}
func (h *BusinessHandler) Reconcile(w http.ResponseWriter, r *http.Request) {
	identity, ok := identity(r)
	if !ok || !internalClient(identity) {
		writeJSON(w, http.StatusForbidden, errorBody("forbidden"))
		return
	}
	value, err := h.queries.Reconcile(r.Context(), r.PathValue("walletId"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reconciliationDTO(value))
}

func identity(r *http.Request) (oidc.Identity, bool) { return oidc.IdentityFromContext(r.Context()) }
func internalClient(identity oidc.Identity) bool {
	return identity.AuthorizedClient == "wallet-internal"
}
func decodeBody(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.NewError(domain.ErrInvalidValue, "invalid JSON body")
	}
	return nil
}
func parseMoney(value moneyDTO) (money.Money, error) {
	return money.Parse(value.Amount, money.Currency(value.Currency))
}
func moneyDTOValue(value money.Money) moneyDTO {
	return moneyDTO{Amount: value.Amount(), Currency: value.Currency().String()}
}
func walletDTO(value domainwallet.Wallet) walletResponse {
	return walletResponse{ID: value.ID(), PlayerID: value.PlayerID(), Balance: moneyDTOValue(value.Balance()), Version: value.Version()}
}
func transactionResultDTO(value services.ProcessTransactionResult) transactionResponse {
	response := transactionResponse{TransactionID: value.Transaction.ID(), Status: string(value.Transaction.Status()), IdempotentReplay: value.IdempotentReplay, FailureCode: value.Transaction.FailureCode()}
	if value.HasBalance {
		balance := moneyDTOValue(value.Balance)
		response.Balance = &balance
	}
	return response
}
func transactionDetailDTO(value domainwagering.WagerTransaction) transactionDetailResponse {
	response := transactionDetailResponse{TransactionID: value.ID(), ProviderID: value.ProviderID(), ExternalTransactionID: value.ExternalID(), Status: string(value.Status()), FailureCode: value.FailureCode()}
	if balance, ok := value.ResultBalance(); ok {
		converted := moneyDTOValue(balance)
		response.Balance = &converted
	}
	return response
}
func ledgerDTO(value ledger.Entry) ledgerEntryResponse {
	return ledgerEntryResponse{ID: value.ID(), TransactionID: value.TransactionID(), Direction: string(value.Direction()), Money: moneyDTOValue(value.Amount()), BalanceBefore: moneyDTOValue(value.BalanceBefore()), BalanceAfter: moneyDTOValue(value.BalanceAfter()), CreatedAt: value.CreatedAt().UTC().Format("2006-01-02T15:04:05.999Z07:00")}
}
func reconciliationDTO(value services.Reconciliation) reconciliationResponse {
	return reconciliationResponse{WalletID: value.WalletID, StoredBalance: moneyDTOValue(value.StoredBalance), CalculatedBalance: moneyDTOValue(value.CalculatedBalance), Difference: moneyDTOValue(value.Difference), Consistent: value.Consistent, CheckedEntries: value.CheckedEntries}
}
func errorBody(message string) map[string]string { return map[string]string{"error": message} }
func writeDomainError(w http.ResponseWriter, err error) {
	var value *domain.Error
	if errors.As(err, &value) {
		writeJSON(w, statusForCode(value.Code), map[string]any{"error": string(value.Code), "message": value.Message})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, errorBody("service unavailable"))
}
func statusForError(err error) int {
	var value *domain.Error
	if errors.As(err, &value) {
		return statusForCode(value.Code)
	}
	return http.StatusServiceUnavailable
}
func statusForCode(code domain.ErrorCode) int {
	switch code {
	case domain.ErrInvalidValue, domain.ErrInvalidCurrency, domain.ErrCurrencyMismatch, domain.ErrNonPositiveAmount, domain.ErrReferenceRequired:
		return http.StatusBadRequest
	case domain.ErrNotFound:
		return http.StatusNotFound
	case domain.ErrIdentityConflict, domain.ErrOptimisticConflict, domain.ErrReversalAlreadyApplied:
		return http.StatusConflict
	case domain.ErrInsufficientBalance, domain.ErrInsufficientReversal, domain.ErrInvalidOperation, domain.ErrReferenceIncompatible:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusServiceUnavailable
	}
}
