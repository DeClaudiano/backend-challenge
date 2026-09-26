# E2E real infrastructure

These tests exercise the running Docker Compose environment instead of replacing PostgreSQL, Keycloak or LocalStack with mocks.

## Run

From the repository root:

```bash
docker compose up --build -d
E2E_RUN=1 go test ./tests/e2e -count=1 -v
```

The suite uses the local Keycloak `client_credentials` clients, creates wallets through the protected HTTP API, processes a wager through HTTP and SQS, verifies persistent idempotent replay across transports, checks provider/internal authorization boundaries, readiness/liveness, and consumes an event published by the transactional Outbox to the real LocalStack SQS event queue.

Optional environment variables:

```text
E2E_API_URL
E2E_OIDC_TOKEN_URL
E2E_SQS_ENDPOINT
E2E_SQS_QUEUE_URL
E2E_SQS_EVENT_QUEUE_URL
```

The tests intentionally skip unless `E2E_RUN=1`, so the normal `go test ./...` suite remains deterministic when infrastructure is not running.
