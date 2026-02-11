param(
  [string]$Namespace = "observability",
  [ValidateSet("agent", "collector", "workloads")]
  [string]$Mode = "collector",
  [int]$Tail = 100
)

$ErrorActionPreference = "Stop"

switch ($Mode) {
  "agent" {
    kubectl -n $Namespace logs -f deploy/k8s-logging-agent -c agent --tail=$Tail
  }
  "collector" {
    kubectl -n $Namespace logs -f deploy/k8s-logging-agent -c otel-collector --tail=$Tail
  }
  "workloads" {
    kubectl -n $Namespace logs -f -l monitor-logs=true --all-containers=true --prefix=true --tail=$Tail --max-log-requests=20
  }
}
