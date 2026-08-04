# Remote deploy helper (Windows OpenSSH client → Ubuntu server)
# Usage: powershell -File scripts/remote.ps1 <check|sync|sync-secret|up|down|restart|logs|ps|status|ssh> [extra ssh args]

param(
  [Parameter(Position = 0, Mandatory = $true)]
  [ValidateSet('check', 'sync', 'sync-secret', 'up', 'down', 'restart', 'logs', 'ps', 'status', 'ssh')]
  [string]$Action,

  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$Rest
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

function Read-DotEnv {
  param([string]$Path)
  $map = @{}
  if (-not (Test-Path $Path)) { return $map }
  Get-Content $Path | ForEach-Object {
    $line = $_.Trim()
    if (-not $line -or $line.StartsWith('#')) { return }
    $eq = $line.IndexOf('=')
    if ($eq -lt 1) { return }
    $k = $line.Substring(0, $eq).Trim()
    $v = $line.Substring($eq + 1).Trim()
    if (($v.StartsWith('"') -and $v.EndsWith('"')) -or ($v.StartsWith("'") -and $v.EndsWith("'"))) {
      $v = $v.Substring(1, $v.Length - 2)
    }
    $map[$k] = $v
  }
  return $map
}

function Resolve-OpenSsh {
  param([Parameter(Mandatory = $true)][ValidateSet('ssh', 'scp')][string]$Name)

  $file = "$Name.exe"
  $candidates = @(
    # 32-bit make/powershell: Sysnative maps to real 64-bit System32
    (Join-Path $env:SystemRoot "Sysnative\OpenSSH\$file"),
    (Join-Path $env:SystemRoot "System32\OpenSSH\$file")
  )

  # where.exe (more reliable than Get-Command under make / -NoProfile)
  try {
    $whereHits = & where.exe $file 2>$null
    if ($whereHits) {
      foreach ($hit in @($whereHits)) {
        if ($hit -and (Test-Path -LiteralPath $hit)) { $candidates += $hit }
      }
    }
  } catch {}

  $gitSsh = Join-Path ${env:ProgramFiles} "Git\usr\bin\$file"
  if (Test-Path -LiteralPath $gitSsh) { $candidates += $gitSsh }

  $cmd = Get-Command $file -ErrorAction SilentlyContinue
  if ($cmd -and $cmd.Source) { $candidates += $cmd.Source }
  $cmd2 = Get-Command $Name -ErrorAction SilentlyContinue
  if ($cmd2 -and $cmd2.Source) { $candidates += $cmd2.Source }

  foreach ($c in $candidates) {
    if ($c -and (Test-Path -LiteralPath $c)) {
      return (Resolve-Path -LiteralPath $c).Path
    }
  }

  throw @"
Missing $file.

Your interactive shell can often see ssh while make cannot (32-bit PATH / System32 redirection).
Install OpenSSH Client, or set a full path. Expected location:
  $env:SystemRoot\System32\OpenSSH\$file
"@
}

$SshExe = Resolve-OpenSsh ssh
$ScpExe = Resolve-OpenSsh scp
Write-Host "Using SSH: $SshExe"

$envMap = Read-DotEnv (Join-Path $Root '.env')
$hostName = $envMap['DEPLOY_HOST']
$user = if ($envMap['DEPLOY_USER']) { $envMap['DEPLOY_USER'] } else { 'ubuntu' }
$deployPath = if ($envMap['DEPLOY_PATH']) { $envMap['DEPLOY_PATH'] } else { '/opt/gantry' }
$sshPort = if ($envMap['DEPLOY_SSH_PORT']) { $envMap['DEPLOY_SSH_PORT'] } else { '22' }
$key = $envMap['DEPLOY_SSH_KEY']

if (-not $hostName) {
  throw "Set DEPLOY_HOST in .env (e.g. DEPLOY_HOST=myserver.example.com)"
}

$sshArgs = @('-p', $sshPort, '-o', 'StrictHostKeyChecking=accept-new')
# scp uses -P for port; -p means "preserve times" and breaks with "stat local 22"
$scpArgs = @('-P', $sshPort, '-o', 'StrictHostKeyChecking=accept-new')
if ($key) {
  if (-not (Test-Path -LiteralPath $key)) { throw "DEPLOY_SSH_KEY not found: $key" }
  $sshArgs += @('-i', $key)
  $scpArgs += @('-i', $key)
}
$target = "${user}@${hostName}"

function Invoke-Remote([string]$RemoteCmd) {
  & $SshExe @sshArgs $target $RemoteCmd
  if ($LASTEXITCODE -ne 0) { throw "Remote command failed ($LASTEXITCODE): $RemoteCmd" }
}

function Invoke-RemoteTTY([string]$RemoteCmd) {
  # -t so sudo can prompt for a password on native hosts.
  & $SshExe @sshArgs -t $target $RemoteCmd
  if ($LASTEXITCODE -ne 0) { throw "Remote command failed ($LASTEXITCODE): $RemoteCmd" }
}

function Get-ManifestPaths([string]$ManifestPath) {
  if (-not (Test-Path -LiteralPath $ManifestPath)) {
    throw "Missing manifest: $ManifestPath"
  }
  return @(
    Get-Content -LiteralPath $ManifestPath |
      ForEach-Object { $_.Trim() } |
      Where-Object { $_ -and -not $_.StartsWith('#') }
  )
}

function Ensure-RemoteParents([string[]]$RelPaths) {
  $dirs = @('data')
  $dirs += $RelPaths | ForEach-Object { ($_ -replace '\\', '/') } |
    Where-Object { $_.Contains('/') } |
    ForEach-Object { $_.Substring(0, $_.LastIndexOf('/')) }
  $mkdirArg = ($dirs | Sort-Object -Unique | ForEach-Object { "'${deployPath}/$_'" }) -join ' '
  if ($mkdirArg) {
    Write-Host "Ensuring remote dirs under ${deployPath}"
    Invoke-Remote "mkdir -p $mkdirArg"
  }
}

function Copy-ToRemote([string]$RelPath) {
  $local = Join-Path $Root $RelPath
  if (-not (Test-Path -LiteralPath $local)) {
    Write-Host "Skip missing $RelPath"
    return $false
  }
  $norm = ($RelPath -replace '\\', '/')
  $item = Get-Item -LiteralPath $local
  if ($item.PSIsContainer) {
    $parent = $norm.Substring(0, $norm.LastIndexOf('/'))
    Invoke-Remote "mkdir -p '${deployPath}/$norm'"
    Write-Host "scp -r $norm/"
    # Drop the directory onto its parent so remote ends as .../credentials/
    & $ScpExe @scpArgs -r $local "${target}:${deployPath}/$parent/"
  } else {
    Write-Host "scp $norm"
    & $ScpExe @scpArgs $local "${target}:${deployPath}/$norm"
  }
  if ($LASTEXITCODE -ne 0) { throw "scp failed: $RelPath" }
  return $true
}

switch ($Action) {
  'check' {
    Write-Host "Checking SSH: ${target}:${sshPort} -> ${deployPath}"
    Invoke-Remote "echo ok && uname -a && docker --version && docker compose version"
    Write-Host "Remote OK"
  }
  'sync' {
    # Code/config only - credentials use sync-secret (see secrets-manifest.txt)
    $files = Get-ManifestPaths (Join-Path $Root 'scripts/deploy-manifest.txt')
    Ensure-RemoteParents $files

    foreach ($f in $files) {
      Copy-ToRemote $f | Out-Null
    }

    # Persona is now SOUL/RULES/USER/TOOLS — scp does not delete leftovers
    $obsoletePersona = @(
      'IDENTITY.md', 'AGENTS.md', 'MEMORY.md', 'HEARTBEAT.md', 'BOOTSTRAP.md'
    ) -join ' '
    Write-Host "Cleaning obsolete persona files on remote (if present)"
    Invoke-Remote "cd '$deployPath/persona' 2>/dev/null && rm -f $obsoletePersona || true"

    Write-Host "Synced to ${target}:${deployPath}"
    Write-Host "Note: token/session secrets are NOT in remote-deploy. Use make garmin-sync / strava-sync / ghealth-sync / youtube-sync / google-sync"
  }
  'sync-secret' {
    $name = if ($Rest -and $Rest.Count -gt 0) { $Rest[0].Trim().ToLowerInvariant() } else { '' }
    if (-not $name) {
      throw "Usage: remote.ps1 sync-secret <garmin|strava|ghealth|youtube|ytmusic|google|all>"
    }
    $manifest = Join-Path $Root 'scripts/secrets-manifest.txt'
    $entries = Get-ManifestPaths $manifest | ForEach-Object {
      $parts = $_ -split '\|', 2
      if ($parts.Count -ne 2) { throw "Bad secrets-manifest line: $_" }
      [pscustomobject]@{ Name = $parts[0].Trim().ToLowerInvariant(); Path = $parts[1].Trim() }
    }
    $wanted = if ($name -eq 'all') { $entries } else { @($entries | Where-Object { $_.Name -eq $name }) }
    if ($wanted.Count -eq 0) {
      $known = ($entries.Name | Sort-Object -Unique) -join ', '
      throw "Unknown secret group '$name'. Known: $known, all"
    }

    # Only paths that exist locally (skip optional legacy secrets/google/*).
    $present = @($wanted | Where-Object { Test-Path -LiteralPath (Join-Path $Root $_.Path) })
    if ($present.Count -eq 0) {
      throw "No local files found for secret group '$name' - run the matching *-auth first"
    }

    # Stage under /tmp (writable by DEPLOY_USER), then sudo install into DEPLOY_PATH.
    # Native /opt/gantry is owned by gantry — plain mkdir/scp as DEPLOY_USER fails.
    $stage = '/tmp/gantry-secret-stage'
    Write-Host "Staging secrets under ${target}:${stage} then sudo install -> ${deployPath}"
    Write-Host "(sudo may prompt for your password)"
    Invoke-Remote "rm -rf $stage && mkdir -p $stage"

    $relPaths = @()
    foreach ($e in $present) {
      $local = Join-Path $Root $e.Path
      $norm = ($e.Path -replace '\\', '/')
      $relPaths += $norm
      $item = Get-Item -LiteralPath $local
      if ($item.PSIsContainer) {
        $parent = $norm.Substring(0, $norm.LastIndexOf('/'))
        Invoke-Remote "mkdir -p '${stage}/${parent}'"
        Write-Host "scp -r $norm/ -> stage"
        & $ScpExe @scpArgs -r $local "${target}:${stage}/${parent}/"
      } else {
        $parent = if ($norm.Contains('/')) { $norm.Substring(0, $norm.LastIndexOf('/')) } else { '' }
        if ($parent) { Invoke-Remote "mkdir -p '${stage}/${parent}'" }
        Write-Host "scp $norm -> stage"
        & $ScpExe @scpArgs $local "${target}:${stage}/${norm}"
      }
      if ($LASTEXITCODE -ne 0) { throw "scp failed: $($e.Path)" }
    }

    # Install script on the remote (UTF-8 no BOM) — sudo -t can prompt.
    $installLines = @(
      '#!/bin/bash',
      'set -euo pipefail',
      "STAGE='$stage'",
      "DEST='$deployPath'",
      'OWNER="$(stat -c %U "$DEST/data" 2>/dev/null || true)"',
      'if [ -z "${OWNER:-}" ]; then',
      '  if id gantry >/dev/null 2>&1; then OWNER=gantry; else OWNER="$(logname 2>/dev/null || echo root)"; fi',
      'fi',
      "for rel in $($relPaths -join ' '); do",
      '  mkdir -p "$DEST/$(dirname "$rel")"',
      '  if [ -d "$STAGE/$rel" ]; then',
      '    mkdir -p "$DEST/$rel"',
      '    cp -a "$STAGE/$rel/." "$DEST/$rel/"',
      '  else',
      '    cp -a "$STAGE/$rel" "$DEST/$rel"',
      '  fi',
      '  chown -R "$OWNER:$OWNER" "$DEST/$rel"',
      '  echo "installed $rel (owner=$OWNER)"',
      'done',
      'rm -rf "$STAGE"'
    )
    $installLocal = Join-Path $env:TEMP 'gantry-install-secrets.sh'
    [System.IO.File]::WriteAllText($installLocal, (($installLines -join "`n") + "`n"))
    & $ScpExe @scpArgs $installLocal "${target}:/tmp/gantry-install-secrets.sh"
    if ($LASTEXITCODE -ne 0) { throw 'scp install-secrets.sh failed' }
    Invoke-RemoteTTY 'sudo bash /tmp/gantry-install-secrets.sh'

    Write-Host ("Secret group '{0}' synced to {1}:{2} ({3} path(s))" -f $name, $target, $deployPath, $present.Count)
  }
  'up' {
    $bust = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
    Invoke-Remote "cd '${deployPath}' && docker compose build --pull --build-arg TOOLS_CACHEBUST=$bust && docker compose up -d"
  }
  'down' {
    Invoke-Remote "cd '${deployPath}' && docker compose down"
  }
  'restart' {
    Invoke-Remote "cd '${deployPath}' && docker compose restart"
  }
  'logs' {
    & $SshExe @sshArgs -t $target "cd '${deployPath}' && docker compose logs -f --tail=100"
  }
  'ps' {
    Invoke-Remote "cd '${deployPath}' && docker compose ps"
  }
  'status' {
    Invoke-Remote "cd '${deployPath}' && docker compose exec -T gantry /usr/local/bin/gantry status && echo OK"
  }
  'ssh' {
    if ($Rest -and $Rest.Count -gt 0) {
      $cmd = ($Rest -join ' ')
      & $SshExe @sshArgs -t $target "cd '${deployPath}' && $cmd"
    } else {
      & $SshExe @sshArgs -t $target "cd '${deployPath}' && exec `$SHELL -l"
    }
  }
}
