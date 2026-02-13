param(
  [string]$ClusterName = "k8s-logging",
  [string]$Namespace = "observability",
  [string]$ImageName = "k8s-logging-agent:dev",
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
  kind create cluster --name $ClusterName --config deploy/kind/multi-node.yaml
}

kubectl config use-context "kind-$ClusterName" | Out-Null
kubectl create namespace $Namespace --dry-run=client -o yaml | kubectl apply -f -

kubectl -n $Namespace apply -f deploy/elastic/elasticsearch.yaml
kubectl -n $Namespace apply -f deploy/elastic/kibana.yaml
kubectl -n $Namespace rollout status deploy/elasticsearch --timeout=300s
kubectl -n $Namespace rollout status deploy/kibana --timeout=300s

go mod download
docker build -t $ImageName .
kind load docker-image $ImageName --name $ClusterName

$imageRepo, $imageTag = $ImageName.Split(":", 2)
if (-not $imageTag) {
  $imageTag = "latest"
}

$release0 = "k8s-logging-agent-shard-0"
$release1 = "k8s-logging-agent-shard-1"

helm upgrade --install $release0 deploy/helm/k8s-logging-agent `
  --namespace $Namespace `
  -f deploy/helm/k8s-logging-agent/values-local.yaml `
  -f deploy/helm/k8s-logging-agent/values-elastic.yaml `
  --set image.repository=$imageRepo `
  --set image.tag=$imageTag `
  --set image.pullPolicy=IfNotPresent `
  --set env[3].value=2 `
  --set extraEnv[0].name=SHARD_ORDINAL `
  --set extraEnv[0].value=0

helm upgrade --install $release1 deploy/helm/k8s-logging-agent `
  --namespace $Namespace `
  -f deploy/helm/k8s-logging-agent/values-local.yaml `
  -f deploy/helm/k8s-logging-agent/values-elastic.yaml `
  --set image.repository=$imageRepo `
  --set image.tag=$imageTag `
  --set image.pullPolicy=IfNotPresent `
  --set env[3].value=2 `
  --set extraEnv[0].name=SHARD_ORDINAL `
  --set extraEnv[0].value=1

kubectl -n $Namespace rollout status deploy/$release0 --timeout=300s
kubectl -n $Namespace rollout status deploy/$release1 --timeout=300s

kubectl -n $Namespace apply -f deploy/sample-workloads-helm-release.yaml
kubectl -n $Namespace rollout status deploy/release-payments --timeout=180s
kubectl -n $Namespace rollout status deploy/release-billing --timeout=180s

Start-Sleep -Seconds 20

Write-Host ""
Write-Host "Elasticsearch indices:"
kubectl -n $Namespace exec deploy/elasticsearch -- curl -s "http://localhost:9200/_cat/indices?v"

Write-Host ""
Write-Host "Quick index checks:"
kubectl -n $Namespace exec deploy/elasticsearch -- curl -s "http://localhost:9200/_cat/indices/*payments*?v"
kubectl -n $Namespace exec deploy/elasticsearch -- curl -s "http://localhost:9200/_cat/indices/*billing*?v"

Write-Host ""
Write-Host "Kibana port-forward:"
Write-Host "  kubectl -n $Namespace port-forward svc/kibana 5601:5601"
Write-Host "  Open http://localhost:5601"

