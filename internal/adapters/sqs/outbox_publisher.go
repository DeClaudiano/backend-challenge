package sqs

import (
	"context"
	"fmt"
	"strings"

	"backend-challenge/internal/application/ports"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
)

type OutboxPublisher struct {
	client   *Client
	eventURL string
}

func NewOutboxPublisher(client *Client) *OutboxPublisher {
	return &OutboxPublisher{client: client, eventURL: client.EventQueueURL}
}

func (p *OutboxPublisher) Publish(ctx context.Context, record ports.OutboxRecord) error {
	if record.EventID == "" || record.AggregateID == "" || len(record.Payload) == 0 {
		return fmt.Errorf("outbox record has incomplete publication data")
	}
	if p.eventURL == "" {
		return fmt.Errorf("outbox event queue URL is empty")
	}
	groupID := record.AggregateID
	deduplicationID := record.EventID
	_, err := p.client.API.SendMessage(ctx, &awssqs.SendMessageInput{
		QueueUrl:               &p.eventURL,
		MessageBody:            stringPointer(string(record.Payload)),
		MessageGroupId:         &groupID,
		MessageDeduplicationId: &deduplicationID,
	})
	if err != nil {
		return fmt.Errorf("publish outbox event %s: %w", record.EventID, err)
	}
	return nil
}

func stringPointer(value string) *string { return &value }

func eventQueueURL(queueURL string) string {
	return strings.TrimSuffix(queueURL, "wager-transactions.fifo") + "wager-events.fifo"
}

var _ ports.OutboxPublisher = (*OutboxPublisher)(nil)
