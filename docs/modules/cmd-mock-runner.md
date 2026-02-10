# Mock Runner

`cmd/mock-runner` runs the agent locally without a Kubernetes cluster. It uses the in-memory mock client from `testutil/k8smock`, emits fake pod logs, and prints both source logs and agent-forwarded logs.

## What You See

1. `PODSRC` lines are the raw mock pod logs as they are generated.
2. `AGENT_FORWARD` lines are what the agent emits after parsing and enrichment.

## How It Works

1. Create a `mock.MockClient`.
2. Define pods and container statuses.
3. Register log stream factories per pod and container.
4. Start the watcher and stream manager just like in real mode.

## Scaling Notes

1. The mock runner is deterministic and single-process.
2. It is useful for functional testing, not for performance benchmarking.
