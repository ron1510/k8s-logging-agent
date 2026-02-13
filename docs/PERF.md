# Performance Benchmark Matrix

This project includes a reproducible local performance runner:

- `cmd/perf-runner`

It uses the in-memory Kubernetes mock client and measures:

- expected vs received log events
- queue drops
- drop rate percent
- events per second
- p95/p99 ingestion lag (`now - parsed log timestamp`)

## Quick Run

```powershell
go run .\cmd\perf-runner --scenarios small,medium,burst --timeout 90s
```

Output is CSV:

```text
scenario,run,status,error,containers,lines_per_stream,expected,received,queue_drops,drop_rate_pct,elapsed_s,events_per_s,drops_per_s,queue_avg_depth,queue_max_depth,active_max,p95_lag_ms,p99_lag_ms
...
```

## Scenarios

1. `small`
- 20 containers
- 2,000 lines/container

2. `medium`
- 100 containers
- 2,000 lines/container

3. `burst`
- 250 containers
- 1,500 lines/container

## Tunable Flags

- `--scenarios`: comma-separated list
- `--line-bytes`: line body bytes (default `256`)
- `--timeout`: per-scenario timeout (default `90s`)
- `--repeat`: run count per scenario (default `1`)
- `--log-level`: `error|warn|info|debug` (default `error`)
- `--queue-size`: override queue size
- `--max-streams`: override max concurrent stream count
- `--batch-size`: override batch size
- `--batch-timeout`: override batch timeout
- `--queue-watermark`: override queue high watermark percent
- `--queue-throttle`: override queue throttle duration

## Production Presets

These are practical starting points; verify with your own workload.

1. Balanced:
- `QUEUE_SIZE=50000`
- `MAX_CONCURRENT_STREAMS=150`
- `QUEUE_HIGH_WATERMARK=90`
- `QUEUE_THROTTLE=1ms`
- `BATCH_SIZE=300`
- `BATCH_TIMEOUT=200ms`

2. Max Throughput:
- `QUEUE_SIZE=100000`
- `MAX_CONCURRENT_STREAMS=300`
- `QUEUE_HIGH_WATERMARK=95`
- `QUEUE_THROTTLE=500us`
- `BATCH_SIZE=500`
- `BATCH_TIMEOUT=100ms`

## Notes

- This is a controlled local benchmark; it is ideal for code-level regressions.
- For production readiness, combine this with cluster benchmarks (real API server, collector, and backend latency).
