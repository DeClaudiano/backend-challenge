package bootstrap

import (
	"context"
	"log/slog"
	"time"

	"backend-challenge/internal/adapters/sqs"
	"backend-challenge/internal/application/consumers"
	"backend-challenge/internal/observability"
	"go.uber.org/fx"
)

type Worker struct {
	queue    *sqs.Client
	consumer *consumers.WagerConsumer
	log      *slog.Logger
	metrics  *observability.Metrics
	stop     context.CancelFunc
	done     chan struct{}
}

func NewWorker(lc fx.Lifecycle, queue *sqs.Client, consumer *consumers.WagerConsumer, log *slog.Logger, metrics *observability.Metrics) *Worker {
	worker := &Worker{queue: queue, consumer: consumer, log: log, metrics: metrics, done: make(chan struct{})}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			worker.stop = cancel
			go worker.run(ctx)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if worker.stop != nil {
				worker.stop()
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

func (w *Worker) run(ctx context.Context) {
	defer close(w.done)
	w.log.Info("worker started", "queue_url", w.queue.QueueURL)
	for {
		select {
		case <-ctx.Done():
			w.log.Info("worker stopped")
			return
		default:
		}
		if w.metrics != nil {
			if depth, err := w.queue.DLQDepth(ctx); err == nil {
				w.metrics.SetDLQDepth(depth)
			}
		}
		messages, err := w.queue.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				w.log.Info("worker stopped")
				return
			}
			w.log.Error("sqs receive failed", "error", err)
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				w.log.Info("worker stopped")
				return
			case <-timer.C:
			}
			continue
		}
		for _, message := range messages {
			if message.Body == nil || message.ReceiptHandle == nil {
				w.log.Error("sqs message missing body or receipt handle")
				continue
			}
			messageLog := w.log
			if decoded, decodeErr := consumers.DecodeWagerTransactionMessage([]byte(*message.Body)); decodeErr == nil {
				messageLog = observability.MessageLogger(ctx, w.log, decoded.MessageID, "", decoded.Data.WalletID, decoded.Data.ProviderID)
			} else {
				messageLog = w.log.With("messageId", "unknown")
			}
			deleteMessage, err := w.consumer.Handle(ctx, []byte(*message.Body))
			if err != nil {
				messageLog.Error("wager message processing failed", "error", err)
				continue
			}
			if !deleteMessage {
				continue
			}
			if err := w.queue.Delete(ctx, *message.ReceiptHandle); err != nil {
				messageLog.Error("sqs message delete failed", "error", err)
			}
		}
	}
}
