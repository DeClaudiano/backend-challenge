#!/bin/sh
set -eu

create_fifo_queue() {
  queue_name=$1
  dlq_arn=${2:-}
  attributes_file=$(mktemp)
  trap 'rm -f "$attributes_file"' EXIT
  python3 - "$attributes_file" "$dlq_arn" <<'PY'
import json
import os
import sys

path, dlq_arn = sys.argv[1:]
attributes = {
    "FifoQueue": "true",
    "ContentBasedDeduplication": "false",
}
if dlq_arn:
    attributes["RedrivePolicy"] = json.dumps({
        "deadLetterTargetArn": dlq_arn,
        "maxReceiveCount": os.environ.get("SQS_MAX_RECEIVE_COUNT", "5"),
    }, separators=(",", ":"))
with open(path, "w", encoding="utf-8") as output:
    json.dump(attributes, output)
PY
  awslocal sqs create-queue --queue-name "$queue_name" --attributes "file://$attributes_file" >/dev/null
  trap - EXIT
  rm -f "$attributes_file"
}

create_fifo_queue wager-transactions-dlq.fifo
DLQ_URL=$(awslocal sqs get-queue-url --queue-name wager-transactions-dlq.fifo --query QueueUrl --output text)
DLQ_ARN=$(awslocal sqs get-queue-attributes --queue-url "$DLQ_URL" --attribute-names QueueArn --query Attributes.QueueArn --output text)
create_fifo_queue wager-transactions.fifo "$DLQ_ARN"

create_fifo_queue wager-events-dlq.fifo
EVENT_DLQ_URL=$(awslocal sqs get-queue-url --queue-name wager-events-dlq.fifo --query QueueUrl --output text)
EVENT_DLQ_ARN=$(awslocal sqs get-queue-attributes --queue-url "$EVENT_DLQ_URL" --attribute-names QueueArn --query Attributes.QueueArn --output text)
create_fifo_queue wager-events.fifo "$EVENT_DLQ_ARN"
