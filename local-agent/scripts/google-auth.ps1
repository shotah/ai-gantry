# One-shot Google Workspace OAuth via a throwaway Python container.
# Writes secrets/google-mcp/credentials/<email>.json (MCP format).
# No local gws / Python required — only Docker + .env.
# Usage (repo root):  make google-auth

param(
  [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path,
  [int]$Port = 4100
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

$envMap = Get-DotEnv (Join-Path $Root '.env')
$email = $envMap['USER_GOOGLE_EMAIL']
$clientId = $envMap['GOOGLE_OAUTH_CLIENT_ID']
$clientSecret = $envMap['GOOGLE_OAUTH_CLIENT_SECRET']
if (-not $email) { throw 'Set USER_GOOGLE_EMAIL in .env' }
if (-not $clientId -or -not $clientSecret) {
  throw 'Set GOOGLE_OAUTH_CLIENT_ID and GOOGLE_OAUTH_CLIENT_SECRET in .env'
}

$credDir = Join-Path $Root 'secrets/google-mcp/credentials'
New-Item -ItemType Directory -Force -Path $credDir | Out-Null
$stale = Join-Path $credDir ($email + '.json')
if (Test-Path -LiteralPath $stale) {
  Remove-Item -Force -LiteralPath $stale
  Write-Host "Cleared stale credential: $stale"
}

$script = Join-Path $Root 'scripts/google-auth-inner.py'
if (-not (Test-Path -LiteralPath $script)) {
  throw "Missing $script"
}

Write-Host "Google OAuth via container (python:3.12-slim). No local gws."
Write-Host "Ensure the Desktop OAuth client allows http://localhost:$Port/oauth2callback"
Write-Host ""

$credMount = "${credDir}:/data/credentials"
$scriptMount = "${script}:/auth.py:ro"

docker run --rm -it `
  -p "${Port}:${Port}" `
  -e "GOOGLE_OAUTH_CLIENT_ID=$clientId" `
  -e "GOOGLE_OAUTH_CLIENT_SECRET=$clientSecret" `
  -e "USER_GOOGLE_EMAIL=$email" `
  -e "GOOGLE_AUTH_PORT=$Port" `
  -e 'WORKSPACE_MCP_CREDENTIALS_DIR=/data/credentials' `
  -v $credMount `
  -v $scriptMount `
  python:3.12-slim-bookworm `
  python /auth.py

if ($LASTEXITCODE -ne 0) {
  throw "google-auth failed (docker exit $LASTEXITCODE)"
}

Write-Host ""
Write-Host "Next: make remote-deploy   (or make up)"
