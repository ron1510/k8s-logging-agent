# k8s-logging-agent

A Kubernetes log agent written in Go, designed to discover pods by label and forward container logs with Kubernetes metadata.

The repository includes:
- The Go agent (`cmd/agent`)
- A sidecar OpenTelemetry Collector pattern using shared volume + `tee`
- Single-release horizontal sharding with StatefulSet ordinals
- A production-ready Helm chart
- Local end-to-end scripts for fast testing on `kind`

## Why This Project

This agent is useful when you need:
- Label-based opt-in log collection (`monitor-logs=true`)
- Backpressure-aware streaming from the Kubernetes API
- Enriched log lines with pod/container/namespace context
- A simple sidecar collector path for local and cluster testing
- Optional JSON log parsing in collector (`collector.parseJsonLogs`)

## Architecture

1. Agent streams logs from matching pods via Kubernetes API.
2. Each agent pod owns a partition of pods by `hash(podUID) % SHARD_TOTAL`.
3. Agent writes `AGENT_FORWARD ... workload=<index> ...` lines to stdout.
4. Docker entrypoint uses `tee` to also write stdout to `/var/log/agent/agent.log`.
5. Collector sidecar tails that file from a shared `emptyDir`.
6. Collector exports logs to the configured backend (debug exporter by default in local mode).

## Repository Layout

- `cmd/agent` runtime entrypoint
- `internal/*` core packages (config, k8s, streamer, sink, metrics)
- `deploy/helm/k8s-logging-agent` Helm chart
- `scripts/local-e2e.ps1` local cluster setup + deploy + sample workloads
- `scripts/verify-partitioner.ps1` shard ownership verification helper
- `scripts/tail-logs.ps1` real-time logs helper
- `deploy/sample-workloads.yaml` test deployments
- `docs/` module-level docs

## Quickstart (Local E2E)

Prerequisites:
- Docker Desktop (running)
- `kind`
- `kubectl`
- `helm`
- Go 1.25+

Run:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\local-e2e.ps1
```

This will:
- create/use `kind-k8s-logging`
- build and load `k8s-logging-agent:dev`
- deploy Helm release `k8s-logging-agent` in namespace `observability`
- deploy sample workload pods for log generation

## Real-Time Logs

Collector output (best signal for forwarding):

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\tail-logs.ps1 -Mode collector
```

Agent process logs:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\tail-logs.ps1 -Mode agent
```

Workload logs (`monitor-logs=true`):

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\tail-logs.ps1 -Mode workloads
```

## Helm Deployment

Chart path:

```text
deploy/helm/k8s-logging-agent
```

Install:

```powershell
helm upgrade --install k8s-logging-agent deploy/helm/k8s-logging-agent `
  --namespace observability `
  --create-namespace
```

Production helper script:

```powershell
powershell -ExecutionPolicy Bypass -File .\deploy\helm\install-production.ps1 `
  -Namespace observability `
  -AgentImageRepository ghcr.io/<org>/k8s-logging-agent `
  -AgentImageTag v0.1.0 `
  -OtlpEndpoint otel-gateway.observability.svc.cluster.local:4317
```

## Configuration

Primary env vars:
- `NAMESPACE`
- `LABEL_SELECTOR`
- `ALLOW_LABELS`
- `DENY_LABELS`
- `MAX_CONCURRENT_STREAMS`
- `QUEUE_SIZE`
- `QUEUE_HIGH_WATERMARK`
- `QUEUE_THROTTLE`
- `BATCH_SIZE`
- `BATCH_TIMEOUT`
- `SHARD_TOTAL`
- `SHARD_ORDINAL`
- `LOG_LEVEL`
- `SERVICE_NAME`

See `docs/REFERENCE.md` and `internal/config/config.go` for full options.

## Security Defaults

Helm defaults include:
- non-root containers
- dropped Linux capabilities
- `allowPrivilegeEscalation: false`
- `seccompProfile: RuntimeDefault`
- rolling update strategy and config checksum rollout

## Production Notes

- Pin both agent and collector image tags.
- For Docker Hub collector image, use `0.145.0` (no `v` prefix).
- Configure `collector.otlpEndpoint` and TLS correctly for your backend.
- Tune `resources` / `collectorResources` based on log volume.

## Development

Run app module tests:

```powershell
go test ./...
```

Run dedicated tests module:

```powershell
cd tests
go test ./...
```

## Partitioner Focus

Bring up local sharded environment:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\local-e2e.ps1 -ReplicaCount 2
```

Verify shard ownership:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\verify-partitioner.ps1
```

Build local image:

```powershell
docker build -t k8s-logging-agent:dev .
```

## Documentation

- Usage guide: `docs/USAGE.md`
- Architecture/details: `docs/OVERVIEW.md`
- API/module reference: `docs/REFERENCE.md`
- Performance benchmarking: `docs/PERF.md`

## Contributing and Security

- Contribution process: `CONTRIBUTING.md`
- Security reporting: `SECURITY.md`
- Code of conduct: `CODE_OF_CONDUCT.md`

## License

MIT (`LICENSE`)
