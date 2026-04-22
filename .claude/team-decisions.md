# Team Decisions Log

## 프로젝트
- **경로:** `D:\dev\chrome\plug-in`
- **유형:** 그린필드 (MV3 Chrome 확장 + Go Native Messaging Host, 크로스플랫폼 Win/macOS)
- **Git:** main, 초기 커밋 `22078c8` (`.gitignore`만 있음)
- **규칙:** CLAUDE.md 없음. 계획 파일 `C:\Users\mellass\.claude\plans\harmonic-chasing-narwhal.md`가 권위.

## 미션
- **원본 요청(1):** "Chrome 웹 스토어로 만들고 있는게 맞지? 계속 진행해봐." → Phase 0 스캐폴딩 완료.
- **원본 요청(2):** "순서대로 진행해줘." (2026-04-22 01:20 KST)
  - A: `install.ps1:97` 잔류 WARNING 정리
  - B: Phase 0 실제 빌드·ping 검증 가이드 정비 (DEV.md 보강)
  - C: Phase 1 착수 — `listDrives`/`readdir`/`stat` op + TS 타입/UI 최소 슬라이스
  - D: 커밋 후 종료
- **맥락:** /plan-ceo-review 완료 후 Phase 0 스캐폴딩 + 2라운드 CRITICAL 해소 완료 상태.
- **시작 시각:** 2026-04-22 00:27 KST

## 배포 모델 확정
- 확장: **Chrome Web Store** 업로드
- Native Host: **OS별 통합 설치관리자** (Windows MSI/WiX v4, macOS signed .pkg)
- 설치관리자가 Web Store 링크를 안내하는 하이브리드 구조

## 아키텍처 결정 (계획 파일에서)
- **스택:** Chrome MV3 확장(React+Vite+TS+Zustand) ↔ Native Host(Go, 빌드 태그로 Win/Mac 분기)
- **IPC:** Chrome 표준 length-prefixed JSON (프레임 최대 1MB)
- **삭제 기본:** OS 휴지통 (Win: SHFileOperationW FOF_ALLOWUNDO, Mac: NSFileManager trashItemAtURL via CGO)
- **보안:** allowed_origins 1개, filepath.Clean+심볼릭 해상, 시스템 allowlist 2단계 confirm
- **OS 범위:** Windows 11 + macOS (Intel/ARM). Linux 제외.

## 디렉터리 구조 (계획 파일)
```
extension/              # Vite+React+TS MV3
native-host/            # Go, internal/{protocol,ops,safety,platform}
installer/windows/      # WiX v4
installer/macos/        # pkgbuild/productbuild
docs/                   # PROTOCOL.md, SECURITY.md, DEV.md
```

## 작업 분석
- **유형:** 신규 개발 (그린필드, Phase 0 스캐폴딩)
- **복잡도:** 복잡 (2개 언어, 2개 OS, IPC 프로토콜, 설치관리자)
- **구성 팀원:** 탐색가 생략(그린필드), 설계자 생략(계획 완료) → 개발자 3명 병렬 + 검토자
- **작업 계획:**
  - 병렬 Task A: `extension/` 스캐폴딩 (Vite+React+TS MV3 + service worker + 스텁 UI + ipc 클라이언트)
  - 병렬 Task B: `native-host/` 스캐폴딩 (Go 모듈, length-prefixed codec, ping op, 유닛 테스트)
  - 병렬 Task C: `docs/` 3개 초기 문서 (PROTOCOL, SECURITY, DEV)
  - 순차 Task D: `installer/` 스크립트 (A+B 완료 후 — manifest 파일에 바이너리 경로 필요)
  - 순차 Task E: 검토자(team-critic)

## Phase 0 수용 기준
- 확장 service worker ↔ Native Host ping/pong 왕복 50ms 이내
- `install.ps1` (Win) / `install.sh` (Mac) 실행으로 레지스트리/manifest 등록
- `uninstall` 스크립트로 완전 제거 가능

## 설계 결정
(스캐폴딩 단계 — 설계는 계획 파일에 이미 있음)

### IPC 프로토콜 초안 (Phase 0용 최소)
```
Request:  { id: string, op: "ping", args?: any }
Response: { id: string, ok: true, data: { pong: true, version: string, os: string } }
Error:    { id: string, ok: false, error: { code: string, message: string, retryable: boolean } }
Frame: [uint32 LE length][UTF-8 JSON body]  (Chrome Native Messaging 표준)
```

### 공유 타입 동기화 전략
- 단일 소스: `extension/src/types/shared.ts` (TypeScript)
- Go 측은 `native-host/internal/protocol/types.go`에 미러. Phase 2 이후 스키마 생성 자동화 고려.

## 완료된 작업
- **[단계 A 완료 (개발자 9)]** `install.ps1:97` WriteAllText 3인자 UTF-8 NoBOM 교체 + `$utf8NoBom` 파일 상단 공용 변수 승격. 전수 조사 — 잔재 0건.
- **[단계 B 완료 (개발자 10)]** `docs/DEV.md` §10 "Phase 0 검증 체크리스트" 추가 (~140줄, Win+macOS 양쪽, 7개 수용 기준 체크박스). 기존 섹션은 손대지 않음 — 다만 B 팀원이 발견한 "DEV.md §3에 `install-dev.ps1`로 잘못 적힌 기존 표기" 는 차후 정합성 수정 필요(후속 플래그).
- **[단계 C1 완료 (개발자 11) — Phase 1 Go ops]**
  - `listDrives` (Windows kernel32 직접 바인딩, macOS `/Volumes` 스캔) + `readdir`(정렬/페이징/숨김) + `stat`(Lstat+Readlink) 구현
  - `safety.CleanPath` 실제 구현: 절대경로만/빈문자열/null byte 거부 + EvalSymlinks best-effort
  - `internal/ops/errmap.go` — `fs.ErrNotExist/ErrPermission/ENOTDIR` → `E_*` 매핑, 그 외 `EIO(retryable)`
  - 16 신규 파일, 41 테스트 케이스. 스펙 권위는 PROTOCOL.md §7. Go 툴체인 부재로 `go test` 미실행.
  - Drive 필드 확정: `path/label/fsType/totalBytes/freeBytes/readOnly`
  - Entry 필드 확정: `name/path/type:"file"|"directory"|"symlink"/sizeBytes:nullable/modifiedTs(ms)/readOnly/hidden?/symlink?`
- **[단계 C2 완료 (개발자 12) — Phase 1 TS 타입/IPC]**
  - `shared.ts`: Drive/Entry/EntryMeta/ReaddirArgs/ReaddirData/StatArgs/OpArgsMap/OpDataMap + OpNoArgs 헬퍼
  - `ipc.ts`: `request<O>()` 오버로드 2종 + `IpcError` 클래스 + `listDrives()/readdir()/stat()` 편의 헬퍼 + `protocolVersion: 1` 자동 주입
  - `background.ts`는 structural guard로 새 op 자동 수용 — 수정 불필요
  - **`tsc --noEmit` PASS** (exactOptionalPropertyTypes 포함 strict 전 항목 통과)
- **[단계 C3 완료 (개발자 13) — Phase 1 최소 UI]**
  - Zustand explorer store: drives/currentPath/entries/history/pagination/selection/error
  - 컴포넌트 7종: Toolbar, Breadcrumb, Sidebar, FileList, StatusBar, ErrorBanner, DevPanel
  - 키보드 네비 8종: Backspace(Up), Alt+Left/Right(Back/Fwd), F5(Reload), Enter(Open), ↑↓(Select), Home/End(Jump), Ctrl+Shift+P(DevPanel)
  - `formatBytes/formatTime/splitPath/parentPath` 유틸, prefers-color-scheme 다크/라이트
  - `npx tsc --noEmit` PASS. `npx vite build` 성공(56 modules, 520ms). `npm run build`는 당시 prebuild 훅 ESM 버그로 실패 → 개발자 14가 수정.
- **[Phase 1 Critic 검토 (1차)]** CRITICAL 3 + WARNING 5 + SUGGESTION 5
  - CRITICAL-1: `scripts/check-manifest-key.js` ESM 비호환 → ship blocker
  - CRITICAL-2: Go readdir 페이지 0-based vs TS 주석 1-based 불일치
  - CRITICAL-3: ping.go에 `hostVersion`/`hostMaxProtocolVersion`/`serverTs` 누락
- **[Phase 1 수정 (개발자 14)]**
  - CRITICAL-1: `check-manifest-key.js` → `.cjs`로 rename + package.json 경로 갱신 → `require()` 정상 동작
  - CRITICAL-2: `shared.ts` page 주석을 "0-based" 수정 + PROTOCOL.md §7.3에 1줄 추가
  - CRITICAL-3: ping.go Data에 3필드 추가 + ping_test.go `TestPing_Phase1HandshakeFields` 신규
  - WARNING-3: explorer store `loadMore` path Set 기반 중복 제거 + FileList key를 `entry.path`로 정리
  - **재빌드 검증**: `npm run build` 성공 (prebuild 정상/tsc pass/vite 56 modules), `tsc --noEmit` exit 0
  - 미해결 잔여 WARNING (후속 추적): stat symlink 주석, readdir 대용량 perf 벤치마크, /Volumes 부분결과, drives_windows errno
- **[단계 D 완료]** 커밋 `bed75fc feat: scaffold Chrome local file explorer extension (Phase 0 + Phase 1 kickoff)` — 69 파일, 9515 줄
- **[미션 3 (2026-04-22)]** "1,2,3,4 네가 다 진행해 줄 수 있잖아?" — Phase 7 보고서의 사용자 다음 액션 4단계를 에이전트가 실행:
  1. Go 툴체인 확인/설치 + `go test ./...` + `go build -o bin\fx-host.exe`
  2. `generate-dev-key.ps1` 실행 → RSA 키페어 + manifest.json `key` 주입 + 확장 ID 산출
  3. `npm run build` + `install.ps1` (HKCU 레지스트리·Native Messaging manifest 등록)
  4. Chrome CLI로 확장 로드 (dist/ 언팩드) + 새 탭에서 탐색기 열기. Ping Host 버튼 클릭은 자동화 제한 가능성.

### 미션 3 실행 결과 (실행 담당자 1명)
- **Go 1.26.2** winget 비대화식 설치 성공. `go build` OK, `bin/fx-host.exe` 3.5MB 생성.
- **Go test 1건 실패**: `TestMapFSError_NotDir` (errmap_test.go:48) — Windows에서 파일을 디렉터리처럼 open 시 `ENOTDIR` 대신 `ENOENT`가 나옴. Windows-specific 매핑 이슈, 빌드·배포 무관.
- **Extension ID**: `cjaibkecpdcabflelcjciceofknnpmck`
- **generate-dev-key.ps1 — PowerShell 5.1 비호환** 발견: `ExportPkcs8PrivateKey`는 PS 7.2+ 필요. 실행 담당자가 `pwsh 7.5.4`로 우회 성공. **install.ps1이 powershell.exe(5.1)로 이 스크립트를 호출하면 재발** → 후속 수정 필요.
- **install.ps1**: `pwsh`로 실행 성공. HKCU Chrome+Edge 양쪽 등록, `%LOCALAPPDATA%\LocalFx\{com.local.fx.json, integrity.json}` 생성 확인.
- **Chrome 실행**: v147.0.7727.56, 격리 프로파일 `$TEMP\LocalFxChromeProfile`로 `--load-extension=dist/` + `chrome-extension://<id>/tab.html` 자동 오픈. 사용자 기존 프로파일 무오염.
- **보너스 — Host stdio smoke**: 직접 length-prefixed JSON 프레임으로 `fx-host.exe` 호출 → `{"ok":true,"data":{"pong":true,"hostVersion":"0.0.1","hostMaxProtocolVersion":1,"os":"windows","arch":"amd64","serverTs":...}}` **실제 응답 수신**. Host 바이너리 자체 정상 동작 입증. exit code 0.

### 남은 과제 (후속 플래그)
- **P1**: `install.ps1` 내 `generate-dev-key` 호출을 `pwsh` 우선 → `powershell.exe` fallback으로 변경
- **P2**: `errmap_test.go:48` Windows 분기 추가 또는 fixture 변경 (TestMapFSError_NotDir)
- **P3**: `installer/windows/smoke-ping.ps1` 추가 (length-prefix stdio ping 왕복 CLI 검증 스크립트, 실행 담당자가 ad-hoc 작성한 것을 영구화)
- **사용자 수동 확인**: Chrome 창에서 "Ping Host" 버튼 클릭 → UI에 pong 응답 표시 확인

## 미션 4 (2026-04-22): Phase 2.1 구현 착수
- **원본 요청:** "다음 확장 기능 설계하고 진행하자" (사용자가 실제 Chrome에서 탐색기 UI 동작 확인 후)
- **승인된 플랜:** `C:\Users\mellass\.claude\plans\harmonic-chasing-narwhal.md` — "## Phase 2 실행 계획" 섹션
- **이번 세션 스코프:** Phase 2.1 (비스트리밍 CRUD + OS 연동 + allowlist confirm)
- **핵심 op:** mkdir, rename, remove(trash/permanent), open, revealInOsExplorer
- **핵심 설계 결정:**
  - macOS Trash = **osascript** (CGO 대신, 서명 복잡도 회피)
  - 원안 2.2(allowlist)를 2.1에 통합
  - 컨텍스트 메뉴 / 다중 선택 / 스트리밍은 후속 세션
  - rename은 same-dir만, permanent remove는 빈 폴더/단일 파일만
  - PROTOCOL_VERSION 1→2 bump (ipc.ts + ping.go 같은 커밋)

### 팀 구성
- **개발자 A (Go)**: errmap 확장 + safety allowlist + ops(mkdir/rename/remove/open) + platform(shell_win/darwin/other) + registry + ping bump
- **개발자 B (TS)**: shared.ts 타입 확장 + ipc.ts 헬퍼 + PROTOCOL_VERSION bump
- **개발자 C (UI)**: store 액션 + ConfirmDialog + RenameDialog + Toolbar "새 폴더" + App.tsx 키바인딩
- **검토자**: team-critic 1회

### 실행 결과
- **[A 완료]** Go: 14 신규 + 8 수정, ~60 테스트 추가. `go test ./...` 101 PASS + 2 skip. `go vet` clean. `go build` 3.8MB. SHFileOperationW 더블-NUL 수동 append, osascript escape `\` + `"`, allowlist hasPrefixBoundary separator 체크, Windows 5/32/80/183/112 errno 매핑, HostMaxProtocolVersion=2.
- **[B 완료]** TS: Op 유니온 9개, OpArgsMap/OpDataMap 각 9개, MkdirArgs/RenameArgs/RemoveArgs/OpenArgs/RevealArgs + RemoveMode + EmptyData, PROTOCOL_VERSION=2 bump, 헬퍼 mkdir/rename/remove/openEntry/revealEntry (window.open 충돌 회피). `tsc --noEmit` PASS.
- **[C 완료]** UI: 3 신규 (ConfirmDialog, RenameDialog, dialogs/index.ts) + 6 수정. store 액션 10→17 (createFolder/renameEntry/deleteEntry/openEntry/revealEntry/resolvePendingConfirm/cancelPendingConfirm + pendingConfirm 상태). 키바인딩 F2/Del/Shift+Del + Enter 파일 open. 다이얼로그 포커스 트랩/variant 3종. `tsc --noEmit` PASS, `vite build` 성공 (JS 166KB gzip 53KB).
- **[Critic 1차]** CRITICAL 2 + WARNING 5 + SUGGESTION 5 적발
- **[수정 1차 (개발자 15)]** CRITICAL 전부 + WARNING 2 수정:
  - P2-CRITICAL-1: ConfirmDialog Enter 전역 핸들러 제거 → 브라우저 기본 동작(포커스된 버튼 클릭)에 위임. danger/warning variant Cancel 포커스에서 Enter → onCancel (우발 영구삭제 방지)
  - P2-CRITICAL-2: SHFileOperationW `AnyOperationsAborted` OUT 필드 검사 삭제. silent mode 거짓 오류 제거, 반환값 0만 성공 기준
  - P2-WARNING-1: osascript NewReplacer에 `\n`/`\r` escape 추가
  - P2-WARNING-2: Windows rename cross-dir 체크에 `strings.EqualFold` 분기 + rename_windows_test.go 2 케이스 추가
  - 재검증: `go test ./...` PASS, `go build` 성공, `tsc --noEmit` PASS, `vite build` 59 modules 540ms

### 잔여 WARNING/SUGGESTION (후속 플래그)
- ErrTrashUnavailable 중복 처리(remove.go + errmap.go) — 논리적으로 안전
- macOS `/Applications` allowlist 포함 여부 재논의 (계획 파일 미명시)
- Windows SHFileOperationW 반환 에러코드 0x7C/0x78 등 전용 매핑 테이블
- FileList 단일 클릭으로 openEntry 호출(더블클릭과 동시) — Phase 2.2 컨텍스트 메뉴에서 정리
- RenameDialog null byte 검증 (Go 측에서 이미 거부되지만 UI 레벨 조기 차단 권장)
- Ping hostMaxProtocolVersion 상수 `internal/version` 패키지 승격 (아직 ops 내부)

### 다음 단계 (후속 세션)
- **Phase 2.2**: 컨텍스트 메뉴, ~~다중 선택(Shift/Ctrl+Click)~~ **사용자 결정으로 제외**, 클립보드(Ctrl+C/X/V)
- **Phase 2.3**: 스트리밍 copy + 진행률 + 취소 + background.ts Event 라우팅 인프라
- **Phase 2.4**: move + 충돌 3경로 + 부분 실패 요약 + 디렉터리 재귀

## 미션 5 (2026-04-22): UI 품질 개선 — 헤더 정렬 + 컬럼 리사이즈/재정렬 + multi-select 제거
- **원본 요청:** "헤더 name/size/modified 클릭 시 정렬, 헤더 컬럼 Width 조정과 이동, Multi 선택은 안돼"
- **스크린샷 관찰:** C:\Windows 진입 성공, 툴바 "+ 새 폴더" 버튼 표시 확인, 드라이브 Sidebar 정상, 111 items 표시
- **사용자 질의:** 다운로드 기능 유무 — 현재 없음, Phase 3+ 로드맵 후보로 기록

### 작업 범위 (UI만)
- **A) 헤더 정렬**: FileList 헤더(Name/Size/Modified) 클릭 시 asc↔desc 토글. 현재 field 표시(↑/↓ 인디케이터). Go readdir의 `sort: { field, order }` args 활용 — 서버 사이드 정렬.
- **B) 컬럼 width 조정**: 헤더 셀 우측 경계에 리사이즈 핸들. mouse drag로 width 변경. 최소 width 보장. localStorage로 세션 간 보존(선택).
- **C) 컬럼 순서 이동**: 헤더 셀 drag & drop으로 컬럼 순서 재정렬. localStorage 보존(선택).
- **D) Multi-select 영구 제외**: Phase 2.2 계획에서 Shift+Click/Ctrl+Click/Ctrl+A 제거. single selection 영구 유지.

### 추가 고려
- `sort.field` Go 지원 값: "name"|"size"|"modified"|"type" (type은 UI에 헤더 없음, 일단 name/size/modified 3개)
- 현재 `OpArgsMap.readdir` 에 `sort` 필드 이미 존재 — TS 타입 확장 불필요
- store에 `sortField`, `sortOrder` 상태 추가 → navigate/reload 호출 시 args에 반영
- store에 `columnWidths`, `columnOrder` 상태 추가 → localStorage 동기화
- 제외: 다운로드 기능 (Phase 3+ 후보로 기록, 이번엔 구현 안 함)

### 실행 결과
- **[개발자 완료]** FileList 전면 재작성 (table→div grid), store에 sortField/sortOrder/columnWidths/columnOrder 상태 + setSort/setColumnWidth/setColumnOrder 액션 + localStorage 동기화. navigate/reload/loadMore 모두 sort args 주입. 플랜 파일 Phase 2.2에서 multi-select 제거.
- **[Critic]** CRITICAL 2 + WARNING 6 + SUGGESTION 4 적발
- **[수정 완료]** CRITICAL 전부 + WARNING 2 수정:
  - M5-CRITICAL-1: `sanitizeWidth` 헬퍼로 NaN/음수/비정상 입력 방어, `setColumnWidth`에도 guard 적용 → localStorage 훼손 시 기본값 fallback
  - M5-CRITICAL-2: window-level `dragend`/`drop`/`mouseup` cleanup 리스너로 stale dragCol 보장 정리 (ESC/window-outside drop 취소 대응)
  - M5-WARNING-1: `persistColumnWidthsDebounced` (300ms) — 리사이즈 mousemove I/O 폭증 제거
  - M5-WARNING-2: `reorderColumns(from, to, side)` 시그니처로 "after" drop 올바르게 처리 → 시각 힌트와 실제 결과 일치
- **재검증**: `tsc --noEmit` PASS, `vite build` 171.05 KB (59 modules, 546ms)

### 후속 플래그 (이번에 손대지 않음)
- Name 셀 단일 클릭 → openEntry (기존 Phase 2.1 이슈, Phase 2.2 컨텍스트 메뉴에서 정리)
- 헤더 접근성 `aria-sort` / `role="columnheader"` 누락
- Firefox `text/x-column` MIME 호환성 (확장은 Chrome 전용이라 영향 무)
- `col-name` CSS 클래스 미정의 (현재 무해)
- columnWidths 새로고침 직전 300ms 내 저장 누락 가능성 (immediate flush 미배선)
- `sortField="type"` UI 미노출 (헤더 컬럼에 type 없음)
- "unsorted" 상태 미지원 (asc↔desc 토글만)
- 다운로드 기능 (Phase 3+ 로드맵 후보)
- **[Phase 6 Round 2 — 신규 CRITICAL 수정 (개발자 8)]**
  - N-1: TS `ErrorCode` 유니온에 `E_UNKNOWN_OP`/`E_BAD_REQUEST`/`E_INTERNAL` 3개 추가 → 총 22개(§8 20 + transport-local 2)
  - N-1: PROTOCOL.md §8에 3개 행 + 서두 총개수 20 명시
  - N-2: Go `Request`에 `Stream bool omitempty`, `ProtocolVersion int omitempty` 추가 + TS `Request`에 `stream?`, `protocolVersion?` 동기화 + types_test.go 케이스 추가 (populated round-trip + Phase 0 ping omitempty + zero-value decode)
  - N-3: generate-dev-key.ps1의 WriteAllText 2개 호출 모두 `UTF8Encoding($false)` 3인자 오버로드로 교체
  - N-4: PROTOCOL.md §3.2 Go 코드 블록을 실제 types.go와 완전 동기화 (OpName 제거, ErrorBody→ErrorPayload, ErrorFrame 통합, Ok→OK, Stream/ProtocolVersion 포함)
  - **2차 Critic 재검증 통과** — CRITICAL 0건, 남은 WARNING 1건: `install.ps1:97`의 `$rendered` WriteAllText 인코딩 미지정 (manifest.json 출력 — Phase 0 수용 기준 무영향, 후속 P2)
- **[Phase 6 Round 1 — CRITICAL/WARNING 수정 (개발자 5, 6, 7)]**
  - **개발자 5 (installer)**: F-1 `install.ps1` 백슬래시 JSON 이스케이프 `'\\\\'` 수정 / F-2 `?.Path` PS5.1 호환 if-분기로 / F-3 `Set-Content UTF8` → `UTF8Encoding $false` BOM-less / F-4 `extension/package.json` prebuild 훅 + `scripts/check-manifest-key.js` 신규 / F-5 `generate-dev-key.sh` openssl fallback
  - **개발자 6 (extension)**: F-6 SW keepalive (`chrome.alarms`, 30초, pending 있을 때만) + `onSuspend` failAll / F-7 `ErrorCode` 유니온 19개 확장 (PROTOCOL.md §8 17 + E_TIMEOUT/E_UNKNOWN) + `details?` / F-8 `crypto.randomUUID()` 직접 / F-9 `mayNeedInstall` details 플래그 + UI 자동 1회 재시도(E_HOST_CRASH/E_TIMEOUT) / manifest permissions에 `"alarms"` 추가
  - **개발자 7 (native-host)**: F-10 `ErrorPayload.Details map[string]interface{}` + `NewErrorWithDetails`/`ErrorResponseWithDetails` 헬퍼 / F-11 에러 카탈로그 20개 확장 (PROTOCOL.md §8 1:1) / F-12 `integration_test.go runHost` io.EOF → nil 반환 / F-13 WriteFrame 단일 버퍼 원자성 (Phase 2 대비) / F-14 `Registered()` sort.Strings / F-15 main.go `fallbackEncoded` 사전 JSON 리터럴 / F-16 Version 승격 TODO 주석 / `types_test.go` 신규 (5 케이스)
- **[Task D 완료]** installer/ dev 설치 스크립트 (PS1 + bash)
  - HKCU만 사용(관리자 권한 불필요), macOS `~/Library/...` 동일
  - `generate-dev-key.ps1` / `.sh` — RSA 2048 키페어 생성 → 공개키 DER SHA-256 첫 16바이트 → hex → 0-9→a-j, a-f→k-p 매핑으로 결정론적 확장 ID 산출 → manifest.json에 `key` 필드 in-place 주입(JSON 파싱 후 재직렬화, 기존 필드 보존)
  - `install.ps1 -HostBinary -ExtensionId -Force` / `install.sh --host-binary --extension-id --force`
  - Edge 지원 기본 포함 (`-SkipEdge`/`--skip-edge` 옵트아웃)
  - `integrity.json` 기록: `host_sha256`, `host_path`, `extension_id`, `manifest`, `installed_at` — Host 자가 검증은 후속 Phase
  - macOS 아키텍처 자동 감지(`uname -m`)로 arm64/amd64 바이너리 선택
  - uninstall은 manifest/레지스트리 제거; `extension/manifest.json`의 `key` 필드는 잔류(다음 generate-dev-key와 호환 유지)
  - Phase 4 이연: MSI/.pkg 서명·공증, HKLM·`/Library/...` 시스템 등록, Chrome Canary/Beta/Dev, Firefox, host 자가 검증, 자동 업데이트
- **[Task C 완료]** docs/ 초기 문서 3종 (PROTOCOL 525줄, SECURITY 236줄, DEV 300줄)
  - PROTOCOL: 와이어 포맷, 타입 정의 TS/Go 양쪽, op 카탈로그 13, 에러 코드 17, 스트리밍·취소 규약
  - SECURITY: 신뢰 경계 ASCII, 공격 표면 4, 완화 매트릭스, allowlist 정책, 감사 로그
  - DEV: 요구사항, 빌드 4단계, 개발 루프, 로그, 테스트, FAQ
- **[Task B 완료]** native-host/ 스캐폴딩 (Go 모듈, stdlib만 사용, 외부 의존성 0)
  - `main` 본체를 `run(ctx, in, out, logger)`로 분리 — io.Pipe 주입 가능, 테스트 용이
  - `ReadFrame`: `io.EOF` 통과(정상 종료 신호), `ErrFrameTooLarge`는 치명적(프로세스 종료)
  - `ops.Handler = func(ctx, Request) Response` + dispatch에 panic recover
  - 테스트: codec round-trip, 1MB 경계, truncated, chunked read, ping 직접/registry, io.Pipe E2E
  - Version 상수: `ops.Version` (Phase 1에서 `internal/version` 승격 고려)
  - `safety.CleanPath`는 스텁 + TODO (Phase 1에서 allowlist 주입 정책 결정 필요)
  - `go build ./...` / `go test ./...`: **환경에 Go 툴체인 없음** → 스킵됨, 설치 후 검증 필요
- **[Task A 완료]** extension/ 스캐폴딩 (MV3 manifest + Vite/React/TS + background SW + 스텁 UI + IPC 클라이언트)
  - `chrome_url_overrides.newtab = "tab.html"` 채택 — 새 탭이 곧 탐색기
  - IPC 허브: background SW가 `Map<id, Pending>`으로 요청/응답 상관관계 관리, 10초 타임아웃
  - 에러 코드 확장: `E_HOST_NOT_FOUND`, `E_HOST_CRASH`, `E_PROTOCOL`, `E_TIMEOUT`, `E_UNKNOWN`
  - Native Host 이름 상수: `HOST_NAME = "com.local.fx"` (shared.ts) — Go 쪽 manifest와 1:1 일치 필요
  - npm install은 미실행 (구조만 완비)

## 변경된 파일
- `extension/manifest.json`
- `extension/package.json`
- `extension/tsconfig.json`
- `extension/vite.config.ts`
- `extension/tab.html` (루트 배치, public/이 아님 — rollup output 경로 이슈 회피)
- `extension/src/background.ts`
- `extension/src/types/shared.ts`
- `extension/src/ui/{main.tsx, App.tsx, ipc.ts}`
- `native-host/go.mod`, `native-host/Makefile`, `native-host/.gitignore`
- `native-host/cmd/fx-host/main.go`
- `native-host/internal/protocol/{codec.go, types.go, errors.go, codec_test.go}`
- `native-host/internal/ops/{registry.go, ping.go, ping_test.go}`
- `native-host/internal/safety/path.go` (스텁)
- `native-host/internal/platform/platform.go` (인터페이스 선언만)
- `native-host/test/integration_test.go`
- `docs/PROTOCOL.md`, `docs/SECURITY.md`, `docs/DEV.md`
- `installer/README.md`
- `installer/shared/{generate-dev-key.ps1, generate-dev-key.sh}`
- `installer/windows/{install.ps1, uninstall.ps1, com.local.fx.json.tmpl}`
- `installer/macos/{install.sh, uninstall.sh, com.local.fx.json.tmpl}`
- `.gitignore` (루트, `/extension/dev-key/` + `/installer/*/logs/` 추가)

## 발견된 이슈
- (개발자 1 보고) `chrome_url_overrides.newtab` 충돌 가능성 — 다른 newtab 확장과 경합. 대안: `action.default_popup` 전환.
- (개발자 1 보고) zip 스크립트는 Windows PowerShell 전용 — macOS에서 동작 안 함, Phase 0 필수 아님.
- (개발자 3 발견) **writeFile 1MB 경계 + base64 오버헤드** → 실효 ~700KB. PROTOCOL §7.6에 Phase 2 잠정 규정 + Phase 3 청크 API 플래그.
- (개발자 3 발견) **충돌 해소(overwrite/skip/rename) 양방향 resume 프레임 규약 없음**. Phase 2 초기 구현은 사전 해소로 우회.
- (개발자 3 발견) **Host 바이너리 치환 Phase 0-3 공백** — Phase 4 서명 이전 방어 없음. 설치 시 SHA-256 기록 + Host 자가 검증 경고로 임시 완화 제안.
- (개발자 3 발견) **확장 ID 고정(`key` 필드) 방침 미정** → 로드할 때마다 ID 바뀜, `allowed_origins` 수동 업데이트 필요. **Phase 0 installer에서 dev `key` 삽입 권장**.
- (개발자 3 발견) **프로덕션 ID 확정 시점** — Chrome Web Store 업로드 전엔 알 수 없음. Phase 4 세부 때 installer가 Web Store ID를 받는 경로 결정 필요.
- (개발자 3 발견) **macOS `/Applications` allowlist 범위** 임시 전체 포함(안전 방향). 필요 시 완화.
- (개발자 3 발견) **`cancel` op 규약**: 새 프레임 별도 id + `args.targetId` 참조로 문서에 확정.
- (개발자 3 발견) **`protocolVersion` 필드** PROTOCOL §4에 도입. 상위 계획 파일 동기화 필요.

## 남은 작업
- Phase 0 스캐폴딩 (이번 세션)
- Phase 1 읽기 전용 탐색기 (다음 세션)
- Phase 2 변경 작업 (다음 세션)
- Phase 3 고급 기능 (선택)
- Phase 4 배포 (MSI/pkg 서명·공증)
