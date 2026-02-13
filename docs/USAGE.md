# Usage Guide

This guide covers local testing, Helm deployment, real-time logs, and cleanup.

## 1. Prerequisites

- Windows PowerShell
- Docker Desktop running
- `kind`
- `kubectl`
- `helm`
- Go 1.25+

## 2. Local End-to-End Setup

From repo root:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\local-e2e.ps1
```

Optional parameters:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\local-e2e.ps1 `
  -ClusterName k8s-logging `
  -Namespace observability `
  -ReleaseName k8s-logging-agent `
  -ImageName k8s-logging-agent:dev `
  -RecreateCluster
```

What the script does:

1. Ensures required tools exist.
2. Creates or reuses a `kind` cluster.
3. Builds agent container image locally.
4. Loads image into `kind`.
5. Deploys Helm chart with local values.
6. Applies sample deployments that generate logs.

## 3. Verify Deployment

```powershell
kubectl -n observability get pods -o wide
kubectl -n observability rollout status deploy/k8s-logging-agent
kubectl -n observability get deploy,pods -l monitor-logs=true -o wide
```

Expected:
- `k8s-logging-agent` pod is `2/2 Running`
- `demo-echo-a` and `demo-echo-b` pods are running

## 4. Real-Time Logs

Run one of these in a terminal and keep it open:

Collector stream:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\tail-logs.ps1 -Mode collector
```

Agent stream:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\tail-logs.ps1 -Mode agent
```

Workload stream:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\tail-logs.ps1 -Mode workloads
```

Tip:
- `collector` mode is best for validating forwarded records.
- Stop any tail with `Ctrl+C`.

## 5. Standard Helm Usage

Install:

```powershell
helm upgrade --install k8s-logging-agent deploy/helm/k8s-logging-agent `
  --namespace observability `
  --create-namespace
```

Install with local overrides:

```powershell
helm upgrade --install k8s-logging-agent deploy/helm/k8s-logging-agent `
  --namespace observability `
  --create-namespace `
  -f deploy/helm/k8s-logging-agent/values-local.yaml `
  --set image.repository=k8s-logging-agent `
  --set image.tag=dev `
  --set collector.image.tag=0.145.0
```

## 6. Production Install

Use the helper:

```powershell
powershell -ExecutionPolicy Bypass -File .\deploy\helm\install-production.ps1 `
  -Namespace observability `
  -AgentImageRepository ghcr.io/<org>/k8s-logging-agent `
  -AgentImageTag v0.1.0 `
  -OtlpEndpoint otel-gateway.observability.svc.cluster.local:4317
```

## 7. Horizontal Sharding

The agent supports static shard ownership by pod UID hash:

- `SHARD_TOTAL`: total shard count
- `SHARD_ORDINAL`: this pod's shard index (0-based)

Notes:

- If `SHARD_TOTAL=1`, sharding is effectively disabled.
- If `SHARD_TOTAL>1`, you must provide `SHARD_ORDINAL` or run with StatefulSet-style pod names that end with `-<ordinal>`.
- Standard Deployment pod names are not stable ordinals, so for multi-replica sharding use explicit ordinals or StatefulSet.

Example (manual static shards):

```powershell
helm upgrade --install k8s-logging-agent-a deploy/helm/k8s-logging-agent `
  --namespace observability `
  --set env[3].name=SHARD_TOTAL --set env[3].value=2 `
  --set env[4].name=SHARD_ORDINAL --set env[4].value=0

helm upgrade --install k8s-logging-agent-b deploy/helm/k8s-logging-agent `
  --namespace observability `
  --set env[3].name=SHARD_TOTAL --set env[3].value=2 `
  --set env[4].name=SHARD_ORDINAL --set env[4].value=1
```

## 8. Troubleshooting

`ImagePullBackOff` for collector:
- Ensure collector tag is valid. Recommended: `0.145.0` (without `v`).

No forwarded logs:
- Confirm workloads are in the namespace watched by the agent.
- Confirm workload pods have `monitor-logs=true`.
- Check `LABEL_SELECTOR` value in Helm env values.

Agent running but no collector output:
- Check collector config map:
  - `kubectl -n observability get configmap k8s-logging-agent-collector -o yaml`
- Check collector logs in follow mode.

## 9. Cleanup

Remove workloads:

```powershell
kubectl -n observability delete -f deploy/sample-workloads.yaml
kubectl delete -f deploy/sample-workloads.yaml
```

Remove Helm release:

```powershell
helm uninstall k8s-logging-agent -n observability
```

Delete cluster:

```powershell
kind delete cluster --name k8s-logging
```
