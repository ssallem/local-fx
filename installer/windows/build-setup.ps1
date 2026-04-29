# build-setup.ps1 — compile setup.iss into localfx-host-setup-v<ver>.exe.
#
# Locates ISCC.exe in the standard Inno Setup 6 install locations, verifies
# that fx-host.exe exists (printing the same build hint install.ps1 uses if
# missing), ensures the output directory exists, runs the compile in quiet
# mode, and then prints the resulting .exe size + SHA-256.
#
# Style mirrors install.ps1 (Set-StrictMode, $ErrorActionPreference = 'Stop',
# [ok]/[warn]/[error] colored Write-Host).

[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Fail([string]$msg, [int]$code = 1) {
    Write-Host "[error] $msg" -ForegroundColor Red
    exit $code
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

# 7. Report size + SHA-256.
$sizeBytes = (Get-Item -LiteralPath $exe).Length
$sizeMB    = [math]::Round($sizeBytes / 1MB, 2)
$sha       = (Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash.ToLower()

Write-Host ""
Write-Host "[ok] built: $exe" -ForegroundColor Green
Write-Host ("     size:   {0:N2} MB" -f $sizeMB)
Write-Host "     sha256: $sha"
exit 0
