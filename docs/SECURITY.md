# SECURITY.md — 위협 모델 및 완화

> 로컬 파일 탐색기 Chrome 확장 + Go Native Host의 보안 경계, 공격 표면, 완화 전략.
> 권위있는 원천: `C:\Users\mellass\.claude\plans\harmonic-chasing-narwhal.md` — "보안 & 위협 모델" / "에러 & 복구 맵" 섹션.

본 문서는 위협 모델의 **권위있는 기준**이다. 변경 시 이 문서를 먼저 수정한 뒤 구현을 맞춘다.

---

## 1. 신뢰 경계 (Trust Boundaries)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                                                                          │
│  [웹페이지]           ← 신뢰 없음 (외부 입력 전부)                       │
│       │                                                                  │
│       │ postMessage / fetch 차단                                         │
│       ▼                                                                  │
│  ┌───────────────────────────────────────────────────────────┐           │
│  │ Chrome 확장 컨텍스트 (Extension Context)                   │          │
│  │   - Service Worker (background.ts)                         │          │
│  │   - UI (React, new-tab)                                    │  ◀── 경계 A
│  │   - 신뢰 수준: 사용자가 확장을 설치했으므로 "준신뢰"        │          │
│  └────────────────────────┬──────────────────────────────────┘           │
│                           │ chrome.runtime.connectNative                 │
│                           │   (length-prefixed JSON, stdio)              │
│                           ▼                                              │
│  ┌───────────────────────────────────────────────────────────┐           │
│  │ Native Messaging 전송 계층 (Chrome 관리)                   │  ◀── 경계 B
│  │   - allowed_origins 검증 (manifest)                        │          │
│  │   - 1MB 프레임 제한 강제                                   │          │
│  └────────────────────────┬──────────────────────────────────┘           │
│                           │ stdin / stdout                               │
│                           ▼                                              │
│  ┌───────────────────────────────────────────────────────────┐           │
│  │ Host 프로세스 (Go, fx-host)                                │  ◀── 경계 C
│  │   - 입력 검증 (JSON schema, path cleanse)                  │          │
│  │   - 시스템 allowlist 게이트                                │          │
│  │   - 실행: internal/ops/* ← 신뢰된 코드                      │          │
│  └────────────────────────┬──────────────────────────────────┘           │
│                           │ syscalls                                     │
│                           ▼                                              │
│  ┌───────────────────────────────────────────────────────────┐           │
│  │ OS 파일시스템 (신뢰됨 — 보호의 마지막 선)                   │          │
│  └───────────────────────────────────────────────────────────┘           │
└─────────────────────────────────────────────────────────────────────────┘
```

**경계별 불변조건(invariant):**
- **경계 A**: 웹페이지는 확장 컨텍스트에 임의 메시지를 보낼 수 없다. `externally_connectable` 필드 생략으로 차단.
- **경계 B**: Chrome이 `allowed_origins`를 강제. Host manifest에 **본 확장 ID 단 1개만** 등록.
- **경계 C**: Host는 stdin으로 들어오는 모든 프레임을 untrusted로 간주하고 [§5](#5-경로-정제-규칙) 파이프라인으로 검증.

---

## 2. 공격 표면 (Attack Surface)

### 2.1 악성 웹페이지의 확장 API 가로채기

웹페이지 JS가 확장의 Native Host를 간접 호출하려 시도할 수 있다. 확장 컨텍스트가 설치되어 있으면 웹페이지는 확장 ID만 알면 `chrome.runtime.sendMessage`(externally_connectable)로 말을 걸 수 있다. 본 확장은 `externally_connectable` 필드를 **완전히 생략**하여 외부 웹페이지의 메시지를 전면 차단한다. content script도 두지 않아 페이지 DOM과의 교차점을 제거한다. 확장 UI는 `chrome.tabs.create({ url: "tab.html" })`로 열리는 새 탭 전용 페이지이며, CSP는 `script-src 'self'`로 inline/remote 스크립트를 금지한다.

### 2.2 경로 조작 (Path Traversal)

공격자(악성 UI 코드 또는 XSS)가 `C:\Windows\..\..\etc\passwd` 같은 트래버설 경로로 시스템 영역을 읽거나 쓰려 할 수 있다. Windows에서 `\\.\PhysicalDrive0` 같은 디바이스 경로, UNC 경로(`\\server\share`), short name(`PROGRA~1`), ADS(Alternate Data Stream `foo.txt:secret`)도 벡터가 된다. macOS에서는 `/private/etc` 심볼릭, `.DS_Store` 숨김 조작, 케이스 무감각 FS로 인한 case-collision 공격이 가능하다. Host는 [§5](#5-경로-정제-규칙) 파이프라인으로 모두 차단한다.

### 2.3 심볼릭/정션 링크 트래버설

읽기 중 심볼릭 링크로 시스템 경로 밖으로 탈출하거나, 링크 순환으로 무한 루프에 빠뜨리는 DoS가 가능하다. `readdir`는 심볼릭 링크 자체는 노출하되 **타겟을 따라가지 않는다** (`type: "symlink"` 반환). `copy`/`move` 재귀 진입 시 `filepath.EvalSymlinks`로 해상한 뒤 허용 루트 소속을 재검증하며, 부모 inode 집합을 추적해 순환 시 `EIO` + `details.cycle: true`로 중단한다. Windows junction(재파싱 포인트)도 같은 규칙을 적용한다.

### 2.4 Native Host 바이너리 치환 (공급망)

로컬 머신에 침투한 악성코드가 Host 바이너리 경로를 자신의 것으로 바꿔치기 할 수 있다(Host manifest `path`의 실행 파일이 다른 것으로 교체됨). Phase 0-3 기간에는 미서명 바이너리이므로 탐지 불가 — 계획 파일의 **Phase 4에서 Authenticode(Win) / Developer ID(Mac) 서명** 및 macOS notarization으로 방어선을 확보한다. 그 전까지는 위협 모델의 "Out of scope" (§6)로 명시한다. 단기 완화로는 installer가 설치 직후 SHA-256 해시를 `%LOCALAPPDATA%\LocalFx\host.sha256`에 기록하고, Host 기동 시 자신의 해시를 비교해 불일치 시 stderr 경고 로그를 남긴다(거부는 하지 않음 — 자기 검증의 한계 고지).

---

## 3. 완화 매트릭스

| 위협 | 완화 | 경계 | Phase |
|------|------|-----|-------|
| 웹페이지의 무단 호출 | `externally_connectable` 생략 + content script 없음 + CSP `script-src 'self'` | A | 0 |
| 타 확장의 호출 | Native manifest `allowed_origins`에 본 확장 ID **1개만** 등록 | B | 0 |
| 경로 트래버설 | Host 입력마다 `filepath.Clean` + 심볼릭 해상 + 허용 루트 검증 | C | 1 |
| 시스템 경로 실수/악의 | 시스템 allowlist 쓰기/삭제 시 `explicitConfirm: true` 플래그 + UI 2단계 confirm | C | 2 |
| 심볼릭 순환 DoS | 재귀 op에서 inode 집합 추적 | C | 1 (readdir), 2 (copy/move) |
| 대형 입력 DoS | `readdir` 기본 페이징 1000, `search` 기본 depth 10, copy/move 동시 1건 | C | 1/2/3 |
| 악성 파일명 (제어문자, RTL override) | UI 렌더 시 제어문자 이스케이프, U+202E 등 차단 | A | 1 |
| Host 바이너리 치환 | Phase 4에서 Authenticode + Developer ID 서명 + notarization; 그 전까지 SHA-256 자가 검증 경고 | C/설치 | 4 (정식), 2 (임시) |
| 로깅·감사 | mutating op마다 NDJSON 레코드; 경로는 SHA-256 truncated 해시로 저장 | C | 2 |
| 프레임 1MB 초과 | Chrome이 강제 drop; Host는 `E_FRAME_TOO_LARGE`로 응답 | B/C | 0 |
| 취소 경합(race) | op 상태 머신(state machine) + `cancel` 수신 시 현재 chunk 완료 후 원자적 중단 | C | 2 |

---

## 4. 시스템 경로 Allowlist

다음 루트(및 그 하위) 안에서 **쓰기/삭제/이름변경/이동**이 요청되면 요청에 `explicitConfirm: true` 플래그가 없을 시 `E_SYSTEM_PATH_CONFIRM_REQUIRED` 로 거절한다. 읽기 및 `open`/`revealInOsExplorer`는 플래그 불필요.

**Windows:**
- `C:\Windows`
- `C:\Program Files`
- `C:\Program Files (x86)`
- `C:\ProgramData`

**macOS:**
- `/System`
- `/usr`
- `/Library`
- `/Applications` — 전체(개별 .app 번들 수정 위험성)
- `/private`

**UI 의무:**
1. 사용자가 allowlist 경로에 대한 변경 작업을 시도하면 1차 모달: "이 폴더는 시스템 영역입니다. 계속하시겠습니까?" → `취소` / `계속`.
2. `계속` 선택 시 2차 모달: 정확한 전체 경로 표시 + 작업 요약 + 5초 지연 버튼 → `취소` / `그래도 진행`.
3. 2단계 통과 후에만 Request에 `explicitConfirm: true`를 실어 Host로 전송.
4. Host는 플래그 유무와 상관없이 allowlist 경로 이탈 여부를 독립 검증한다(UI 신뢰 금지).

**Go sentinel (참고):**
```go
var systemAllowlist = map[string][]string{
    "windows": {`C:\Windows`, `C:\Program Files`, `C:\Program Files (x86)`, `C:\ProgramData`},
    "darwin":  {"/System", "/usr", "/Library", "/Applications", "/private"},
}
```

---

## 5. 경로 정제 규칙 (Path Sanitization Pipeline)

Host는 모든 경로 인자(`path`, `src`, `dst`, `root`)에 대해 다음 파이프라인을 순서대로 적용한다. 한 단계라도 실패하면 즉시 `E_PATH_REJECTED`로 거부한다.

```
1. 타입 검증
   - 비어있지 않은 string
   - UTF-8 유효
   - 길이 ≤ 32768 (Windows long path 한계 여유)
   - 제어문자(0x00-0x1F, 0x7F) 포함 금지 → 거부

2. 절대 경로 요구
   - Windows: `^[A-Za-z]:\\` 또는 `^\\\\?\\` 프리픽스
   - macOS: `^/` 시작
   - 상대 경로 → 거부

3. 금지 프리픽스
   - Windows: `\\.\`, `\\?\PhysicalDrive`, `\\?\Volume{...}`, UNC (`\\server\share`) → 거부
   - macOS: `/dev`, `/proc` (FUSE 마운트 제외) → 거부

4. filepath.Clean
   - `.` `..` 해소, 중복 슬래시 압축

5. 심볼릭/정션 해상
   - filepath.EvalSymlinks(cleaned)
   - 실패 시 (ENOENT는 mkdir/writeFile 계열에서 허용, 그 외 거부)

6. Allowlist 루트 이탈 확인
   - 해상 결과가 현재 드라이브 루트(Win) 또는 `/` 마운트 내부인지 확인
   - 해상 결과가 [§4](#4-시스템-경로-allowlist) allowlist 내부이고 op가 mutating이며 explicitConfirm 없음 → `E_SYSTEM_PATH_CONFIRM_REQUIRED`

7. ADS / 특수 스트림 차단
   - Windows: 경로에 `:` 추가 포함 (드라이브 문자 이후) → 거부

8. 통과 → ops 레이어로 전달
```

**구현 위치:** `native-host/internal/safety/path.go`. 커버리지 100% 요구(계획 파일 테스트 계획).

---

## 6. 로그·감사 (Audit Logging)

**포맷:** NDJSON (줄당 한 JSON 객체).

**경로:**
- Windows: `%LOCALAPPDATA%\LocalFx\host.log`
- macOS: `~/Library/Logs/LocalFx/host.log`

**로테이션:** 최소 30일 보존. 파일당 10MB, 최대 10개(= ~100MB). 초과 시 오래된 파일 삭제.

**기록 대상:** mutating op (mkdir, writeFile, rename, remove, copy, move)와 보안 거절 (`E_PATH_REJECTED`, `E_SYSTEM_PATH_CONFIRM_REQUIRED`).

**레코드 스키마:**
```json
{
  "ts": "2026-04-22T12:34:56.789Z",
  "op": "copy",
  "id": "uuid",
  "user": "mellass",
  "pathHash": "sha256-trunc16:1a2b3c4d5e6f7890",
  "dstHash": "sha256-trunc16:abcdef0123456789",
  "bytes": 1048576,
  "durationMs": 234,
  "result": "ok",
  "errorCode": null,
  "protocolVersion": 2,
  "hostVersion": "0.2.0"
}
```

**PII 고려:** 경로 자체(파일명, 사용자 디렉터리 이름 등)는 기록하지 않는다. SHA-256 을 계산한 뒤 앞 16 hex 글자만 `pathHash` 로 저장. 감사 시 동일 경로 동일 해시 속성은 유지된다. 사용자 이름(`user`)은 OS 로그인 계정 — 다중 사용자 머신 식별용으로 최소만 기록.

**민감 op 추가 기록:** `remove mode=permanent`, allowlist 경로 변경은 별도 WARN 레벨 엔트리.

---

## 7. Out of Scope (책임 범위 외)

본 위협 모델은 다음을 **보호 대상에서 제외**한다.

- **로컬 머신이 이미 침해된 경우**: 악성코드가 동일 사용자 권한으로 실행 중이면 그 코드가 직접 FS를 만지면 되므로 본 확장을 경유할 필요가 없다.
- **Chrome 자체의 취약점**: V8 sandbox escape, Native Messaging 구현 버그 등은 Chromium 프로젝트의 책임.
- **사용자의 명시적 confirm 이후의 동작**: 2단계 confirm을 거친 시스템 경로 삭제는 "의도된 파괴". 복구는 OS 휴지통/백업에 의존.
- **Phase 4 이전의 바이너리 무결성**: 미서명. Phase 4에서 서명·notarization 도입 전까지 바이너리 치환은 best-effort SHA-256 자가 검증만.
- **비(非)공식 설치 경로**: 사용자가 installer 우회하여 수동 배치한 manifest/binary. 지원 대상 아님.
- **사이드 채널(timing, cache)**: 파일 존재 여부를 timing으로 탐지하는 류의 정보 노출은 본 문서에서 다루지 않는다.
- **Linux 지원**: 계획 파일에서 NOT in scope. 위협 모델도 동일하게 제외.

---

## 8. 보안 이슈 제보

- **Phase 4 이전:** GitHub **private security advisory** 사용. 저장소 설정의 "Security → Report a vulnerability" 경로로 제출. 공개 이슈로 올리지 말 것.
- **Phase 4 이후:** 릴리스 페이지와 `README.md`에 PGP 키/전용 이메일을 명시한다(구체 값은 Phase 4 시 확정).
- **대응 SLA (초기 목표):** 접수 확인 72시간, 분류 1주, 수정 or 완화 공개 30일 이내.
- **포상:** 없음(오픈소스, 자원봉사 전제).

---

## 9. 체크리스트 (코드 리뷰어용)

신규 op 추가 / mutating 경로 추가 시 이 체크리스트를 통과해야 한다.

- [ ] 입력 args의 모든 경로 필드가 [§5](#5-경로-정제-규칙) 파이프라인을 통과하는가?
- [ ] op가 mutating이라면 allowlist 게이트(`explicitConfirm`)가 걸려 있는가?
- [ ] 에러는 catch-all 없이 [§8 of PROTOCOL.md](./PROTOCOL.md#8-에러-코드-카탈로그) 의 코드로 매핑되는가?
- [ ] 로그 기록이 해시된 경로만 남기는가(PII 누출 금지)?
- [ ] 테스트에 경로 트래버설 20+ 케이스의 해당 op 버전이 포함되는가?
- [ ] UI가 시스템 경로 변경을 2단계 confirm으로 강제하는가?
