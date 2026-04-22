# generate-dev-key.ps1
# Generates a 2048-bit RSA keypair for deterministic Chrome extension ID,
# injects the public key (base64 SPKI DER) into extension/manifest.json,
# computes the resulting Chrome extension ID, and prints it to stdout.
#
# Usage:
#   .\generate-dev-key.ps1                 # uses default paths (project root)
#   .\generate-dev-key.ps1 -Force          # overwrite existing key
#
# Output (stdout, last line): <extension-id>

[CmdletBinding()]
param(
    [string]$ProjectRoot,
    [switch]$Force
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Resolve-ProjectRoot {
    param([string]$Hint)
    if ($Hint) { return (Resolve-Path -LiteralPath $Hint).Path }
    # script sits at <root>/installer/shared/generate-dev-key.ps1
    $scriptDir = Split-Path -Parent $PSCommandPath
    return (Resolve-Path -LiteralPath (Join-Path $scriptDir '..\..')).Path
}

function Convert-HexToExtensionId {
    # Chrome extension ID: first 16 bytes of SHA-256(public_key_DER),
    # lowercase hex, then each char 0-9a-f mapped to a-p.
    param([byte[]]$Digest)
    $first16 = $Digest[0..15]
    $hex = ($first16 | ForEach-Object { $_.ToString('x2') }) -join ''
    $map = @{
        '0'='a'; '1'='b'; '2'='c'; '3'='d'; '4'='e'; '5'='f'; '6'='g'; '7'='h';
        '8'='i'; '9'='j'; 'a'='k'; 'b'='l'; 'c'='m'; 'd'='n'; 'e'='o'; 'f'='p'
    }
    $sb = New-Object System.Text.StringBuilder
    foreach ($c in $hex.ToCharArray()) {
        [void]$sb.Append($map[[string]$c])
    }
    return $sb.ToString()
}

$root = Resolve-ProjectRoot -Hint $ProjectRoot
$devKeyDir = Join-Path $root 'extension\dev-key'
$privPath  = Join-Path $devKeyDir 'key.pem'
$pubPath   = Join-Path $devKeyDir 'key.pub.der'
$manifestPath = Join-Path $root 'extension\manifest.json'

# Explicit UTF-8 without BOM. PS 5.1's default WriteAllText encoding is UTF-8
# *with* BOM via .NET defaults in some environments; a BOM in manifest.json
# or a PKCS#8 PEM confuses downstream parsers (Chrome's manifest loader,
# OpenSSL). Mirrors the pattern in installer/windows/install.ps1.
$utf8NoBom = New-Object System.Text.UTF8Encoding $false

if (-not (Test-Path -LiteralPath $manifestPath)) {
    Write-Error "extension/manifest.json not found at: $manifestPath"
    exit 1
}

if (-not (Test-Path -LiteralPath $devKeyDir)) {
    New-Item -ItemType Directory -Path $devKeyDir -Force | Out-Null
}

$keyExists = (Test-Path -LiteralPath $privPath) -and (Test-Path -LiteralPath $pubPath)
if ($keyExists -and -not $Force) {
    Write-Host "[info] existing keypair found, reusing. Use -Force to regenerate." -ForegroundColor Yellow
} else {
    Write-Host "[info] generating new RSA 2048 keypair..." -ForegroundColor Cyan
    $rsa = [System.Security.Cryptography.RSA]::Create(2048)
    try {
        # PKCS#8 private key -> PEM
        $privDer = $rsa.ExportPkcs8PrivateKey()
        $privB64 = [Convert]::ToBase64String($privDer, [Base64FormattingOptions]::InsertLineBreaks)
        $pem = "-----BEGIN PRIVATE KEY-----`n$privB64`n-----END PRIVATE KEY-----`n"
        [System.IO.File]::WriteAllText($privPath, $pem, $utf8NoBom)

        # SubjectPublicKeyInfo DER (this is what Chrome hashes)
        $pubDer = $rsa.ExportSubjectPublicKeyInfo()
        [System.IO.File]::WriteAllBytes($pubPath, $pubDer)
    } finally {
        $rsa.Dispose()
    }
}

# Re-read public key DER and compute ID
$pubDer = [System.IO.File]::ReadAllBytes($pubPath)
$pubB64 = [Convert]::ToBase64String($pubDer)

$sha = [System.Security.Cryptography.SHA256]::Create()
try {
    $digest = $sha.ComputeHash($pubDer)
} finally {
    $sha.Dispose()
}
$extId = Convert-HexToExtensionId -Digest $digest

# Inject `key` into manifest.json preserving other fields
$manifestText = [System.IO.File]::ReadAllText($manifestPath)
$manifest = $manifestText | ConvertFrom-Json
# Add or overwrite "key"
if ($manifest.PSObject.Properties.Name -contains 'key') {
    $manifest.key = $pubB64
} else {
    $manifest | Add-Member -NotePropertyName 'key' -NotePropertyValue $pubB64
}
$newText = ($manifest | ConvertTo-Json -Depth 20)
# ConvertTo-Json uses 4-space indent; normalize trailing newline.
[System.IO.File]::WriteAllText($manifestPath, $newText + "`n", $utf8NoBom)

Write-Host "[ok] public key DER : $pubPath" -ForegroundColor Green
Write-Host "[ok] private key    : $privPath" -ForegroundColor Green
Write-Host "[ok] manifest.json key field updated" -ForegroundColor Green
Write-Host "[ok] extension id   : $extId" -ForegroundColor Green

# Emit ID as last stdout line so callers can capture it.
Write-Output $extId
