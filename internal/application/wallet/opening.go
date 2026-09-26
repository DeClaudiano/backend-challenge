package wallet

import (
	"time"

	"backend-challenge/internal/domain/ledger"
	domainmoney "backend-challenge/internal/domain/money"
	"backend-challenge/internal/domain/wagering"
	domainwallet "backend-challenge/internal/domain/wallet"
)

type OpenInput struct {
	WalletID, TransactionID, LedgerID, PlayerID string
	Currency                                    domainmoney.Currency
	InitialBalance                              domainmoney.Money
	Now                                         time.Time
}
type OpenResult struct {
	Wallet      domainwallet.Wallet
	Transaction *wagering.WagerTransaction
	Ledger      *ledger.Entry
}

func Open(input OpenInput) (OpenResult, error) {
	w, err := domainwallet.New(input.WalletID, input.PlayerID, input.Currency, input.InitialBalance, input.Now)
	if err != nil {
		return OpenResult{}, err
	}
	result := OpenResult{Wallet: w}
	if input.InitialBalance.IsZero() {
		return result, nil
	}
	tx, err := wagering.NewOpening(wagering.OpeningInput{ID: input.TransactionID, WalletID: input.WalletID, PlayerID: input.PlayerID, Money: input.InitialBalance, Now: input.Now})
	if err != nil {
		return OpenResult{}, err
	}
	if err := tx.MarkProcessed(w.Balance(), input.Now); err != nil {
		return OpenResult{}, err
	}
	change := domainwallet.Change{Direction: wagering.Credit, Before: mustZero(input.Currency), After: w.Balance()}
	ledgerID := input.LedgerID
	if ledgerID == "" {
		ledgerID = input.TransactionID + "-ledger"
	}
	entry, err := ledger.New(ledgerID, input.WalletID, input.TransactionID, change.Direction, input.InitialBalance, change.Before, change.After, input.Now)
	if err != nil {
		return OpenResult{}, err
	}
	result.Transaction, result.Ledger = &tx, &entry
	return result, nil
}
func mustZero(currency domainmoney.Currency) domainmoney.Money {
	z, _ := domainmoney.Zero(currency)
	return z
}
