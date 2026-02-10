# Reference Documentation

This document lists each module, struct, interface, and function with a short purpose statement. It is intended to give you full ownership of the code.

## cmd/agent

1. `main()`: Program entrypoint for running the agent against a real Kubernetes API.

## cmd/mock-runner

1. `main()`: Program entrypoint for running the agent locally with the mock client.
2. `mockLines(prefix string) []string`: Generates a small set of timestamped mock log lines.

## internal/config

1. `Config`: Runtime configuration for the agent.
2. `Load() (Config, error)`: Loads defaults, YAML, and env vars, then validates.
3. `LabelMap`: Custom type that parses `key=value` lists for labels.
4. `LabelMap.SetValue(s string)`: Populates the map from env strings.
5. `parseKVList(raw string) map[string]string`: Parses comma-separated `key=value` pairs.
6. `validate(cfg *Config) error`: Validates config invariants and returns errors.

## internal/metrics

1. `Counters`: Internal counter set.
2. `IncQueueDrop()`: Increments queue drop counter.
3. `IncStdoutDrop()`: Increments stdout drop counter.
4. `IncOversizeLine()`: Increments oversize line counter.
5. `Snapshot()`: Returns current counter values.
6. `Reset()`: Clears counters (used in tests).
7. `StartReporter(logger, interval, stop)`: Logs counters periodically.

## internal/k8s

1. `Client`: Interface for listing pods, watching pods, and streaming logs.
2. `PodEvent`: Wrapper for typed pod watch events.
3. `NewClient(cfg Config) (Client, error)`: Creates a Kubernetes client from kubeconfig or in-cluster config.
4. `WatchPods(ctx, client, cfg, logger, out)`: Starts pod event streaming, preferring informers.
5. `PreflightRBAC(ctx, client, namespace)`: Validates RBAC permissions for pod and log access.
6. `buildConfig(cfg Config) (*rest.Config, error)`: Selects kubeconfig, in-cluster, or default config.
7. `buildOutOfClusterConfig(path, contextName string) (*rest.Config, error)`: Loads kubeconfig and validates the context.
8. `watchWithInformer(ctx, clientset, cfg, logger, out)`: Runs a shared informer and emits events.
9. `kubeClient`: Real client-go based implementation of `Client`.
10. `kubeClient.ListPods(...)`: Lists pods with a label selector.
11. `kubeClient.WatchPods(...)`: Watches pods from a resource version.
12. `kubeClient.StreamLogs(...)`: Streams pod container logs.

## internal/streamer

1. `LogEntry`: Normalized log record with timestamp and attributes.
2. `Sink`: Interface for consuming single log entries.
3. `BatchSink`: Interface for consuming batches of log entries.
4. `Manager`: Orchestrates streams, queueing, and emission to the sink.
5. `NewManager(...) *Manager`: Constructs a stream manager.
6. `StartQueue(ctx)`: Starts the queue worker goroutine.
7. `Shutdown()`: Cancels streams and stops queue processing.
8. `HandlePodEvent(ctx, ev)`: Reacts to pod add/update/delete events.
9. `streamContainer(ctx, st)`: Streams one container with reconnect logic.
10. `ensurePodStreams(ctx, pod)`: Starts streams for running containers.
11. `stopPodStreams(pod)`: Cancels all streams for a pod.
12. `buildAttributes(st)`: Produces K8s attributes for a log record.
13. `parseLogLine(line)`: Parses RFC3339Nano timestamped log lines.
14. `ErrLineTooLong`: Sentinel error for oversized log lines.
15. `lineResult`: Internal struct for read loop results.
16. `readLoop(r, maxBytes, out, done)`: Reads lines in a goroutine and sends results.
17. `readLine(r, maxBytes)`: Reads a single line with a max size limit.
17. `matchLabels(podLabels, allow, deny)`: Applies allow/deny label filters.
18. `sanitizeLabelKey(key)`: Makes label keys safe for attribute names.
19. `copyMap(src)`: Clones a label map.
20. `wait(ctx, d)`: Sleeps with context cancellation support.
22. `resetTimer(t, d)`: Safely resets a timer used for batching.
23. `streamKey`: Internal key for active streams.
24. `streamState`: Internal stream metadata and cancel handle.

## internal/sink

1. `Stdout`: Sink that prints `AGENT_FORWARD` lines.
2. `NewStdout(cfg, logger)`: Constructs a buffered stdout sink.
3. `Stdout.Emit(ctx, entry)`: Prints one log entry to stdout.

## Retry/Backoff

The project uses `github.com/cenkalti/backoff/v5` for exponential backoff with jitter.

## testutil/k8smock

1. `Client`: In-memory implementation of `k8s.Client`.
2. `New() *Client`: Constructs a mock client.
3. `ListPods(...)`: Returns the current mock pod list.
4. `WatchPods(...)`: Returns a fake watcher emitting mock events.
5. `StreamLogs(...)`: Returns a mock log stream for a container.
6. `SetPods(...)`: Replaces the entire in-memory pod list.
7. `AddPod(pod)`: Adds a pod and emits an add event.
8. `UpdatePod(pod)`: Updates a pod and emits a modify event.
9. `DeletePod(ns, name)`: Deletes a pod and emits a delete event.
10. `SetLogStreamFactory(ns, pod, container, factory)`: Registers a log stream factory.
11. `LineStream(lines, delay)`: Streams lines with an optional delay.
12. `LineStreamWithTap(lines, delay, tap)`: Streams lines and calls tap for each line.
13. `resourceVersion()`: Returns a monotonically increasing resource version.
14. `podKey(ns, name)`: Creates a stable pod map key.
