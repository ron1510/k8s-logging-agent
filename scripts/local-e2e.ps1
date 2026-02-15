param(
  [string]$ClusterName = "k8s-logging",
  [string]$Namespace = "observability",
  [string]$ReleaseName = "k8s-logging-agent",
  [string]$ImageName = "k8s-logging-agent:dev",
  [int]$ReplicaCount = 2,
  [switch]$RecreateCluster
)

$ErrorActionPreference = "Stop"

function Require-Command([string]$Name) {
  if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
    throw "Required command not found: $Name"
  }
}

Require-Command "kind"
Require-Command "docker"
Require-Command "kubectl"
Require-Command "helm"
Require-Command "go"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

if ($RecreateCluster) {
  $existing = (kind get clusters) -split "`n" | ForEach-Object { $_.Trim() } | Where-Object { $_ -eq $ClusterName }
  if ($existing) {
    kind delete cluster --name $ClusterName
  }
}

$clusters = (kind get clusters) -split "`n" | ForEach-Object { $_.Trim() }
if (-not ($clusters -contains $ClusterName)) {
  kind create cluster --name $ClusterName
}

kubectl config use-context "kind-$ClusterName" | Out-Null

go mod download
docker build -t $ImageName .
kind load docker-image $ImageName --name $ClusterName

$imageRepo, $imageTag = $ImageName.Split(":", 2)
if (-not $imageTag) {
  $imageTag = "latest"
}

helm upgrade --install $ReleaseName deploy/helm/k8s-logging-agent `
  --namespace $Namespace `
  --create-namespace `
  -f deploy/helm/k8s-logging-agent/values-local.yaml `
  --set image.repository=$imageRepo `
  --set image.tag=$imageTag `
  --set image.pullPolicy=IfNotPresent `
  --set replicaCount=$ReplicaCount `
  --set collector.image.tag=0.145.0

kubectl -n $Namespace rollout status statefulset/$ReleaseName --timeout=240s
kubectl -n $Namespace apply -f deploy/sample-workloads.yaml

Write-Host ""
Write-Host "Local e2e environment is ready."
Write-Host ""
Write-Host "Realtime logs:"
Write-Host "  .\scripts\tail-logs.ps1 -Namespace $Namespace -Mode collector"
Write-Host "  .\scripts\tail-logs.ps1 -Namespace $Namespace -Mode agent"
Write-Host "  .\scripts\tail-logs.ps1 -Namespace $Namespace -Mode workloads"
Write-Host ""
Write-Host "Partitioner check:"
Write-Host "  .\scripts\verify-partitioner.ps1 -Namespace $Namespace -ReleaseName $ReleaseName"
