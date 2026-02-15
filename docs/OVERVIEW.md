# Architecture Overview

This document describes the current architecture and runtime behavior of `k8s-logging-agent`.

## High-Level Flow

1. The agent watches pods in a target namespace.
2. Matching pods are selected by label policy (`LABEL_SELECTOR`, allow/deny labels).
3. The agent streams container logs via Kubernetes API.
4. Multi-replica agents partition pods deterministically (`hash(podUID) % SHARD_TOTAL`).
5. The agent emits structured `AGENT_FORWARD` lines to stdout (`workload=<index>`).
5. In container runtime, stdout is duplicated to `/var/log/agent/agent.log` using `tee`.
6. A sidecar OpenTelemetry Collector tails that file from a shared volume and exports logs.

## Main Components

1. `cmd/agent/main.go`
   Wires config, Kubernetes client, metrics, sink, and stream manager.
2. `internal/config`
   Loads env/YAML config and validates runtime constraints.
3. `internal/k8s`
   Real client implementation, list/watch logic, and RBAC preflight.
4. `internal/streamer`
   Core orchestration for per-container streams, queueing, reconnect, and enrichment.
5. `internal/sink`
   Sink abstraction and stdout implementation.
6. `internal/metrics`
   Lightweight counters and periodic reporting.
7. `testutil/k8smock` and `cmd/mock-runner`
   Local simulation and development harness without a real cluster.

## Concurrency Model

1. One watcher goroutine receives pod events.
2. One goroutine per active container stream handles log tailing.
3. One queue worker emits entries to the sink.
4. A semaphore limits maximum concurrent streams.
5. StatefulSet ordinal selects shard ownership for each replica.

## Reliability Model

1. Stream reconnects use exponential backoff with jitter.
2. Queue is bounded to avoid unbounded memory growth.
3. Oversized lines are dropped defensively (`MAX_LINE_BYTES`).
4. Reconnect uses `SinceTime` to reduce duplicate forwarding.

## Deployment Model

Current deployment is Helm-first:

- Chart: `deploy/helm/k8s-logging-agent`
- Local bootstrap script: `scripts/local-e2e.ps1`
- Realtime logs helper: `scripts/tail-logs.ps1`
- Sample log-generating workloads: `deploy/sample-workloads.yaml`

The old standalone manifests under `deploy/namespace-agent*.yaml` are intentionally retired.

## Security and Operations Defaults

Helm defaults include:

1. Non-root container execution.
2. Dropped Linux capabilities.
3. `allowPrivilegeEscalation: false`.
4. `seccompProfile: RuntimeDefault`.
5. Collector health endpoint with liveness/readiness probes.
6. Checksum-based rollout on collector config changes.

## Local Validation Loop

1. Run `scripts/local-e2e.ps1`.
2. Tail collector logs with `scripts/tail-logs.ps1 -Mode collector`.
3. Confirm `AGENT_FORWARD` entries for labeled workloads in the target namespace.

For command-level instructions, see `docs/USAGE.md`.
