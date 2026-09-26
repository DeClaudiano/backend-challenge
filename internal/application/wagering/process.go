package wagering

import (
	"time"

	"backend-challenge/internal/domain"
	"backend-challenge/internal/domain/ledger"
	"backend-challenge/internal/domain/money"
	domainwagering "backend-challenge/internal/domain/wagering"
	"backend-challenge/internal/domain/wallet"
)

type ProcessInput struct {
	TransactionID, LedgerID, ProviderID, ExternalTransactionID, IdempotencyKey string
	PlayerID, WalletID, RoundID, GameID                                        string
	Kind                                                                       domainwagering.Kind
	Money                                                                      money.Money
	ReferenceExternalTransactionID                                             string
	Now                                                                        time.Time
	PayloadHash                                                                string
}
type ProcessResult struct {
	Transaction domainwagering.WagerTransaction
	Ledger      *ledger.Entry
	Wallet      wallet.Wallet
}

func Process(input ProcessInput, current wallet.Wallet, reference *domainwagering.WagerTransaction) (ProcessResult, error) {
	if input.WalletID != current.ID() || input.PlayerID != current.PlayerID() {
		return ProcessResult{}, domain.NewError(domain.ErrReferenceIncompatible, "transaction does not belong to wallet")
	}
	if input.Money.Currency() != current.Currency() {
		return ProcessResult{}, domain.NewError(domain.ErrCurrencyMismatch, "transaction currency differs from wallet")
	}
	transaction, err := domainwagering.NewExternal(domainwagering.ExternalInput{ID: input.TransactionID, ProviderID: input.ProviderID, ExternalTransactionID: input.ExternalTransactionID, IdempotencyKey: input.IdempotencyKey, PlayerID: input.PlayerID, WalletID: input.WalletID, RoundID: input.RoundID, GameID: input.GameID, Kind: input.Kind, Money: input.Money, ReferenceExternalTransactionID: input.ReferenceExternalTransactionID}, input.PayloadHash, input.Now)
	if err != nil {
		return ProcessResult{}, err
	}
	result := ProcessResult{Transaction: transaction, Wallet: current}
	if (input.Kind == domainwagering.Refund || input.Kind == domainwagering.Rollback) && reference == nil {
		_ = result.Transaction.MarkPendingReference(input.Now)
		return result, nil
	}
	if input.Kind == domainwagering.Win && input.ReferenceExternalTransactionID != "" {
		if reference == nil || reference.Status() != domainwagering.Processed || reference.Kind() != domainwagering.Bet || reference.ProviderID() != input.ProviderID || reference.ExternalID() != input.ReferenceExternalTransactionID || reference.PlayerID() != input.PlayerID || reference.WalletID() != input.WalletID || reference.RoundID() != input.RoundID {
			_ = result.Transaction.Reject(string(domain.ErrReferenceIncompatible), input.Now)
			return result, domain.NewError(domain.ErrReferenceIncompatible, "WIN reference must be a processed bet from the same round")
		}
	}
	if input.Kind == domainwagering.Refund || input.Kind == domainwagering.Rollback {
		if err := validateReference(input, reference); err != nil {
			_ = result.Transaction.Reject(codeOf(err), input.Now)
			return result, err
		}
		if err := reference.ReserveReversal(input.TransactionID, input.Kind); err != nil {
			_ = result.Transaction.Reject(codeOf(err), input.Now)
			return result, err
		}
	}
	var change wallet.Change
	switch input.Kind {
	case domainwagering.Bet:
		change, err = result.Wallet.Debit(input.Money, input.Now)
	case domainwagering.Rollback:
		if reference != nil && reference.Kind() == domainwagering.Bet {
			change, err = result.Wallet.Credit(input.Money, input.Now)
		} else {
			change, err = result.Wallet.Debit(input.Money, input.Now)
		}
	case domainwagering.Win, domainwagering.Refund:
		change, err = result.Wallet.Credit(input.Money, input.Now)
	case domainwagering.Loss:
		if !input.Money.IsZero() {
			err = domain.NewError(domain.ErrInvalidOperation, "LOSS requires zero amount")
		}
	default:
		err = domain.NewError(domain.ErrInvalidOperation, "unsupported transaction kind")
	}
	if err != nil {
		if input.Kind == domainwagering.Rollback && codeOf(err) == string(domain.ErrInsufficientBalance) {
			err = domain.NewError(domain.ErrInsufficientReversal, "rollback cannot debit the available wallet balance")
		}
		_ = result.Transaction.Reject(codeOf(err), input.Now)
		return result, err
	}
	if input.Kind == domainwagering.Loss {
		if err := result.Transaction.MarkProcessed(result.Wallet.Balance(), input.Now); err != nil {
			return ProcessResult{}, err
		}
		return result, nil
	}
	ledgerID := input.LedgerID
	if ledgerID == "" {
		ledgerID = input.TransactionID + "-ledger"
	}
	entry, err := ledger.New(ledgerID, input.WalletID, input.TransactionID, change.Direction, input.Money, change.Before, change.After, input.Now)
	if err != nil {
		return ProcessResult{}, err
	}
	if err := result.Transaction.MarkProcessed(result.Wallet.Balance(), input.Now); err != nil {
		return ProcessResult{}, err
	}
	result.Ledger = &entry
	return result, nil
}

func validateReference(input ProcessInput, reference *domainwagering.WagerTransaction) error {
	if reference.Status() != domainwagering.Processed || reference.ProviderID() != input.ProviderID || reference.ExternalID() != input.ReferenceExternalTransactionID || reference.PlayerID() != input.PlayerID || reference.WalletID() != input.WalletID || reference.RoundID() != input.RoundID || reference.Amount().Currency() != input.Money.Currency() || !reference.Amount().Equal(input.Money) {
		return domain.NewError(domain.ErrReferenceIncompatible, "reference does not match reversal")
	}
	if input.Kind == domainwagering.Refund && reference.Kind() != domainwagering.Bet {
		return domain.NewError(domain.ErrReferenceIncompatible, "refund must reference a processed bet")
	}
	if input.Kind == domainwagering.Rollback && reference.Kind() != domainwagering.Bet && reference.Kind() != domainwagering.Win && reference.Kind() != domainwagering.Refund {
		return domain.NewError(domain.ErrReferenceIncompatible, "rollback reference kind is invalid")
	}
	return nil
}
func codeOf(err error) string {
	if e, ok := err.(*domain.Error); ok {
		return string(e.Code)
	}
	return string(domain.ErrInvalidOperation)
}
