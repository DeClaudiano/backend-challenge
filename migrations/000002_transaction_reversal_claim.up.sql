ALTER TABLE wager_transactions
    ADD COLUMN IF NOT EXISTS reversed_by_transaction_id UUID REFERENCES wager_transactions(id) ON DELETE RESTRICT;

ALTER TABLE wager_transactions
    ADD COLUMN IF NOT EXISTS reversal_kind TEXT;

ALTER TABLE wager_transactions
    DROP CONSTRAINT IF EXISTS wager_transactions_reversal_kind_check;

ALTER TABLE wager_transactions
    ADD CONSTRAINT wager_transactions_reversal_kind_check
    CHECK (reversal_kind IS NULL OR reversal_kind IN ('REFUND', 'ROLLBACK'));

ALTER TABLE wager_transactions
    DROP CONSTRAINT IF EXISTS wager_transactions_reversal_pair_check;

ALTER TABLE wager_transactions
    ADD CONSTRAINT wager_transactions_reversal_pair_check
    CHECK ((reversed_by_transaction_id IS NULL) = (reversal_kind IS NULL));
