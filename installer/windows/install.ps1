# install.ps1 — Phase 0 dev install for Windows (HKCU, no admin needed).
#
# Registers com.local.fx Native Messaging Host for Chrome (and optionally Edge)
# for the current user only. Records host binary SHA-256 for later self-check.
#
# Usage:
#   .\install.ps1                                      # auto: build key + pick default paths
#   .\install.ps1 -HostBinary C:\path\fx-host.exe
#   .\install.ps1 -ExtensionId abcdefghijklmnopabcdefghijklmnop
#   .\install.ps1 -Force                               # overwrite existing registration
#   .\install.ps1 -SkipEdge                            # Chrome only

[CmdletBinding()]
param(
    [string]$HostBinary,
    [string]$ExtensionId,
    [switch]$Force,
    [switch]$SkipEdge
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# PS 5.1 의 [System.IO.File]::WriteAllText 2인자 오버로드는 환경에 따라 UTF-8 BOM 을
# 붙일 수 있다. manifest.json 에 BOM 이 붙으면 Chrome Native Messaging 매니페스트
# 파서 / Go JSON 디코더가 실패할 수 있으므로, 공용 BOM-less UTF-8 인코더를 선언해
# 이 스크립트의 모든 텍스트 쓰기에 재사용한다. (PS 5.1 호환)
$utf8NoBom = New-Object System.Text.UTF8Encoding $false

$HostName        = 'com.local.fx'
$scriptDir       = Split-Path -Parent $PSCommandPath
$projectRoot     = (Resolve-Path -LiteralPath (Join-Path $scriptDir '..\..')).Path
$templatePath    = Join-Path $scriptDir 'com.local.fx.json.tmpl'
$keyGenScript    = Join-Path $projectRoot 'installer\shared\generate-dev-key.ps1'
$installDir      = Join-Path $env:LOCALAPPDATA 'LocalFx'
$manifestOutPath = Join-Path $installDir "$HostName.json"
$integrityPath   = Join-Path $installDir 'integrity.json'

function Fail([string]$msg, [int]$code) {
    Write-Host "[error] $msg" -ForegroundColor Red
    exit $code
}

# 1. Resolve host binary path
if (-not $HostBinary) {
    $HostBinary = Join-Path $projectRoot 'native-host\bin\fx-host.exe'
}
if (-not [System.IO.Path]::IsPathRooted($HostBinary)) {
    # PS 5.1 호환: null-conditional (?.) 금지. if 체크로 분리.
    $resolved = Resolve-Path -LiteralPath $HostBinary -ErrorAction SilentlyContinue
    if ($resolved) { $HostBinary = $resolved.Path }
}

if (-not $HostBinary -or -not (Test-Path -LiteralPath $HostBinary)) {
    Write-Host "[error] host binary not found: $HostBinary" -ForegroundColor Red
    Write-Host "Build it first:" -ForegroundColor Yellow
    Write-Host "  cd $projectRoot\native-host" -ForegroundColor Yellow
    Write-Host "  go build -o bin\fx-host.exe .\cmd\fx-host" -ForegroundColor Yellow
    exit 1
}
$HostBinary = (Resolve-Path -LiteralPath $HostBinary).Path

# 2. Resolve extension ID (generate key if missing)
if (-not $ExtensionId) {
    if (-not (Test-Path -LiteralPath $keyGenScript)) {
        Fail "key generator missing: $keyGenScript" 1
    }
    Write-Host "[info] no -ExtensionId given, running key generator..." -ForegroundColor Cyan
    $genOut = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $keyGenScript -ProjectRoot $projectRoot
    if ($LASTEXITCODE -ne 0) { Fail "generate-dev-key.ps1 failed" 1 }
    $ExtensionId = ($genOut | Where-Object { $_ -match '^[a-p]{32}$' } | Select-Object -Last 1)
    if (-not $ExtensionId) { Fail "could not parse extension ID from key generator output" 1 }
}
if ($ExtensionId -notmatch '^[a-p]{32}$') {
    Fail "invalid extension ID format (expected 32 chars a-p): $ExtensionId" 1
}

# 3. Ensure install dir
try {
    if (-not (Test-Path -LiteralPath $installDir)) {
        New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    }
} catch {
    Fail "failed to create install dir $installDir : $_" 3
}

# 4. Render manifest from template
if ((Test-Path -LiteralPath $manifestOutPath) -and -not $Force) {
    Fail "$manifestOutPath already exists. Pass -Force to overwrite." 3
}
if (-not (Test-Path -LiteralPath $templatePath)) {
    Fail "template missing: $templatePath" 3
}
$tmpl = [System.IO.File]::ReadAllText($templatePath)
# JSON-escape backslashes for the Windows path.
# PowerShell -replace의 replacement string에서 '\\' 는 리터럴 백슬래시 1개(no-op),
# '\\\\' 가 리터럴 백슬래시 2개. 이걸로 C:\Users\... 가 JSON에 C:\\Users\\... 로 들어간다.
$hostPathJson = $HostBinary -replace '\\', '\\\\'
$rendered = $tmpl `
    -replace '\{\{HOST_BINARY_PATH\}\}', $hostPathJson `
    -replace '\{\{EXTENSION_ID\}\}', $ExtensionId
try {
    # BOM-less UTF-8: Chrome Native Messaging 매니페스트 파서는 BOM 이 있으면
    # 거부할 수 있다. 3인자 오버로드로 명시적 인코딩 지정. ($utf8NoBom 은 상단 선언)
    [System.IO.File]::WriteAllText($manifestOutPath, $rendered, $utf8NoBom)
} catch {
    Fail "failed to write manifest $manifestOutPath : $_" 3
}

# 5. Register in HKCU for Chrome (and optionally Edge)
$regTargets = @('HKCU:\Software\Google\Chrome\NativeMessagingHosts\' + $HostName)
if (-not $SkipEdge) {
    $regTargets += 'HKCU:\Software\Microsoft\Edge\NativeMessagingHosts\' + $HostName
}
foreach ($key in $regTargets) {
    try {
        if (-not (Test-Path -LiteralPath $key)) {
            New-Item -Path $key -Force | Out-Null
        }
        Set-ItemProperty -LiteralPath $key -Name '(Default)' -Value $manifestOutPath
        Write-Host "[ok] registered: $key" -ForegroundColor Green
    } catch {
        Fail "registry write failed for $key : $_" 2
    }
}

# 6. Integrity record (SHA-256 of host binary + timestamp)
try {
    $sha = (Get-FileHash -LiteralPath $HostBinary -Algorithm SHA256).Hash.ToLower()
    $record = [ordered]@{
        host_name    = $HostName
        host_path    = $HostBinary
        host_sha256  = $sha
        extension_id = $ExtensionId
        manifest     = $manifestOutPath
        installed_at = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    }
    $recordJson = $record | ConvertTo-Json -Depth 5
    # BOM-less UTF-8: Go/Node JSON 파서는 BOM 이 있으면 실패 가능. $utf8NoBom 은 상단에서 공용 선언.
    [System.IO.File]::WriteAllText($integrityPath, $recordJson, $utf8NoBom)
    Write-Host "[ok] integrity record: $integrityPath" -ForegroundColor Green
} catch {
    Fail "failed to write integrity record: $_" 3
}

Write-Host ""
Write-Host "---- install complete ----" -ForegroundColor Green
Write-Host "host binary  : $HostBinary"
Write-Host "manifest     : $manifestOutPath"
Write-Host "extension id : $ExtensionId"
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Cyan
Write-Host "  1. Build the extension:  cd $projectRoot\extension ; npm install ; npm run build"
Write-Host "  2. Load unpacked:        chrome://extensions -> Developer mode -> Load unpacked -> $projectRoot\extension\dist"
Write-Host "  3. Open a new tab and click 'Ping Host' to verify the connection."
