package postgres

import (
	"context"
	"testing"
	"time"

	"backend-challenge/internal/application/ports"
	"backend-challenge/internal/domain"
)

func TestOutboxRepositoryImplementsPort(t *testing.T) {
	var _ ports.OutboxRepository = (*OutboxRepository)(nil)
}

func TestOutboxRepositoryValidatesClaimParameters(t *testing.T) {
	repository := &OutboxRepository{}
	now := time.Unix(1, 0).UTC()
	if _, err := repository.ClaimPending(context.Background(), now, now, 1); !hasOutboxCode(err, domain.ErrInvalidValue) {
		t.Fatalf("claim error=%v", err)
	}
	if _, err := repository.ClaimPending(context.Background(), now, now.Add(time.Minute), 0); !hasOutboxCode(err, domain.ErrInvalidValue) {
		t.Fatalf("limit error=%v", err)
	}
}

func TestOutboxRepositoryValidatesPublicationAndFailureParameters(t *testing.T) {
	repository := &OutboxRepository{}
	if err := repository.MarkPublished(context.Background(), "", time.Time{}, time.Time{}); !hasOutboxCode(err, domain.ErrInvalidValue) {
		t.Fatalf("published error=%v", err)
	}
	if err := repository.RecordFailure(context.Background(), "", time.Time{}, time.Time{}, ""); !hasOutboxCode(err, domain.ErrInvalidValue) {
		t.Fatalf("failure error=%v", err)
	}
}

func hasOutboxCode(err error, code domain.ErrorCode) bool {
	value, ok := err.(*domain.Error)
	return ok && value.Code == code
}
