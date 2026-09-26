package bootstrap

import (
	"context"
	"log/slog"
	"time"

	"backend-challenge/internal/application/publisher"
	"backend-challenge/internal/config"
	"backend-challenge/internal/observability"
	"go.uber.org/fx"
)

type OutboxWorker struct {
	publisher    *publisher.OutboxPublisher
	log          *slog.Logger
	metrics      *observability.Metrics
	pollInterval time.Duration
	stop         chan struct{}
	done         chan struct{}
}

func NewOutboxWorker(lc fx.Lifecycle, publisher *publisher.OutboxPublisher, log *slog.Logger, metrics *observability.Metrics, cfg config.Config) *OutboxWorker {
	worker := &OutboxWorker{publisher: publisher, log: log, metrics: metrics, pollInterval: cfg.OutboxPollInterval, stop: make(chan struct{}), done: make(chan struct{})}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go worker.run(context.Background())
			return nil
		},
		OnStop: func(ctx context.Context) error {
			select {
			case <-worker.stop:
			default:
				close(worker.stop)
			}
			select {
			case <-worker.done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
	return worker
}

func (w *OutboxWorker) run(ctx context.Context) {
	defer close(w.done)
	w.log.Info("outbox publisher started")
	for {
		select {
		case <-ctx.Done():
			w.log.Info("outbox publisher stopped")
			return
		case <-w.stop:
			w.log.Info("outbox publisher stopped")
			return
		default:
		}
		if err := w.publisher.PublishBatch(ctx); err != nil {
			w.log.Error("outbox publish batch failed", "error", err)
		}
		timer := time.NewTimer(w.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			w.log.Info("outbox publisher stopped")
			return
		case <-w.stop:
			timer.Stop()
			w.log.Info("outbox publisher stopped")
			return
		case <-timer.C:
		}
	}
}
