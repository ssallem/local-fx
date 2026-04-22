# DEV.md — 로컬 개발 세팅 가이드

> Chrome 확장 + Go Native Host를 로컬에서 빌드·실행·테스트하기 위한 개발자 가이드.
> 권위있는 원천: `C:\Users\mellass\.claude\plans\harmonic-chasing-narwhal.md` — "디렉터리 구조" / "구현 단계" / "검증" 섹션. 프로토콜은 [PROTOCOL.md](./PROTOCOL.md), 보안은 [SECURITY.md](./SECURITY.md) 참조.

---

## 1. 요구 사항 (Prerequisites)

| 도구 | 최소 버전 | 용도 |
|------|----------|-----|
| Node.js | 20 LTS | 확장 빌드(Vite), 테스트(Vitest) |
| npm | 10+ | (Node 20에 동봉) |
| Go | 1.22 | Native Host 빌드 |
| Chrome | 120+ | MV3 + Native Messaging 완전 지원 |
| OS | Windows 11 **또는** macOS 13 (Ventura) 이상 | Linux 미지원 |

**macOS 추가:** Xcode Command Line Tools (`xcode-select --install`) — CGO로 휴지통 API(Phase 2) 빌드 시 필요.

**Windows 추가 (Phase 4 이후):** WiX Toolset v4 — MSI 설치관리자 빌드용. Phase 0-3 개발에는 불필요.

버전 확인:
```bash
node --version   # v20.x 이상
go version       # go1.22 이상
```

---

## 2. 저장소 구조

```
D:\dev\chrome\plug-in\
├── extension/              # Chrome MV3 확장 (Vite + React + TS + Zustand)
│   ├── manifest.json
│   ├── src/
│   │   ├── background.ts       # service worker, native host bridge
│   │   ├── ui/                 # React UI (App, components, hooks, store)
│   │   ├── ipc.ts              # typed client, shared protocol types 사용
│   │   └── types/shared.ts     # 확장/호스트 공유 프로토콜 타입
│   ├── public/tab.html
│   ├── vite.config.ts
│   └── package.json
├── native-host/            # Go Native Messaging Host
│   ├── cmd/fx-host/main.go
│   ├── internal/
│   │   ├── protocol/           # length-prefixed codec, 타입, 스키마
│   │   ├── ops/                # readdir, copy, move, ...
│   │   ├── safety/             # 경로 정제, 시스템 allowlist
│   │   └── platform/           # build-tagged OS 분기
│   ├── go.mod
│   └── test/                   # testdata + fixtures
├── installer/
│   ├── windows/                # WiX v4 + PowerShell 스크립트 (Phase 0 MVP)
│   └── macos/                  # productbuild + bash 스크립트 (Phase 0 MVP)
└── docs/
    ├── PROTOCOL.md
    ├── SECURITY.md
    └── DEV.md                  # (이 파일)
```

---

## 3. 빌드 순서

아래 순서대로 최초 1회 수행. 이후 개발 루프는 [§4](#4-개발-루프)를 따른다.

### 3.1 Native Host 빌드

**Windows (PowerShell):**
```powershell
cd D:\dev\chrome\plug-in\native-host
go build -o bin\fx-host.exe .\cmd\fx-host
```

**macOS (bash):**
```bash
cd ~/dev/chrome/plug-in/native-host
go build -o bin/fx-host ./cmd/fx-host
```

산출물 경로(Phase 0 기준):
- Windows: `D:\dev\chrome\plug-in\native-host\bin\fx-host.exe`
- macOS: `~/dev/chrome/plug-in/native-host/bin/fx-host`

### 3.2 확장 빌드

```bash
cd D:\dev\chrome\plug-in\extension
npm install
npm run build
```

산출물: `extension/dist/` (Vite가 MV3 manifest와 자산을 여기에 번들링).

### 3.3 Native Messaging manifest 등록

확장이 Host를 찾아 스폰할 수 있게 OS에 manifest를 등록한다. Phase 0 스캐폴딩에 `installer/windows/install-dev.ps1` 과 `installer/macos/install-dev.sh` 가 포함된다. **확장 로드 전에 반드시 실행**한다.

**Windows:**
```powershell
cd D:\dev\chrome\plug-in\installer\windows
.\install-dev.ps1
```
이 스크립트는:
- `%LOCALAPPDATA%\LocalFx\com.local.fx.json` manifest 작성
- 레지스트리 `HKCU\Software\Google\Chrome\NativeMessagingHosts\com.local.fx` 의 기본값을 위 manifest 경로로 설정
- manifest 내 `path` 필드를 `native-host\bin\fx-host.exe` 의 절대 경로로 치환
- manifest 내 `allowed_origins` 를 개발용 확장 ID로 설정 (초기값은 placeholder, [§3.4](#34-chrome에-확장-로드)에서 확정)

**macOS:**
```bash
cd ~/dev/chrome/plug-in/installer/macos
./install-dev.sh
```
이 스크립트는 `~/Library/Application Support/Google/Chrome/NativeMessagingHosts/com.local.fx.json` 을 작성한다. 레지스트리 대신 파일 위치가 Chrome의 검색 경로다.

### 3.4 Chrome에 확장 로드

1. `chrome://extensions` 방문
2. 우상단 **개발자 모드** 토글 ON
3. **압축해제된 확장 프로그램 로드** 클릭
4. `D:\dev\chrome\plug-in\extension\dist\` 디렉터리 선택
5. 카드에 표시되는 **확장 ID**(32자 소문자) 복사
6. [§3.3](#33-native-messaging-manifest-등록) 의 manifest 파일을 열어 `allowed_origins` 값을 `"chrome-extension://<ID>/"` 로 교체하고 저장
7. 확장 카드에서 "새로고침" 버튼 클릭

확장 ID가 바뀔 때마다 6번을 반복해야 한다. 재현성을 위해 `manifest.json` 에 `key` 필드를 넣어 ID를 고정하는 방법도 있으나(Phase 1에서 검토), 초기에는 수동 업데이트로 진행한다.

---

## 4. 개발 루프

| 변경 대상 | 명령 | 반영 방식 |
|----------|-----|----------|
| 확장 TS/TSX/CSS | `npm run dev` (in `extension/`) | Vite HMR로 자동 리빌드. 서비스 워커는 `chrome://extensions` 에서 **수동 새로고침** 필요 |
| 확장 `manifest.json` | `npm run build` | 새로고침 필수 |
| Go Host 코드 | `go build -o bin/fx-host[.exe] ./cmd/fx-host` | 다음 Host 스폰부터 반영. 이미 떠있는 Host는 확장 UI를 한 번 닫았다 열거나 `chrome://extensions` 에서 확장 "새로고침" |
| Native manifest | `install-dev.{ps1,sh}` 재실행 | 즉시. 확장 페이지 재로드 |

**팁:** Host는 확장 서비스 워커가 `connectNative` 할 때마다 Chrome이 매번 프로세스를 스폰한다. 따라서 Host 재빌드 후 별도 kill 절차는 불필요 — 다음 요청이 새 바이너리로 뜬다. 단, 서비스 워커가 포트를 살려두고 있으면 기존 Host 프로세스가 유지되므로 확장 카드에서 "서비스 워커 종료" 또는 카드 새로고침.

---

## 5. 로그 확인

### 5.1 확장 (Service Worker / UI)

- `chrome://extensions` → "Local File Explorer" 카드 → **서비스 워커: 검사** 링크 → DevTools Console & Network
- UI 탭(새 탭 페이지)에서 우클릭 → **검사** → 일반 DevTools

### 5.2 Host stderr / log file

**Phase 0:** Host는 stderr 출력을 Chrome이 포착해 확장 서비스 워커의 DevTools Console에 표시한다. 별도 파일 로그는 Phase 1부터.

**Phase 1+:** 구조화된 NDJSON 로그를 아래 경로에 쓴다 ([SECURITY.md §6](./SECURITY.md#6-로그감사-audit-logging) 참조).
- Windows: `%LOCALAPPDATA%\LocalFx\host.log`
- macOS: `~/Library/Logs/LocalFx/host.log`

Windows 실시간 tailing:
```powershell
Get-Content -Wait "$env:LOCALAPPDATA\LocalFx\host.log"
```

macOS:
```bash
tail -F ~/Library/Logs/LocalFx/host.log
```

---

## 6. 테스트

### 6.1 Go (Native Host)

```bash
cd D:\dev\chrome\plug-in\native-host
go test ./...
```

커버리지 확인 (safety 패키지 100% 요구, ops 85%+ 목표):
```bash
go test -cover ./internal/...
go test -coverprofile=cover.out ./...
go tool cover -html=cover.out
```

통합 테스트는 `native-host/test/integration/` — tmp dir fixture 사용. `go test ./test/integration`.

### 6.2 확장 (Vitest, Phase 1부터)

```bash
cd D:\dev\chrome\plug-in\extension
npm test
```

Phase 0 스캐폴딩 시점에는 vitest가 아직 package에 추가되지 않았을 수 있다. Phase 1 초입에 `vitest` + `@testing-library/react` 의존성 추가가 예정되어 있다.

### 6.3 E2E (Phase 2 후반)

Playwright로 Chrome을 headed 로드하여 실제 확장을 붙인 스모크 10케이스 (탐색, 생성, 삭제, 복사, 충돌). 명령은 Phase 2 도달 시 확정.

### 6.4 보안 회귀

경로 조작 20+ 케이스 전용 Go 테스트 (`native-host/internal/safety/path_test.go`). 100% 차단 목표. `go test ./internal/safety -run TestPathReject`.

---

## 7. 수동 E2E 검증 (Phase 0 수용 기준)

계획 파일의 Phase 0 수용 기준을 그대로 재인용한다:

1. Chrome 확장 로드 후 UI에서 "ping" 버튼 클릭
2. 서비스 워커 DevTools Console에 `pong`(Response 프레임) 로그 확인
3. 요청-응답 왕복 지연 **< 50ms** (확장의 `performance.now()` 차이로 측정)
4. `install-dev.{ps1,sh}` 실행 후 Host 스폰 성공
5. `uninstall-dev.{ps1,sh}` 실행 후 레지스트리 키/manifest 파일 **완전 제거** 확인

---

## 8. FAQ / 자주 겪는 문제

### "Specified native messaging host not found" / "Host not found"

Chrome 서비스 워커 콘솔에 위 에러가 뜨면 manifest 탐색 실패.

**확인:**
- Windows: `reg query "HKCU\Software\Google\Chrome\NativeMessagingHosts\com.local.fx"` — 기본값이 manifest 파일의 절대 경로여야 함. 키 자체가 없으면 `install-dev.ps1` 재실행.
- macOS: `ls -la ~/Library/Application\ Support/Google/Chrome/NativeMessagingHosts/com.local.fx.json` — 파일 존재 여부 확인. 없으면 `install-dev.sh` 재실행.
- manifest 안의 `path` 필드가 실제 존재하는 `fx-host(.exe)` 를 가리키는지 확인. 상대 경로 금지 — Chrome은 절대 경로만 스폰한다.
- manifest `name` 필드가 `"com.local.fx"` 와 정확히 일치하는지 확인.

### "Access to the specified native messaging host is forbidden" / "Origin not allowed"

manifest의 `allowed_origins`에 현재 확장 ID가 없다.

**확인:**
- `chrome://extensions` 에서 확장 ID (32자 소문자) 확인
- manifest 파일을 열어 `allowed_origins` 배열에 `"chrome-extension://<현재-ID>/"` 가 **슬래시 포함**으로 들어 있는지 확인
- 확장 ID가 바뀌었다면 [§3.4](#34-chrome에-확장-로드) 단계 6 반복

### Host가 스폰되자마자 바로 종료됨 (1초 이내)

**원인 후보:**
- 프레임 파싱 에러 (확장이 stdin에 잘못된 프레임 전송)
- panic (Go 측 recover 미설치)
- 환경변수/작업 디렉터리 문제

**확인:**
- 서비스 워커 DevTools Console에서 Host stderr 확인 (Phase 0)
- Phase 1+ 라면 `host.log` 의 마지막 엔트리 확인
- `go build` 를 debug 모드로 다시: `go build -gcflags="all=-N -l" -o bin/fx-host(.exe) ./cmd/fx-host` 후 delve로 attach

### `npm run build` 실패: "Cannot find module '@types/chrome'"

```bash
cd extension
npm install --save-dev @types/chrome
```
Phase 0 스캐폴딩이 해당 의존성 포함을 보장한다. 만약 없으면 위 명령.

### macOS: Host가 "code signature invalid" 로 거부됨

개발 빌드는 미서명이다. Gatekeeper가 차단하면:
```bash
xattr -d com.apple.quarantine ~/dev/chrome/plug-in/native-host/bin/fx-host
```
이 명령은 개발 바이너리에만 사용하라. Phase 4에서 Developer ID 서명 + notarization이 도입된 뒤에는 불필요.

### 확장 UI는 뜨는데 아무 요청도 가지 않음

확장 ID ↔ `allowed_origins` 불일치, 또는 서비스 워커 종료 상태.

- `chrome://extensions` → "서비스 워커" 상태가 "비활성"이면 한 번 클릭해 깨우기
- UI의 첫 요청이 에러 없이 pending 상태로 남으면 서비스 워커 콘솔에서 `chrome.runtime.connectNative` 예외 여부 확인

---

## 9. 유용한 명령 모음

```bash
# 전체 클린 리빌드 (Win PowerShell)
Remove-Item -Recurse -Force D:\dev\chrome\plug-in\extension\dist, D:\dev\chrome\plug-in\native-host\bin
cd D:\dev\chrome\plug-in\native-host; go build -o bin\fx-host.exe .\cmd\fx-host
cd D:\dev\chrome\plug-in\extension; npm ci; npm run build

# 전체 클린 리빌드 (macOS bash)
rm -rf ~/dev/chrome/plug-in/extension/dist ~/dev/chrome/plug-in/native-host/bin
cd ~/dev/chrome/plug-in/native-host && go build -o bin/fx-host ./cmd/fx-host
cd ~/dev/chrome/plug-in/extension && npm ci && npm run build

# Go 포맷/린트
cd D:\dev\chrome\plug-in\native-host
gofmt -w .
go vet ./...

# 확장 lint (Phase 1부터 eslint 추가 예정)
cd D:\dev\chrome\plug-in\extension
npm run lint
```

---

## 10. Phase 0 검증 체크리스트

> Phase 0 빌드·등록·Ping 왕복을 **사용자가 수동 재현**할 때 사용하는 복붙용 체크리스트. [§7](#7-수동-e2e-검증-phase-0-수용-기준)이 수용 기준 요약이라면, 이 섹션은 "정확히 어떤 명령을 어떤 순서로 친다"의 런북이다.
> 실제 스크립트 이름은 `installer/windows/install.ps1`, `installer/macos/install.sh` 기준. 기존 §3.3/§3.4 본문이 `install-dev.*` 로 표기된 부분은 Phase 0 스캐폴딩 완료 시점의 최종 이름과 다를 수 있다 — 체크리스트가 우선.

### 10.1 전제 조건 체크

**Windows (PowerShell 5.1 또는 7+):**
```powershell
# PowerShell 버전
$PSVersionTable.PSVersion

# Go 1.22+
go version

# Node 20+, npm 10+
node --version
npm --version

# Chrome 버전 (120+) — chrome://version 에서 확인
```

**macOS (Terminal / zsh):**
```bash
# PowerShell 7 (pwsh)은 선택 — bash 버전 스크립트만 쓰면 불필요
pwsh --version   # 또는 bash --version (4+ 권장; 3.2 기본 셸 동작은 되지만 주의)

go version
node --version
npm --version
```

기대 하한선: Go `go1.22` 이상, Node `v20` 이상, Chrome `120+`.

### 10.2 빌드 (Windows)

```powershell
# Native Host 테스트 + 빌드
cd D:\dev\chrome\plug-in\native-host
go test ./...
go build -o bin\fx-host.exe .\cmd\fx-host
# 기대: bin\fx-host.exe 생성, go test 전부 통과 (codec, ping, integration 포함)

# dev 키 생성 + manifest.json에 "key" 주입 + 확장 ID 산출
cd ..\installer\shared
powershell -ExecutionPolicy Bypass -File .\generate-dev-key.ps1
# 기대: 출력에 "Extension ID: <32자 소문자>" 포함,
#       extension\manifest.json 최상위에 "key": "MIIBI..." 필드 추가됨

# 확장 빌드 (prebuild 훅이 manifest.json `key` 존재을 강제 확인)
cd ..\..\extension
npm install
npm run build
# 기대: extension\dist\manifest.json, background.js, tab.html, assets\* 생성

# Native Host 등록
cd ..\installer\windows
powershell -ExecutionPolicy Bypass -File .\install.ps1
# 기대: HKCU\Software\Google\Chrome\NativeMessagingHosts\com.local.fx 등록
#       %LOCALAPPDATA%\LocalFx\com.local.fx.json 생성 (native manifest)
#       %LOCALAPPDATA%\LocalFx\integrity.json 생성 (바이너리 해시 기록)
```

### 10.3 빌드 (macOS)

```bash
# Native Host 테스트 + 빌드
cd ~/dev/chrome/plug-in/native-host
go test ./...
# Apple Silicon
go build -o bin/fx-host-darwin-arm64 ./cmd/fx-host
# Intel
# go build -o bin/fx-host-darwin-amd64 ./cmd/fx-host

# dev 키 생성 + manifest.json "key" 주입
cd ../installer/shared
bash ./generate-dev-key.sh
# 기대: "Extension ID: <32자>" 출력, extension/manifest.json에 "key" 추가

# 확장 빌드
cd ../../extension
npm install
npm run build

# Native Host 등록
cd ../installer/macos
bash ./install.sh
# 기대: ~/Library/Application Support/Google/Chrome/NativeMessagingHosts/com.local.fx.json 생성
#       ~/Library/Application Support/LocalFx/integrity.json 생성 (경로는 설치 스크립트 기준)
```

### 10.4 확장 로드 & Ping 왕복 확인

1. Chrome 주소창에 `chrome://extensions` 입력
2. 우상단 **개발자 모드** ON
3. **압축해제된 확장 프로그램 로드** → `D:\dev\chrome\plug-in\extension\dist` (macOS: `~/dev/chrome/plug-in/extension/dist`) 선택
4. 카드에 표시된 **확장 ID**가 `generate-dev-key.{ps1,sh}` 출력의 Extension ID와 **완전히 동일**한지 확인 — 불일치 시 `allowed_origins` 가 틀렸다는 뜻이므로 §10.5 표 참조 후 재등록
5. 새 탭 열기 → "Local Explorer (dev)" 로고 + **Ping Host** 버튼 표시 확인
6. **Ping Host** 클릭 → 응답 박스에 아래 형태의 JSON 표시 확인
   ```json
   { "pong": true, "version": "0.0.1", "os": "windows" }
   ```
   macOS 는 `"os": "darwin"`.
7. 서비스 워커 로그 확인: `chrome://extensions` → 이 확장 카드의 **서비스 워커** 링크 → DevTools Console 에 에러 없음
8. 왕복 지연 관측: DevTools Console 또는 UI의 timing 로그에서 요청→응답 < **50ms**

### 10.5 자주 발생하는 문제 해결

| 증상 | 원인 | 해결 |
|------|-----|-----|
| `E_HOST_NOT_FOUND` | native manifest 미등록 또는 `path` 경로 오류 | `install.ps1` 재실행. `%LOCALAPPDATA%\LocalFx\com.local.fx.json` 의 `path` 필드가 실제 `fx-host.exe` 절대경로와 일치하는지 확인 |
| `E_HOST_CRASH` + 즉시 발생 | Host 바이너리 실행 실패 | `cd native-host && bin\fx-host.exe` 로 직접 실행. stdin EOF 로 조용히 종료되면 정상. 즉시 패닉 출력이면 빌드 문제 |
| 확장 ID 불일치 | `generate-dev-key.{ps1,sh}` 미실행 또는 `manifest.json` 의 `key` 누락 | `extension\manifest.json` 최상위에 `"key": "MIIBI..."` 존재하는지 확인. 없으면 키 생성 스크립트 재실행 후 `npm run build` |
| `npm run build` 실패 — `prebuild` 훅 | manifest `key` 필드 없음 | §10.2 의 `generate-dev-key.ps1` (macOS: `.sh`) 실행 후 재시도 |
| `go build` 실패 — `cannot find module` | go.mod 경로 오류 또는 의존성 미해결 | `cd native-host && go mod tidy` 후 재시도 |
| `Specified native messaging host not found` (서비스 워커 콘솔) | Chrome 프로필 문제 또는 레지스트리 미쓰기 | Windows: `reg query "HKCU\Software\Google\Chrome\NativeMessagingHosts\com.local.fx"` 로 기본값 경로 확인. macOS: `ls ~/Library/Application\ Support/Google/Chrome/NativeMessagingHosts/com.local.fx.json` |
| `Access to the specified native messaging host is forbidden` | `allowed_origins` 에 현재 확장 ID 없음 | native manifest 내 `allowed_origins` 가 `"chrome-extension://<ID>/"` (**끝 슬래시 포함**) 인지 확인. ID 변경 시 `install.ps1` 재실행 |

### 10.6 Uninstall (원복)

**Windows:**
```powershell
cd D:\dev\chrome\plug-in\installer\windows
powershell -ExecutionPolicy Bypass -File .\uninstall.ps1
# 기대: 레지스트리 키 제거, %LOCALAPPDATA%\LocalFx\ 하위 파일 정리
# chrome://extensions 에서 확장 카드는 수동 "제거"
```

**macOS:**
```bash
cd ~/dev/chrome/plug-in/installer/macos
bash ./uninstall.sh
# 기대: ~/Library/Application Support/Google/Chrome/NativeMessagingHosts/com.local.fx.json 제거
# chrome://extensions 에서 확장 카드 수동 제거
```

검증:
```powershell
# Windows
reg query "HKCU\Software\Google\Chrome\NativeMessagingHosts\com.local.fx"
# 결과: "지정된 레지스트리 키 또는 값을 찾을 수 없습니다." = 성공
Test-Path "$env:LOCALAPPDATA\LocalFx"
# False 가 정상
```
```bash
# macOS
ls ~/Library/Application\ Support/Google/Chrome/NativeMessagingHosts/com.local.fx.json 2>/dev/null || echo "removed"
```

### 10.7 Phase 0 수용 기준 체크박스

- [ ] `go test ./...` 통과 (codec, ping, integration 포함)
- [ ] `npm run build` → `extension/dist/` 생성 (prebuild `key` 체크 통과)
- [ ] `install.ps1` (또는 `install.sh`) 에러 없이 완료, Extension ID 출력됨
- [ ] Chrome 확장 카드의 ID 가 출력된 Extension ID 와 일치
- [ ] **Ping Host** 클릭 → `pong` 응답 수신, `os` 필드가 `"windows"` 또는 `"darwin"`
- [ ] 왕복 시간 **< 50ms** (DevTools Network 또는 Console timing 로그 기준)
- [ ] `uninstall.ps1` / `uninstall.sh` 실행 후 레지스트리 키 없음, `%LOCALAPPDATA%\LocalFx\` (또는 macOS 대응 경로) 제거됨
