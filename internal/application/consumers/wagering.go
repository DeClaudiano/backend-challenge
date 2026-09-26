package consumers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"backend-challenge/internal/application/ports"
	"backend-challenge/internal/application/services"
	"backend-challenge/internal/domain/money"
	domainwagering "backend-challenge/internal/domain/wagering"
)

const WagerConsumerName = "wager-transactions"

type WagerTransactionMessage struct {
	MessageID  string               `json:"messageId"`
	Type       string               `json:"type"`
	OccurredAt time.Time            `json:"occurredAt"`
	Data       WagerTransactionData `json:"data"`
}

type WagerTransactionData struct {
	ProviderID            string `json:"providerId"`
	ExternalTransactionID string `json:"externalTransactionId"`
	IdempotencyKey        string `json:"idempotencyKey"`
	PlayerID              string `json:"playerId"`
	WalletID              string `json:"walletId"`
	RoundID               string `json:"roundId"`
	GameID                string `json:"gameId"`
	Kind                  string `json:"kind"`
	Money                 struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
	} `json:"money"`
	ReferenceExternalTransactionID string `json:"referenceExternalTransactionId,omitempty"`
}

type WagerConsumer struct {
	unitOfWork   ports.UnitOfWork
	inbox        ports.InboxRepository
	transactions *services.TransactionService
	metrics      ports.Metrics
	now          func() time.Time
}

func NewWagerConsumer(unitOfWork ports.UnitOfWork, inbox ports.InboxRepository, transactions *services.TransactionService) *WagerConsumer {
	return &WagerConsumer{unitOfWork: unitOfWork, inbox: inbox, transactions: transactions, now: time.Now}
}

func NewWagerConsumerWithMetrics(unitOfWork ports.UnitOfWork, inbox ports.InboxRepository, transactions *services.TransactionService, metrics ports.Metrics) *WagerConsumer {
	return &WagerConsumer{unitOfWork: unitOfWork, inbox: inbox, transactions: transactions, metrics: metrics, now: time.Now}
}

func (c *WagerConsumer) Handle(ctx context.Context, body []byte) (bool, error) {
	started := time.Now()
	defer func() {
		if c.metrics != nil {
			c.metrics.ObserveProcessing(time.Since(started))
		}
	}()
	message, err := DecodeWagerTransactionMessage(body)
	if err != nil {
		return false, err
	}
	hash := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(hash[:])
	completed := false
	err = c.unitOfWork.Within(ctx, func(txCtx context.Context) error {
		claim, err := c.inbox.Claim(txCtx, WagerConsumerName, message.MessageID, payloadHash)
		if err != nil {
			return err
		}
		if !claim.Created && claim.Record.CompletedAt != nil {
			completed = true
			return nil
		}
		moneyValue, err := parseMessageMoney(message.Data.Money.Amount, message.Data.Money.Currency)
		if err != nil {
			return err
		}
		result, processErr := c.transactions.Process(txCtx, services.ProcessTransactionInput{ProviderID: message.Data.ProviderID, ExternalTransactionID: message.Data.ExternalTransactionID, IdempotencyKey: message.Data.IdempotencyKey, PlayerID: message.Data.PlayerID, WalletID: message.Data.WalletID, RoundID: message.Data.RoundID, GameID: message.Data.GameID, Kind: domainwagering.Kind(message.Data.Kind), Money: moneyValue, ReferenceExternalTransactionID: message.Data.ReferenceExternalTransactionID})
		if processErr != nil && result.Transaction.ID() == "" {
			return processErr
		}
		if err := c.inbox.MarkCompleted(txCtx, WagerConsumerName, message.MessageID, c.now().UTC()); err != nil {
			return err
		}
		completed = true
		return nil
	})
	if err != nil && c.metrics != nil {
		c.metrics.IncRetry()
	}
	return completed && err == nil, err
}

func DecodeWagerTransactionMessage(body []byte) (WagerTransactionMessage, error) {
	var message WagerTransactionMessage
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&message); err != nil {
		return WagerTransactionMessage{}, fmt.Errorf("invalid wager message: %w", err)
	}
	if message.MessageID == "" || message.Type != "WagerTransactionRequested" || message.Data.IdempotencyKey == "" {
		return WagerTransactionMessage{}, fmt.Errorf("invalid wager message envelope")
	}
	return message, nil
}

func parseMessageMoney(amount, currency string) (money.Money, error) {
	return money.Parse(amount, money.Currency(currency))
}
