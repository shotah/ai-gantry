# Convert secrets/google/credentials.json (gws export) into the magks MCP
# credential file format under secrets/google-mcp/credentials/<email>.json.
# Usage (repo root):  make google-mcp-import
# Requires: USER_GOOGLE_EMAIL, and either GOOGLE_OAUTH_* in .env or client_secret.json

param(
  [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
)

$ErrorActionPreference = 'Stop'

function Get-DotEnv([string]$Path) {
  $map = @{}
  if (-not (Test-Path -LiteralPath $Path)) { return $map }
  Get-Content -LiteralPath $Path | ForEach-Object {
    $line = $_.Trim()
    if (-not $line -or $line.StartsWith('#')) { return }
    $i = $line.IndexOf('=')
    if ($i -lt 1) { return }
    $k = $line.Substring(0, $i).Trim()
    $v = $line.Substring($i + 1).Trim()
    if (($v.StartsWith('"') -and $v.EndsWith('"')) -or ($v.StartsWith("'") -and $v.EndsWith("'"))) {
      $v = $v.Substring(1, $v.Length - 2)
    }
    $map[$k] = $v
  }
  return $map
}

function Set-DotEnvKey([string]$Path, [string]$Key, [string]$Value) {
  $lines = @()
  if (Test-Path -LiteralPath $Path) {
    $lines = @(Get-Content -LiteralPath $Path)
  }
  $found = $false
  $out = foreach ($line in $lines) {
    if ($line -match ("^\s*" + [regex]::Escape($Key) + "\s*=")) {
      $found = $true
      "{0}={1}" -f $Key, $Value
    } else {
      $line
    }
  }
  if (-not $found) {
    $list = [System.Collections.Generic.List[string]]::new()
    foreach ($x in @($out)) { $list.Add([string]$x) }
    if ($list.Count -gt 0 -and $list[$list.Count - 1] -ne '') { $list.Add('') }
    $list.Add('# Google Workspace MCP (magks) - docs/google-workspace.md')
    $list.Add(("{0}={1}" -f $Key, $Value))
    $out = $list.ToArray()
  }
  [System.IO.File]::WriteAllLines($Path, @($out))
}

$envMap = Get-DotEnv (Join-Path $Root '.env')
$email = $envMap['USER_GOOGLE_EMAIL']
if (-not $email) { throw 'Set USER_GOOGLE_EMAIL in .env (e.g. you@gmail.com)' }

$gwsCredPath = Join-Path $Root 'secrets/google/credentials.json'
if (-not (Test-Path -LiteralPath $gwsCredPath)) {
  throw "Missing $gwsCredPath - export via gws auth export first (docs/google-workspace.md)"
}
$gws = Get-Content -LiteralPath $gwsCredPath -Raw | ConvertFrom-Json

$clientId = $envMap['GOOGLE_OAUTH_CLIENT_ID']
$clientSecret = $envMap['GOOGLE_OAUTH_CLIENT_SECRET']
if (-not $clientId -or -not $clientSecret) {
  $secretPath = Join-Path $Root 'secrets/google/client_secret.json'
  if (Test-Path -LiteralPath $secretPath) {
    $cs = Get-Content -LiteralPath $secretPath -Raw | ConvertFrom-Json
    $installed = $cs.installed
    if (-not $installed) { $installed = $cs.web }
    if ($installed) {
      if (-not $clientId) { $clientId = [string]$installed.client_id }
      if (-not $clientSecret) { $clientSecret = [string]$installed.client_secret }
    }
  }
}
if (-not $clientId) { $clientId = [string]$gws.client_id }
if (-not $clientSecret) { $clientSecret = [string]$gws.client_secret }
if (-not $clientId -or -not $clientSecret) {
  throw 'Need GOOGLE_OAUTH_CLIENT_ID / GOOGLE_OAUTH_CLIENT_SECRET in .env (or client_secret.json)'
}
if (-not $gws.refresh_token) { throw 'credentials.json has no refresh_token' }

$scopes = @(
  'https://www.googleapis.com/auth/gmail.modify',
  'https://www.googleapis.com/auth/drive',
  'https://www.googleapis.com/auth/calendar',
  'https://www.googleapis.com/auth/documents',
  'https://www.googleapis.com/auth/spreadsheets',
  'https://www.googleapis.com/auth/tasks',
  'https://www.googleapis.com/auth/contacts'
)

$outDir = Join-Path $Root 'secrets/google-mcp/credentials'
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$outPath = Join-Path $outDir ($email + '.json')

$payload = [ordered]@{
  token           = ''
  refresh_token   = [string]$gws.refresh_token
  token_uri       = 'https://oauth2.googleapis.com/token'
  client_id       = $clientId
  client_secret   = $clientSecret
  scopes          = $scopes
  expiry          = '2000-01-01T00:00:00Z'
}
$json = ($payload | ConvertTo-Json -Depth 5) + "`n"
[System.IO.File]::WriteAllText($outPath, $json, [System.Text.UTF8Encoding]::new($false))

$dotenvPath = Join-Path $Root '.env'
Set-DotEnvKey $dotenvPath 'GOOGLE_OAUTH_CLIENT_ID' $clientId
Set-DotEnvKey $dotenvPath 'GOOGLE_OAUTH_CLIENT_SECRET' $clientSecret
Set-DotEnvKey $dotenvPath 'USER_GOOGLE_EMAIL' $email

Write-Host "Wrote $outPath"
Write-Host "Updated .env GOOGLE_OAUTH_* + USER_GOOGLE_EMAIL (values not printed)"
Write-Host "Then: make remote-deploy  (or make up)"
