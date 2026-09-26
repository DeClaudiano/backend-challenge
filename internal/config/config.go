package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv               string
	HTTPAddr             string
	ShutdownWait         time.Duration
	DatabaseURL          string
	SQSQueueURL          string
	SQSRegion            string
	SQSEndpoint          string
	SQSVisibilityTimeout int32
	SQSWaitTimeSeconds   int32
	SQSMaxReceiveCount   int32
	OutboxBatchSize      int
	OutboxLeaseDuration  time.Duration
	OutboxPollInterval   time.Duration
	ReferenceBatchSize   int
	ReferenceMaxAttempts int
	ReferenceBaseBackoff time.Duration
	ReferenceMaxBackoff  time.Duration
	ReferenceTTL         time.Duration
	OIDCIssuer           string
	OIDCAudience         string
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:               env("APP_ENV", "development"),
		HTTPAddr:             env("HTTP_ADDR", ":8080"),
		ShutdownWait:         duration("SHUTDOWN_WAIT", 15*time.Second),
		DatabaseURL:          env("DATABASE_URL", "postgres://wallet:wallet@localhost:5432/wallet?sslmode=disable"),
		SQSQueueURL:          env("SQS_QUEUE_URL", "http://localhost:4566/000000000000/wager-transactions.fifo"),
		SQSRegion:            env("SQS_REGION", "us-east-1"),
		SQSEndpoint:          env("SQS_ENDPOINT", "http://localhost:4566"),
		SQSVisibilityTimeout: int32Value("SQS_VISIBILITY_TIMEOUT", 30),
		SQSWaitTimeSeconds:   int32Value("SQS_WAIT_TIME_SECONDS", 1),
		SQSMaxReceiveCount:   int32Value("SQS_MAX_RECEIVE_COUNT", 5),
		OutboxBatchSize:      intValue("OUTBOX_BATCH_SIZE", 10),
		OutboxLeaseDuration:  duration("OUTBOX_LEASE_DURATION", 30*time.Second),
		OutboxPollInterval:   duration("OUTBOX_POLL_INTERVAL", time.Second),
		ReferenceBatchSize:   intValue("REFERENCE_BATCH_SIZE", 100),
		ReferenceMaxAttempts: intValue("REFERENCE_MAX_ATTEMPTS", 8),
		ReferenceBaseBackoff: duration("REFERENCE_BASE_BACKOFF", time.Second),
		ReferenceMaxBackoff:  duration("REFERENCE_MAX_BACKOFF", time.Hour),
		ReferenceTTL:         duration("REFERENCE_TTL", 24*time.Hour),
		OIDCIssuer:           env("OIDC_ISSUER", "http://localhost:8081/realms/wallet"),
		OIDCAudience:         env("OIDC_AUDIENCE", "wallet-api"),
	}
	if cfg.HTTPAddr == "" || cfg.DatabaseURL == "" || cfg.SQSQueueURL == "" || cfg.SQSRegion == "" {
		return Config{}, fmt.Errorf("required configuration is empty")
	}
	if cfg.ShutdownWait <= 0 || cfg.OutboxLeaseDuration <= 0 || cfg.OutboxPollInterval <= 0 || cfg.ReferenceBaseBackoff <= 0 || cfg.ReferenceMaxBackoff <= 0 || cfg.ReferenceTTL <= 0 {
		return Config{}, fmt.Errorf("duration configuration must be positive")
	}
	if cfg.SQSVisibilityTimeout < 0 || cfg.SQSWaitTimeSeconds < 0 || cfg.SQSWaitTimeSeconds > 20 || cfg.SQSMaxReceiveCount <= 0 || cfg.OutboxBatchSize <= 0 || cfg.ReferenceBatchSize <= 0 || cfg.ReferenceMaxAttempts <= 0 {
		return Config{}, fmt.Errorf("operational configuration contains invalid values")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func duration(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return -1
	}
	return parsed
}

func Bool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func intValue(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return parsed
}

func int32Value(key string, fallback int32) int32 {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return -1
	}
	return int32(parsed)
}
