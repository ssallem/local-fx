# sign-and-publish.ps1 - local-only finisher for a CI-built draft GitHub release.
#
# WORKFLOW (tag -> draft -> sign -> publish):
#   1. CI runs .github/workflows/release.yml on a `vX.Y.Z` tag push and produces
#      an UNSIGNED installer + extension zip + fx-host.exe artifact, attached to
#      a DRAFT GitHub release.
#   2. A workstation operator runs THIS script with the SafeNet UCert USB token
#      plugged in. It downloads the draft .exe assets, signs them via the
#      Sign-Binary helper in lib\Signing.psm1, regenerates SHA256SUMS.txt, and
#      either dry-runs or re-uploads + un-drafts the release.
#
# Why local: the SafeNet KSP requires the physical USB token AND a SAC PIN
# prompt that opens a Windows GUI dialog. Hosted CI runners can't satisfy
# either, so the hybrid CI-builds + local-signs split is structural, not
# stylistic.
#
# Usage:
#   .\sign-and-publish.ps1 -Tag v0.3.0
#   .\sign-and-publish.ps1 -Tag v0.3.0 -DryRun
#   .\sign-and-publish.ps1 -Tag v0.3.0 -Thumbprint 0123456789ABCDEF...40hex
#   .\sign-and-publish.ps1 -Tag v0.3.0 -Subject "LocalFx Inc."
#
# ENV CONTRACT:
#   $env:LOCALFX_SIGN_THUMBPRINT  -> preferred cert selector (40-hex SHA-1)
#   $env:LOCALFX_SIGN_SUBJECT     -> fallback cert selector (subject substring)
#   GH_TOKEN / gh auth login      -> required, this script does NOT manage auth
#
# KNOWN LIMITATION (TODO):
#   The fx-host.exe embedded inside localfx-host-setup-v*.exe was built by CI
#   and is therefore UNSIGNED. The outer setup.exe IS signed by this script,
#   which is enough to satisfy SmartScreen for the user-facing install entry
#   point, but a security-conscious user inspecting %LOCALAPPDATA%\LocalFx
#   afterwards would see an unsigned fx-host.exe. Fixing this requires either
#   (a) running the full -Sign build locally instead of using CI artifacts, or
#   (b) signing fx-host.exe in CI - which needs the USB token on the runner,
#   which is rejected. Acceptable trade-off for v0.3.x; revisit if telemetry
#   shows it matters.

[CmdletBinding()]
param(
    [Parameter(Mandatory)] [string]$Tag,
    [string]$Subject = $env:LOCALFX_SIGN_SUBJECT,
    [string]$Thumbprint = $env:LOCALFX_SIGN_THUMBPRINT,
    [switch]$ForceSign,
    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$utf8NoBom = New-Object System.Text.UTF8Encoding $false

# NOTE: Fail throws (does not call `exit`) so any enclosing try/finally still
# runs (e.g. tempdir cleanup at the bottom of the script). The thrown message
# is encoded as "LOCALFX_FAIL:<code>:<msg>" so the outer catch can recover the
# original exit code intended by the caller.
function Fail([string]$msg, [int]$code = 1) {
    Write-Host "[error] $msg" -ForegroundColor Red
    throw [System.Exception]::new("LOCALFX_FAIL:${code}:${msg}")
}

# All side-effecting work runs inside this outer try so the `finally` block
# below is guaranteed to clean up the scratch tempdir even when Fail throws
# from any depth. The catch translates the encoded throw back into the original
# numeric exit code; only after `finally` runs do we `exit $exitCode` so the
# tempdir is never leaked into %TEMP%.
$tempDir  = $null
$exitCode = 0
try {
    # --- 0. Load shared signing helpers ----------------------------------------
    $signingModule = Join-Path (Split-Path -Parent $PSCommandPath) 'lib\Signing.psm1'
    if (-not (Test-Path -LiteralPath $signingModule)) {
        Fail "signing module not found: $signingModule" 1
    }
    Import-Module -Name $signingModule -Force -DisableNameChecking
    Write-Host "[ok] signing module loaded: $signingModule" -ForegroundColor Green

    # --- 1. Verify gh CLI is available + authenticated -------------------------
    $ghCmd = Get-Command -Name gh -ErrorAction SilentlyContinue
    if (-not $ghCmd) {
        Fail "gh CLI not found on PATH. Install from https://cli.github.com/ and run 'gh auth login'." 1
    }
    Write-Host "[ok] gh : $($ghCmd.Source)" -ForegroundColor Green

    # `gh auth status` exits non-zero when not authenticated. We rely on exit
    # code rather than parsing stdout because the human-readable text is
    # locale-dependent.
    & gh auth status *> $null
    if ($LASTEXITCODE -ne 0) {
        Fail "gh CLI is not authenticated. Run 'gh auth login' first." 1
    }
    Write-Host "[ok] gh auth : OK" -ForegroundColor Green

    # --- 2. Verify the release exists AND is draft ----------------------------
    # `gh release view --json isDraft` returns {"isDraft":true}; anything else
    # means the release is published OR doesn't exist (gh prints to stderr,
    # exit != 0).
    $releaseJson = & gh release view $Tag --json isDraft,tagName,url 2>&1
    if ($LASTEXITCODE -ne 0) {
        Fail "release $Tag not found or gh error: $releaseJson" 1
    }
    try {
        $releaseObj = $releaseJson | ConvertFrom-Json
    } catch {
        Fail "could not parse 'gh release view' output as JSON: $releaseJson" 1
    }
    if (-not $releaseObj.isDraft) {
        Fail "release $Tag is already published (not draft). Refusing to re-sign + re-upload over a published release." 1
    }
    Write-Host "[ok] release : $Tag is in DRAFT state ($($releaseObj.url))" -ForegroundColor Green

    # --- 3. Create scratch dir -------------------------------------------------
    $stamp   = Get-Date -Format 'yyyyMMddHHmmss'
    $tempDir = Join-Path $env:TEMP "localfx-sign-$Tag-$stamp"
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
    Write-Host "[ok] tempdir : $tempDir" -ForegroundColor Green

    # --- 4. Download all .exe AND .zip assets ---------------------------------
    # We need both:
    #   * .exe -> sign + re-upload
    #   * .zip -> hash so SHA256SUMS.txt continues to cover the extension zip
    #            that `release-draft` originally uploaded. Without this, the
    #            republished SUMS would silently drop the zip and `sha256sum -c`
    #            would fail for downstream consumers.
    Write-Host "[info] downloading *.exe assets from release $Tag ..." -ForegroundColor Cyan
    & gh release download $Tag --pattern '*.exe' --dir $tempDir --clobber
    if ($LASTEXITCODE -ne 0) {
        Fail "gh release download (*.exe) failed (exit $LASTEXITCODE)" $LASTEXITCODE
    }

    Write-Host "[info] downloading *.zip assets from release $Tag ..." -ForegroundColor Cyan
    & gh release download $Tag --pattern '*.zip' --dir $tempDir --clobber
    if ($LASTEXITCODE -ne 0) {
        Fail "gh release download (*.zip) failed (exit $LASTEXITCODE)" $LASTEXITCODE
    }

    $exeFiles = @(Get-ChildItem -LiteralPath $tempDir -Filter '*.exe' -File)
    if ($exeFiles.Count -eq 0) {
        Fail "no .exe assets found in release $Tag (downloaded 0 files)" 1
    }
    $zipFiles = @(Get-ChildItem -LiteralPath $tempDir -Filter '*.zip' -File)
    Write-Host "[ok] downloaded $($exeFiles.Count) .exe + $($zipFiles.Count) .zip asset(s):" -ForegroundColor Green
    foreach ($f in $exeFiles + $zipFiles) {
        Write-Host "       $($f.Name)  ($([math]::Round($f.Length / 1MB, 2)) MB)"
    }

    # --- 5. Identify the setup.exe to sign ------------------------------------
    # Pattern: localfx-host-setup-v*.exe (matches setup.iss OutputBaseFilename).
    # Note (TODO): the embedded fx-host.exe inside this setup is unsigned
    # because CI built it without the SafeNet token. See script header.
    $setupExes = @($exeFiles | Where-Object { $_.Name -like 'localfx-host-setup-v*.exe' })
    if ($setupExes.Count -eq 0) {
        Fail "no localfx-host-setup-v*.exe found in downloaded assets" 1
    }

    $signedFiles = @()
    foreach ($setup in $setupExes) {
        Write-Host "[info] signing $($setup.FullName) ..." -ForegroundColor Cyan
        try {
            Sign-Binary -FilePath $setup.FullName `
                        -Subject $Subject `
                        -Thumbprint $Thumbprint `
                        -ForceSign:$ForceSign | Out-Null
        } catch {
            Fail "Sign-Binary failed for $($setup.Name): $_" 5
        }
        $signedFiles += $setup.FullName

        # T3 dependency: extension/src/ui/components/HostMissingOnboarding.tsx
        # links to the stable `latest/download/localfx-host-setup-windows.exe`
        # URL, but the workflow uploads versioned `localfx-host-setup-v<ver>.exe`.
        # Copy the just-signed binary to the stable filename so it ships as a
        # second asset on the release. Copy POST-sign (not pre-sign) so we don't
        # re-invoke Sign-Binary, which would prompt for the SafeNet PIN twice.
        $stableCopy = Join-Path $setup.DirectoryName 'localfx-host-setup-windows.exe'
        Copy-Item -LiteralPath $setup.FullName -Destination $stableCopy -Force
        $signedFiles += $stableCopy
        Write-Host "[ok] alias  : $stableCopy (stable-name copy of signed setup)" -ForegroundColor Green
    }

    # --- 6. Verify each signed file with `signtool verify /pa /v` -------------
    # Sign-Binary already does this internally, but re-verify here so the
    # script can't lie about a corrupted post-sign state if some other process
    # touched the file between sign and now. Also covers the stable-name copy.
    $signtool = Find-SignTool
    foreach ($f in $signedFiles) {
        $verifyOut = & $signtool 'verify' '/pa' '/v' $f 2>&1
        if ($LASTEXITCODE -ne 0 -or (($verifyOut -join "`n") -notmatch 'Successfully verified')) {
            Fail "post-sign verify failed for ${f}:`n$($verifyOut -join "`n")" 6
        }
        Write-Host "[ok] verify : $f" -ForegroundColor Green
    }

    # --- 7. Regenerate SHA256SUMS.txt ----------------------------------------
    # Format mirrors the GNU coreutils `sha256sum` output (lowercase hex, two
    # spaces, basename), so curl-based verification scripts on Linux/macOS can
    # consume it directly: `sha256sum -c SHA256SUMS.txt`.
    # Hash BOTH .exe (signed -> new hash) AND .zip (unchanged -> original hash)
    # so the republished SUMS still covers every published asset.
    $hashTargets = @(Get-ChildItem -LiteralPath $tempDir -File |
        Where-Object { $_.Extension -in '.exe','.zip' } |
        Sort-Object Name)
    $sumsPath = Join-Path $tempDir 'SHA256SUMS.txt'
    $lines = foreach ($f in $hashTargets) {
        $hash = (Get-FileHash -LiteralPath $f.FullName -Algorithm SHA256).Hash.ToLower()
        "$hash  $($f.Name)"
    }
    [System.IO.File]::WriteAllText($sumsPath, ($lines -join "`n") + "`n", $utf8NoBom)
    Write-Host "[ok] wrote   : $sumsPath" -ForegroundColor Green
    foreach ($l in $lines) { Write-Host "       $l" }

    # --- 8. DryRun branch -----------------------------------------------------
    if ($DryRun) {
        Write-Host ""
        Write-Host "---- DRY RUN: signed files NOT uploaded, release NOT published ----" -ForegroundColor Yellow
        Write-Host "Signed file(s):" -ForegroundColor Cyan
        foreach ($f in $signedFiles) { Write-Host "  $f" }
        Write-Host "SHA256SUMS.txt:" -ForegroundColor Cyan
        Write-Host "  $sumsPath"
        Write-Host ""
        Write-Host "Re-run without -DryRun to upload + publish." -ForegroundColor Yellow
    } else {
        # --- 9. Upload signed assets + SHA256SUMS.txt -------------------------
        # We do NOT re-upload the .zip - its bytes are unchanged, so the existing
        # asset on the draft release is already correct. Re-uploading would just
        # waste bandwidth and risk a transient failure mid-publish.
        Write-Host "[info] uploading signed assets + SHA256SUMS.txt to $Tag ..." -ForegroundColor Cyan
        $uploadArgs = @($Tag) + $signedFiles + @($sumsPath) + @('--clobber')
        & gh release upload @uploadArgs
        if ($LASTEXITCODE -ne 0) {
            Fail "gh release upload failed (exit $LASTEXITCODE)" $LASTEXITCODE
        }
        Write-Host "[ok] uploaded $($signedFiles.Count) signed exe(s) + SHA256SUMS.txt" -ForegroundColor Green

        # --- 10. Un-draft the release ----------------------------------------
        Write-Host "[info] publishing release $Tag (removing draft flag) ..." -ForegroundColor Cyan
        & gh release edit $Tag --draft=false
        if ($LASTEXITCODE -ne 0) {
            Fail "gh release edit --draft=false failed (exit $LASTEXITCODE)" $LASTEXITCODE
        }

        # Re-fetch URL post-publish (the URL itself doesn't change, but the
        # printed confirmation should reflect the live state).
        $finalJson = & gh release view $Tag --json url,isDraft 2>&1
        $finalObj  = $finalJson | ConvertFrom-Json

        Write-Host ""
        Write-Host "---- PUBLISHED ----" -ForegroundColor Green
        Write-Host "Tag      : $Tag"
        Write-Host "Draft    : $($finalObj.isDraft)"
        Write-Host "URL      : $($finalObj.url)" -ForegroundColor Cyan
    }
} catch {
    # Decode the throw shape from Fail back into a numeric exit code. Anything
    # not matching the LOCALFX_FAIL: prefix is an unexpected runtime error and
    # gets exit code 99 so the operator can distinguish "we failed cleanly" vs
    # "PowerShell threw something we didn't anticipate".
    if ($_.Exception.Message -match '^LOCALFX_FAIL:(\d+):') {
        $exitCode = [int]$Matches[1]
    } else {
        Write-Host "[error] unexpected: $($_.Exception.Message)" -ForegroundColor Red
        $exitCode = 99
    }
} finally {
    if ($tempDir -and (Test-Path -LiteralPath $tempDir)) {
        Remove-Item -LiteralPath $tempDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host "[ok] cleaned up: $tempDir" -ForegroundColor DarkGray
    }
}

exit $exitCode
