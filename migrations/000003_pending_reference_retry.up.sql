ALTER TABLE wager_transactions
    ADD COLUMN IF NOT EXISTS reference_attempts INTEGER NOT NULL DEFAULT 0 CHECK (reference_attempts >= 0),
    ADD COLUMN IF NOT EXISTS reference_next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS reference_expires_at TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '24 hours');

CREATE INDEX IF NOT EXISTS wager_transactions_pending_reference_due_idx
    ON wager_transactions (reference_next_attempt_at, reference_expires_at, updated_at)
    WHERE status = 'PENDING_REFERENCE';
