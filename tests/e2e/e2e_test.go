package e2e

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

type walletResponse struct {
	ID string `json:"id"`
}

type transactionResponse struct {
	TransactionID    string `json:"transactionId"`
	Status           string `json:"status"`
	IdempotentReplay bool   `json:"idempotentReplay"`
}

func TestE2EHTTPAuthenticationIdempotencyAndIsolation(t *testing.T) {
	env := loadEnv(t)
	getJSON[map[string]any](t, env.api+"/wagering/transactions/invalid", "", http.StatusUnauthorized)
	getJSON[map[string]any](t, env.api+"/wagering/transactions/invalid", "Bearer invalid", http.StatusUnauthorized)
	internalToken := token(t, env, "wallet-internal", "wallet-internal-secret")
	providerToken := token(t, env, "wallet-api", "wallet-api-secret")
	otherProviderToken := token(t, env, "wallet-provider-b", "wallet-provider-b-secret")
	providerID := subject(t, providerToken)

	playerID := newID(t)
	wallet := postJSON[walletResponse](t, env.api+"/wallets", internalToken, map[string]any{
		"playerId":       playerID,
		"initialBalance": map[string]string{"amount": "100.00", "currency": "BRL"},
	}, http.StatusCreated, "")

	body := map[string]any{
		"providerId":            "ignored-client-value",
		"externalTransactionId": "e2e-bet-" + newID(t),
		"playerId":              playerID,
		"walletId":              wallet.ID,
		"roundId":               "round-e2e",
		"gameId":                "game-e2e",
		"kind":                  "BET",
		"money":                 map[string]string{"amount": "25.00", "currency": "BRL"},
	}
	key := "e2e-http-" + newID(t)
	first := postJSON[transactionResponse](t, env.api+"/wagering/transactions", providerToken, body, http.StatusOK, key)
	if first.Status != "PROCESSED" || first.IdempotentReplay {
		t.Fatalf("unexpected first transaction response: %+v", first)
	}

	replay := postJSON[transactionResponse](t, env.api+"/wagering/transactions", providerToken, body, http.StatusOK, key)
	if !replay.IdempotentReplay || replay.TransactionID != first.TransactionID {
		t.Fatalf("unexpected replay response: %+v", replay)
	}

	getJSON[map[string]any](t, env.api+"/wagering/transactions/"+first.TransactionID, providerToken, http.StatusOK)
	getJSON[map[string]any](t, env.api+"/wagering/transactions/"+first.TransactionID, otherProviderToken, http.StatusForbidden)
	getJSON[map[string]any](t, env.api+"/providers/"+url.PathEscape(providerID)+"/wagering/transactions/"+url.PathEscape(body["externalTransactionId"].(string)), otherProviderToken, http.StatusForbidden)
	getJSON[map[string]any](t, env.api+"/wagering/transactions/"+first.TransactionID, internalToken, http.StatusForbidden)
	getJSON[map[string]any](t, env.api+"/wallets/"+wallet.ID, internalToken, http.StatusOK)
	getJSON[map[string]any](t, env.api+"/wallets/"+wallet.ID, providerToken, http.StatusForbidden)

	getJSON[map[string]any](t, env.api+"/health/live", "", http.StatusOK)
	getJSON[map[string]any](t, env.api+"/health/ready", "", http.StatusOK)

	if providerID == "" {
		t.Fatal("provider token subject is empty")
	}
}

func TestE2ESQSHTTPReplayAndOutbox(t *testing.T) {
	env := loadEnv(t)
	internalToken := token(t, env, "wallet-internal", "wallet-internal-secret")
	providerToken := token(t, env, "wallet-api", "wallet-api-secret")
	providerID := subject(t, providerToken)

	playerID := newID(t)
	wallet := postJSON[walletResponse](t, env.api+"/wallets", internalToken, map[string]any{
		"playerId":       playerID,
		"initialBalance": map[string]string{"amount": "100.00", "currency": "BRL"},
	}, http.StatusCreated, "")

	messageID := "e2e-sqs-message-" + newID(t)
	externalID := "e2e-sqs-" + newID(t)
	idempotencyKey := "e2e-sqs-key-" + newID(t)
	message := map[string]any{
		"messageId":  messageID,
		"type":       "WagerTransactionRequested",
		"occurredAt": time.Now().UTC().Format(time.RFC3339Nano),
		"data": map[string]any{
			"providerId": providerID, "externalTransactionId": externalID, "idempotencyKey": idempotencyKey,
			"playerId": playerID, "walletId": wallet.ID, "roundId": "round-sqs", "gameId": "game-sqs", "kind": "BET",
			"money": map[string]string{"amount": "10.00", "currency": "BRL"},
		},
	}
	payload, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	client := sqs.NewFromConfig(cfg, func(options *sqs.Options) { options.BaseEndpoint = &env.sqsEndpoint })
	_, err = client.SendMessage(context.Background(), &sqs.SendMessageInput{
		QueueUrl:               &env.sqsQueue,
		MessageBody:            stringPtr(string(payload)),
		MessageGroupId:         stringPtr("wallet:" + wallet.ID),
		MessageDeduplicationId: stringPtr(messageID),
	})
	if err != nil {
		t.Fatal(err)
	}

	var processed transactionResponse
	waitFor(t, 20*time.Second, func() bool {
		response, status := getRaw(t, env.api+"/providers/"+url.PathEscape(providerID)+"/wagering/transactions/"+url.PathEscape(externalID), providerToken)
		if status != http.StatusOK {
			return false
		}
		if err := json.Unmarshal(response, &processed); err != nil {
			t.Fatal(err)
		}
		return processed.Status == "PROCESSED"
	})

	replayBody := map[string]any{
		"providerId": providerID, "externalTransactionId": externalID, "playerId": playerID, "walletId": wallet.ID,
		"roundId": "round-sqs", "gameId": "game-sqs", "kind": "BET", "money": map[string]string{"amount": "10.00", "currency": "BRL"},
	}
	replay := postJSON[transactionResponse](t, env.api+"/wagering/transactions", providerToken, replayBody, http.StatusOK, idempotencyKey)
	if !replay.IdempotentReplay || replay.TransactionID != processed.TransactionID {
		t.Fatalf("cross-transport replay failed: %+v processed=%+v", replay, processed)
	}

	waitForOutboxEvent(t, client, env.eventQueue, processed.TransactionID)
}

func waitForOutboxEvent(t *testing.T, client *sqs.Client, queueURL, aggregateID string) {
	t.Helper()
	waitFor(t, 20*time.Second, func() bool {
		result, err := client.ReceiveMessage(context.Background(), &sqs.ReceiveMessageInput{QueueUrl: &queueURL, MaxNumberOfMessages: 10, WaitTimeSeconds: 1})
		if err != nil {
			t.Fatal(err)
		}
		for _, message := range result.Messages {
			if message.Body == nil {
				continue
			}
			var envelope struct {
				AggregateID string `json:"aggregateId"`
				EventID     string `json:"eventId"`
			}
			if json.Unmarshal([]byte(*message.Body), &envelope) == nil && envelope.AggregateID == aggregateID && envelope.EventID != "" {
				if message.ReceiptHandle != nil {
					_, _ = client.DeleteMessage(context.Background(), &sqs.DeleteMessageInput{QueueUrl: &queueURL, ReceiptHandle: message.ReceiptHandle})
				}
				return true
			}
		}
		return false
	})
}

type environment struct {
	api         string
	sqsEndpoint string
	sqsQueue    string
	eventQueue  string
}

func loadEnv(t *testing.T) environment {
	t.Helper()
	api := envOr("E2E_API_URL", "http://localhost:8080")
	sqsEndpoint := envOr("E2E_SQS_ENDPOINT", "http://localhost:4566")
	sqsQueue := envOr("E2E_SQS_QUEUE_URL", "http://localhost:4566/000000000000/wager-transactions.fifo")
	eventQueue := envOr("E2E_SQS_EVENT_QUEUE_URL", "http://localhost:4566/000000000000/wager-events.fifo")
	if os.Getenv("E2E_RUN") != "1" {
		t.Skip("set E2E_RUN=1 after docker compose up --build to run real Keycloak/PostgreSQL/LocalStack E2E tests")
	}
	return environment{api: api, sqsEndpoint: sqsEndpoint, sqsQueue: sqsQueue, eventQueue: eventQueue}
}

func token(t *testing.T, env environment, clientID, secret string) string {
	t.Helper()
	endpoint := envOr("E2E_OIDC_TOKEN_URL", "http://localhost:8081/realms/wallet/protocol/openid-connect/token")
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {clientID}, "client_secret": {secret}}
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("token request status=%d body=%s", response.StatusCode, data)
	}
	var value tokenResponse
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	if value.AccessToken == "" {
		t.Fatal("empty access token")
	}
	return value.AccessToken
}

func subject(t *testing.T, token string) string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatal("invalid JWT")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(data, &claims); err != nil {
		t.Fatal(err)
	}
	return claims.Sub
}

func postJSON[T any](t *testing.T, endpoint, bearer string, body any, wantStatus int, idempotencyKey string) T {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	request.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status=%d want=%d body=%s", endpoint, response.StatusCode, wantStatus, data)
	}
	var result T
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func getJSON[T any](t *testing.T, endpoint, bearer string, wantStatus int) T {
	t.Helper()
	data, status := getRaw(t, endpoint, bearer)
	if status != wantStatus {
		t.Fatalf("GET %s status=%d want=%d body=%s", endpoint, status, wantStatus, data)
	}
	var result T
	if status != http.StatusNoContent && len(data) > 0 {
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func getRaw(t *testing.T, endpoint, bearer string) ([]byte, int) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return data, response.StatusCode
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("condition was not satisfied before timeout")
}

func newID(t *testing.T) string {
	t.Helper()
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func stringPtr(value string) *string { return &value }
