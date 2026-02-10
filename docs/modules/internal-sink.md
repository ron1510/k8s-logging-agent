# Sinks

This package defines output sinks for log entries. A sink is a small component that receives `LogEntry` objects and forwards them to a destination.

## Stdout Sink

`stdout.go` prints `AGENT_FORWARD` lines with namespace, pod, container, and the message body. It uses an internal queue to avoid blocking the agent if stdout is slow.

## Scaling Notes

1. Sinks should be fast because they run in the queue worker goroutine.
2. `STDOUT_QUEUE_SIZE` and `STDOUT_FLUSH_INTERVAL` control buffering and flush cadence.
3. Tune `BATCH_SIZE` and `BATCH_TIMEOUT` for throughput and latency.
