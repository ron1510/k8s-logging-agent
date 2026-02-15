param(
  [string]$ClusterName = "k8s-logging",
  [string]$Namespace = "observability",
  [string]$ReleaseName = "k8s-logging-agent",
  [string]$ImageName = "k8s-logging-agent:dev",
  [int]$ReplicaCount = 2,
  [switch]$RecreateCluster,
  [int]$ValidationTimeoutSec = 180,
  [int]$PollSec = 10
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

$localArgs = @(
  "-ExecutionPolicy", "Bypass",
  "-File", (Join-Path $PSScriptRoot "local-e2e.ps1"),
  "-ClusterName", $ClusterName,
  "-Namespace", $Namespace,
  "-ReleaseName", $ReleaseName,
  "-ImageName", $ImageName,
  "-ReplicaCount", "$ReplicaCount"
)
if ($RecreateCluster) {
  $localArgs += "-RecreateCluster"
}

powershell @localArgs

function Get-MonitoredPods {
  $lines = kubectl -n $Namespace get pods -l monitor-logs=true -o custom-columns=NAME:.metadata.name,PHASE:.status.phase --no-headers
  if (-not $lines) { return @() }
  return $lines -split "`n" |
    ForEach-Object { $_.Trim() } |
    Where-Object { $_ -ne "" } |
    ForEach-Object {
      $parts = ($_ -split "\s+")
      [pscustomobject]@{ Name = $parts[0]; Phase = $parts[1] }
    } |
    Where-Object { $_.Phase -eq "Running" } |
    ForEach-Object { $_.Name }
}

function Get-AgentPods {
  $out = kubectl -n $Namespace get pods -l "app.kubernetes.io/name=k8s-logging-agent,app.kubernetes.io/instance=$ReleaseName" -o name
  if (-not $out) { return @() }
  return $out -split "`n" |
    ForEach-Object { $_.Trim() } |
    Where-Object { $_ -ne "" } |
    ForEach-Object { ($_ -split "/", 2)[1] }
}

function Collect-Ownership([int]$SinceSec, [string[]]$AgentPods) {
  $seenBy = @{}
  $owners = @{}

  foreach ($agentPod in $AgentPods) {
    $owners[$agentPod] = New-Object 'System.Collections.Generic.HashSet[string]'
    try {
      $logs = kubectl -n $Namespace logs $agentPod -c agent --since="${SinceSec}s"
    } catch {
      continue
    }

    foreach ($line in ($logs -split "`n")) {
      if ($line -match 'AGENT_FORWARD .* pod=([^ ]+) ') {
        $podName = $Matches[1]
        $null = $owners[$agentPod].Add($podName)
        if (-not $seenBy.ContainsKey($podName)) {
          $seenBy[$podName] = New-Object 'System.Collections.Generic.List[string]'
        }
        if (-not $seenBy[$podName].Contains($agentPod)) {
          $seenBy[$podName].Add($agentPod)
        }
      }
    }
  }

  return [pscustomobject]@{ SeenBy = $seenBy; Owners = $owners }
}

$start = Get-Date
$lastStatus = ""

while ($true) {
  $elapsed = [int]((Get-Date) - $start).TotalSeconds
  if ($elapsed -ge $ValidationTimeoutSec) {
    throw "Validation timed out after ${ValidationTimeoutSec}s. Last status: $lastStatus"
  }

  $monitoredPods = Get-MonitoredPods
  $agentPods = Get-AgentPods
  if ($agentPods.Count -eq 0) {
    $lastStatus = "no agent pods found"
    Start-Sleep -Seconds $PollSec
    continue
  }

  $window = [Math]::Max(60, $elapsed + 20)
  $snapshot = Collect-Ownership -SinceSec $window -AgentPods $agentPods

  $overlaps = @()
  foreach ($kvp in $snapshot.SeenBy.GetEnumerator()) {
    if ($kvp.Value.Count -gt 1) {
      $overlaps += "$($kvp.Key) => $([string]::Join(', ', $kvp.Value))"
    }
  }

  $missing = @()
  foreach ($pod in $monitoredPods) {
    if (-not $snapshot.SeenBy.ContainsKey($pod)) {
      $missing += $pod
    }
  }

  if ($overlaps.Count -eq 0 -and $missing.Count -eq 0) {
    Write-Host ""
    Write-Host "Validation passed: no duplicate collection and full monitored-pod coverage."
    Write-Host ""
    Write-Host "Shard ownership:"
    foreach ($agentPod in $agentPods) {
      $owned = @($snapshot.Owners[$agentPod]) | Sort-Object
      Write-Host "  $agentPod => $($owned.Count) pod(s)"
      foreach ($p in $owned) {
        Write-Host "    - $p"
      }
    }
    exit 0
  }

  $lastStatus = "missing=$($missing.Count), overlaps=$($overlaps.Count), monitored=$($monitoredPods.Count), agents=$($agentPods.Count)"
  Write-Host "Waiting: $lastStatus"
  Start-Sleep -Seconds $PollSec
}
