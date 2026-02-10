# Stream Manager

This package owns the log streaming lifecycle. It watches pod events, starts a stream per container, enriches log lines, and forwards them to a sink.

## Responsibilities

1. Track active streams using `(pod UID, container)` as the key.
2. Start or stop streams in response to pod watch events.
3. Read log lines and convert them into `LogEntry` values.
4. Apply backpressure with a bounded queue.

## Concurrency Model

1. One goroutine per active container stream.
2. One goroutine for the queue worker.
3. A semaphore channel limits concurrent streams.

## Logging

The manager uses structured logging (slog) for operational messages such as queue drops and stream errors.

## Stream Lifecycle

1. When a pod is running, the manager starts a stream for each running container.
2. The stream uses the Kubernetes log API with `Follow: true` and `Timestamps: true`.
3. Each log line is read with a buffered reader, parsed, enriched, and enqueued.
4. When the stream ends, it reconnects with exponential backoff.

## Duplicate Reduction

The stream stores the most recent timestamp and uses `SinceTime` on reconnect. A 1 ns offset is applied so the last line is not repeated.

## Backpressure and Safety

1. The queue has a fixed size. If full, lines are dropped to avoid unbounded memory growth.
2. If the sink supports batching, the queue worker groups entries before emit.

## Line Size Control

The maximum log line size is controlled by `MAX_LINE_BYTES`. Lines larger than this are rejected to avoid unbounded memory use.

## Stream Idle Timeout

`STREAM_IDLE_TIMEOUT` controls how long a stream can remain silent before it is reset. This protects against stuck connections.

## Scaling Notes

1. Hundreds of pods are handled via per-container goroutines, which is reasonable in Go.
2. Backoff and bounded buffering protect the agent when the collector is slow or down.
3. Tuning `MAX_CONCURRENT_STREAMS` and `QUEUE_SIZE` is the main lever for large namespaces.
