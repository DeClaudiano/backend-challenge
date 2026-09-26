package bootstrap

import (
	"context"
	"log/slog"
	"time"

	"backend-challenge/internal/application/references"
	"backend-challenge/internal/config"
	"go.uber.org/fx"
)

type ReferenceWorker struct {
	service *references.Service
	config  references.Config
	log     *slog.Logger
	stop    chan struct{}
	done    chan struct{}
}

func NewReferenceWorker(lc fx.Lifecycle, service *references.Service, log *slog.Logger, cfg config.Config) *ReferenceWorker {
	worker := &ReferenceWorker{
		service: service,
		config: references.Config{
			BatchSize: cfg.ReferenceBatchSize, MaxAttempts: cfg.ReferenceMaxAttempts,
			BaseBackoff: cfg.ReferenceBaseBackoff, MaxBackoff: cfg.ReferenceMaxBackoff, ReferenceTTL: cfg.ReferenceTTL,
		},
		log: log, stop: make(chan struct{}), done: make(chan struct{}),
	}
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

func (w *ReferenceWorker) run(ctx context.Context) {
	defer close(w.done)
	w.log.Info("pending reference worker started")
	for {
		select {
		case <-ctx.Done():
			w.log.Info("pending reference worker stopped")
			return
		case <-w.stop:
			w.log.Info("pending reference worker stopped")
			return
		default:
		}
		if err := w.service.RunOnce(ctx, w.config); err != nil {
			w.log.Error("pending reference processing failed", "error", err)
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-w.stop:
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
