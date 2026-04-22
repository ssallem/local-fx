#!/usr/bin/env bash
# uninstall.sh — remove com.local.fx Native Messaging registration on macOS.
#
# Usage:
#   ./uninstall.sh
#   ./uninstall.sh --yes
#   ./uninstall.sh --remove-dev-key
#   ./uninstall.sh --skip-edge

set -euo pipefail

HOST_NAME="com.local.fx"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

YES=0
REMOVE_DEV_KEY=0
SKIP_EDGE=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --yes)            YES=1; shift ;;
        --remove-dev-key) REMOVE_DEV_KEY=1; shift ;;
        --skip-edge)      SKIP_EDGE=1; shift ;;
        -h|--help)        sed -n '2,10p' "$0"; exit 0 ;;
        *) echo "[err] unknown arg: $1" >&2; exit 1 ;;
    esac
done

NM_CHROME="$HOME/Library/Application Support/Google/Chrome/NativeMessagingHosts/${HOST_NAME}.json"
NM_EDGE="$HOME/Library/Application Support/Microsoft Edge/NativeMessagingHosts/${HOST_NAME}.json"
INSTALL_DIR="$HOME/Library/Application Support/LocalFx"
DEV_KEY_DIR="$PROJECT_ROOT/extension/dev-key"

remove_file() {
    local f="$1"
    if [[ -f "$f" ]]; then
        rm -f "$f"
        echo "[ok] removed: $f" >&2
    else
        echo "[skip] not present: $f" >&2
    fi
}

remove_file "$NM_CHROME"
[[ "$SKIP_EDGE" -eq 0 ]] && remove_file "$NM_EDGE"

if [[ -d "$INSTALL_DIR" ]]; then
    if [[ "$YES" -eq 1 ]]; then
        rm -rf "$INSTALL_DIR"
        echo "[ok] removed: $INSTALL_DIR" >&2
    else
        read -r -p "delete $INSTALL_DIR ? [y/N] " ans
        if [[ "$ans" =~ ^[yY]([eE][sS])?$ ]]; then
            rm -rf "$INSTALL_DIR"
            echo "[ok] removed: $INSTALL_DIR" >&2
        else
            echo "[skip] kept: $INSTALL_DIR" >&2
        fi
    fi
fi

if [[ "$REMOVE_DEV_KEY" -eq 1 && -d "$DEV_KEY_DIR" ]]; then
    rm -rf "$DEV_KEY_DIR"
    echo "[ok] removed dev-key: $DEV_KEY_DIR" >&2
    echo "[note] manifest.json 'key' field is still present; re-run generate-dev-key.sh to regenerate." >&2
fi

echo "uninstall complete." >&2
