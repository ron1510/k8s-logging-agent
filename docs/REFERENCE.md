# Reference

This document lists key runtime entrypoints, packages, and important interfaces/functions.

## Entrypoints

1. `cmd/agent/main.go`
   - `main()`: starts the real in-cluster/out-of-cluster agent process.
2. `cmd/mock-runner/main.go`
   - `main()`: runs the agent with mock Kubernetes data.

## Configuration (`internal/config`)

1. `Config`
   - Runtime settings loaded from env and optional YAML.
2. `Load() (Config, error)`
   - Loads config and validates values.
3. `LabelMap`
   - Parses `key=value,key2=value2` label maps from env.

Key environment variables:

1. `NAMESPACE`, `POD_NAMESPACE`
2. `LABEL_SELECTOR`, `ALLOW_LABELS`, `DENY_LABELS`
3. `MAX_CONCURRENT_STREAMS`, `QUEUE_SIZE`
4. `BATCH_SIZE`, `BATCH_TIMEOUT`
5. `MAX_LINE_BYTES`, `STREAM_IDLE_TIMEOUT`
6. `STDOUT_QUEUE_SIZE`, `STDOUT_FLUSH_INTERVAL`
7. `METRICS_INTERVAL`, `LOG_LEVEL`, `SERVICE_NAME`

## Kubernetes Layer (`internal/k8s`)

1. `Client` interface
   - `ListPods(...)`
   - `WatchPods(...)`
   - `StreamLogs(...)`
2. `NewClient(cfg Config) (Client, error)`
   - Creates real Kubernetes client from kubeconfig/in-cluster config.
3. `WatchPods(ctx, client, cfg, logger, out)`
   - Emits pod add/modify/delete events.
4. `PreflightRBAC(ctx, client, namespace)`
   - Checks pod list/watch and pod/log read permissions.

## Streaming Core (`internal/streamer`)

1. `LogEntry`
   - Normalized log envelope (`Body`, `Timestamp`, attributes).
2. `Sink` and `BatchSink`
   - Output contracts for single and batched emit.
3. `Manager`
   - Owns active stream state, queue, and fan-out.
4. `NewManager(...) *Manager`
5. `StartQueue(ctx)`
6. `HandlePodEvent(ctx, ev)`
7. `Shutdown()`

Behavior notes:

1. One stream per `(pod UID, container)`.
2. Stream reconnection uses exponential backoff.
3. Queue is bounded and drops under pressure (by design).
4. Labels are attached as `k8s.pod.label.*` attributes.

## Sinks (`internal/sink`)

1. `Stdout`
   - Emits `AGENT_FORWARD` lines.
2. `NewStdout(cfg, logger)`

## Metrics (`internal/metrics`)

1. Counter increments:
   - `IncQueueDrop()`
   - `IncStdoutDrop()`
   - `IncOversizeLine()`
2. Snapshot/reporting:
   - `Snapshot()`
   - `StartReporter(logger, interval, stop)`

## Mock Utilities (`testutil/k8smock`)

1. `New() *Client`
2. `SetPods(...)`, `AddPod(...)`, `UpdatePod(...)`, `DeletePod(...)`
3. `SetLogStreamFactory(...)`
4. `LineStream(...)`, `LineStreamWithTap(...)`

## Deployment Assets

1. Helm chart:
   - `deploy/helm/k8s-logging-agent`
2. Local bootstrap script:
   - `scripts/local-e2e.ps1`
3. Realtime log helper:
   - `scripts/tail-logs.ps1`
4. Sample workloads:
   - `deploy/sample-workloads.yaml`

For end-user commands and operational runbooks, see `docs/USAGE.md`.
