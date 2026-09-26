package integration

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"backend-challenge/internal/adapters/postgres"
	"backend-challenge/internal/application/services"
	applicationwallet "backend-challenge/internal/application/wallet"
	"backend-challenge/internal/domain"
	"backend-challenge/internal/domain/money"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProcessHelper(t *testing.T) {
	if os.Getenv("INTEGRATION_HELPER") != "1" {
		return
	}

	pool := openPool(t)
	defer pool.Close()

	waitForStart(t)

	service := newTransactionService(pool)

	amount, err := money.Parse("80.00", money.BRL)
	if err != nil {
		t.Fatal(err)
	}

	kind := os.Getenv("HELPER_KIND")

	key := os.Getenv("TEST_IDEMPOTENCY_KEY")
	external := os.Getenv("TEST_EXTERNAL_ID")

	switch kind {
	case "bet-a":
		if key == "" {
			key = "provider:bet-a"
		}
		if external == "" {
			external = "bet-a"
		}

	case "bet-b":
		if key == "" {
			key = "provider:bet-b"
		}
		if external == "" {
			external = "bet-b"
		}

	case "duplicate":
		if key == "" {
			key = "provider:bet-a"
		}
		if external == "" {
			external = "bet-a"
		}
	}

	result, err := service.Process(context.Background(), services.ProcessTransactionInput{
		ProviderID:            "provider",
		ExternalTransactionID: external,
		IdempotencyKey:        key,
		PlayerID:              os.Getenv("TEST_PLAYER_ID"),
		WalletID:              os.Getenv("TEST_WALLET_ID"),
		RoundID:               "round",
		GameID:                "game",
		Kind:                  "BET",
		Money:                 amount,
	})

	if err != nil && result.Transaction.ID() == "" {
		t.Fatal(err)
	}
}

func TestPostgresAtomicityConstraintsAndPersistence(t *testing.T) {
	pool := openPool(t)
	defer pool.Close()
	resetDatabase(t, pool)

	walletService := newWalletService(pool)
	opening, err := walletService.Open(context.Background(), services.OpenWalletInput{
		PlayerID: "00000000-0000-0000-0000-000000000001", Currency: money.BRL, InitialBalance: mustMoney(t, "100.00"),
	})
	if err != nil {
		t.Fatal(err)
	}
	walletID := opening.Wallet.ID()

	var ledgerCount, outboxCount int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM wallet_ledger_entries WHERE wallet_id=$1`, walletID).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM outbox_events`).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 1 || outboxCount != 2 {
		t.Fatalf("opening persistence ledger=%d outbox=%d", ledgerCount, outboxCount)
	}

	if _, err := pool.Exec(context.Background(), `UPDATE wallet_ledger_entries SET amount_minor=1 WHERE wallet_id=$1`, walletID); err == nil {
		t.Fatal("ledger update unexpectedly succeeded")
	}
	if _, err := pool.Exec(context.Background(), `DELETE FROM wallet_ledger_entries WHERE wallet_id=$1`, walletID); err == nil {
		t.Fatal("ledger delete unexpectedly succeeded")
	}

	transactionService := newTransactionService(pool)
	result, err := transactionService.Process(context.Background(), services.ProcessTransactionInput{
		ProviderID: "provider", ExternalTransactionID: "atomic-bet", IdempotencyKey: "provider:atomic-bet",
		PlayerID: "00000000-0000-0000-0000-000000000001", WalletID: walletID, RoundID: "round", GameID: "game",
		Kind: "BET", Money: mustMoney(t, "25.00"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Balance.Amount() != "75.00" {
		t.Fatalf("balance=%s", result.Balance.Amount())
	}

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO wallets(player_id,currency,balance_minor,version) VALUES($1,'BRL',0,1)`, "00000000-0000-0000-0000-000000000099")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var rolledBack int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM wallets WHERE player_id=$1`, "00000000-0000-0000-0000-000000000099").Scan(&rolledBack); err != nil {
		t.Fatal(err)
	}
	if rolledBack != 0 {
		t.Fatal("rolled-back wallet remained persisted")
	}

	rollbackPlayer := "00000000-0000-0000-0000-000000000098"
	opened, err := applicationwallet.Open(applicationwallet.OpenInput{
		WalletID: "00000000-0000-0000-0000-000000000098", TransactionID: "00000000-0000-0000-0000-000000000097", LedgerID: "00000000-0000-0000-0000-000000000096",
		PlayerID: rollbackPlayer, Currency: money.BRL, InitialBalance: mustMoney(t, "10.00"), Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	uow := postgres.NewUnitOfWork(pool)
	if err := uow.Within(ctx, func(txCtx context.Context) error {
		if err := postgres.NewWalletRepository(pool).Create(txCtx, opened.Wallet); err != nil {
			return err
		}
		if err := postgres.NewTransactionRepository(pool).Create(txCtx, *opened.Transaction); err != nil {
			return err
		}
		if err := postgres.NewLedgerRepository(pool).Append(txCtx, *opened.Ledger); err != nil {
			return err
		}
		return postgres.NewLedgerRepository(pool).Append(txCtx, *opened.Ledger)
	}); err == nil {
		t.Fatal("duplicate ledger append unexpectedly committed")
	}
	var walletRows, transactionRows, ledgerRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM wallets WHERE player_id=$1`, rollbackPlayer).Scan(&walletRows); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM wager_transactions WHERE wallet_id=$1`, "00000000-0000-0000-0000-000000000098").Scan(&transactionRows); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM wallet_ledger_entries WHERE wallet_id=$1`, "00000000-0000-0000-0000-000000000098").Scan(&ledgerRows); err != nil {
		t.Fatal(err)
	}
	if walletRows != 0 || transactionRows != 0 || ledgerRows != 0 {
		t.Fatalf("partial transaction remained wallet=%d transaction=%d ledger=%d", walletRows, transactionRows, ledgerRows)
	}
}

func TestPostgresDistributedWalletContention(t *testing.T) {
	pool := openPool(t)
	defer pool.Close()
	resetDatabase(t, pool)
	opening, err := newWalletService(pool).Open(context.Background(), services.OpenWalletInput{
		PlayerID: "00000000-0000-0000-0000-000000000002", Currency: money.BRL, InitialBalance: mustMoney(t, "100.00"),
	})
	if err != nil {
		t.Fatal(err)
	}

	startAt := time.Now().Add(500 * time.Millisecond)
	children := make([]*exec.Cmd, 0, 3)
	for _, kind := range []string{"bet-a", "bet-b", "duplicate"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestProcessHelper$")
		cmd.Env = append(os.Environ(),
			"INTEGRATION_HELPER=1", "HELPER_KIND="+kind,
			"TEST_PLAYER_ID=00000000-0000-0000-0000-000000000002",
			"TEST_WALLET_ID="+opening.Wallet.ID(), "TEST_START_AT="+fmt.Sprint(startAt.UnixNano()),
		)
		children = append(children, cmd)
	}
	outputs := make([]bytes.Buffer, len(children))
	for i, child := range children {
		child.Stdout = &outputs[i]
		child.Stderr = &outputs[i]
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errCh := make(chan error, len(children))

	for i, child := range children {
		wg.Add(1)

		go func(i int, child *exec.Cmd) {
			defer wg.Done()

			if err := child.Wait(); err != nil {
				errCh <- fmt.Errorf(
					"child %d failed: %w\n%s",
					i,
					err,
					outputs[i].String(),
				)
			}
		}(i, child)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatal(err)
	}

	var processed, rejected, debits int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM wager_transactions WHERE kind='BET' AND status='PROCESSED'`).Scan(&processed); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM wager_transactions WHERE kind='BET' AND status='REJECTED' AND failure_code=$1`, string(domain.ErrInsufficientBalance)).Scan(&rejected); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM wallet_ledger_entries WHERE direction='DEBIT'`).Scan(&debits); err != nil {
		t.Fatal(err)
	}
	if processed != 1 || rejected != 1 || debits != 1 {
		t.Fatalf("processed=%d rejected=%d debits=%d", processed, rejected, debits)
	}
	var balance int64
	if err := pool.QueryRow(context.Background(), `SELECT balance_minor FROM wallets WHERE id=$1`, opening.Wallet.ID()).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != 2000 {
		t.Fatalf("balance_minor=%d, want 2000", balance)
	}
}

func TestPostgresIndependentWalletsProceedInParallel(t *testing.T) {
	pool := openPool(t)
	defer pool.Close()
	resetDatabase(t, pool)
	service := newWalletService(pool)
	first, err := service.Open(context.Background(), services.OpenWalletInput{PlayerID: "00000000-0000-0000-0000-000000000004", Currency: money.BRL, InitialBalance: mustMoney(t, "100.00")})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Open(context.Background(), services.OpenWalletInput{PlayerID: "00000000-0000-0000-0000-000000000005", Currency: money.BRL, InitialBalance: mustMoney(t, "100.00")})
	if err != nil {
		t.Fatal(err)
	}
	startAt := time.Now().Add(500 * time.Millisecond)
	children := []*exec.Cmd{
		exec.Command(os.Args[0], "-test.run=^TestProcessHelper$"),
		exec.Command(os.Args[0], "-test.run=^TestProcessHelper$"),
	}
	players := []string{"00000000-0000-0000-0000-000000000004", "00000000-0000-0000-0000-000000000005"}
	wallets := []string{first.Wallet.ID(), second.Wallet.ID()}
	outputs := make([]bytes.Buffer, len(children))
	for i, child := range children {
		child.Env = append(os.Environ(), "INTEGRATION_HELPER=1", "HELPER_KIND=independent", "TEST_PLAYER_ID="+players[i], "TEST_WALLET_ID="+wallets[i], "TEST_EXTERNAL_ID=independent-bet-"+fmt.Sprint(i), "TEST_IDEMPOTENCY_KEY=provider:independent-"+fmt.Sprint(i), "TEST_START_AT="+fmt.Sprint(startAt.UnixNano()))
		child.Stdout, child.Stderr = &outputs[i], &outputs[i]
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for i, child := range children {
		if err := child.Wait(); err != nil {
			t.Fatalf("independent child failed: %v\n%s", err, outputs[i].String())
		}
	}
	for _, walletID := range wallets {
		var balance int64
		if err := pool.QueryRow(context.Background(), `SELECT balance_minor FROM wallets WHERE id=$1`, walletID).Scan(&balance); err != nil {
			t.Fatal(err)
		}
		if balance != 2000 {
			t.Fatalf("wallet %s balance_minor=%d, want 2000", walletID, balance)
		}
	}
}

func TestPostgresConcurrentIdempotency(t *testing.T) {
	pool := openPool(t)
	defer pool.Close()
	resetDatabase(t, pool)
	opening, err := newWalletService(pool).Open(context.Background(), services.OpenWalletInput{
		PlayerID: "00000000-0000-0000-0000-000000000003", Currency: money.BRL, InitialBalance: mustMoney(t, "100.00"),
	})
	if err != nil {
		t.Fatal(err)
	}
	startAt := time.Now().Add(500 * time.Millisecond)
	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			child := exec.Command(os.Args[0], "-test.run=^TestProcessHelper$")
			child.Env = append(os.Environ(), "INTEGRATION_HELPER=1", "HELPER_KIND=bet-a", "TEST_PLAYER_ID=00000000-0000-0000-0000-000000000003", "TEST_WALLET_ID="+opening.Wallet.ID(), "TEST_START_AT="+fmt.Sprint(startAt.UnixNano()))
			if output, err := child.CombinedOutput(); err != nil {
				errs <- fmt.Errorf("%v: %s", err, output)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var movements, debits int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM wager_transactions WHERE external_transaction_id='bet-a'`).Scan(&movements); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM wallet_ledger_entries WHERE wallet_id=$1 AND direction='DEBIT'`, opening.Wallet.ID()).Scan(&debits); err != nil {
		t.Fatal(err)
	}
	if movements != 1 || debits != 1 {
		t.Fatalf("movements=%d debits=%d", movements, debits)
	}
}

func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for real PostgreSQL integration tests")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 2
	config.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("PostgreSQL unavailable: %v", err)
	}
	return pool
}

func resetDatabase(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, migration := range []string{"migrations/000001_bootstrap.up.sql", "migrations/000002_transaction_reversal_claim.up.sql", "migrations/000003_pending_reference_retry.up.sql"} {
		data, err := os.ReadFile("../../" + migration)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `TRUNCATE wallet_ledger_entries, outbox_events, inbox_messages, wager_transactions, wallets CASCADE`); err != nil {
		t.Fatal(err)
	}
}

func waitForStart(t *testing.T) {
	t.Helper()
	raw := os.Getenv("TEST_START_AT")
	if raw == "" {
		return
	}
	var nanos int64
	if _, err := fmt.Sscanf(raw, "%d", &nanos); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Until(time.Unix(0, nanos)))
}

func newWalletService(pool *pgxpool.Pool) *services.WalletService {
	uow := postgres.NewUnitOfWork(pool)
	return services.NewWalletService(uow, postgres.NewWalletRepository(pool), postgres.NewTransactionRepository(pool), postgres.NewLedgerRepository(pool), postgres.NewOutboxRepository(pool))
}
func newTransactionService(pool *pgxpool.Pool) *services.TransactionService {
	uow := postgres.NewUnitOfWork(pool)
	return services.NewTransactionService(uow, postgres.NewWalletRepository(pool), postgres.NewTransactionRepository(pool), postgres.NewLedgerRepository(pool), postgres.NewOutboxRepository(pool))
}
func mustMoney(t *testing.T, amount string) money.Money {
	t.Helper()
	value, err := money.Parse(amount, money.BRL)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
