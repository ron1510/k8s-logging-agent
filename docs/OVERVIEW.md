# Kubernetes Log Agent (Go) - Detailed Code Walkthrough

This document explains the current codebase in detail for someone new to Go, but familiar with DevOps and Python. It covers project structure, data flow, concurrency, key structs, and how the mock runner works.

## Project Layout

1. `cmd/agent/main.go`
   Real entrypoint for running inside a Kubernetes cluster. Loads configuration, creates a K8s client, and starts the pipeline.
2. `cmd/mock-runner/main.go`
   Local entrypoint for running without a cluster. Uses the mock K8s client, emits fake pod logs, and prints both source logs and agent-forwarded logs.
3. `internal/config/config.go`
   Configuration loader (YAML + env vars + defaults via cleanenv) and namespace discovery.
4. `internal/k8s/client.go`
   Small interface (`k8s.Client`) that abstracts the subset of Kubernetes API calls used by the agent.
5. `internal/k8s/watcher.go`
   Real Kubernetes client (in-cluster) and pod watch loop.
6. `testutil/k8smock/mock.go`
   In-memory mock implementation of `k8s.Client` for local testing.
7. `internal/streamer/streamer.go`
   Core log streaming logic with reconnects, enrichment, and bounded buffering.
8. `internal/sink/stdout.go`
   Default sink that prints `AGENT_FORWARD` lines.
9. `deploy/namespace-agent.yaml`
   Example ServiceAccount, Role, RoleBinding, and Deployment.
12. `deploy/namespace-agent-sidecar.yaml`
   Example sidecar deployment that tails the agent's tee output with rotation-friendly settings.
13. `Dockerfile`
   Runs the agent and uses `tee` to duplicate stdout into a file for sidecar collection.
14. `internal/metrics/metrics.go`
   Lightweight counters and periodic metric emission.

## High-Level Data Flow

1. Pod watcher lists and watches pods in a namespace.
2. Stream manager starts a goroutine for each running container in matching pods.
3. Each stream goroutine reads log lines via the Kubernetes API and pushes them into a bounded queue.
4. A queue worker emits log entries to the configured sink (stdout by default).

## Core Concepts (Go Basics)

1. Goroutine: lightweight thread-like function started with `go f(...)`.
2. Channel: typed FIFO queue used for concurrency (`chan T`).
3. Context: cancellation and deadline propagation across goroutines.
4. Interface: set of method signatures that allows swapping implementations (real vs mock).

## Configuration (`internal/config/config.go`)

The `Config` struct holds all runtime settings:

```go
type Config struct {
    Namespace            string
    LabelSelector        string
    AllowLabels          map[string]string
    DenyLabels           map[string]string
    MaxConcurrentStreams int
    QueueSize            int
    LogLevel             string
    ServiceName          string
}
```

Load order:

1. Defaults (hard-coded)
2. Optional YAML file (path from `CONFIG_PATH`)
3. Environment variables override YAML
4. Namespace auto-detection (from `POD_NAMESPACE` or serviceaccount file)

Environment variables:

1. `NAMESPACE`, `LABEL_SELECTOR`, `ALLOW_LABELS`, `DENY_LABELS`
2. `KUBECONFIG`, `KUBECONFIG_CONTEXT`
3. `MAX_CONCURRENT_STREAMS`, `QUEUE_SIZE`, `BATCH_SIZE`, `BATCH_TIMEOUT`, `MAX_LINE_BYTES`
4. `STREAM_IDLE_TIMEOUT`, `STDOUT_QUEUE_SIZE`, `STDOUT_FLUSH_INTERVAL`, `METRICS_INTERVAL`
5. `LOG_LEVEL`, `SERVICE_NAME`

`ALLOW_LABELS` and `DENY_LABELS` are CSV `key=value` pairs, for example `system=payments,team=core`.

Out-of-cluster usage:

1. Set `KUBECONFIG` to your kubeconfig path.
2. Optionally set `KUBECONFIG_CONTEXT` to select a context.
3. `NAMESPACE` controls which namespace is watched.

## Kubernetes Interface (`internal/k8s/client.go`)

We only use three API operations, so we define a small interface:

```go
type Client interface {
    ListPods(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error)
    WatchPods(ctx context.Context, namespace, labelSelector, resourceVersion string) (watch.Interface, error)
    StreamLogs(ctx context.Context, namespace, podName string, opts *corev1.PodLogOptions) (io.ReadCloser, error)
}
```

Why this matters:

1. Real K8s usage with `client-go` implements the interface.
2. Mocked usage for local testing also implements the interface.

## Real Client and Watcher (`internal/k8s/watcher.go`)

`NewClient` creates a client using `KUBECONFIG` if provided, otherwise it tries in-cluster config and then falls back to default kubeconfig loading rules.

`WatchPods` does:

1. Prefer a shared informer for pod events when a real clientset is available.
2. Fall back to list/watch when informers are not available or fail.
3. On error, backoff and retry.

Backoff is exponential with jitter via `github.com/cenkalti/backoff/v5`.

## Stream Manager (`internal/streamer/streamer.go`)

The stream manager owns:

1. `active` map: tracks which (pod UID, container) streams are running.
2. `sem` channel: enforces max concurrent streams.
3. `queue` channel: bounded buffer of log entries.

`Manager.HandlePodEvent` behavior:

1. If the pod does not match allow/deny labels, all its streams are stopped.
2. On `Deleted`, stop all its streams.
3. On `Added` or `Modified`, ensure streams exist for all running containers.

`ensurePodStreams` behavior:

1. Build a `streamState` with metadata (namespace, pod name, UID, container, labels).
2. Add it to the `active` map.
3. Start `go m.streamContainer(...)`.

`streamContainer` behavior:

1. Acquire a slot in the semaphore (`MaxConcurrentStreams`).
2. Start log stream using `Follow: true`, `Timestamps: true`, and `SinceTime` if reconnecting.
3. Scan line-by-line with a `bufio.Scanner`.
4. Parse timestamp from each line and update `lastTS` for reconnect.
5. Enrich with Kubernetes metadata and enqueue.
6. If the stream ends or errors, reconnect with backoff.

Duplicate reduction uses `SinceTime` from the last timestamp plus 1 nanosecond.

### LogEntry and Sink

To keep the stream logic agnostic to output destination:

```go
type LogEntry struct {
    Body       string
    Timestamp  time.Time
    Attributes []attribute.KeyValue
}

type Sink interface {
    Emit(ctx context.Context, entry LogEntry)
}
```

This allows swapping outputs. The default is a stdout sink. If the sink supports batching, the queue worker will batch entries.

### Enrichment

Each log line gets:

1. `k8s.namespace.name`
2. `k8s.pod.name`
3. `k8s.pod.uid`
4. `k8s.container.name`
5. `k8s.container.restart_count`
6. `k8s.node.name` (if available)
7. All pod labels prefixed with `k8s.pod.label.`

## Default Sink (`internal/sink/stdout.go`)

This sink prints `AGENT_FORWARD` lines with Kubernetes identifiers extracted from attributes.


## Mock K8s Client (`testutil/k8smock`)

`MockClient` stores:

1. `pods`: map of pod name to pod
2. `watcher`: a `watch.FakeWatcher`
3. `streams`: map of `(namespace,pod,container)` to log stream factory

Useful methods:

1. `SetPods` initializes in-memory pod list.
2. `AddPod`, `UpdatePod`, `DeletePod` trigger watch events.
3. `SetLogStreamFactory` sets a stream provider for a pod/container.
4. `LineStream` returns an `io.ReadCloser` streaming a list of lines.
5. `LineStreamWithTap` does the same and calls a callback for each line (used to print `PODSRC`).

## Mock Runner (`cmd/mock-runner/main.go`)

This entrypoint is for local testing. It:

1. Creates a mock client.
2. Defines two pods (`podA` and `podB`).
3. Registers log stream factories for each container.
4. Starts the watcher and stream manager.
5. Prints both:
   `PODSRC` (raw mock lines)
   `AGENT_FORWARD` (what the agent emits)

Why you see two lines per log:

Because we tap the mock stream (source logs) and also print the forwarded logs.

## Concurrency Model

1. One goroutine for pod watch.
2. One goroutine per active container stream.
3. One goroutine for queue emission.

## Suggested Next Steps

1. Add a YAML fixture loader for mock pods and log lines.
2. Add unit tests for `matchLabels` and `parseLogLine`.
