#!/usr/bin/env bash
# generate-dev-key.sh
# Generates 2048-bit RSA keypair for deterministic Chrome extension ID,
# injects SPKI DER base64 into extension/manifest.json under "key",
# computes the Chrome extension ID and prints it as the last stdout line.
#
# Requires: openssl, python3 (stdlib json only).
#
# Usage:
#   ./generate-dev-key.sh [--project-root PATH] [--force]

set -euo pipefail

PROJECT_ROOT=""
FORCE=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --project-root) PROJECT_ROOT="$2"; shift 2 ;;
        --force)        FORCE=1; shift ;;
        -h|--help)
            sed -n '2,12p' "$0"
            exit 0
            ;;
        *) echo "[err] unknown arg: $1" >&2; exit 1 ;;
    esac
done

if [[ -z "$PROJECT_ROOT" ]]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

command -v openssl >/dev/null 2>&1 || { echo "[err] openssl not found in PATH" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "[err] python3 required for JSON patch" >&2; exit 1; }

DEV_KEY_DIR="$PROJECT_ROOT/extension/dev-key"
PRIV="$DEV_KEY_DIR/key.pem"
PUB_DER="$DEV_KEY_DIR/key.pub.der"
MANIFEST="$PROJECT_ROOT/extension/manifest.json"

[[ -f "$MANIFEST" ]] || { echo "[err] manifest not found: $MANIFEST" >&2; exit 1; }
mkdir -p "$DEV_KEY_DIR"

if [[ -f "$PRIV" && -f "$PUB_DER" && "$FORCE" -eq 0 ]]; then
    echo "[info] existing keypair found, reusing. Pass --force to regenerate." >&2
else
    echo "[info] generating RSA 2048 keypair..." >&2
    # macOS 의 LibreSSL 2.x 는 -pkeyopt 미지원일 수 있다. 실패 시 genrsa 로 fallback.
    if openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$PRIV" >/dev/null 2>&1; then
        :
    else
        echo "[info] openssl genpkey -pkeyopt 미지원, genrsa 로 fallback." >&2
        openssl genrsa -out "$PRIV" 2048 >/dev/null 2>&1
    fi
    openssl rsa -in "$PRIV" -pubout -outform DER -out "$PUB_DER" >/dev/null 2>&1
    chmod 600 "$PRIV"
fi

PUB_B64="$(openssl base64 -A -in "$PUB_DER")"

# Chrome extension ID = SHA-256(pub_der)[0:16], hex -> map 0-9a-f to a-p.
EXT_ID="$(openssl dgst -sha256 -binary "$PUB_DER" \
    | head -c 16 \
    | xxd -p -c 32 \
    | tr '0-9a-f' 'a-p' \
    | tr -d '\n')"

# Inject "key" into manifest.json preserving all other fields + ordering.
python3 - "$MANIFEST" "$PUB_B64" <<'PY'
import json, sys, collections
path, pub_b64 = sys.argv[1], sys.argv[2]
with open(path, 'r', encoding='utf-8') as f:
    data = json.load(f, object_pairs_hook=collections.OrderedDict)
data['key'] = pub_b64
with open(path, 'w', encoding='utf-8') as f:
    json.dump(data, f, indent=2)
    f.write('\n')
PY

echo "[ok] public key DER : $PUB_DER" >&2
echo "[ok] private key    : $PRIV" >&2
echo "[ok] manifest.json key field updated" >&2
echo "[ok] extension id   : $EXT_ID" >&2

# Emit ID as last stdout line for caller capture.
printf '%s\n' "$EXT_ID"
