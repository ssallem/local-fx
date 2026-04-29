# uninstall.ps1 — remove com.local.fx HKCU registration and installed manifest.
#
# Usage:
#   .\uninstall.ps1                  # remove registry + install dir (asks)
#   .\uninstall.ps1 -Yes             # no confirmation
#   .\uninstall.ps1 -RemoveDevKey    # also delete extension/dev-key/
#
# Env-var contract — $env:LocalFxKeepFiles
# ----------------------------------------
# When set to '1', this script will NOT remove $installDir itself; it only
# deletes the two JSON files it knows install.ps1 generated (com.local.fx.json
# and integrity.json). This contract exists so Inno Setup's [UninstallRun] can
# call uninstall.ps1 to clear the registry + generated files, and then let
# Inno Setup's own tracked-file cleanup remove $installDir cleanly. If the dev
# runs uninstall.ps1 directly without the env var, the legacy behavior of
# removing the whole directory (with confirmation) is preserved — except we
# now bail out instead of recursively deleting a non-empty dir, since that
# would also wipe the running script itself.

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
$manifestPath  = Join-Path $installDir ($HostName + '.json')
$integrityPath = Join-Path $installDir 'integrity.json'
$keepFiles     = ($env:LocalFxKeepFiles -eq '1')

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

# Always remove the two JSON files install.ps1 generated. These are NOT tracked
# by Inno Setup's [Files] section, so nobody else will clean them up.
foreach ($f in @($manifestPath, $integrityPath)) {
    if (Test-Path -LiteralPath $f) {
        try {
            Remove-Item -LiteralPath $f -Force
            Write-Host "[ok] removed: $f" -ForegroundColor Green
        } catch {
            Write-Host "[warn] could not remove $f : $_" -ForegroundColor Yellow
        }
    }
}

# Conditionally remove the install dir. See LocalFxKeepFiles contract at top.
if (Test-Path -LiteralPath $installDir) {
    if ($keepFiles) {
        Write-Host "[skip] LocalFxKeepFiles=1 set; leaving $installDir for Inno Setup tracked-file cleanup" -ForegroundColor DarkGray
    } else {
        # Recursive delete is dangerous: $installDir contains this very script
        # when invoked via Inno Setup's installed copy. Only delete if EMPTY.
        $remaining = @(Get-ChildItem -LiteralPath $installDir -Force -ErrorAction SilentlyContinue)
        if ($remaining.Count -eq 0) {
            try {
                Remove-Item -LiteralPath $installDir -Force
                Write-Host "[ok] removed empty dir: $installDir" -ForegroundColor Green
            } catch {
                Write-Host "[warn] could not remove $installDir : $_" -ForegroundColor Yellow
            }
        } else {
            $doDelete = $Yes
            if (-not $doDelete) {
                Write-Host "[info] $installDir still contains $($remaining.Count) item(s):" -ForegroundColor Yellow
                $remaining | ForEach-Object { Write-Host "       $($_.Name)" }
                Write-Host "[info] Recursive delete would also wipe this script. If you ran this directly" -ForegroundColor Yellow
                Write-Host "       (not via Inno Setup uninstaller), pass -Yes to force the recursive delete." -ForegroundColor Yellow
                $answer = Read-Host "recursively delete $installDir ? [y/N]"
                $doDelete = ($answer -match '^(y|Y|yes|YES)$')
            }
            if ($doDelete) {
                Remove-Item -LiteralPath $installDir -Recurse -Force
                Write-Host "[ok] removed: $installDir" -ForegroundColor Green
            } else {
                Write-Host "[skip] kept: $installDir" -ForegroundColor DarkGray
            }
        }
    }
}

if ($RemoveDevKey -and (Test-Path -LiteralPath $devKeyDir)) {
    Remove-Item -LiteralPath $devKeyDir -Recurse -Force
    Write-Host "[ok] removed dev-key: $devKeyDir" -ForegroundColor Green
    Write-Host "[note] manifest.json 'key' field is still present; re-run generate-dev-key.ps1 to regenerate." -ForegroundColor Yellow
}

Write-Host "uninstall complete." -ForegroundColor Green
