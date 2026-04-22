#!/usr/bin/env bash
# install.sh — Phase 0 dev install for macOS (current user, Chrome stable only).
#
# Usage:
#   ./install.sh
#   ./install.sh --host-binary /abs/path/fx-host-darwin-arm64
#   ./install.sh --extension-id abcdefghijklmnopabcdefghijklmnop
#   ./install.sh --force
#   ./install.sh --skip-edge

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
        -h|--help)      sed -n '2,12p' "$0"; exit 0 ;;
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

# 2. Extension ID
if [[ -z "$EXTENSION_ID" ]]; then
    [[ -x "$KEYGEN" ]] || chmod +x "$KEYGEN"
    echo "[info] no --extension-id given, running key generator..." >&2
    EXTENSION_ID="$("$KEYGEN" --project-root "$PROJECT_ROOT" | tail -n1)"
fi
[[ "$EXTENSION_ID" =~ ^[a-p]{32}$ ]] || fail "invalid extension id: $EXTENSION_ID" 1

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
# Path has no backslashes on macOS; still sed-safe with | delimiter.
rendered="$(sed -e "s|{{HOST_BINARY_PATH}}|$HOST_BINARY|g" \
                -e "s|{{EXTENSION_ID}}|$EXTENSION_ID|g" "$TEMPLATE")"

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
cat > "$INTEGRITY" <<EOF
{
  "host_name": "$HOST_NAME",
  "host_path": "$HOST_BINARY",
  "host_sha256": "$HOST_SHA",
  "extension_id": "$EXTENSION_ID",
  "manifest": "$MANIFEST_CHROME",
  "installed_at": "$NOW"
}
EOF
echo "[ok] integrity record: $INTEGRITY" >&2

cat >&2 <<MSG

---- install complete ----
host binary  : $HOST_BINARY
manifest     : $MANIFEST_CHROME
extension id : $EXTENSION_ID

Next steps:
  1. Build the extension:  cd "$PROJECT_ROOT/extension" && npm install && npm run build
  2. Load unpacked:        chrome://extensions -> Developer mode -> Load unpacked -> $PROJECT_ROOT/extension/dist
  3. Open a new tab and click "Ping Host" to verify.
MSG
