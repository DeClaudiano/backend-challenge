package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"backend-challenge/internal/adapters/postgres"
	sqsadapter "backend-challenge/internal/adapters/sqs"
	"backend-challenge/internal/application/consumers"
	"backend-challenge/internal/application/ports"
	"backend-challenge/internal/application/publisher"
	"backend-challenge/internal/application/services"
	"backend-challenge/internal/config"
	"backend-challenge/internal/domain/money"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSQSRedeliveryAfterDurableCommit(t *testing.T) {
	env := recoveryEnv(t)
	pool := openPool(t)
	defer pool.Close()
	resetDatabase(t, pool)

	wallet, err := newWalletService(pool).Open(context.Background(), services.OpenWalletInput{
		PlayerID: "00000000-0000-0000-0000-000000000001", Currency: money.BRL, InitialBalance: mustMoney(t, "100.00"),
	})
	if err != nil {
		t.Fatal(err)
	}

	client := newSQSClient(t, env.sqsEndpoint)
	messageID := "recovery-redelivery-" + fmt.Sprint(time.Now().UnixNano())
	body := wagerMessage(messageID, wallet.Wallet.ID(), "recovery-external-"+messageID, "recovery-key-"+messageID)
	if _, err := client.SendMessage(context.Background(), &awssqs.SendMessageInput{
		QueueUrl: &env.queueURL, MessageBody: aws.String(body),
		MessageGroupId: aws.String("wallet:" + wallet.Wallet.ID()), MessageDeduplicationId: aws.String(messageID),
	}); err != nil {
		t.Fatal(err)
	}

	message := receiveOne(t, client, env.queueURL, 1)
	consumer := consumers.NewWagerConsumer(postgres.NewUnitOfWork(pool), postgres.NewInboxRepository(pool), newTransactionService(pool))
	deleted, err := consumer.Handle(context.Background(), []byte(*message.Body))
	if err != nil || !deleted {
		t.Fatalf("first delivery: deleted=%v err=%v", deleted, err)
	}

	// Deliberately do not delete the SQS message: this simulates a crash after
	// the database commit and before DeleteMessage.
	time.Sleep(1200 * time.Millisecond)
	redelivered := receiveOne(t, client, env.queueURL, 1)
	deleted, err = consumer.Handle(context.Background(), []byte(*redelivered.Body))
	if err != nil || !deleted {
		t.Fatalf("redelivery: deleted=%v err=%v", deleted, err)
	}
	if _, err := client.DeleteMessage(context.Background(), &awssqs.DeleteMessageInput{QueueUrl: &env.queueURL, ReceiptHandle: redelivered.ReceiptHandle}); err != nil {
		t.Fatal(err)
	}

	var debits int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM wallet_ledger_entries WHERE wallet_id=$1 AND direction='DEBIT'`, wallet.Wallet.ID()).Scan(&debits); err != nil {
		t.Fatal(err)
	}
	if debits != 1 {
		t.Fatalf("redelivery created %d debits, want 1", debits)
	}
}

func TestSQSPoisonMessageReachesDLQ(t *testing.T) {
	env := recoveryEnv(t)
	client := newSQSClient(t, env.sqsEndpoint)
	messageID := "recovery-poison-" + fmt.Sprint(time.Now().UnixNano())
	if _, err := client.SendMessage(context.Background(), &awssqs.SendMessageInput{
		QueueUrl: &env.queueURL, MessageBody: aws.String(`{"invalid":true}`),
		MessageGroupId: aws.String("recovery-poison"), MessageDeduplicationId: aws.String(messageID),
	}); err != nil {
		t.Fatal(err)
	}

	// The Compose bootstrap configures maxReceiveCount=5. A one-second visibility
	// timeout makes this recovery test deterministic without changing production code.
	for attempt := 0; attempt < 5; attempt++ {
		message := receiveOne(t, client, env.queueURL, 1)

		if message.Body == nil || *message.Body != `{"invalid":true}` {
			t.Fatal("unexpected poison message received")
		}

		if attempt < 4 {
			time.Sleep(1200 * time.Millisecond)
		}
	}
	waitFor(t, 5*time.Second, func() bool {
		result, err := client.ReceiveMessage(context.Background(), &awssqs.ReceiveMessageInput{QueueUrl: &env.dlqURL, MaxNumberOfMessages: 1, WaitTimeSeconds: 1})
		if err != nil {
			t.Fatal(err)
		}
		return len(result.Messages) == 1
	})
}

func TestOutboxLeaseIsRecoveredByAnotherPublisher(t *testing.T) {
	pool := openPool(t)
	defer pool.Close()
	resetDatabase(t, pool)
	if _, err := newWalletService(pool).Open(context.Background(), services.OpenWalletInput{
		PlayerID: "00000000-0000-0000-0000-000000000002", Currency: money.BRL, InitialBalance: mustMoney(t, "100.00"),
	}); err != nil {
		t.Fatal(err)
	}

	repo := postgres.NewOutboxRepository(pool)
	now := time.Now().UTC()
	first, err := repo.ClaimPending(context.Background(), now, now.Add(100*time.Millisecond), 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim: records=%d err=%v", len(first), err)
	}
	time.Sleep(150 * time.Millisecond)
	second, err := repo.ClaimPending(context.Background(), time.Now().UTC(), time.Now().UTC().Add(time.Second), 1)
	if err != nil || len(second) != 1 {
		t.Fatalf("recovered claim: records=%d err=%v", len(second), err)
	}
	if first[0].EventID != second[0].EventID {
		t.Fatalf("recovered different event: first=%s second=%s", first[0].EventID, second[0].EventID)
	}
}

func TestOutboxRepublishesAfterPublishBeforeConfirmation(t *testing.T) {
	env := recoveryEnv(t)
	pool := openPool(t)
	defer pool.Close()
	resetDatabase(t, pool)
	wallet, err := newWalletService(pool).Open(context.Background(), services.OpenWalletInput{
		PlayerID: "00000000-0000-0000-0000-000000000003", Currency: money.BRL, InitialBalance: mustMoney(t, "0.00"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newTransactionService(pool).Process(context.Background(), services.ProcessTransactionInput{
		ProviderID: "provider", ExternalTransactionID: "outbox-recovery-loss", IdempotencyKey: "provider:outbox-recovery-loss",
		PlayerID: "00000000-0000-0000-0000-000000000003", WalletID: wallet.Wallet.ID(), RoundID: "round", GameID: "game", Kind: "LOSS", Money: mustMoney(t, "0.00"),
	}); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{SQSRegion: "us-east-1", SQSEndpoint: env.sqsEndpoint, SQSQueueURL: env.queueURL}
	client, err := sqsadapter.Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	realBroker := sqsadapter.NewOutboxPublisher(client)
	failingBroker := &publishThenFail{delegate: realBroker}
	firstPublisher := publisher.NewOutboxPublisher(postgres.NewOutboxRepository(pool), failingBroker)
	var eligible int
	if err := pool.QueryRow(
		context.Background(),
		`SELECT count(*)
     FROM outbox_events
     WHERE published_at IS NULL
       AND next_attempt_at <= NOW()
       AND (lease_until IS NULL OR lease_until <= NOW())`,
	).Scan(&eligible); err != nil {
		t.Fatal(err)
	}
	t.Logf("eligible before first publisher=%d", eligible)

	err = firstPublisher.PublishBatch(context.Background())
	t.Logf("first publisher error: %v", err)
	if err == nil {
		t.Fatal("expected failure after broker publication")
	}
	if failingBroker.calls() != 1 {
		t.Fatalf("broker calls=%d, want 1", failingBroker.calls())
	}

	time.Sleep(1200 * time.Millisecond)
	var expectedEventID string
	if err := pool.QueryRow(context.Background(), `SELECT event_id::text FROM outbox_events WHERE published_at IS NULL LIMIT 1`).Scan(&expectedEventID); err != nil {
		t.Fatal(err)
	}
	secondPublisher := publisher.NewOutboxPublisher(postgres.NewOutboxRepository(pool), realBroker)
	if err := secondPublisher.PublishBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if failingBroker.calls() != 1 {
		t.Fatal("unexpected additional failing broker call")
	}

	var pending int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM outbox_events WHERE published_at IS NULL`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("pending outbox events=%d, want 0", pending)
	}

	// FIFO deduplication uses eventId, so the broker may expose one message even
	// though publication was attempted twice. The important invariant is that
	// both attempts carry the same immutable event identity.
	messages := receiveAll(t, client.API, env.eventQueue)
	if len(messages) == 0 {
		t.Fatal("no outbox event was published")
	}
	seen := make(map[string]bool)
	for _, message := range messages {
		if message.Body == nil {
			continue
		}
		var envelope struct {
			EventID string `json:"eventId"`
		}
		if err := json.Unmarshal([]byte(*message.Body), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.EventID == "" {
			continue
		}
		if envelope.EventID != expectedEventID {
			continue
		}
		seen[envelope.EventID] = true
		if message.ReceiptHandle != nil {
			_, _ = client.API.DeleteMessage(context.Background(), &awssqs.DeleteMessageInput{QueueUrl: &env.eventQueue, ReceiptHandle: message.ReceiptHandle})
		}
	}
	if len(seen) != 1 {
		t.Fatalf("published event identities=%d, want 1 stable event identity", len(seen))
	}
}

type publishThenFail struct {
	delegate ports.OutboxPublisher
	mu       sync.Mutex
	n        int
}

func (p *publishThenFail) Publish(ctx context.Context, record ports.OutboxRecord) error {
	if err := p.delegate.Publish(ctx, record); err != nil {
		return fmt.Errorf("delegate publish failed: %w", err)
	}
	p.mu.Lock()
	p.n++
	p.mu.Unlock()
	return fmt.Errorf("simulated crash after broker publication")
}

func (p *publishThenFail) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.n
}

type recoveryEnvironment struct {
	sqsEndpoint string
	queueURL    string
	dlqURL      string
	eventQueue  string
}

func recoveryEnv(t *testing.T) recoveryEnvironment {
	t.Helper()
	if os.Getenv("RECOVERY_RUN") != "1" {
		t.Skip("set RECOVERY_RUN=1 with PostgreSQL and LocalStack running")
	}
	return recoveryEnvironment{
		sqsEndpoint: envOr("TEST_SQS_ENDPOINT", "http://localhost:4566"),
		queueURL:    envOr("TEST_SQS_QUEUE_URL", "http://localhost:4566/000000000000/wager-transactions.fifo"),
		dlqURL:      envOr("TEST_SQS_DLQ_URL", "http://localhost:4566/000000000000/wager-transactions-dlq.fifo"),
		eventQueue:  envOr("TEST_SQS_EVENT_QUEUE_URL", "http://localhost:4566/000000000000/wager-events.fifo"),
	}
}

func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("PostgreSQL unavailable: %v", err)
	}
	return pool
}

func resetDatabase(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	for _, migration := range []string{"../migrations/000001_bootstrap.up.sql",
		"../migrations/000002_transaction_reversal_claim.up.sql",
		"../migrations/000003_pending_reference_retry.up.sql"} {
		data, err := os.ReadFile(migration)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), string(data)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(context.Background(), `TRUNCATE wallet_ledger_entries, outbox_events, inbox_messages, wager_transactions, wallets CASCADE`); err != nil {
		t.Fatal(err)
	}
}

func newWalletService(pool *pgxpool.Pool) *services.WalletService {
	return services.NewWalletService(postgres.NewUnitOfWork(pool), postgres.NewWalletRepository(pool), postgres.NewTransactionRepository(pool), postgres.NewLedgerRepository(pool), postgres.NewOutboxRepository(pool))
}

func newTransactionService(pool *pgxpool.Pool) *services.TransactionService {
	return services.NewTransactionService(postgres.NewUnitOfWork(pool), postgres.NewWalletRepository(pool), postgres.NewTransactionRepository(pool), postgres.NewLedgerRepository(pool), postgres.NewOutboxRepository(pool))
}

func newSQSClient(t *testing.T, endpoint string) *awssqs.Client {
	t.Helper()
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion("us-east-1"), awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")))
	if err != nil {
		t.Fatal(err)
	}
	return awssqs.NewFromConfig(cfg, func(options *awssqs.Options) { options.BaseEndpoint = &endpoint })
}

func receiveOne(t *testing.T, client *awssqs.Client, queueURL string, visibility int32) *types.Message {
	t.Helper()
	result, err := client.ReceiveMessage(context.Background(), &awssqs.ReceiveMessageInput{QueueUrl: &queueURL, MaxNumberOfMessages: 1, VisibilityTimeout: visibility, WaitTimeSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("received %d messages, want 1", len(result.Messages))
	}
	return &result.Messages[0]
}

func receiveAll(t *testing.T, client *awssqs.Client, queueURL string) []types.Message {
	t.Helper()
	result, err := client.ReceiveMessage(context.Background(), &awssqs.ReceiveMessageInput{QueueUrl: &queueURL, MaxNumberOfMessages: 10, WaitTimeSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	return result.Messages
}

func wagerMessage(messageID, walletID, externalID, idempotencyKey string) string {
	message := map[string]any{
		"messageId": messageID, "type": "WagerTransactionRequested", "occurredAt": time.Now().UTC().Format(time.RFC3339Nano),
		"data": map[string]any{
			"providerId": "provider", "externalTransactionId": externalID, "idempotencyKey": idempotencyKey,
			"playerId": "00000000-0000-0000-0000-000000000001", "walletId": walletID, "roundId": "round", "gameId": "game", "kind": "BET",
			"money": map[string]string{"amount": "10.00", "currency": "BRL"},
		},
	}
	data, _ := json.Marshal(message)
	return string(data)
}

func mustMoney(t *testing.T, amount string) money.Money {
	t.Helper()
	value, err := money.Parse(amount, money.BRL)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("condition was not satisfied before timeout")
}

func containsMessageID(body, messageID string) bool {
	var envelope struct {
		MessageID string `json:"messageId"`
	}
	if json.Unmarshal([]byte(body), &envelope) != nil {
		return false
	}
	return envelope.MessageID == messageID
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
