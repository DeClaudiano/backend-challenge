package events

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"backend-challenge/internal/domain/ledger"
	"backend-challenge/internal/domain/money"
	"backend-challenge/internal/domain/wagering"
)

const Version int = 1

const (
	WagerTransactionProcessed        = "WagerTransactionProcessed"
	WagerTransactionRejected         = "WagerTransactionRejected"
	WalletBalanceChanged             = "WalletBalanceChanged"
	WagerTransactionPendingReference = "WagerTransactionPendingReference"
)

type Envelope struct {
	EventID       string    `json:"eventId"`
	EventType     string    `json:"eventType"`
	AggregateID   string    `json:"aggregateId"`
	CorrelationID string    `json:"correlationId"`
	CausationID   string    `json:"causationId,omitempty"`
	OccurredAt    time.Time `json:"occurredAt"`
	Version       int       `json:"version"`
	Data          any       `json:"data"`
}

func NewEnvelope(eventID, eventType, aggregateID, correlationID, causationID string, occurredAt time.Time, data any) (Envelope, error) {
	if eventID == "" || eventType == "" || aggregateID == "" || correlationID == "" || occurredAt.IsZero() || data == nil {
		return Envelope{}, fmt.Errorf("event envelope identity and data are required")
	}
	if _, err := json.Marshal(data); err != nil {
		return Envelope{}, fmt.Errorf("marshal event data: %w", err)
	}
	return Envelope{EventID: eventID, EventType: eventType, AggregateID: aggregateID, CorrelationID: correlationID, CausationID: causationID, OccurredAt: occurredAt.UTC(), Version: Version, Data: data}, nil
}

func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate event id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

type MoneyData struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}
type WagerTransactionProcessedData struct {
	TransactionID         string     `json:"transactionId"`
	ProviderID            string     `json:"providerId,omitempty"`
	ExternalTransactionID string     `json:"externalTransactionId,omitempty"`
	PlayerID              string     `json:"playerId"`
	WalletID              string     `json:"walletId"`
	Kind                  string     `json:"kind"`
	Money                 MoneyData  `json:"money"`
	Status                string     `json:"status"`
	Balance               *MoneyData `json:"balance,omitempty"`
}
type WagerTransactionRejectedData struct {
	TransactionID         string    `json:"transactionId"`
	ProviderID            string    `json:"providerId,omitempty"`
	ExternalTransactionID string    `json:"externalTransactionId,omitempty"`
	PlayerID              string    `json:"playerId"`
	WalletID              string    `json:"walletId"`
	Kind                  string    `json:"kind"`
	Money                 MoneyData `json:"money"`
	FailureCode           string    `json:"failureCode"`
}
type WagerTransactionPendingReferenceData struct {
	TransactionID                  string `json:"transactionId"`
	ProviderID                     string `json:"providerId"`
	ExternalTransactionID          string `json:"externalTransactionId"`
	WalletID                       string `json:"walletId"`
	ReferenceExternalTransactionID string `json:"referenceExternalTransactionId"`
}
type WalletBalanceChangedData struct {
	WalletID      string    `json:"walletId"`
	TransactionID string    `json:"transactionId"`
	Direction     string    `json:"direction"`
	Money         MoneyData `json:"money"`
	BalanceBefore MoneyData `json:"balanceBefore"`
	BalanceAfter  MoneyData `json:"balanceAfter"`
	WalletVersion int64     `json:"walletVersion"`
}

func moneyData(value money.Money) MoneyData {
	return MoneyData{Amount: value.Amount(), Currency: value.Currency().String()}
}

func Processed(tx wagering.WagerTransaction, correlationID string) (Envelope, error) {
	var balance *MoneyData
	if value, ok := tx.ResultBalance(); ok {
		converted := moneyData(value)
		balance = &converted
	}
	id, err := NewID()
	if err != nil {
		return Envelope{}, err
	}
	return NewEnvelope(id, WagerTransactionProcessed, tx.ID(), correlationID, "", tx.UpdatedAt(), WagerTransactionProcessedData{TransactionID: tx.ID(), ProviderID: tx.ProviderID(), ExternalTransactionID: tx.ExternalID(), PlayerID: tx.PlayerID(), WalletID: tx.WalletID(), Kind: string(tx.Kind()), Money: moneyData(tx.Amount()), Status: string(tx.Status()), Balance: balance})
}
func Rejected(tx wagering.WagerTransaction, correlationID string) (Envelope, error) {
	id, err := NewID()
	if err != nil {
		return Envelope{}, err
	}
	return NewEnvelope(id, WagerTransactionRejected, tx.ID(), correlationID, "", tx.UpdatedAt(), WagerTransactionRejectedData{TransactionID: tx.ID(), ProviderID: tx.ProviderID(), ExternalTransactionID: tx.ExternalID(), PlayerID: tx.PlayerID(), WalletID: tx.WalletID(), Kind: string(tx.Kind()), Money: moneyData(tx.Amount()), FailureCode: tx.FailureCode()})
}
func PendingReference(tx wagering.WagerTransaction, correlationID string) (Envelope, error) {
	id, err := NewID()
	if err != nil {
		return Envelope{}, err
	}
	return NewEnvelope(id, WagerTransactionPendingReference, tx.ID(), correlationID, "", tx.UpdatedAt(), WagerTransactionPendingReferenceData{TransactionID: tx.ID(), ProviderID: tx.ProviderID(), ExternalTransactionID: tx.ExternalID(), WalletID: tx.WalletID(), ReferenceExternalTransactionID: tx.ReferenceExternalID()})
}
func BalanceChanged(tx wagering.WagerTransaction, entry ledger.Entry, walletVersion int64, correlationID string) (Envelope, error) {
	id, err := NewID()
	if err != nil {
		return Envelope{}, err
	}
	return NewEnvelope(id, WalletBalanceChanged, tx.WalletID(), correlationID, tx.ID(), entry.CreatedAt(), WalletBalanceChangedData{WalletID: entry.WalletID(), TransactionID: entry.TransactionID(), Direction: string(entry.Direction()), Money: moneyData(entry.Amount()), BalanceBefore: moneyData(entry.BalanceBefore()), BalanceAfter: moneyData(entry.BalanceAfter()), WalletVersion: walletVersion})
}
