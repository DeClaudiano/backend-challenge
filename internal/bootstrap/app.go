package bootstrap

import (
	"context"
	"log/slog"
	"os"
	"time"

	"backend-challenge/internal/observability"

	httpadapter "backend-challenge/internal/adapters/http"
	"backend-challenge/internal/adapters/oidc"
	"backend-challenge/internal/adapters/postgres"
	"backend-challenge/internal/adapters/sqs"
	"backend-challenge/internal/application/consumers"
	"backend-challenge/internal/application/ports"
	"backend-challenge/internal/application/publisher"
	"backend-challenge/internal/application/references"
	"backend-challenge/internal/application/services"
	"backend-challenge/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"
)

func NewLogger() *slog.Logger                               { return slog.New(slog.NewJSONHandler(os.Stdout, nil)) }
func NewMetrics() *observability.Metrics                    { return observability.NewMetrics() }
func NewMetricsPort(m *observability.Metrics) ports.Metrics { return m }
func NewConfig() (config.Config, error)                     { return config.Load() }
func NewHTTPAddr(cfg config.Config) string                  { return cfg.HTTPAddr }

func NewPostgres(lc fx.Lifecycle, cfg config.Config) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error { pool.Close(); return nil }})
	return pool, nil
}

func NewSQS(cfg config.Config) (*sqs.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return sqs.Open(ctx, cfg)
}

func Common() fx.Option {
	return fx.Options(fx.Provide(NewLogger, NewConfig, NewHTTPAddr, NewSQS, NewMetrics, NewMetricsPort, oidc.NewConfig))
}

func API() fx.Option {
	return fx.Options(Common(), fx.Provide(
		NewPostgres,
		fx.Annotate(postgres.NewUnitOfWork, fx.As(new(ports.UnitOfWork))),
		fx.Annotate(postgres.NewWalletRepository, fx.As(new(ports.WalletRepository))),
		fx.Annotate(postgres.NewTransactionRepository, fx.As(new(ports.TransactionRepository))),
		fx.Annotate(postgres.NewLedgerRepository, fx.As(new(ports.LedgerRepository))),
		fx.Annotate(postgres.NewOutboxRepository, fx.As(new(ports.OutboxRepository))),
		services.NewWalletService,
		services.NewTransactionServiceWithMetrics,
		services.NewQueryServiceWithMetrics,
		oidc.NewVerifier,
		httpadapter.NewBusinessHandler,
		httpadapter.NewServer,
	), fx.Invoke(func(*httpadapter.Server) {}))
}

func NewConfiguredOutboxPublisher(repository ports.OutboxRepository, broker ports.OutboxPublisher, metrics ports.Metrics, cfg config.Config) *publisher.OutboxPublisher {
	return publisher.NewOutboxPublisherWithConfigAndMetrics(repository, broker, cfg.OutboxBatchSize, cfg.OutboxLeaseDuration, metrics)
}

func WorkerApp() fx.Option {
	return fx.Options(Common(), fx.Provide(
		NewPostgres,
		fx.Annotate(postgres.NewUnitOfWork, fx.As(new(ports.UnitOfWork))),
		fx.Annotate(postgres.NewWalletRepository, fx.As(new(ports.WalletRepository))),
		fx.Annotate(postgres.NewTransactionRepository, fx.As(new(ports.TransactionRepository)), fx.As(new(ports.PendingReferenceRepository))),
		fx.Annotate(postgres.NewLedgerRepository, fx.As(new(ports.LedgerRepository))),
		fx.Annotate(postgres.NewInboxRepository, fx.As(new(ports.InboxRepository))),
		fx.Annotate(postgres.NewOutboxRepository, fx.As(new(ports.OutboxRepository))),
		fx.Annotate(sqs.NewOutboxPublisher, fx.As(new(ports.OutboxPublisher))),
		services.NewTransactionServiceWithMetrics,
		references.NewServiceWithMetrics,
		consumers.NewWagerConsumerWithMetrics,
		NewReferenceWorker,
		NewConfiguredOutboxPublisher,
		NewWorker,
		NewOutboxWorker,
	), fx.Invoke(func(*Worker) {}, func(*OutboxWorker) {}, func(*ReferenceWorker) {}))
}
