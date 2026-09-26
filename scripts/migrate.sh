#!/bin/sh
set -eu

DIRECTION=${1:-up}
DATABASE_URL=${DATABASE_URL:-postgres://wallet:wallet@localhost:5432/wallet?sslmode=disable}

case "$DIRECTION" in
  up) psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/000001_bootstrap.up.sql && psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/000002_transaction_reversal_claim.up.sql && psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/000003_pending_reference_retry.up.sql ;;
  down) psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/000003_pending_reference_retry.down.sql && psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/000002_transaction_reversal_claim.down.sql && psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/000001_bootstrap.down.sql ;;
  *) echo "usage: $0 {up|down}" >&2; exit 2 ;;
esac
