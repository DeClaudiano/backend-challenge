package wagering

import (
	"time"

	"backend-challenge/internal/domain"
	"backend-challenge/internal/domain/money"
)

type ExternalInput struct {
	ID, ProviderID, ExternalTransactionID, IdempotencyKey string
	PlayerID, WalletID, RoundID, GameID                   string
	Kind                                                  Kind
	Money                                                 money.Money
	ReferenceExternalTransactionID                        string
}
type OpeningInput struct {
	ID, WalletID, PlayerID string
	Money                  money.Money
	Now                    time.Time
}

type WagerTransaction struct {
	id, providerID, externalID, idempotencyKey, payloadHash string
	playerID, walletID, roundID, gameID                     string
	kind                                                    Kind
	source                                                  Source
	amount                                                  money.Money
	referenceExternalID, referenceID                        string
	status                                                  Status
	failureCode                                             string
	resultBalance                                           *money.Money
	createdAt, updatedAt                                    time.Time
	reversedBy                                              string
	reversalKind                                            Kind
}

func NewExternal(input ExternalInput, payloadHash string, now time.Time) (WagerTransaction, error) {
	if input.ID == "" || input.ProviderID == "" || input.ExternalTransactionID == "" || input.IdempotencyKey == "" || input.PlayerID == "" || input.WalletID == "" || now.IsZero() || payloadHash == "" {
		return WagerTransaction{}, domain.NewError(domain.ErrInvalidValue, "external transaction identity and payload hash are required")
	}
	if !input.Kind.Valid() || input.Kind == Opening {
		return WagerTransaction{}, domain.NewError(domain.ErrInvalidOperation, "invalid external transaction kind")
	}
	if !input.Money.Currency().Valid() {
		return WagerTransaction{}, domain.NewError(domain.ErrInvalidCurrency, "transaction currency is invalid")
	}
	if input.Kind == Loss && !input.Money.IsZero() {
		return WagerTransaction{}, domain.NewError(domain.ErrInvalidOperation, "LOSS requires zero amount")
	}
	if input.Kind != Loss && !input.Money.IsPositive() {
		return WagerTransaction{}, domain.NewError(domain.ErrNonPositiveAmount, "transaction amount must be positive")
	}
	if (input.Kind == Refund || input.Kind == Rollback) && input.ReferenceExternalTransactionID == "" {
		return WagerTransaction{}, domain.NewError(domain.ErrReferenceRequired, "reversal reference is required")
	}
	return WagerTransaction{id: input.ID, providerID: input.ProviderID, externalID: input.ExternalTransactionID, idempotencyKey: input.IdempotencyKey, payloadHash: payloadHash, playerID: input.PlayerID, walletID: input.WalletID, roundID: input.RoundID, gameID: input.GameID, kind: input.Kind, source: External, amount: input.Money, referenceExternalID: input.ReferenceExternalTransactionID, status: Pending, createdAt: now, updatedAt: now}, nil
}

func NewOpening(input OpeningInput) (WagerTransaction, error) {
	if input.ID == "" || input.WalletID == "" || input.PlayerID == "" || input.Now.IsZero() {
		return WagerTransaction{}, domain.NewError(domain.ErrInvalidOpening, "opening identity is required")
	}
	if !input.Money.Currency().Valid() || input.Money.IsNegative() {
		return WagerTransaction{}, domain.NewError(domain.ErrInvalidOpening, "opening money is invalid")
	}
	return WagerTransaction{id: input.ID, walletID: input.WalletID, playerID: input.PlayerID, kind: Opening, source: Internal, amount: input.Money, status: Pending, createdAt: input.Now, updatedAt: input.Now}, nil
}

func Rehydrate(id, providerID, externalID, idempotencyKey, payloadHash, playerID, walletID, roundID, gameID string, kind Kind, source Source, amount money.Money, referenceExternalID, referenceID string, status Status, failureCode string, resultBalance *money.Money, createdAt, updatedAt time.Time, reversedBy string, reversalKind Kind) (WagerTransaction, error) {
	if id == "" || playerID == "" || walletID == "" || createdAt.IsZero() || updatedAt.IsZero() || !kind.Valid() || !status.Valid() || !amount.Valid() || amount.IsNegative() {
		return WagerTransaction{}, domain.NewError(domain.ErrInvalidValue, "invalid persisted transaction state")
	}
	if source != Internal && source != External {
		return WagerTransaction{}, domain.NewError(domain.ErrInvalidValue, "invalid persisted transaction source")
	}
	if source == External && (providerID == "" || externalID == "" || idempotencyKey == "" || payloadHash == "") {
		return WagerTransaction{}, domain.NewError(domain.ErrInvalidValue, "external transaction identity is incomplete")
	}
	if source == Internal && kind != Opening {
		return WagerTransaction{}, domain.NewError(domain.ErrInvalidValue, "internal transaction must be OPENING")
	}
	if kind == Loss && !amount.IsZero() {
		return WagerTransaction{}, domain.NewError(domain.ErrInvalidOperation, "LOSS requires zero amount")
	}
	if resultBalance != nil && (!resultBalance.Valid() || resultBalance.Currency() != amount.Currency() || resultBalance.IsNegative()) {
		return WagerTransaction{}, domain.NewError(domain.ErrInvalidValue, "invalid persisted result balance")
	}
	return WagerTransaction{id: id, providerID: providerID, externalID: externalID, idempotencyKey: idempotencyKey, payloadHash: payloadHash, playerID: playerID, walletID: walletID, roundID: roundID, gameID: gameID, kind: kind, source: source, amount: amount, referenceExternalID: referenceExternalID, referenceID: referenceID, status: status, failureCode: failureCode, resultBalance: resultBalance, createdAt: createdAt, updatedAt: updatedAt, reversedBy: reversedBy, reversalKind: reversalKind}, nil
}

func (t WagerTransaction) ID() string                  { return t.id }
func (t WagerTransaction) ProviderID() string          { return t.providerID }
func (t WagerTransaction) ExternalID() string          { return t.externalID }
func (t WagerTransaction) IdempotencyKey() string      { return t.idempotencyKey }
func (t WagerTransaction) PayloadHash() string         { return t.payloadHash }
func (t WagerTransaction) PlayerID() string            { return t.playerID }
func (t WagerTransaction) WalletID() string            { return t.walletID }
func (t WagerTransaction) RoundID() string             { return t.roundID }
func (t WagerTransaction) GameID() string              { return t.gameID }
func (t WagerTransaction) Kind() Kind                  { return t.kind }
func (t WagerTransaction) Source() Source              { return t.source }
func (t WagerTransaction) Amount() money.Money         { return t.amount }
func (t WagerTransaction) ReferenceExternalID() string { return t.referenceExternalID }
func (t WagerTransaction) ReferenceID() string         { return t.referenceID }
func (t WagerTransaction) Status() Status              { return t.status }
func (t WagerTransaction) FailureCode() string         { return t.failureCode }
func (t WagerTransaction) ResultBalance() (money.Money, bool) {
	if t.resultBalance == nil {
		return money.Money{}, false
	}
	return *t.resultBalance, true
}
func (t WagerTransaction) CreatedAt() time.Time { return t.createdAt }
func (t WagerTransaction) UpdatedAt() time.Time { return t.updatedAt }
func (t WagerTransaction) ReversedBy() string   { return t.reversedBy }
func (t WagerTransaction) ReversalKind() Kind   { return t.reversalKind }

func (t *WagerTransaction) MarkProcessed(balance money.Money, now time.Time) error {
	if err := t.transition(Processed, now); err != nil {
		return err
	}
	t.resultBalance = &balance
	return nil
}
func (t *WagerTransaction) MarkPendingReference(now time.Time) error {
	return t.transition(PendingReference, now)
}
func (t *WagerTransaction) Reject(code string, now time.Time) error {
	if code == "" {
		return domain.NewError(domain.ErrInvalidValue, "failure code is required")
	}
	if err := t.transition(Rejected, now); err != nil {
		return err
	}
	t.failureCode = code
	return nil
}
func (t *WagerTransaction) Fail(code string, now time.Time) error {
	if code == "" {
		return domain.NewError(domain.ErrInvalidValue, "failure code is required")
	}
	if err := t.transition(Failed, now); err != nil {
		return err
	}
	t.failureCode = code
	return nil
}
func (t *WagerTransaction) ResolveReference(id string, now time.Time) error {
	if t.status != PendingReference || id == "" {
		return domain.NewError(domain.ErrInvalidTransition, "reference can only be resolved from pending reference")
	}
	t.referenceID = id
	t.updatedAt = now
	return nil
}

func (t *WagerTransaction) LinkReference(id string) error {
	if t.referenceID != "" || id == "" {
		return domain.NewError(domain.ErrInvalidValue, "reference is already linked or invalid")
	}
	t.referenceID = id
	return nil
}

func (t *WagerTransaction) ReserveReversal(reversalID string, reversalKind Kind) error {
	if t.status != Processed {
		return domain.NewError(domain.ErrReferenceIncompatible, "only processed transactions can be reversed")
	}
	if reversalID == "" || (reversalKind != Refund && reversalKind != Rollback) {
		return domain.NewError(domain.ErrInvalidOperation, "invalid reversal")
	}
	if t.reversedBy != "" {
		return domain.NewError(domain.ErrReversalAlreadyApplied, "transaction already has a direct reversal")
	}
	if t.kind != Bet && t.kind != Win && t.kind != Refund {
		return domain.NewError(domain.ErrReferenceIncompatible, "transaction kind cannot be reversed")
	}
	if t.kind == Refund && reversalKind != Rollback {
		return domain.NewError(domain.ErrReferenceIncompatible, "a refund can only be rolled back")
	}
	t.reversedBy, t.reversalKind = reversalID, reversalKind
	return nil
}
func (t *WagerTransaction) transition(next Status, now time.Time) error {
	if now.IsZero() {
		return domain.NewError(domain.ErrInvalidValue, "transition timestamp is invalid")
	}
	if t.status.Terminal() {
		return domain.NewError(domain.ErrTerminalTransaction, "terminal transaction cannot transition")
	}
	valid := (t.status == Pending && (next == Processed || next == PendingReference || next == Rejected || next == Failed)) || (t.status == PendingReference && (next == Processed || next == Rejected || next == Failed))
	if !valid {
		return domain.NewError(domain.ErrInvalidTransition, "invalid transaction state transition")
	}
	t.status, t.updatedAt = next, now
	return nil
}
