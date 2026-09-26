package consumers

import "testing"

func TestDecodeWagerTransactionMessage(t *testing.T) {
	message, err := DecodeWagerTransactionMessage([]byte(`{"messageId":"msg-123","type":"WagerTransactionRequested","occurredAt":"2026-09-08T12:00:00Z","data":{"providerId":"provider-a","externalTransactionId":"transaction-123","idempotencyKey":"provider-a:transaction-123","playerId":"player","walletId":"wallet","roundId":"round","gameId":"game","kind":"BET","money":{"amount":"25.00","currency":"BRL"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if message.MessageID != "msg-123" || message.Data.IdempotencyKey != "provider-a:transaction-123" {
		t.Fatalf("message=%+v", message)
	}
}

func TestDecodeWagerTransactionMessageRejectsWrongEnvelope(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"messageId":"msg","type":"Other","data":{"idempotencyKey":"key"}}`),
		[]byte(`{"messageId":"msg","type":"WagerTransactionRequested","data":{}}`),
		[]byte(`{"messageId":"msg","type":"WagerTransactionRequested","data":{"idempotencyKey":"key"},"unexpected":true}`),
	}
	for _, body := range cases {
		if _, err := DecodeWagerTransactionMessage(body); err == nil {
			t.Fatalf("expected invalid message for %s", body)
		}
	}
}
