# Native remote deploy (Windows OpenSSH -> Ubuntu binary + systemd, no Docker).
# Usage: powershell -File scripts/remote-native.ps1 <action>
# Actions: check|env|fetch|sync|install|up|down|restart|logs|status|deploy

param(
  [Parameter(Position = 0, Mandatory = $true)]
  [ValidateSet('check', 'env', 'fetch', 'build-dev', 'sync', 'install', 'up', 'down', 'restart', 'logs', 'status', 'deploy', 'deploy-dev')]
  [string]$Action,
  # Skip MCP tool fetch/stage (reuse /opt/gantry/bin). For small gantry/persona iterates
  # without hitting GitHub (download_tag=latest rate limits). Env: NATIVE_SKIP_TOOLS=1
  [switch]$SkipTools
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
    # strip inline comments for simple KEY=value  # comment lines
    if ($v -match '^([^#]*?)(\s+#.*)$') { $v = $Matches[1].Trim() }
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
    (Join-Path $env:SystemRoot "Sysnative\OpenSSH\$file"),
    (Join-Path $env:SystemRoot "System32\OpenSSH\$file")
  )
  try {
    $whereHits = & where.exe $file 2>$null
    if ($whereHits) { foreach ($hit in @($whereHits)) { if ($hit -and (Test-Path -LiteralPath $hit)) { $candidates += $hit } } }
  } catch {}
  $gitSsh = Join-Path ${env:ProgramFiles} "Git\usr\bin\$file"
  if (Test-Path -LiteralPath $gitSsh) { $candidates += $gitSsh }
  foreach ($c in $candidates) {
    if ($c -and (Test-Path -LiteralPath $c)) { return (Resolve-Path -LiteralPath $c).Path }
  }
  throw "Missing $file - install OpenSSH Client"
}

$SshExe = Resolve-OpenSsh ssh
$ScpExe = Resolve-OpenSsh scp
$envMap = Read-DotEnv (Join-Path $Root '.env')
$hostName = $envMap['DEPLOY_HOST']
$user = if ($envMap['DEPLOY_USER']) { $envMap['DEPLOY_USER'] } else { 'ubuntu' }
$deployPath = if ($envMap['DEPLOY_PATH']) { $envMap['DEPLOY_PATH'] } else { '/opt/gantry' }
$sshPort = if ($envMap['DEPLOY_SSH_PORT']) { $envMap['DEPLOY_SSH_PORT'] } else { '22' }
$key = $envMap['DEPLOY_SSH_KEY']
$gantryVersion = if ($envMap['GANTRY_VERSION']) { $envMap['GANTRY_VERSION'] } else { 'latest' }
$dockerHost = $envMap['NATIVE_DOCKER_HOST']
$dockerContainer = if ($envMap['NATIVE_DOCKER_CONTAINER']) { $envMap['NATIVE_DOCKER_CONTAINER'] } else { 'gantry' }
$DefaultLlmModel = 'qwen3.6:35b-a3b'
if (-not $SkipTools) {
  $v = $envMap['NATIVE_SKIP_TOOLS']
  if ($v -eq '1' -or $v -eq 'true' -or $v -eq 'yes') { $SkipTools = $true }
}

if (-not $hostName -and $Action -ne 'env' -and $Action -ne 'fetch') {
  throw "Set DEPLOY_HOST in .env"
}

$sshArgs = @('-p', $sshPort, '-o', 'StrictHostKeyChecking=accept-new')
$scpArgs = @('-P', $sshPort, '-o', 'StrictHostKeyChecking=accept-new')
if ($key) {
  if (-not (Test-Path -LiteralPath $key)) { throw "DEPLOY_SSH_KEY not found: $key" }
  $sshArgs += @('-i', $key)
  $scpArgs += @('-i', $key)
}
$target = "${user}@${hostName}"
$Cache = Join-Path $Root '.cache\native'
$StageRemote = '/tmp/gantry-native'
$DeployDir = Join-Path $Root 'deploy'

function Invoke-Remote([string]$RemoteCmd) {
  & $SshExe @sshArgs $target $RemoteCmd
  if ($LASTEXITCODE -ne 0) { throw "Remote command failed ($LASTEXITCODE): $RemoteCmd" }
}

function Invoke-RemoteTTY([string]$RemoteCmd) {
  & $SshExe @sshArgs -t $target $RemoteCmd
  if ($LASTEXITCODE -ne 0) { throw "Remote command failed ($LASTEXITCODE): $RemoteCmd" }
}

function Get-ReleaseTag {
  if ($gantryVersion -ne 'latest') { return $gantryVersion }
  $api = Invoke-RestMethod -Uri 'https://api.github.com/repos/shotah/ai-gantry/releases/latest' -Headers @{ 'User-Agent' = 'ai-gantry-remote-native' }
  return $api.tag_name
}

function Resolve-LlmModel {
  # Precedence: explicit pin -> existing gantry.env -> current default.
  # deploy/deploy-dev do NOT call Write-NativeEnv; edit NATIVE_LLM_MODEL (or
  # deploy/gantry.env) then make remote-native-env when you want a rewrite.
  $src = Read-DotEnv (Join-Path $Root '.env')
  if ($src['NATIVE_LLM_MODEL']) { return $src['NATIVE_LLM_MODEL'] }
  if ($src['LLM_MODEL']) { return $src['LLM_MODEL'] }
  $existing = Join-Path $DeployDir 'gantry.env'
  if (Test-Path $existing) {
    $prev = Read-DotEnv $existing
    if ($prev['LLM_MODEL']) { return $prev['LLM_MODEL'] }
  }
  return $DefaultLlmModel
}

function Ensure-NativeEnv {
  $envFile = Join-Path $DeployDir 'gantry.env'
  if (Test-Path $envFile) {
    $prev = Read-DotEnv $envFile
    $model = if ($prev['LLM_MODEL']) { $prev['LLM_MODEL'] } else { '(unset)' }
    Write-Host "Using existing deploy/gantry.env (LLM_MODEL=$model). Regenerate: make remote-native-env"
    return
  }
  Write-Host "deploy/gantry.env missing - generating from .env"
  Write-NativeEnv
}

function Write-NativeEnv {
  $src = Read-DotEnv (Join-Path $Root '.env')
  if (-not $src['TELEGRAM_BOT_TOKEN']) { throw ".env missing TELEGRAM_BOT_TOKEN" }
  if (-not $src['TELEGRAM_ALLOWED_USERS']) { throw ".env missing TELEGRAM_ALLOWED_USERS" }

  $llmModel = Resolve-LlmModel
  $out = Join-Path $DeployDir 'gantry.env'
  $tz = if ($src['TZ']) { $src['TZ'] } else { 'America/Los_Angeles' }
  $stream = if ($src['STREAM_REPLIES']) { $src['STREAM_REPLIES'] } else { 'true' }
  $lines = [System.Collections.Generic.List[string]]@(
    '# Generated by make remote-native-env - do not commit',
    'HOME=/opt/gantry/data',
    'DATA_DIR=/opt/gantry/data',
    'PERSONA_DIR=/opt/gantry/persona',
    'MCP_MANIFEST=/opt/gantry/mcp.toml',
    'LLM_BASE_URL=http://127.0.0.1:11434/v1',
    'LLM_API_KEY=ollama',
    "LLM_MODEL=$llmModel",
    # 8192 default: prompt (persona+tools+history) + completion must fit the
    # 32k Ollama context; thinking tokens count toward the completion cap.
    "LLM_MAX_TOKENS=$(if ($src['LLM_MAX_TOKENS']) { $src['LLM_MAX_TOKENS'] } else { '8192' })"
  )
  # Native default is no CoT: on a local model, thinking tokens are decoded at
  # full price before any tool fires, and that is the bulk of the perceived lag.
  # Set LLM_REASONING_EFFORT=low|medium in .env to get thinking back; set it
  # empty to omit the field entirely (Ollama's own default).
  if ($src.ContainsKey('LLM_REASONING_EFFORT')) {
    if ($src['LLM_REASONING_EFFORT']) {
      $lines.Add("LLM_REASONING_EFFORT=$($src['LLM_REASONING_EFFORT'])")
    }
  } else {
    $lines.Add('LLM_REASONING_EFFORT=none')
  }
  # Tool payloads are re-sent on every loop iteration, so the cap is a prefill
  # cost multiplier, not just a one-off trim. 6000 is the native tradeoff.
  $lines.Add("TOOL_RESULT_MAX_CHARS=$(if ($src['TOOL_RESULT_MAX_CHARS']) { $src['TOOL_RESULT_MAX_CHARS'] } else { '6000' })")
  $lines.AddRange([string[]]@(
    'CHANNEL=telegram',
    "TELEGRAM_BOT_TOKEN=$($src['TELEGRAM_BOT_TOKEN'])",
    "TELEGRAM_ALLOWED_USERS=$($src['TELEGRAM_ALLOWED_USERS'])",
    "STREAM_REPLIES=$stream",
    "TELEGRAM_ERROR_REPORTING=$(if ($src['TELEGRAM_ERROR_REPORTING']) { $src['TELEGRAM_ERROR_REPORTING'] } else { 'error' })",
    'LOG_LEVEL=info',
    "TZ=$tz",
    "CRON_TZ=$tz",
    'CRON_ENABLED=true'
  ))
  # Spark of life — opt-in; only emit when SPARK_QTY is set in .env
  if ($src['SPARK_QTY']) {
    $lines.Add("SPARK_QTY=$($src['SPARK_QTY'])")
    $lines.Add("SPARK_START_HOUR=$(if ($src['SPARK_START_HOUR']) { $src['SPARK_START_HOUR'] } else { '6' })")
    $lines.Add("SPARK_END_HOUR=$(if ($src['SPARK_END_HOUR']) { $src['SPARK_END_HOUR'] } else { '21' })")
    $lines.Add("SPARK_SKIP_RECENT_MINUTES=$(if ($src['SPARK_SKIP_RECENT_MINUTES']) { $src['SPARK_SKIP_RECENT_MINUTES'] } else { '15' })")
    if ($src['SPARK_PROMPT']) {
      $lines.Add("SPARK_PROMPT=$($src['SPARK_PROMPT'])")
    }
  }
  $lines.AddRange([string[]]@(
    "GEMINI_API_KEY=$($src['GEMINI_API_KEY'])",
    "GEMINI_MODEL=$(if ($src['GEMINI_MODEL']) { $src['GEMINI_MODEL'] } else { 'gemini-3.5-flash' })",
    "GOOGLE_OAUTH_CLIENT_ID=$($src['GOOGLE_OAUTH_CLIENT_ID'])",
    "GOOGLE_OAUTH_CLIENT_SECRET=$($src['GOOGLE_OAUTH_CLIENT_SECRET'])",
    "USER_GOOGLE_EMAIL=$($src['USER_GOOGLE_EMAIL'])",
    'WORKSPACE_MCP_CREDENTIALS_DIR=/opt/gantry/data/.config/google-mcp/credentials',
    "STRAVA_CLIENT_ID=$($src['STRAVA_CLIENT_ID'])",
    "STRAVA_CLIENT_SECRET=$($src['STRAVA_CLIENT_SECRET'])",
    'STRAVA_TOKEN_PATH=/opt/gantry/data/.config/strava/tokens.json',
    'YTMUSIC_HEADERS_PATH=/opt/gantry/data/.config/ytmusic/headers.json'
  ))
  New-Item -ItemType Directory -Force -Path $DeployDir | Out-Null
  Set-Content -LiteralPath $out -Value $lines -Encoding utf8
  Write-Host "Wrote $out (Ollama model=$llmModel)"
}

function Fetch-GantryBinary {
  New-Item -ItemType Directory -Force -Path $Cache | Out-Null
  $tag = Get-ReleaseTag
  $ver = $tag.TrimStart('v')
  $asset = "gantry_${ver}_linux_amd64.tar.gz"
  $url = "https://github.com/shotah/ai-gantry/releases/download/$tag/$asset"
  $tarball = Join-Path $Cache $asset
  Write-Host "Fetching $url"
  Invoke-WebRequest -Uri $url -OutFile $tarball -UseBasicParsing
  $extract = Join-Path $Cache 'extract'
  if (Test-Path $extract) { Remove-Item -Recurse -Force $extract }
  New-Item -ItemType Directory -Force -Path $extract | Out-Null
  tar -xzf $tarball -C $extract
  $bin = Join-Path $extract 'gantry'
  if (-not (Test-Path $bin)) { throw "tarball missing gantry binary" }
  Copy-Item $bin (Join-Path $Cache 'gantry') -Force
  Write-Host "Cached gantry $tag -> .cache/native/gantry"
}

function Build-GantryDev {
  # Cross-compile linux/amd64 from the repo working tree (skip GitHub release).
  $repoRoot = Split-Path -Parent $Root
  $cmdDir = Join-Path $repoRoot 'cmd\gantry'
  if (-not (Test-Path $cmdDir)) { throw "Missing $cmdDir - run from ai-gantry checkout" }
  New-Item -ItemType Directory -Force -Path $Cache | Out-Null
  $out = Join-Path $Cache 'gantry'
  $version = 'dev'
  $commit = 'none'
  try { $version = (& git -C $repoRoot describe --tags --always --dirty 2>$null).Trim() } catch {}
  if (-not $version) { $version = 'dev' }
  try { $commit = (& git -C $repoRoot rev-parse --short HEAD 2>$null).Trim() } catch {}
  if (-not $commit) { $commit = 'none' }
  $ldflags = "-s -w -X main.version=$version -X main.commit=$commit -X main.date=dev"
  Write-Host "Building linux/amd64 gantry ($version) -> .cache/native/gantry"
  $prevCgo, $prevGoos, $prevGoarch = $env:CGO_ENABLED, $env:GOOS, $env:GOARCH
  $env:CGO_ENABLED = '0'
  $env:GOOS = 'linux'
  $env:GOARCH = 'amd64'
  try {
    Push-Location $repoRoot
    & go build -trimpath -ldflags $ldflags -o $out ./cmd/gantry
    if ($LASTEXITCODE -ne 0) { throw "go build failed ($LASTEXITCODE)" }
  } finally {
    Pop-Location
    $env:CGO_ENABLED, $env:GOOS, $env:GOARCH = $prevCgo, $prevGoos, $prevGoarch
  }
  if (-not (Test-Path $out)) { throw "build missing $out" }
  Write-Host "Built $out"
}

function Get-McpToolsPlan {
  # Inventory from mcp.toml via gantry tools-plan (download_url + all commands).
  # Hits GitHub when download_tag=latest — use -SkipTools / Get-McpCommandsFromToml for iterates.
  $repoRoot = Split-Path -Parent $Root
  $manifest = Join-Path $Root 'mcp.toml'
  if (-not (Test-Path $manifest)) { throw "Missing $manifest" }
  Push-Location $repoRoot
  try {
    $json = & go run ./cmd/gantry tools-plan --manifest $manifest --os linux --arch amd64
    if ($LASTEXITCODE -ne 0) { throw "gantry tools-plan failed ($LASTEXITCODE)" }
  } finally {
    Pop-Location
  }
  return ($json | ConvertFrom-Json)
}

function Get-McpCommandsFromToml {
  # command = "…" lines only — no GitHub / tools-plan.
  $manifest = Join-Path $Root 'mcp.toml'
  if (-not (Test-Path $manifest)) { throw "Missing $manifest" }
  $cmds = [System.Collections.Generic.List[string]]::new()
  Get-Content -LiteralPath $manifest | ForEach-Object {
    if ($_ -match '^\s*command\s*=\s*"([^"]+)"\s*(#.*)?$') {
      $cmds.Add($Matches[1])
    }
  }
  return @($cmds | Select-Object -Unique)
}

function Fetch-McpDownloadTools {
  # GET every mcp.toml server with download_url into .cache/native/bin.
  # Skip when the versioned archive + extracted binary are already cached
  # (download_tag=latest still refreshes when the resolved filename changes).
  $plan = Get-McpToolsPlan
  if (-not $plan.downloads -or $plan.downloads.Count -lt 1) {
    Write-Host "mcp.toml: no download_url servers - skipping tool fetch"
    return
  }
  $binDir = Join-Path $Cache 'bin'
  New-Item -ItemType Directory -Force -Path $binDir | Out-Null
  $headers = @{ 'User-Agent' = 'ai-gantry-remote-native' }
  foreach ($d in $plan.downloads) {
    $url = [string]$d.url
    $file = [string]$d.filename
    if (-not $file) { $file = Split-Path -Leaf ([uri]$url).AbsolutePath }
    $tarball = Join-Path $Cache $file
    $dest = Join-Path $binDir $d.command
    if ((Test-Path -LiteralPath $tarball) -and (Test-Path -LiteralPath $dest)) {
      Write-Host "Skip $($d.name): already have $file -> bin/$($d.command)"
      continue
    }
    if (-not (Test-Path -LiteralPath $tarball)) {
      Write-Host "Fetching $($d.name) ($($d.command)) from $url"
      Invoke-WebRequest -Uri $url -OutFile $tarball -UseBasicParsing -Headers $headers
    } else {
      Write-Host "Reusing cached archive $file for $($d.name)"
    }
    $extract = Join-Path $Cache ("extract-" + $d.command)
    if (Test-Path $extract) { Remove-Item -Recurse -Force $extract }
    New-Item -ItemType Directory -Force -Path $extract | Out-Null
    tar -xzf $tarball -C $extract
    $bin = Join-Path $extract $d.command
    if (-not (Test-Path $bin)) { throw "archive missing $($d.command) ($file)" }
    Copy-Item $bin $dest -Force
    Write-Host "Cached $($d.command) -> .cache/native/bin/$($d.command)"
  }
  $wanted = @(
    @($plan.downloads | ForEach-Object { [string]$_.command }) + @($plan.commands)
  ) | Where-Object { $_ } | Select-Object -Unique
  Prune-CachedToolBins -Wanted $wanted
}

function Prune-CachedToolBins {
  param([string[]]$Wanted)
  $binDir = Join-Path $Cache 'bin'
  if (-not (Test-Path $binDir)) { return }
  $keep = @{}
  foreach ($name in $Wanted) {
    if ($name) { $keep[$name] = $true }
  }
  Get-ChildItem $binDir -File | ForEach-Object {
    if ($keep.ContainsKey($_.Name)) { return }
    Write-Host "Pruning stale cache bin/$($_.Name) (not in mcp.toml)"
    Remove-Item -LiteralPath $_.FullName -Force
  }
}

function Fetch-ToolsFromDocker {
  # Optional one-time bootstrap from a *separate* Docker TIM host.
  # Native DEPLOY_HOST has no Docker — leave NATIVE_DOCKER_HOST unset.
  # Command list comes from mcp.toml (gantry tools-plan), not a hardcoded array.
  if (-not $dockerHost) {
    Write-Host "NATIVE_DOCKER_HOST unset - skipping MCP tool extract (using mcp.toml download_url + host /opt/gantry/bin)"
    return
  }
  if ($dockerHost -eq $hostName) {
    Write-Host "NATIVE_DOCKER_HOST equals DEPLOY_HOST ($dockerHost) - skipping docker extract (native host)"
    return
  }
  $plan = Get-McpToolsPlan
  $tools = @($plan.commands)
  if ($tools.Count -lt 1) {
    Write-Host "mcp.toml: no server commands - skipping docker extract"
    return
  }
  $binDir = Join-Path $Cache 'bin'
  New-Item -ItemType Directory -Force -Path $binDir | Out-Null
  $remoteTmp = '/tmp/gantry-docker-bin'
  $dockerUser = if ($envMap['NATIVE_DOCKER_USER']) { $envMap['NATIVE_DOCKER_USER'] } else { $user }
  Write-Host "Optional: extracting MCP tools from docker:$dockerContainer on $dockerHost"
  $extractSh = @"
set -e
command -v docker >/dev/null || { echo 'docker not installed on this host'; exit 1; }
rm -rf $remoteTmp
mkdir -p $remoteTmp
for b in $($tools -join ' '); do
  docker cp ${dockerContainer}:/usr/local/bin/`$b $remoteTmp/`$b
done
chmod a+x $remoteTmp/*
ls -la $remoteTmp
"@
  $extractLocal = Join-Path $Cache 'extract-tools.sh'
  # UTF-8 no BOM - bash on Linux rejects BOM as a bad interpreter line
  [System.IO.File]::WriteAllText($extractLocal, ($extractSh -replace "`r`n", "`n"))
  & $ScpExe @scpArgs $extractLocal "${dockerUser}@${dockerHost}:/tmp/extract-gantry-tools.sh"
  if ($LASTEXITCODE -ne 0) {
    Write-Warning "scp extract script to $dockerHost failed - continuing without docker tools"
    return
  }
  & $SshExe @sshArgs "${dockerUser}@${dockerHost}" "bash /tmp/extract-gantry-tools.sh"
  if ($LASTEXITCODE -ne 0) {
    Write-Warning "docker tool extract failed on $dockerHost - continuing (release gantry still deploys)"
    return
  }
  & $ScpExe @scpArgs -r "${dockerUser}@${dockerHost}:${remoteTmp}/." "$binDir/"
  if ($LASTEXITCODE -ne 0) {
    Write-Warning "scp tools from $dockerHost failed - continuing without docker tools"
    return
  }
  # Never overwrite the GitHub release gantry with a docker copy.
  Remove-Item (Join-Path $binDir 'gantry') -ErrorAction SilentlyContinue
  Write-Host "Tools cached under .cache/native/bin"
}

function Sync-Stage {
  $gantryBin = Join-Path $Cache 'gantry'
  if (-not (Test-Path $gantryBin)) { throw "Missing .cache/native/gantry - run fetch first" }
  $envFile = Join-Path $DeployDir 'gantry.env'
  if (-not (Test-Path $envFile)) { throw "Missing deploy/gantry.env - run make remote-native-env" }

  Write-Host "Staging -> ${target}:${StageRemote}"
  if ($SkipTools) {
    Write-Host "SkipTools: not staging MCP bins (host /opt/gantry/bin left as-is)"
    Invoke-Remote "rm -rf $StageRemote && mkdir -p $StageRemote/persona"
  } else {
    Invoke-Remote "rm -rf $StageRemote && mkdir -p $StageRemote/bin $StageRemote/persona"
  }
  & $ScpExe @scpArgs $gantryBin "${target}:${StageRemote}/gantry"
  if ($LASTEXITCODE -ne 0) { throw 'scp gantry failed' }
  & $ScpExe @scpArgs (Join-Path $DeployDir 'gantry.service') "${target}:${StageRemote}/gantry.service"
  & $ScpExe @scpArgs (Join-Path $DeployDir 'install.sh') "${target}:${StageRemote}/install.sh"
  & $ScpExe @scpArgs $envFile "${target}:${StageRemote}/gantry.env"
  $ollamaConf = Join-Path $DeployDir 'ollama-gantry.conf'
  if (Test-Path $ollamaConf) { & $ScpExe @scpArgs $ollamaConf "${target}:${StageRemote}/ollama-gantry.conf" }
  $mcp = Join-Path $Root 'mcp.toml'
  if (Test-Path $mcp) { & $ScpExe @scpArgs $mcp "${target}:${StageRemote}/mcp.toml" }

  if (-not $SkipTools) {
    # Commands from mcp.toml only (no GitHub). Fetch already resolved archives into cache.
    $wantedBins = @(Get-McpCommandsFromToml | Where-Object { $_ })
    Prune-CachedToolBins -Wanted $wantedBins
    $binDir = Join-Path $Cache 'bin'
    foreach ($name in $wantedBins) {
      $src = Join-Path $binDir $name
      if (-not (Test-Path -LiteralPath $src)) {
        Write-Host "Note: mcp.toml command '$name' not in .cache/native/bin (run fetch, or deploy with -SkipTools)"
        continue
      }
      & $ScpExe @scpArgs $src "${target}:${StageRemote}/bin/$name"
      if ($LASTEXITCODE -ne 0) { throw "scp tool $name failed" }
    }
  }

  # Persona: only the canonical four (SOUL/RULES/USER/TOOLS). install.sh rsync --delete
  # clears obsolete ZeroClaw-era names on the host.
  $persona = Join-Path $Root 'persona'
  $personaWanted = @('SOUL.md', 'RULES.md', 'USER.md', 'TOOLS.md')
  $personaFiles = @()
  if (Test-Path $persona) {
    $personaFiles = @(
      Get-ChildItem $persona -Filter '*.md' -File |
        Where-Object { $personaWanted -contains $_.Name }
    )
  }
  if ($personaFiles.Count -gt 0) {
    foreach ($f in $personaFiles) {
      & $ScpExe @scpArgs $f.FullName "${target}:${StageRemote}/persona/$($f.Name)"
    }
  } else {
    Write-Host "No local persona/*.md - leaving host persona untouched at install"
  }

  Invoke-Remote "chmod +x $StageRemote/gantry $StageRemote/install.sh $StageRemote/bin/* 2>/dev/null; ls -la $StageRemote; ls -la $StageRemote/bin 2>/dev/null || true"
  Write-Host "Staged. Install with: make remote-native-install"
}

function Install-Remote {
  param([switch]$Restart)
  Write-Host "Installing via sudo (password prompt if needed)..."
  # One SSH + one sudo so deploy doesn't prompt twice (install, then restart).
  if ($Restart) {
    Invoke-RemoteTTY "sudo bash -c 'bash $StageRemote/install.sh && systemctl restart gantry && systemctl --no-pager --full status gantry | head -n 25'"
  } else {
    Invoke-RemoteTTY "sudo bash $StageRemote/install.sh"
  }
}

switch ($Action) {
  'check' {
    Write-Host "Checking native host ${target} ($deployPath)"
    Invoke-Remote "echo ok && uname -a && systemctl is-active ollama && test -d $deployPath && id gantry && ollama ps && echo NATIVE_OK"
  }
  'env' { Write-NativeEnv }
  'fetch' {
    Fetch-GantryBinary
    Fetch-McpDownloadTools
    Fetch-ToolsFromDocker
  }
  'build-dev' { Build-GantryDev }
  'sync' { Sync-Stage }
  'install' { Install-Remote }
  'up' { Invoke-RemoteTTY "sudo systemctl start gantry && systemctl --no-pager --full status gantry | head -n 20" }
  'down' { Invoke-RemoteTTY "sudo systemctl stop gantry && echo stopped" }
  'restart' { Invoke-RemoteTTY "sudo systemctl restart gantry && systemctl --no-pager --full status gantry | head -n 20" }
  'logs' { Invoke-RemoteTTY "journalctl -u gantry -f -n 100" }
  'status' { Invoke-RemoteTTY "systemctl is-active gantry; sudo -u gantry $deployPath/gantry status; echo OK" }
  'deploy' {
    Ensure-NativeEnv
    Fetch-GantryBinary
    Fetch-McpDownloadTools
    Fetch-ToolsFromDocker
    Sync-Stage
    Install-Remote -Restart
    Write-Host "Deployed. Message TIM on Telegram; logs: make remote-native-logs"
  }
  'deploy-dev' {
    Ensure-NativeEnv
    Build-GantryDev
    if ($SkipTools) {
      Write-Host "SkipTools: not fetching MCP binaries from GitHub"
    } else {
      Fetch-McpDownloadTools
    }
    Sync-Stage
    Install-Remote -Restart
    Write-Host "Dev deploy done (local linux/amd64 build). logs: make remote-native-logs"
  }
}
