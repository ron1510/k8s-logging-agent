# Metrics

This package provides lightweight counters for operational visibility. Counters are emitted as log lines at a fixed interval so the sidecar collector can ingest them.

## Counters

1. `queue_drops`: Number of log entries dropped because the queue was full.
2. `stdout_drops`: Number of lines dropped because stdout was blocked.
3. `oversize_lines`: Number of lines dropped due to `MAX_LINE_BYTES`.

## Configuration

1. `METRICS_INTERVAL` controls how often counters are logged.
