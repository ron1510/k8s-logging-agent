# Configuration

This package loads configuration from an optional YAML file and environment variables using `cleanenv`.

## Load Order

1. YAML is loaded when `CONFIG_PATH` is set.
2. Environment variables override YAML.
3. Defaults are applied via struct tags (`env-default`).
4. Namespace is auto-detected from `POD_NAMESPACE` or the serviceaccount namespace file.

## Environment Variables

1. `NAMESPACE`, `LABEL_SELECTOR`, `ALLOW_LABELS`, `DENY_LABELS`
2. `KUBECONFIG`, `KUBECONFIG_CONTEXT`
3. `MAX_CONCURRENT_STREAMS`, `QUEUE_SIZE`, `BATCH_SIZE`, `BATCH_TIMEOUT`, `MAX_LINE_BYTES`
4. `STREAM_IDLE_TIMEOUT`, `STDOUT_QUEUE_SIZE`, `STDOUT_FLUSH_INTERVAL`, `METRICS_INTERVAL`
5. `LOG_LEVEL`, `SERVICE_NAME`

`ALLOW_LABELS` and `DENY_LABELS` are CSV `key=value` pairs. YAML can provide a map.

Out-of-cluster usage:

1. Set `KUBECONFIG` to your kubeconfig path.
2. Optionally set `KUBECONFIG_CONTEXT` to select a context.
3. `NAMESPACE` controls which namespace is watched.

File output:

The Dockerfile uses `tee` to duplicate stdout to a file for sidecar collection.

## Scaling Notes

1. `MAX_CONCURRENT_STREAMS` limits the number of active log streams.
2. `QUEUE_SIZE` controls buffering and backpressure.
3. `BATCH_SIZE` and `BATCH_TIMEOUT` control sink batching.
