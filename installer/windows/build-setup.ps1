# build-setup.ps1 — compile setup.iss into localfx-host-setup-v<ver>.exe.
#
# Locates ISCC.exe in the standard Inno Setup 6 install locations, verifies
# that fx-host.exe exists (printing the same build hint install.ps1 uses if
# missing), ensures the output directory exists, runs the compile in quiet
# mode, and then prints the resulting .exe size + SHA-256.
#
# Style mirrors install.ps1 (Set-StrictMode, $ErrorActionPreference = 'Stop',
# [ok]/[warn]/[error] colored Write-Host).
#
# CODE SIGNING (-Sign switch)
#   When -Sign is passed, the script performs TWO-STAGE Authenticode signing:
#     1. Sign native-host\bin\fx-host.exe BEFORE Inno Setup compiles, so the
#        copy embedded inside the resulting setup.exe is also signed.
#     2. Sign the output localfx-host-setup-v<ver>.exe AFTER Inno Setup, because
#        Inno Setup emits a fresh PE that must be signed independently.
#
#   Both signing operations go through Sign-Binary in lib\Signing.psm1, which:
#     - Locates signtool.exe in the Windows Kits or via PATH.
#     - Tries timestamp servers in order: DigiCert -> Sectigo -> GlobalSign.
#     - Selects the cert via $env:LOCALFX_SIGN_THUMBPRINT (preferred), or
#       $env:LOCALFX_SIGN_SUBJECT, or `signtool /a` (auto-pick) as a last resort.
#     - Verifies the signature via `signtool verify /pa /v` and aborts on miss.
#
#   SafeNet UCert / EV cert: the SafeNet KSP is auto-discovered when the USB
#   token is connected. signtool may surface a native PIN prompt via the SafeNet
#   Authentication Client (SAC) - this is a Windows dialog, not console, so
#   running this script over SSH or under headless CI will hang/fail. That is
#   the architectural reason for the local-only -Sign workflow.
#
#   -ForceSign re-signs files that are already validly signed (idempotency
#   override). Without it, an already-Valid file is skipped with a warning.

[CmdletBinding()]
param(
    [switch]$Sign,
    [switch]$ForceSign
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Fail([string]$msg, [int]$code = 1) {
    Write-Host "[error] $msg" -ForegroundColor Red
    exit $code
}

# Import shared signing helpers ONLY when -Sign is requested. Keeps the
# unsigned-build path (default behavior) free of any module load cost or
# signtool/SDK dependency.
if ($Sign) {
    $signingModule = Join-Path (Split-Path -Parent $PSCommandPath) 'lib\Signing.psm1'
    if (-not (Test-Path -LiteralPath $signingModule)) {
        Fail "signing module not found: $signingModule" 1
    }
    Import-Module -Name $signingModule -Force -DisableNameChecking
    Write-Host "[ok] signing : enabled (module $signingModule)" -ForegroundColor Green
}

$scriptDir   = Split-Path -Parent $PSCommandPath
$projectRoot = (Resolve-Path -LiteralPath (Join-Path $scriptDir '..\..')).Path
$iss         = Join-Path $scriptDir 'setup.iss'
$hostBinary  = Join-Path $projectRoot 'native-host\bin\fx-host.exe'
$outDir      = Join-Path $projectRoot 'extension\dist-prod'

# 1. Locate ISCC.exe — prefer per-user install, then common Program Files paths.
$candidates = @(
    (Join-Path $env:LOCALAPPDATA 'Programs\Inno Setup 6\ISCC.exe'),
    (Join-Path ${env:ProgramFiles(x86)} 'Inno Setup 6\ISCC.exe'),
    (Join-Path $env:ProgramFiles 'Inno Setup 6\ISCC.exe')
)
$iscc = $null
foreach ($c in $candidates) {
    if ($c -and (Test-Path -LiteralPath $c)) { $iscc = $c; break }
}
if (-not $iscc) {
    Write-Host "[error] ISCC.exe not found in any of:" -ForegroundColor Red
    foreach ($c in $candidates) { Write-Host "  $c" -ForegroundColor Yellow }
    Write-Host "Install Inno Setup 6 from https://jrsoftware.org/isdl.php" -ForegroundColor Yellow
    exit 1
}
Write-Host "[ok] ISCC : $iscc" -ForegroundColor Green

# 2. Verify .iss script exists.
if (-not (Test-Path -LiteralPath $iss)) {
    Fail "setup.iss not found: $iss" 1
}

# 3. Verify host binary exists. Use the same hint install.ps1 prints so users
#    see a consistent message regardless of which entry point they hit first.
if (-not (Test-Path -LiteralPath $hostBinary)) {
    Write-Host "[error] host binary not found: $hostBinary" -ForegroundColor Red
    Write-Host "Build it first:" -ForegroundColor Yellow
    Write-Host "  cd $projectRoot\native-host" -ForegroundColor Yellow
    Write-Host "  go build -o bin\fx-host.exe .\cmd\fx-host" -ForegroundColor Yellow
    exit 1
}
Write-Host "[ok] host : $hostBinary" -ForegroundColor Green

# 3a. Sign fx-host.exe BEFORE Inno Setup runs.
#     Inno Setup [Files] embeds a copy of this PE into the setup payload, so the
#     embedded fx-host.exe inherits whatever signature state the on-disk file has
#     at compile time. Signing post-compile would NOT propagate to the embedded
#     copy. Hence: sign-then-compile-then-sign-the-installer (two-stage).
if ($Sign) {
    Write-Host "[info] stage 1/2: signing host binary $hostBinary ..." -ForegroundColor Cyan
    try {
        Sign-Binary -FilePath $hostBinary -ForceSign:$ForceSign | Out-Null
    } catch {
        Fail "stage-1 signing of $hostBinary failed: $_" 5
    }
}

# 4. Ensure output dir exists (Inno Setup will fail if OutputDir is missing).
if (-not (Test-Path -LiteralPath $outDir)) {
    try {
        New-Item -ItemType Directory -Path $outDir -Force | Out-Null
        Write-Host "[ok] created output dir: $outDir" -ForegroundColor Green
    } catch {
        Fail "failed to create output dir $outDir : $_" 3
    }
}

# 5. Compile. /Q = quiet (suppress per-file output) but still report errors.
Write-Host "[info] compiling $iss ..." -ForegroundColor Cyan
& $iscc /Q $iss
if ($LASTEXITCODE -ne 0) {
    Fail "ISCC.exe failed with exit code $LASTEXITCODE" $LASTEXITCODE
}

# 6. Locate the built .exe (OutputBaseFilename in setup.iss).
#    Glob the dist-prod dir for localfx-host-setup-v*.exe and pick the newest.
# PS 5.1 + StrictMode: a single-result Get-ChildItem returns a scalar (no .Count),
# so always wrap with @() to get an array. .Count is then safe even for 0 / 1 hits.
$builtExes = @(Get-ChildItem -LiteralPath $outDir -Filter 'localfx-host-setup-v*.exe' -ErrorAction SilentlyContinue |
               Sort-Object LastWriteTime -Descending)
if ($builtExes.Count -eq 0) {
    Fail "compile reported success but no localfx-host-setup-v*.exe found in $outDir" 4
}
$exe = $builtExes[0].FullName

# 6a. Sign the freshly compiled installer (stage 2/2).
#     Inno Setup emits a brand-new PE with its own headers; the signature on the
#     embedded fx-host.exe does not extend to this outer .exe, so we sign it as
#     a separate Authenticode operation.
if ($Sign) {
    Write-Host "[info] stage 2/2: signing installer $exe ..." -ForegroundColor Cyan
    try {
        Sign-Binary -FilePath $exe -ForceSign:$ForceSign | Out-Null
    } catch {
        Fail "stage-2 signing of $exe failed: $_" 6
    }
}

# 7. Report size + SHA-256. When -Sign was used, the SHA-256 here will differ
#    from the unsigned build because Authenticode embeds bytes into the PE
#    security directory; that is the new content-addressed hash users will see.
$sizeBytes = (Get-Item -LiteralPath $exe).Length
$sizeMB    = [math]::Round($sizeBytes / 1MB, 2)
$sha       = (Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash.ToLower()

Write-Host ""
Write-Host "[ok] built: $exe" -ForegroundColor Green
Write-Host ("     size:   {0:N2} MB" -f $sizeMB)
Write-Host "     sha256: $sha"
if ($Sign) {
    Write-Host "     signed: yes (host + installer, two-stage)" -ForegroundColor Green
} else {
    Write-Host "     signed: no  (pass -Sign to enable Authenticode)" -ForegroundColor Yellow
}
exit 0
