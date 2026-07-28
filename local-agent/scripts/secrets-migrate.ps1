# Copy legacy secrets/<tool> trees into data/.config/<tool> (Docker + native layout).
# Idempotent: does not overwrite existing destination files.
# Usage: make secrets-migrate

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$pairs = @(
  @{ Src = 'secrets/google-mcp'; Dst = 'data/.config/google-mcp' },
  @{ Src = 'secrets/strava'; Dst = 'data/.config/strava' },
  @{ Src = 'secrets/garmin'; Dst = 'data/.config/garmin' },
  @{ Src = 'secrets/ytmusic'; Dst = 'data/.config/ytmusic' }
)

$copied = 0
foreach ($p in $pairs) {
  $src = Join-Path $Root $p.Src
  $dst = Join-Path $Root $p.Dst
  if (-not (Test-Path -LiteralPath $src)) {
    Write-Host "skip $($p.Src) (missing)"
    continue
  }
  New-Item -ItemType Directory -Force -Path $dst | Out-Null
  Get-ChildItem -LiteralPath $src -Recurse -File | ForEach-Object {
    $rel = $_.FullName.Substring($src.Length).TrimStart('\', '/')
    if ($rel -match '(^|[\\/])\.gitkeep$' -or $rel -match '(^|[\\/])\.gitignore$') { return }
    $target = Join-Path $dst $rel
    $parent = Split-Path -Parent $target
    if (-not (Test-Path -LiteralPath $parent)) {
      New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    if (Test-Path -LiteralPath $target) {
      Write-Host "keep existing $(($p.Dst + '/' + $rel) -replace '\\','/')"
      return
    }
    Copy-Item -LiteralPath $_.FullName -Destination $target -Force
    Write-Host "copied $($p.Src)/$rel -> $($p.Dst)/$rel"
    $copied++
  }
}

Write-Host ""
Write-Host "Migrated $copied file(s) into data/.config/."
Write-Host "Docker and native both use that tree. Push with: make secrets-sync"
Write-Host "Legacy secrets/* can stay as backup until you are happy, then delete."
