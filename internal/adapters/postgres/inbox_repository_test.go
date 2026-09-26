package postgres

import (
	"context"
	"testing"
	"time"

	"backend-challenge/internal/application/ports"
	"backend-challenge/internal/domain"
)

func TestInboxRepositoryImplementsPort(t *testing.T) {
	var _ ports.InboxRepository = (*InboxRepository)(nil)
}

func TestInboxRepositoryValidatesClaimAndCompletionInput(t *testing.T) {
	repository := &InboxRepository{}
	if _, err := repository.Claim(context.Background(), "", "message", "hash"); !hasInboxCode(err, domain.ErrInvalidValue) {
		t.Fatalf("claim error=%v", err)
	}
	if err := repository.MarkCompleted(context.Background(), "consumer", "message", time.Time{}); !hasInboxCode(err, domain.ErrInvalidValue) {
		t.Fatalf("completion error=%v", err)
	}
}

func hasInboxCode(err error, code domain.ErrorCode) bool {
	value, ok := err.(*domain.Error)
	return ok && value.Code == code
}
