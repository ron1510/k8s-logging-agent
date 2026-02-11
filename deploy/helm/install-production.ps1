param(
  [Parameter(Mandatory = $true)]
  [string]$Namespace,

  [Parameter(Mandatory = $true)]
  [string]$AgentImageRepository,

  [Parameter(Mandatory = $true)]
  [string]$AgentImageTag,

  [Parameter(Mandatory = $true)]
  [string]$OtlpEndpoint,

  [string]$ReleaseName = "k8s-logging-agent"
)

$ErrorActionPreference = "Stop"

helm upgrade --install $ReleaseName .\k8s-logging-agent `
  --namespace $Namespace `
  --create-namespace `
  --values .\k8s-logging-agent\values-production.yaml `
  --set image.repository=$AgentImageRepository `
  --set image.tag=$AgentImageTag `
  --set collector.otlpEndpoint=$OtlpEndpoint
