#!/usr/bin/env bash
# install.sh — Phase 0 dev install for macOS (current user, Chrome stable only).
#
# Usage:
#   ./install.sh
#   ./install.sh --host-binary /abs/path/fx-host-darwin-arm64
#   ./install.sh --extension-id abcdefghijklmnopabcdefghijklmnop
#   ./install.sh --extension-id "devid32chars...,prodid32chars..."  # CSV: register multiple IDs
#   ./install.sh --force
#   ./install.sh --skip-edge
#
# --extension-id accepts a comma-separated list of 32-char Chrome extension IDs.
# Each ID is validated against ^[a-p]{32}$ and emitted as a separate entry in
# the manifest's allowed_origins array. A single ID (no comma) keeps the legacy
# single-entry behavior unchanged.

set -euo pipefail

HOST_NAME="com.local.fx"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TEMPLATE="$SCRIPT_DIR/${HOST_NAME}.json.tmpl"
KEYGEN="$PROJECT_ROOT/installer/shared/generate-dev-key.sh"

HOST_BINARY=""
EXTENSION_ID=""
FORCE=0
SKIP_EDGE=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --host-binary)  HOST_BINARY="$2"; shift 2 ;;
        --extension-id) EXTENSION_ID="$2"; shift 2 ;;
        --force)        FORCE=1; shift ;;
        --skip-edge)    SKIP_EDGE=1; shift ;;
        -h|--help)      sed -n '2,16p' "$0"; exit 0 ;;
        *) echo "[err] unknown arg: $1" >&2; exit 1 ;;
    esac
done

fail() { echo "[error] $1" >&2; exit "${2:-1}"; }

# 1. Resolve host binary (auto-pick arch)
if [[ -z "$HOST_BINARY" ]]; then
    arch="$(uname -m)"
    case "$arch" in
        arm64)           HOST_BINARY="$PROJECT_ROOT/native-host/bin/fx-host-darwin-arm64" ;;
        x86_64|amd64)    HOST_BINARY="$PROJECT_ROOT/native-host/bin/fx-host-darwin-amd64" ;;
        *) fail "unsupported arch: $arch" 1 ;;
    esac
fi
if [[ ! -f "$HOST_BINARY" ]]; then
    echo "[error] host binary not found: $HOST_BINARY" >&2
    echo "Build it first:" >&2
    echo "  cd \"$PROJECT_ROOT/native-host\"" >&2
    echo "  GOOS=darwin GOARCH=$(uname -m | sed 's/x86_64/amd64/') \\" >&2
    echo "    go build -o \"bin/fx-host-darwin-$(uname -m | sed 's/x86_64/amd64/')\" ./cmd/fx-host" >&2
    exit 1
fi
[[ -x "$HOST_BINARY" ]] || chmod +x "$HOST_BINARY"
HOST_BINARY="$(cd "$(dirname "$HOST_BINARY")" && pwd)/$(basename "$HOST_BINARY")"

# 2. Extension ID(s)
# --extension-id accepts a single 32-char ID or a comma-separated list ("id1,id2").
# When omitted, generate-dev-key.sh emits a single dev ID (legacy behavior).
if [[ -z "$EXTENSION_ID" ]]; then
    [[ -x "$KEYGEN" ]] || chmod +x "$KEYGEN"
    echo "[info] no --extension-id given, running key generator..." >&2
    EXTENSION_ID="$("$KEYGEN" --project-root "$PROJECT_ROOT" | tail -n1)"
fi

# Split CSV, trim, validate each. Single ID (no comma) yields a 1-element array.
EXT_IDS=()
IFS=',' read -ra _EXT_IDS_RAW <<< "$EXTENSION_ID"
for _raw in "${_EXT_IDS_RAW[@]}"; do
    # Trim leading/trailing whitespace
    _id="${_raw#"${_raw%%[![:space:]]*}"}"
    _id="${_id%"${_id##*[![:space:]]}"}"
    [[ -z "$_id" ]] && continue
    [[ "$_id" =~ ^[a-p]{32}$ ]] || fail "invalid extension id: $_id" 1
    EXT_IDS+=("$_id")
done
[[ "${#EXT_IDS[@]}" -gt 0 ]] || fail "no valid extension IDs found in --extension-id value" 1

# 3. Paths (Chrome stable on macOS)
NM_DIR_CHROME="$HOME/Library/Application Support/Google/Chrome/NativeMessagingHosts"
NM_DIR_EDGE="$HOME/Library/Application Support/Microsoft Edge/NativeMessagingHosts"
INSTALL_DIR="$HOME/Library/Application Support/LocalFx"
INTEGRITY="$INSTALL_DIR/integrity.json"

mkdir -p "$NM_DIR_CHROME" "$INSTALL_DIR"
[[ "$SKIP_EDGE" -eq 0 ]] && mkdir -p "$NM_DIR_EDGE"

MANIFEST_CHROME="$NM_DIR_CHROME/${HOST_NAME}.json"
MANIFEST_EDGE="$NM_DIR_EDGE/${HOST_NAME}.json"

if [[ -f "$MANIFEST_CHROME" && "$FORCE" -eq 0 ]]; then
    fail "manifest already exists: $MANIFEST_CHROME (use --force)" 3
fi

# 4. Render template
[[ -f "$TEMPLATE" ]] || fail "template missing: $TEMPLATE" 3

# Build the {{ALLOWED_ORIGINS}} block: one JSON-quoted "chrome-extension://<id>/"
# per ID, separated by ',' + newline + 4 spaces (matches the indent already
# present in the template at the placeholder location, so the rendered manifest
# is uniformly 4-space-indented inside the allowed_origins array).
ALLOWED_ORIGINS=""
for _i in "${!EXT_IDS[@]}"; do
    if [[ $_i -gt 0 ]]; then
        ALLOWED_ORIGINS+=$',\n    '
    fi
    ALLOWED_ORIGINS+="\"chrome-extension://${EXT_IDS[$_i]}/\""
done

# Render template via bash string substitution (avoids sed newline-escaping
# pitfalls on BSD sed since ALLOWED_ORIGINS may contain literal newlines).
# command-sub strips trailing newlines — same behavior as the previous sed
# pipeline, so the manifest output keeps no trailing newline (matches printf '%s').
tmpl="$(cat "$TEMPLATE")"
rendered="${tmpl//'{{HOST_BINARY_PATH}}'/$HOST_BINARY}"
rendered="${rendered//'{{ALLOWED_ORIGINS}}'/$ALLOWED_ORIGINS}"

printf '%s' "$rendered" > "$MANIFEST_CHROME"
echo "[ok] wrote $MANIFEST_CHROME" >&2

if [[ "$SKIP_EDGE" -eq 0 ]]; then
    printf '%s' "$rendered" > "$MANIFEST_EDGE"
    echo "[ok] wrote $MANIFEST_EDGE" >&2
fi

# 5. Integrity record
if command -v shasum >/dev/null 2>&1; then
    HOST_SHA="$(shasum -a 256 "$HOST_BINARY" | awk '{print $1}')"
else
    HOST_SHA="$(openssl dgst -sha256 "$HOST_BINARY" | awk '{print $NF}')"
fi
NOW="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"

# Build inline JSON for extension_id (backward-compat contract for Phase 4
# self-verification):
#   - single ID install  -> JSON string scalar  ("abcd...")  — matches pre-patch shape
#   - multi-ID  install  -> JSON array of strings (["abcd...", "efgh..."])
# Future readers must accept both via typeof / type-switch.
if [[ "${#EXT_IDS[@]}" -eq 1 ]]; then
    EXT_IDS_JSON="\"${EXT_IDS[0]}\""
else
    EXT_IDS_JSON=""
    for _i in "${!EXT_IDS[@]}"; do
        if [[ $_i -gt 0 ]]; then
            EXT_IDS_JSON+=", "
        fi
        EXT_IDS_JSON+="\"${EXT_IDS[$_i]}\""
    done
    EXT_IDS_JSON="[${EXT_IDS_JSON}]"
fi

cat > "$INTEGRITY" <<EOF
{
  "host_name": "$HOST_NAME",
  "host_path": "$HOST_BINARY",
  "host_sha256": "$HOST_SHA",
  "extension_id": $EXT_IDS_JSON,
  "manifest": "$MANIFEST_CHROME",
  "installed_at": "$NOW"
}
EOF
echo "[ok] integrity record: $INTEGRITY" >&2

# Pretty-print extension IDs for the summary (one per line if multiple).
if [[ "${#EXT_IDS[@]}" -eq 1 ]]; then
    EXT_IDS_SUMMARY="${EXT_IDS[0]}"
else
    EXT_IDS_SUMMARY="$(printf '\n  - %s' "${EXT_IDS[@]}")"
fi

cat >&2 <<MSG

---- install complete ----
host binary  : $HOST_BINARY
manifest     : $MANIFEST_CHROME
extension ids: $EXT_IDS_SUMMARY

Next steps:
  1. Build the extension:  cd "$PROJECT_ROOT/extension" && npm install && npm run build
  2. Load unpacked:        chrome://extensions -> Developer mode -> Load unpacked -> $PROJECT_ROOT/extension/dist
  3. Open a new tab and click "Ping Host" to verify.
MSG
