# uninstall.ps1 — remove com.local.fx HKCU registration and installed manifest.
#
# Usage:
#   .\uninstall.ps1                  # remove registry + install dir (asks)
#   .\uninstall.ps1 -Yes             # no confirmation
#   .\uninstall.ps1 -RemoveDevKey    # also delete extension/dev-key/

[CmdletBinding()]
param(
    [switch]$Yes,
    [switch]$RemoveDevKey,
    [switch]$SkipEdge
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$HostName    = 'com.local.fx'
$scriptDir   = Split-Path -Parent $PSCommandPath
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $scriptDir '..\..')).Path
$installDir  = Join-Path $env:LOCALAPPDATA 'LocalFx'
$devKeyDir   = Join-Path $projectRoot 'extension\dev-key'

$regTargets = @('HKCU:\Software\Google\Chrome\NativeMessagingHosts\' + $HostName)
if (-not $SkipEdge) {
    $regTargets += 'HKCU:\Software\Microsoft\Edge\NativeMessagingHosts\' + $HostName
}

foreach ($key in $regTargets) {
    if (Test-Path -LiteralPath $key) {
        try {
            Remove-Item -LiteralPath $key -Recurse -Force
            Write-Host "[ok] removed: $key" -ForegroundColor Green
        } catch {
            Write-Host "[warn] could not remove $key : $_" -ForegroundColor Yellow
        }
    } else {
        Write-Host "[skip] not present: $key" -ForegroundColor DarkGray
    }
}

if (Test-Path -LiteralPath $installDir) {
    $doDelete = $Yes
    if (-not $doDelete) {
        $answer = Read-Host "delete $installDir ? [y/N]"
        $doDelete = ($answer -match '^(y|Y|yes|YES)$')
    }
    if ($doDelete) {
        Remove-Item -LiteralPath $installDir -Recurse -Force
        Write-Host "[ok] removed: $installDir" -ForegroundColor Green
    } else {
        Write-Host "[skip] kept: $installDir" -ForegroundColor DarkGray
    }
}

if ($RemoveDevKey -and (Test-Path -LiteralPath $devKeyDir)) {
    Remove-Item -LiteralPath $devKeyDir -Recurse -Force
    Write-Host "[ok] removed dev-key: $devKeyDir" -ForegroundColor Green
    Write-Host "[note] manifest.json 'key' field is still present; re-run generate-dev-key.ps1 to regenerate." -ForegroundColor Yellow
}

Write-Host "uninstall complete." -ForegroundColor Green
