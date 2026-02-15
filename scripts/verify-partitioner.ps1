param(
  [string]$Namespace = "observability",
  [string]$ReleaseName = "k8s-logging-agent",
  [int]$SinceSeconds = 90
)

$ErrorActionPreference = "Stop"

$agentPods = kubectl -n $Namespace get pods `
  -l "app.kubernetes.io/name=k8s-logging-agent,app.kubernetes.io/instance=$ReleaseName" `
  -o name

if (-not $agentPods) {
  throw "No agent pods found for release '$ReleaseName' in namespace '$Namespace'."
}

$podNames = $agentPods -split "`n" `
  | ForEach-Object { $_.Trim() } `
  | Where-Object { $_ -ne "" } `
  | ForEach-Object { ($_ -split "/", 2)[1] }
$owners = @{}

foreach ($agentPod in $podNames) {
  $lines = kubectl -n $Namespace logs $agentPod -c agent --since="${SinceSeconds}s"
  $workloadPods = @{}
  foreach ($line in ($lines -split "`n")) {
    if ($line -match 'AGENT_FORWARD .* pod=([^ ]+) ') {
      $workloadPods[$Matches[1]] = $true
    }
  }
  $owners[$agentPod] = $workloadPods.Keys | Sort-Object
}

Write-Host ""
Write-Host "Partition ownership summary (last ${SinceSeconds}s):"
foreach ($agentPod in $podNames) {
  $owned = $owners[$agentPod]
  Write-Host "  $agentPod => $($owned.Count) pod(s)"
  foreach ($p in $owned) {
    Write-Host "    - $p"
  }
}

$seenBy = @{}
foreach ($agentPod in $podNames) {
  foreach ($p in $owners[$agentPod]) {
    if (-not $seenBy.ContainsKey($p)) {
      $seenBy[$p] = @()
    }
    $seenBy[$p] += $agentPod
  }
}

$overlaps = $seenBy.GetEnumerator() | Where-Object { $_.Value.Count -gt 1 }
Write-Host ""
if ($overlaps.Count -eq 0) {
  Write-Host "No overlap detected: each workload pod appears on at most one agent shard."
} else {
  Write-Host "Overlap detected:"
  foreach ($ov in $overlaps) {
    Write-Host "  $($ov.Key) => $($ov.Value -join ', ')"
  }
}
