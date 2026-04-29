# Signing.psm1 — shared Authenticode signing helpers for LocalFx (Tab Explorer).
#
# Used by:
#   - installer/windows/build-setup.ps1         (signs fx-host.exe + setup.exe locally during -Sign builds)
#   - installer/windows/sign-and-publish.ps1    (downloads draft release artifacts, signs, re-uploads)
#
# Why a module: PowerShell scripts cannot be `dot-sourced` reliably across
# directories on PS 5.1 when the caller's $PSScriptRoot differs. A .psm1 with
# Export-ModuleMember gives us a clean Import-Module surface usable from both
# build-setup.ps1 (same dir) and sign-and-publish.ps1 (same dir, but invoked from
# different working directories).
#
# Hardware token notes (SafeNet UCert / EV cert):
#   - The SafeNet KSP is auto-discovered by Windows when the USB token is connected.
#     We therefore DO NOT pass `/csp` to signtool unless $env:LOCALFX_SIGN_CSP is
#     explicitly set.
#   - signtool may prompt for the token PIN via SafeNet Authentication Client
#     (SAC). The prompt is a native Windows dialog, NOT a console read — running
#     under non-interactive sessions (CI hosted runners, ssh) will fail. This is
#     the architectural reason CI builds UNSIGNED artifacts and signing happens
#     on a workstation with the token plugged in.
#   - PIN caching: SAC has its own PIN cache policy (configurable in the SAC UI).
#     We do not try to manage it here.
#
# Strict-mode safety: every variable is initialized before use; arrays use @() so
# .Count is always defined.

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Timestamp servers tried in order. SHA-256 RFC 3161 endpoints only — SHA-1
# Authenticode timestamps are deprecated and rejected by modern Windows.
$script:TimestampServers = @(
    'http://timestamp.digicert.com',
    'http://timestamp.sectigo.com',
    'http://timestamp.globalsign.com/tsa/r6advanced1'
)

function Find-SignTool {
    <#
        .SYNOPSIS
            Locate signtool.exe.

        .DESCRIPTION
            Search order:
              1. $env:LOCALFX_SIGNTOOL (explicit override)
              2. C:\Program Files (x86)\Windows Kits\10\bin\<sdkver>\x64\signtool.exe
                 (newest SDK version wins)
              3. C:\Program Files (x86)\Windows Kits\10\bin\x64\signtool.exe
              4. PATH lookup via Get-Command

            Throws with an actionable message if none of the above resolve.
    #>
    [CmdletBinding()]
    param()

    if ($env:LOCALFX_SIGNTOOL -and (Test-Path -LiteralPath $env:LOCALFX_SIGNTOOL)) {
        return (Resolve-Path -LiteralPath $env:LOCALFX_SIGNTOOL).Path
    }

    $kitsRoot = Join-Path ${env:ProgramFiles(x86)} 'Windows Kits\10\bin'
    if (Test-Path -LiteralPath $kitsRoot) {
        # Versioned subfolders look like 10.0.22621.0 — sort descending so newest wins.
        $versioned = @(Get-ChildItem -LiteralPath $kitsRoot -Directory -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -match '^\d+\.\d+\.\d+\.\d+$' } |
            Sort-Object -Property @{Expression = { [version]$_.Name }} -Descending)

        foreach ($dir in $versioned) {
            $candidate = Join-Path $dir.FullName 'x64\signtool.exe'
            if (Test-Path -LiteralPath $candidate) {
                return (Resolve-Path -LiteralPath $candidate).Path
            }
        }

        # Legacy unversioned layout (older Win 10 SDKs).
        $legacy = Join-Path $kitsRoot 'x64\signtool.exe'
        if (Test-Path -LiteralPath $legacy) {
            return (Resolve-Path -LiteralPath $legacy).Path
        }
    }

    # PATH fallback. Get-Command throws on miss with a noisy error; swallow it
    # so we can deliver our own guidance.
    $cmd = Get-Command -Name signtool.exe -ErrorAction SilentlyContinue
    if ($cmd) {
        return $cmd.Source
    }

    throw "signtool.exe not found. Install Windows SDK or Visual Studio Build Tools (Windows 10/11 SDK component), or set `$env:LOCALFX_SIGNTOOL to a full path."
}

function Test-SignedAndValid {
    <#
        .SYNOPSIS
            Returns $true if $FilePath has a Valid Authenticode signature.

        .DESCRIPTION
            Uses Get-AuthenticodeSignature. A "Valid" status means: signature
            verifies against the embedded cert chain AND the cert chain is
            trusted by the local machine. Unsigned, hash-mismatch, untrusted-root,
            or expired-without-timestamp all return $false.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)] [string]$FilePath
    )

    if (-not (Test-Path -LiteralPath $FilePath)) {
        return $false
    }

    $sig = Get-AuthenticodeSignature -LiteralPath $FilePath
    if ($null -eq $sig) { return $false }
    return ($sig.Status -eq 'Valid')
}

function Sign-Binary {
    <#
        .SYNOPSIS
            Authenticode-sign a PE file using a SafeNet USB token (EV cert) or
            any cert reachable via the Windows cert stores.

        .DESCRIPTION
            Calls signtool.exe with /fd SHA256 /td SHA256 and one of:
              - /sha1 <thumbprint>   (preferred — exact cert pinning)
              - /n "<subject>"       (subject-name match — ambiguous if multiple match)
              - /a                   (auto-pick first valid cert)

            Iterates over $script:TimestampServers, retrying signtool with the
            next /tr URL on failure. Returns $true on success and throws if all
            timestamp servers fail OR signature verification fails.

            CSP/KSP: not specified by default; Windows resolves the SafeNet KSP
            automatically when the USB token is plugged in. Override via
            $env:LOCALFX_SIGN_CSP if needed.

        .PARAMETER FilePath
            Absolute path to the PE file to sign (.exe / .dll).

        .PARAMETER Subject
            Cert subject substring (passed to signtool /n). Used only if
            -Thumbprint is not supplied. Defaults to $env:LOCALFX_SIGN_SUBJECT.

        .PARAMETER Thumbprint
            SHA-1 thumbprint of the cert (40 hex chars, no spaces). Preferred over
            -Subject because it pins to one exact cert. Defaults to
            $env:LOCALFX_SIGN_THUMBPRINT.

        .PARAMETER ForceSign
            Re-sign even if the file already has a Valid signature. Without this
            switch, an already-Valid file is skipped with a warning (idempotent).

        .OUTPUTS
            [bool] $true on successful signing + verification.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)] [string]$FilePath,
        [string]$Subject = $env:LOCALFX_SIGN_SUBJECT,
        [string]$Thumbprint = $env:LOCALFX_SIGN_THUMBPRINT,
        [switch]$ForceSign
    )

    if (-not (Test-Path -LiteralPath $FilePath)) {
        throw "Sign-Binary: file not found: $FilePath"
    }
    $FilePath = (Resolve-Path -LiteralPath $FilePath).Path

    # Idempotency: skip if already validly signed and not forced.
    if ((Test-SignedAndValid -FilePath $FilePath) -and -not $ForceSign) {
        Write-Host "[warn] $FilePath is already signed and Valid - skipping (use -ForceSign to override)" -ForegroundColor Yellow
        return $true
    }

    $signtool = Find-SignTool
    Write-Host "[ok] signtool : $signtool" -ForegroundColor Green

    # Build cert-selector args: /sha1 > /n > /a.
    $certArgs = @()
    if ($Thumbprint) {
        $clean = ($Thumbprint -replace '\s', '')
        if ($clean -notmatch '^[0-9A-Fa-f]{40}$') {
            throw "Sign-Binary: -Thumbprint must be 40 hex chars (SHA-1). Got: $Thumbprint"
        }
        $certArgs = @('/sha1', $clean)
        Write-Host "[info] cert selector: /sha1 $clean" -ForegroundColor Cyan
    }
    elseif ($Subject) {
        $certArgs = @('/n', $Subject)
        Write-Host "[info] cert selector: /n `"$Subject`"" -ForegroundColor Cyan
    }
    else {
        $certArgs = @('/a')
        Write-Host "[info] cert selector: /a (auto-pick)" -ForegroundColor Cyan
    }

    # Optional CSP override (rare - SafeNet KSP is auto-discovered).
    $cspArgs = @()
    if ($env:LOCALFX_SIGN_CSP) {
        $cspArgs = @('/csp', $env:LOCALFX_SIGN_CSP)
        Write-Host "[info] csp override: $env:LOCALFX_SIGN_CSP" -ForegroundColor Cyan
    }

    # Try each timestamp server in order; first success wins.
    $signed = $false
    $lastError = $null
    foreach ($tsUrl in $script:TimestampServers) {
        Write-Host "[info] signing $FilePath (timestamp: $tsUrl)..." -ForegroundColor Cyan

        $signArgs = @('sign', '/fd', 'SHA256', '/tr', $tsUrl, '/td', 'SHA256') +
                    $cspArgs + $certArgs + @($FilePath)

        # Capture stdout+stderr for diagnostic. signtool prints a single
        # "Done Adding Additional Store" / "Successfully signed" line on success.
        $stdout = & $signtool @signArgs 2>&1
        $exit   = $LASTEXITCODE

        if ($exit -eq 0) {
            Write-Host "[ok] signed via $tsUrl" -ForegroundColor Green
            $signed = $true
            break
        }
        else {
            $lastError = "signtool exit $exit via $tsUrl :`n$($stdout -join "`n")"
            Write-Host "[warn] timestamp server $tsUrl failed (exit $exit) - trying next..." -ForegroundColor Yellow
        }
    }

    if (-not $signed) {
        throw "Sign-Binary: all timestamp servers failed. Last error:`n$lastError"
    }

    # Post-sign verification - fail loudly if /pa /v doesn't accept the result.
    Write-Host "[info] verifying signature..." -ForegroundColor Cyan
    $verifyOut = & $signtool 'verify' '/pa' '/v' $FilePath 2>&1
    $verifyExit = $LASTEXITCODE
    if ($verifyExit -ne 0) {
        throw "Sign-Binary: signtool verify failed (exit $verifyExit):`n$($verifyOut -join "`n")"
    }
    # Belt-and-suspenders: verify the "Successfully verified" banner is present.
    $verifyText = ($verifyOut -join "`n")
    if ($verifyText -notmatch 'Successfully verified') {
        throw "Sign-Binary: signtool verify exited 0 but missing 'Successfully verified' banner:`n$verifyText"
    }

    Write-Host "[ok] verified: $FilePath" -ForegroundColor Green
    return $true
}

Export-ModuleMember -Function Find-SignTool, Test-SignedAndValid, Sign-Binary
