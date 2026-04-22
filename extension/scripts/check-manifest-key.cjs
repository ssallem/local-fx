#!/usr/bin/env node
// check-manifest-key.js
// prebuild 훅. extension/manifest.json 에 "key" 필드가 있는지 확인.
// 없으면 Chrome 이 임시 ID 를 할당하게 되어 Native Messaging allowed_origins 불일치로
// Host 연결이 차단된다. 빌드 전에 명시적으로 차단하는 게 안전.
//
// 건너뛰기: SKIP_MANIFEST_KEY_CHECK=1 (CI 등에서 유용)

'use strict';

const fs = require('fs');
const path = require('path');

if (process.env.SKIP_MANIFEST_KEY_CHECK === '1') {
    process.stderr.write('[check-manifest-key] SKIP_MANIFEST_KEY_CHECK=1, skipping.\n');
    process.exit(0);
}

const manifestPath = path.resolve(__dirname, '..', 'manifest.json');

if (!fs.existsSync(manifestPath)) {
    process.stderr.write(
        `ERROR: extension/manifest.json 파일이 없습니다: ${manifestPath}\n`
    );
    process.exit(1);
}

let manifest;
try {
    const raw = fs.readFileSync(manifestPath, 'utf8');
    manifest = JSON.parse(raw);
} catch (err) {
    process.stderr.write(
        `ERROR: extension/manifest.json 파싱 실패: ${err.message}\n`
    );
    process.exit(1);
}

if (!manifest || typeof manifest.key !== 'string' || manifest.key.length === 0) {
    process.stderr.write(
        [
            '',
            "ERROR: extension/manifest.json 에 'key' 필드가 없습니다.",
            "먼저 'installer/shared/generate-dev-key.ps1' (Windows) 또는",
            "     'installer/shared/generate-dev-key.sh' (macOS/Linux) 를 실행하세요.",
            '',
            '이 훅을 건너뛰려면 환경변수 SKIP_MANIFEST_KEY_CHECK=1 을 설정하세요.',
            '',
        ].join('\n')
    );
    process.exit(1);
}

process.stderr.write('[check-manifest-key] ok: manifest.json has "key" field.\n');
process.exit(0);
