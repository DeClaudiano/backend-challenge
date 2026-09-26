package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{"APP_ENV", "HTTP_ADDR", "SHUTDOWN_WAIT", "DATABASE_URL", "SQS_QUEUE_URL", "SQS_REGION", "SQS_ENDPOINT", "OIDC_ISSUER", "OIDC_AUDIENCE"} {
		_ = os.Unsetenv(key)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.SQSRegion != "us-east-1" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.ShutdownWait != 15*time.Second {
		t.Fatalf("unexpected shutdown wait: %s", cfg.ShutdownWait)
	}
}

func TestLoadRejectsInvalidShutdownWait(t *testing.T) {
	t.Setenv("SHUTDOWN_WAIT", "not-a-duration")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid duration error")
	}
}

func TestLoadOperationalConfiguration(t *testing.T) {
	t.Setenv("SQS_VISIBILITY_TIMEOUT", "45")
	t.Setenv("SQS_WAIT_TIME_SECONDS", "10")
	t.Setenv("SQS_MAX_RECEIVE_COUNT", "7")
	t.Setenv("OUTBOX_BATCH_SIZE", "25")
	t.Setenv("OUTBOX_LEASE_DURATION", "45s")
	t.Setenv("OUTBOX_POLL_INTERVAL", "2s")
	t.Setenv("REFERENCE_BATCH_SIZE", "50")
	t.Setenv("REFERENCE_MAX_ATTEMPTS", "12")
	t.Setenv("REFERENCE_BASE_BACKOFF", "2s")
	t.Setenv("REFERENCE_MAX_BACKOFF", "30m")
	t.Setenv("REFERENCE_TTL", "48h")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SQSVisibilityTimeout != 45 || cfg.SQSWaitTimeSeconds != 10 || cfg.SQSMaxReceiveCount != 7 {
		t.Fatalf("unexpected SQS configuration: %+v", cfg)
	}
	if cfg.OutboxBatchSize != 25 || cfg.OutboxLeaseDuration != 45*time.Second || cfg.OutboxPollInterval != 2*time.Second {
		t.Fatalf("unexpected Outbox configuration: %+v", cfg)
	}
	if cfg.ReferenceBatchSize != 50 || cfg.ReferenceMaxAttempts != 12 || cfg.ReferenceBaseBackoff != 2*time.Second || cfg.ReferenceMaxBackoff != 30*time.Minute || cfg.ReferenceTTL != 48*time.Hour {
		t.Fatalf("unexpected reference configuration: %+v", cfg)
	}
}
