package sqs

import (
	"context"
	"fmt"

	"backend-challenge/internal/config"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type Client struct {
	API               *sqs.Client
	QueueURL          string
	EventQueueURL     string
	VisibilityTimeout int32
	WaitTimeSeconds   int32
}

func Open(ctx context.Context, cfg config.Config) (*Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(cfg.SQSRegion),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)

	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}

	client := sqs.NewFromConfig(awsCfg, func(options *sqs.Options) {
		options.BaseEndpoint = &cfg.SQSEndpoint
	})

	return &Client{
		API:               client,
		QueueURL:          cfg.SQSQueueURL,
		EventQueueURL:     eventQueueURL(cfg.SQSQueueURL),
		VisibilityTimeout: cfg.SQSVisibilityTimeout,
		WaitTimeSeconds:   cfg.SQSWaitTimeSeconds,
	}, nil
}

func (c *Client) DLQDepth(ctx context.Context) (uint64, error) {
	queueURL := c.QueueURL
	if len(queueURL) > 0 {
		if idx := lastSlash(queueURL); idx >= 0 {
			queueURL = queueURL[:idx+1] + "wager-transactions-dlq.fifo"
		}
	}
	result, err := c.API.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{QueueUrl: &queueURL, AttributeNames: []types.QueueAttributeName{"ApproximateNumberOfMessages"}})
	if err != nil {
		return 0, err
	}
	value := result.Attributes["ApproximateNumberOfMessages"]
	var depth uint64
	_, _ = fmt.Sscanf(value, "%d", &depth)
	return depth, nil
}

func lastSlash(value string) int {
	for i := len(value) - 1; i >= 0; i-- {
		if value[i] == '/' {
			return i
		}
	}
	return -1
}

func (c *Client) Ready(ctx context.Context) error {
	_, err := c.API.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{QueueUrl: &c.QueueURL})
	return err
}

func (c *Client) Receive(ctx context.Context) ([]types.Message, error) {
	result, err := c.API.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            &c.QueueURL,
		MaxNumberOfMessages: 1,
		VisibilityTimeout:   c.VisibilityTimeout,
		WaitTimeSeconds:     c.WaitTimeSeconds,
	})
	if err != nil {
		return nil, err
	}
	return result.Messages, nil
}

func (c *Client) Delete(ctx context.Context, receiptHandle string) error {
	_, err := c.API.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: &c.QueueURL, ReceiptHandle: &receiptHandle})
	return err
}
