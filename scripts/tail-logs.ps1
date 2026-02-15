param(
  [string]$Namespace = "observability",
  [string]$ReleaseName = "k8s-logging-agent",
  [ValidateSet("agent", "collector", "workloads")]
  [string]$Mode = "collector",
  [int]$Tail = 100
)

$ErrorActionPreference = "Stop"

switch ($Mode) {
  "agent" {
    kubectl -n $Namespace logs -f -l "app.kubernetes.io/name=k8s-logging-agent,app.kubernetes.io/instance=$ReleaseName" -c agent --prefix=true --tail=$Tail --max-log-requests=20
  }
  "collector" {
    kubectl -n $Namespace logs -f -l "app.kubernetes.io/name=k8s-logging-agent,app.kubernetes.io/instance=$ReleaseName" -c otel-collector --prefix=true --tail=$Tail --max-log-requests=20
  }
  "workloads" {
    kubectl -n $Namespace logs -f -l monitor-logs=true --all-containers=true --prefix=true --tail=$Tail --max-log-requests=20
  }
}
