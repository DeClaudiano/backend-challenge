ALTER TABLE wager_transactions DROP CONSTRAINT IF EXISTS wager_transactions_reversal_pair_check;
ALTER TABLE wager_transactions DROP CONSTRAINT IF EXISTS wager_transactions_reversal_kind_check;
ALTER TABLE wager_transactions DROP COLUMN IF EXISTS reversed_by_transaction_id;
ALTER TABLE wager_transactions DROP COLUMN IF EXISTS reversal_kind;
