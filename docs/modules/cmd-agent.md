# Agent Entrypoint

`cmd/agent` is the real in-cluster entrypoint. It wires together config, the real Kubernetes client, the sink, and the stream manager.

## Startup Flow

1. Create a cancellable context that listens for `SIGTERM` and `SIGINT`.
2. Load config from defaults, YAML (optional), and env vars.
3. Build a real Kubernetes client using in-cluster credentials.
4. The agent writes to stdout. The Dockerfile uses `tee` to duplicate stdout to a file for sidecar collection.

In the Dockerfile, stdout is duplicated to a file using `tee` so a sidecar collector can tail it.

In the sidecar manifest, the collector's `filelog` receiver watches `/var/log/agent/*.log` and starts at end to avoid replay on restart.
5. Start the stream manager queue worker.
6. Start the pod watcher and forward events to the stream manager.

## Shutdown Behavior

1. Context cancellation stops the pod watcher and log streams.
2. The stream manager cancels all active streams.

## Scaling Notes

1. One goroutine per active container stream.
2. A bounded queue prevents unbounded memory usage.
