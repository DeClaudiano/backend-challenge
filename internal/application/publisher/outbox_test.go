package publisher

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"backend-challenge/internal/application/events"
	"backend-challenge/internal/application/ports"
)

type fakeOutboxRepository struct {
	mu        sync.Mutex
	record    ports.OutboxRecord
	claimed   bool
	published bool
	failed    bool
}

func (f *fakeOutboxRepository) Append(context.Context, events.Envelope) error { return nil }
func (f *fakeOutboxRepository) ClaimPending(_ context.Context, now, leaseUntil time.Time, _ int) ([]ports.OutboxRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.published || (f.claimed && f.record.LeaseUntil != nil && f.record.LeaseUntil.After(now)) {
		return nil, nil
	}
	f.claimed = true
	f.record.LeaseUntil = &leaseUntil
	return []ports.OutboxRecord{f.record}, nil
}
func (f *fakeOutboxRepository) MarkPublished(_ context.Context, _ string, _, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = true
	f.record.LeaseUntil = nil
	return nil
}
func (f *fakeOutboxRepository) RecordFailure(_ context.Context, _ string, _, next time.Time, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed = true
	f.record.Attempts++
	f.record.NextAttemptAt = next
	f.record.LeaseUntil = nil
	return nil
}

type fakeBroker struct {
	err     error
	payload []byte
	calls   int
}

func (f *fakeBroker) Publish(_ context.Context, record ports.OutboxRecord) error {
	f.calls++
	f.payload = append([]byte(nil), record.Payload...)
	return f.err
}

func TestPublishBatchMarksOnlyAfterBrokerSuccessAndPreservesPayload(t *testing.T) {
	repository := &fakeOutboxRepository{record: ports.OutboxRecord{EventID: "event-1", AggregateID: "wallet-1", Payload: []byte(`{"eventId":"event-1"}`), Attempts: 0}}
	broker := &fakeBroker{}
	publisher := NewOutboxPublisher(repository, broker)
	publisher.now = func() time.Time { return time.Unix(100, 0).UTC() }
	if err := publisher.PublishBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !repository.published || repository.failed || broker.calls != 1 || string(broker.payload) != `{"eventId":"event-1"}` {
		t.Fatalf("repo=%+v broker=%+v", repository, broker)
	}
}

func TestPublishBatchRecordsFailureAndDoesNotMarkPublished(t *testing.T) {
	repository := &fakeOutboxRepository{record: ports.OutboxRecord{EventID: "event-1", AggregateID: "wallet-1", Payload: []byte(`{"eventId":"event-1"}`)}}
	broker := &fakeBroker{err: errors.New("broker unavailable")}
	publisher := NewOutboxPublisher(repository, broker)
	publisher.now = func() time.Time { return time.Unix(100, 0).UTC() }
	if err := publisher.PublishBatch(context.Background()); err == nil {
		t.Fatal("expected publish error")
	}
	if repository.published || !repository.failed || repository.record.Attempts != 1 {
		t.Fatalf("repo=%+v", repository)
	}
}

func TestConcurrentPublishersDoNotClaimActiveRecordTwice(t *testing.T) {
	repository := &fakeOutboxRepository{record: ports.OutboxRecord{EventID: "event-1", AggregateID: "wallet-1", Payload: []byte(`{"eventId":"event-1"}`)}}
	firstBroker, secondBroker := &fakeBroker{}, &fakeBroker{}
	first, second := NewOutboxPublisher(repository, firstBroker), NewOutboxPublisher(repository, secondBroker)
	first.now = func() time.Time { return time.Unix(100, 0).UTC() }
	second.now = first.now
	var wait sync.WaitGroup
	wait.Add(2)
	go func() { defer wait.Done(); _ = first.PublishBatch(context.Background()) }()
	go func() { defer wait.Done(); _ = second.PublishBatch(context.Background()) }()
	wait.Wait()
	if firstBroker.calls+secondBroker.calls != 1 {
		t.Fatalf("published calls=%d", firstBroker.calls+secondBroker.calls)
	}
}

func TestExpiredLeaseCanBeClaimedAgain(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	lease := now.Add(-time.Second)
	repository := &fakeOutboxRepository{record: ports.OutboxRecord{EventID: "event-1", AggregateID: "wallet-1", Payload: []byte(`{"eventId":"event-1"}`), LeaseUntil: &lease}, claimed: true}
	broker := &fakeBroker{}
	publisher := NewOutboxPublisher(repository, broker)
	publisher.now = func() time.Time { return now }
	if err := publisher.PublishBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if broker.calls != 1 || !repository.published {
		t.Fatalf("lease recovery failed: repo=%+v broker=%+v", repository, broker)
	}
}

func TestRunStopsBeforeClaimWhenContextIsCanceled(t *testing.T) {
	repository := &fakeOutboxRepository{record: ports.OutboxRecord{EventID: "event-1", AggregateID: "wallet-1", Payload: []byte(`{"eventId":"event-1"}`)}}
	broker := &fakeBroker{}
	publisher := NewOutboxPublisher(repository, broker)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	publisher.Run(ctx)
	if broker.calls != 0 {
		t.Fatalf("shutdown should stop before publishing, calls=%d", broker.calls)
	}
}
