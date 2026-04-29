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

## 미션 6 (2026-04-22 밤): 잔여 전량 진행
- **원본 요청:** "P2.2 + P2.3 + P2.4 + WARNING 잔여 4건 전부 진행해줘. 내일 일어나서 확인. 완료 시 한국 시간 표시"
- **범위:**
  - P2.2: 컨텍스트 메뉴 + 다중 선택 + 클립보드(Ctrl+C/X/V) + revealEntry UI 진입점
  - P2.3: 스트리밍 copy + 진행률 토스트 + 취소 + background.ts Event 라우팅
  - P2.4: move + 충돌 3경로 + 부분 실패 요약 + 디렉터리 재귀
  - WARNING: /Applications allowlist 재논의, SHFileOperationW 반환 에러코드 테이블, RenameDialog null byte 조기 차단, internal/version 패키지 승격

### 사용자 결정 override
- "Multi 선택은 안돼" (미션 5) → 미션 6 요청에 "다중 선택" 명시 포함으로 **재활성화 결정**.
  근거: Ctrl+C/X/V 클립보드와 paste-to-many가 다중 선택 전제. 사용자가 잔여 리스트에 원문 그대로 포함시켰음을 존중.

### 실행 전략 (리더 계획)
- **Round 1 (3명 병렬)**: W1(WARNING 4 일괄) + P2.2-A(다중 선택 store+FileList) + P2.3-G(Go 스트리밍 인프라: copy/cancel/jobs + main.go StreamHandler + safeWriter mutex)
- **Round 2 (3명 병렬)**: P2.2-B(컨텍스트 메뉴+클립보드 store+Ctrl+C/X/V) + P2.3-T(TS Event 라우팅: background Port + requestStream + EventFrame 타입) + P2.4-G(Go move + conflict + 재귀 + uniqueName)
- **Round 3 (2명 병렬)**: P2.3-U(ProgressToasts + jobs store) + P2.4-U(ConflictDialog + FailureSummary + pre-scan 충돌 로직)
- **Round 4**: team-critic 전체 검토
- **Round 5**: CRITICAL/WARNING 수정
- **Round 6**: 재검토 (필요 시) + **프로덕션 배포 준비 (Chrome Web Store 등록 가능 수준)**
- **Round 7**: 커밋 + Phase 7 보고 (한국 시간 포함)

## 미션 6-Addendum (2026-04-23): Chrome Web Store 등록 준비 자동 진행
- **원본 요청:** "지금 진행 중인 작업 전부 다 완료되면 실제 크롬 확장 프로그램 등록 가능하도록 직접 구현해줘. 나한테 동의 구하지 말고 직접 모든걸 알아서 결정해"
- **해석:** P2.2~P2.4 및 Critic·수정 완료 후 Web Store 업로드 ready 상태까지 자동 달성

### Round 6 Production 배포 준비 범위 (자율 결정)
- **manifest.json 프로덕션 정비**:
  - name/short_name/description(ko/en) 충실화
  - version semver (`0.0.1` → `0.2.0` — Phase 2 완성 의미)
  - icons 필드 16/48/128 경로 지정
  - author/homepage_url (GitHub placeholder)
  - Web Store 업로드 시 dev `key` 필드 제거된 별도 빌드 필요 → production 모드 분기
- **아이콘 자산 생성**:
  - SVG 원본 디자인 1개 (파일/폴더 모티브)
  - Node 스크립트(sharp 등 경량) 또는 Canvas로 SVG→PNG 3사이즈 변환
  - `extension/public/icons/icon-{16,48,128}.png` 배치
- **프로덕션 빌드 스크립트**: `extension/scripts/package.mjs`
  - `VITE_MODE=production` 빌드
  - `manifest.json`에서 dev `key` 필드 제거한 복사본 생성
  - `dist/` → `extension/dist-prod/localfx-v{version}.zip` 패키징
  - SHA-256 체크섬 기록
- **문서 3종**:
  - `docs/PRIVACY.md`: Native Messaging 사용 고지, 수집 데이터 없음, 로컬 파일만 조작
  - `docs/PUBLISHING.md`: Chrome Web Store 단계별 가이드 (계정 준비, 업로드, 심사, 공개)
  - `docs/NATIVE_HOST_DISTRIBUTION.md`: Native Host를 별도 채널(GitHub Releases)로 배포, 확장 웹스토어 리스팅에서 "Native Host 필요" 안내 링크 포함하는 방법
- **README.md 갱신**: 프로젝트 루트 README — 설치·개발·배포·라이선스 전체 요약
- **package.json scripts**:
  - `npm run package` → production zip 생성
  - `npm run build:prod` → production vite build
- **Store Listing Assets 폴더** (`store-assets/`):
  - promotional tile (440x280) — placeholder SVG
  - screenshot 템플릿 안내 (실제 캡처는 사용자가 제공해야 함 — README에 위치 명시)

### Round 1 진행 상황
- **[W1 완료]** /Applications allowlist / SHFileOperationW DE_* 24개 매핑 / RenameDialog 제어문자 차단 / `internal/version` 패키지 승격 (Version=0.0.2, MaxProtocolVersion=2). +5 테스트. go test PASS, go build 3.8MB. 부가 발견: App.tsx tsc 21건 에러는 P2.2-A 중간 상태 (해결 완료).
- **[P2.2-A 완료]** `selectedIndex: number` → `selectedIndices: Set<number>` + `lastAnchorIndex`. 신규 액션 5개 (selectOnly/selectRange/toggleSelect/selectAll/clearSelection). FileList `applyClickSelection`(Shift/Ctrl 분기), App.tsx 키바인딩 재설계(Ctrl+A/Esc/Shift+Arrow), StatusBar에 선택 개수 표시. scrollIntoView는 anchor 기준. navigate/reload/goHome 등에서 세트+앵커 리셋. tsc PASS, vite build PASS. 4 동작 자체 점검 전부 PASS.
- **[P2.3-G 완료]** 파일 아티팩트 확인: `internal/ops/{copy.go, cancel.go, jobs.go}` 신규 생성. `main.go`, `codec.go`, `types.go`, `registry.go` 수정. `internal/version/` 패키지 생성. Round 1 완료.

### Round 2 완료
- **[P2.2-B 완료]** ContextMenu.tsx / clipboard.ts 신규. App.tsx/FileList.tsx/App.css 수정. 우클릭 메뉴 2 variant + Ctrl+C/X/V + revealEntry UI 진입점. Paste는 P2.4-U에서 배선.
- **[P2.3-T 완료]** background.ts / shared.ts 확장. EventFrame/ProgressPayload/DonePayload 타입 + Op 유니온 11개(copy/cancel 추가) + requestStream/copyFile/cancel 헬퍼 + event broadcast 라우팅.
- **[P2.4-G 복구 완료]** Claude limit 중단 후 재실행. `move.go`, `rename_util.go`, `move_test.go`, `rename_util_test.go` 신규. `registry.go`에 Move 등록. `copy_test.go` Phase 2.4 지원 반영 갱신. `go test 146 PASS`, `go vet clean`, `go build OK`.

### Round 3 진행
- **[P2.3-U 완료]** jobs store (`store/jobs.ts`) + `ProgressToasts.tsx` 컴포넌트 + App.tsx 마운트 + App.css 스타일. **중요 발견**: P2.3-T가 limit로 `ipc.ts`에 requestStream/copyFile/cancel 헬퍼와 streamListeners 라우팅을 미완성했음. P2.3-U가 이를 대신 완성 (+ moveFile 추가). `startCopyJob/startMoveJob` 헬퍼로 jobs store 자동 연결. tsc PASS, vite build 성공 (JS 181KB gzip 58KB).
- **[P2.4-U 완료]** Op 유니온 12개 (move 추가), MoveArgs, moveFile 헬퍼. ConflictDialog (focus-trap, 3-button, applyToAll), FailureSummary (sticky 테이블). paste 배선 완전 구현: pre-scan → conflict 순차 resolve → dispatch. FailureSummary 자동 트리거 (useJobs.subscribe). tsc PASS, vite 189KB gzip 60KB.

### Round 3 완료 — 전 단계 build 검증
- tsc --noEmit: PASS (strict + exactOptionalPropertyTypes)
- vite build: 65 modules, 606ms
- go test ./...: 146 PASS, go vet clean, go build OK

### Round 6 제외 (현실적 한계)
- 실제 프로덕션 서명 (Authenticode/Developer ID) — 유료 인증서 필요, 사용자가 별도 진행
- Chrome Web Store 개발자 등록 수수료 $5 — 사용자 계정 필요
- 실제 스크린샷 이미지 (실행 중인 확장 캡처 5장 필요) — 사용자가 로컬에서 촬영
- macOS .pkg 서명/공증 — Apple Developer 계정 필요
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

## 미션 7 (2026-04-28): 배포 후 E_HOST_NOT_FOUND 운영 이슈
- **원본 요청:** "탭탐색기 확장 프로그램 배포를 했는데 오류가 나고 있어..[Image #1]"
- **증상:** Chrome Web Store v0.2.1 정상 로드, UI 정상, 하단 "드라이브 0개", DevPanel 호스트 ping → `E_HOST_NOT_FOUND: Specified native messaging host not found.`

### 진단 (Researcher 1차)
1. **`extension/scripts/package.mjs:24-33`이 production zip 빌드 시 manifest의 `key` 필드를 strip한다.** → Web Store는 자체적으로 새 RSA 키페어를 발급하고 그로부터 production extension ID를 산출.
2. **dev ID `cjaibkecpdcabflelcjciceofknnpmck` (decisions.md L113)는 dev `key` 기반.** Web Store production ID는 이와 다름(아직 미확인).
3. **`installer/windows/com.local.fx.json.tmpl`의 allowed_origins는 `chrome-extension://{{EXTENSION_ID}}/` 단일 항목.** install.ps1이 받은 `-ExtensionId`로만 치환됨.
4. **사용자 PC의 `%LOCALAPPDATA%\LocalFx\com.local.fx.json`은 dev ID로 등록되어 있을 가능성 높음** (이전 미션 3에서 install.ps1 실행 기록). 또는 아예 미설치.

### 근본 원인 후보 (확정 위해 사용자 입력 필요)
- **(a)** Web Store 버전 production ID ≠ dev ID → allowed_origins mismatch
- **(b)** native host 자체가 이 PC에 미설치 → 레지스트리 `HKCU\Software\Google\Chrome\NativeMessagingHosts\com.local.fx` 키 자체 부재
- **(c)** install.ps1 실행 후 fx-host.exe 경로가 변경되어 manifest의 `path` invalid

### 3-track 해결안 (사용자 선택 필요)
- **Track 1 — 즉시 해결:** Web Store production ID 받아서 사용자 PC에 install.ps1 재실행 (단발성).
- **Track 2 — installer 다중 ID 지원:** `*.tmpl`의 allowed_origins를 배열로 변경, install.ps1이 `-ExtensionId` CSV 또는 반복 파라미터 수용. dev+prod 동시 지원. 향후 모든 사용자에게 효과.
- **Track 3 — 재배포로 ID 통일:** package.mjs에서 key strip을 옵션화 → key 유지한 zip을 Web Store에 재업로드 → Web Store가 동일 키 사용 → production ID = dev ID. 이후 모든 설치가 단일 ID로 통일. **단, Web Store는 첫 업로드 시 키를 결정하므로 이미 v0.2.x로 publish 끝났다면 변경 불가**.

### 권장 (리더 판단)
- 단기: **Track 1** + **Track 2** 병행. 사용자에게 production ID 확인 요청.
- 장기: docs/NATIVE_HOST_DISTRIBUTION.md에 "사용자가 chrome://extensions에서 ID 확인 → installer에 인자로 전달" 워크플로 명시. installer GitHub Release 자산 게시.

### 차단 항목
- **사용자 입력 필요:** chrome://extensions에서 "탭 탐색기" production extension ID
- 의사결정 필요: Track 2 / Track 3 채택 여부

### 진행 결과 (2026-04-28)
- **사용자 응답:** prod ID `hkopameeeinhkodnddimfmeogkjnpidf` ≠ dev ID `cjaibkecpdcabflelcjciceofknnpmck` → 원인 (a) 확정.
- **자율 결정:** Track 2 (다중 ID 지원) 채택. Track 1(즉시 재설치 명령)은 사용자에게 안내.
- **[Implementer 1차 완료]** 4 파일 패치:
  - `installer/windows/com.local.fx.json.tmpl` + `installer/macos/com.local.fx.json.tmpl`: `{{EXTENSION_ID}}` → `{{ALLOWED_ORIGINS}}` 플레이스홀더로 변경
  - `installer/windows/install.ps1`: `-ExtensionId` CSV 수용, 각 ID `^[a-p]{32}$` 검증, 4-space 들여쓰기 + `,\r\n` 구분자로 join. PS 5.1 ConvertTo-Json 단일배열 unwrap 우회를 위해 placeholder + post-substitute. 
  - `installer/macos/install.sh`: `--extension-id` CSV 수용, bash 배열 + IFS 파싱, BSD-sed 회피용 bash parameter substitution (`${var//pat/repl}`).
- **[Reviewer 1차]** CRITICAL 0 + HIGH 3 + MEDIUM 3 + LOW 2:
  - HIGH-1: `installer/README.md:36` `{{EXTENSION_ID}}` 문서 잔존 (placeholder 삭제됨)
  - HIGH-2: `installer/README.md:108` `allowed_origins[0]` 단일 인덱스 안내 (다중 ID 시 오해)
  - HIGH-3: `integrity.json` `extension_id` 항상 array → pre-patch 단일 ID install과 schema 어긋남
- **[Implementer 2차 완료]** HIGH 3건 수정:
  - README placeholder 문서 갱신 + troubleshooting 다중 ID 가이드 + Win/macOS 매니페스트 inspect 명령 + CSV 재설치 예제
  - integrity.json `extension_id`: 단일 ID → JSON string 스칼라(pre-patch와 동일), 다중 → JSON array. Phase 4 self-verifier는 `typeof`/type-switch로 양쪽 수용. 양 스크립트에 contract comment 인라인.
- **[Reviewer 2차 PASS]** HIGH 3건 모두 해결. LOW 1건(`README:104` 더블쿼트 안 `~` 미전개) → 직접 수정 완료(`$HOME` 치환).

### 사용자 PC 즉시 복구 명령 (Track 1)
```powershell
cd D:\Dev\Chrome\local-fx\installer\windows
.\install.ps1 -Force -ExtensionId "cjaibkecpdcabflelcjciceofknnpmck,hkopameeeinhkodnddimfmeogkjnpidf"
```
- `-Force`: 기존 `%LOCALAPPDATA%\LocalFx\com.local.fx.json` 덮어쓰기
- 두 ID 모두 등록 → dev unpacked + Web Store 양쪽 작동
- fx-host.exe 가 `native-host/bin/`에 없으면 `cd ../../native-host && go build -o bin/fx-host.exe ./cmd/fx-host` 선행

### 잔여/후속 (이번 세션 외)
- Reviewer 1차 MEDIUM 3건: (1) Win CRLF/LF 일관성, (2) ID 중복 제거, (3) macOS heredoc JSON-escape
- Reviewer 1차 LOW 1건: PS1 단일/다중 라벨 정렬 미세 차이
- `docs/NATIVE_HOST_DISTRIBUTION.md`에 "chrome://extensions ID 확인 → installer CSV 인자로 전달" 워크플로 명시 (별도 PR)
- GitHub Releases에 v0.2.1 native host 자산(install.ps1 + install.sh + fx-host bin) 패키징 (별도 작업)

## 미션 8 (2026-04-29): Windows 1-click 인스톨러 (Inno Setup .exe)

### 배경
- 미션 7 패치는 사용자 본인 PC만 해결. 원격 PC owner는 Q1=Windows / Q2=명령 직접 실행 불가 / Q3=fx-host도 install.ps1도 없음.
- 사용자 요청: "크롬 확장프로그램 설치시 모두 자동으로 설치되게 안돼?" → Chrome 보안 모델상 완전 자동 불가, 단 더블클릭 한 번까지는 가능.

### 전문가 패널 합의
- 리누스: Chrome 확장이 임의 native binary 자동 설치 불가능. 보안 모델 자체.
- 파울러: GitHub Release self-contained `.exe` 인스톨러가 합리적 답.
- 켄트벡: A(zip+ps1)→B(.exe wrapper)→C(코드서명) 단계적. 이번엔 B까지.

### 채택 (Track B)
- **Inno Setup 6.7.1** (winget으로 user-scope 설치 완료, `C:\Users\<u>\AppData\Local\Programs\Inno Setup 6\ISCC.exe`).
- **wrapper 전략**: setup.iss는 (1) fx-host.exe + installer/* 를 `{localappdata}\LocalFx\` 에 풀고 (2) 기존 `install.ps1` 을 `-Force -HostBinary <path> -ExtensionId "<dev>,<prod>"` 인자로 자동 실행. → 미션 7의 multi-ID install.ps1 100% 재사용. 코드 중복 0.
- **권한**: `PrivilegesRequired=lowest` — admin 불필요. 모든 동작 HKCU + %LOCALAPPDATA%.
- **하드코딩**: prod ID `hkopameeeinhkodnddimfmeogkjnpidf` + dev ID `cjaibkecpdcabflelcjciceofknnpmck` 둘 다 default. (사용자가 dev 모드도 동시에 쓸 수 있음.)
- **언어**: Inno Setup의 ISL 한국어 + 영어. 사용자 선택.
- **uninstall**: Inno Setup 자동 등록 → 제어판에서 제거 가능. `[UninstallRun]` 으로 `uninstall.ps1 -Yes` 호출.
- **출력**: `dist-prod/localfx-host-setup-v0.2.1.exe` (단일 파일, ~4MB 예상: fx-host 3.86MB + installer scripts + Inno Setup runtime).
- **SmartScreen**: 코드서명 없으므로 첫 실행 시 "추가 정보 → 실행" 1번 클릭 필요. README에 명시.

### 파일 추가 예정
- `installer/windows/setup.iss` — Inno Setup 스크립트
- `installer/windows/build-setup.ps1` — `iscc.exe` 호출 빌드 래퍼
- `installer/windows/README-DEPLOY.ko.md` — 원격 PC owner 한국어 가이드 (3-4단계)
- `dist-prod/localfx-host-setup-v0.2.1.exe` — 빌드 산출물 (.gitignore 추가)

### 검증 계획
- Reviewer: setup.iss 코드 리뷰 (path escape, 권한, uninstall 정합성)
- Smoke test: 격리된 임시 폴더에서 setup.exe `/SILENT` 실행 → manifest + 레지스트리 확인 → uninstall 호출 → 깨끗히 제거 확인

### 진행 결과
- **[Implementer 1차 완료]** 3 파일 신규: `setup.iss`, `build-setup.ps1`, `README-DEPLOY.ko.md`. 첫 빌드 성공 (3.88MB).
- **[Reviewer 1차 NEEDS_REVISION]** CRITICAL 1 + HIGH 3:
  - C-1: `[Run] runhidden` 으로 install.ps1 실패 silent. "Installation Complete"만 보임.
  - H-1: uninstall.ps1이 자기 부모 디렉터리(`installer/`)를 recursive 삭제 — Inno Setup 추적 정리와 충돌.
  - H-2: README "확장 ID 다른 경우" — "필요할 수 있음" 모호, self-service 암시.
  - H-3: README "외부 통신 없음" 미검증.
- **[Implementer 2차 완료]** 모두 수정:
  - C-1: `[Run]` 비우고 `[Code] RegisterNativeHost` + `CurStepChanged(ssPostInstall)` 도입. `Exec` 결과코드 검사 → MsgBox 한국어 + `Abort`. PS는 `Start-Transcript ... try { exit $LASTEXITCODE } finally { Stop-Transcript }`로 transcript 보존 (`%LOCALAPPDATA%\LocalFx\install.log`).
  - H-1: uninstall.ps1에 `LocalFxKeepFiles` env-var contract. `=1` 이면 dir 보존(레지스트리+JSON만 삭제). `[UninstallRun]`이 PS wrapper로 `$env:LocalFxKeepFiles='1'` 설정 후 호출. `RunOnceId` 제거.
  - H-2/H-3: README 두 문구 명확화. native-host/ grep `net/http|net.Dial|...` 0건 검증 인라인 명시.
- **[Reviewer 2차 PASS]** 4건 모두 RESOLVED. 신규 LOW 2건만 남음:
  - LOW-1: `WizardForm.Update;` 누락 → 직접 1줄 수정 완료.
  - LOW-2: `Get-ChildItem -ErrorAction SilentlyContinue` 권한거부 시 misleading warning — 비현실적 엣지케이스, 후속.
- **[최종 빌드]** `dist-prod/localfx-host-setup-v0.2.1.exe`, 3.88 MB, SHA256 `82b81b47b230cf59aa8c83c7c8f151a1e834f3fd007fafe7c7df1732a1782ab9`.

### 배포 워크플로 (사용자가 원격 PC owner에게 전달)
1. `dist-prod/localfx-host-setup-v0.2.1.exe` (3.88 MB) 단일 파일 전달 (이메일/메신저/USB).
2. `installer/windows/README-DEPLOY.ko.md` 텍스트 함께 전달 (또는 README 내용을 메시지에 그대로 붙임).
3. 원격 사용자: 더블클릭 → SmartScreen "추가 정보 → 실행" → 언어 → 다음 → 설치 → 마침 → Chrome 재시작 → 완료.
4. 실패 시 `%LOCALAPPDATA%\LocalFx\install.log` 가 자동 생성됨 → 그 파일 받아서 디버그.

### 후속 (선택)
- 코드서명 인증서로 SmartScreen 경고 제거 (~$200/년)
- macOS .pkg 동등 인스톨러 (Apple Developer 계정 필요)
- GitHub Releases 자동 업로드 (gh CLI workflow)
- 확장 안 onboarding 페이지 ("호스트 미설치 → 다운로드 링크")

## 미션 9 (2026-04-29): 4-track 설치 자동화 (T1+T2+T3+T6)

### 배경
- 사용자: "code sign은 desktop에서 가능해. 이 설치과정을 자동화 하는 방법을 더 심도있게 다각적으로 고민해봐."
- Plan Mode로 9 트랙 평가 → 4개 핵심(T1+T2+T3+T6) + 1개 옵션(T4) 채택
- 사용자 결정: SafeNet USB(EV) cert / macOS v0.4.x 미룸 / 자동 업데이트 옵트인 추가
- Plan 파일: `C:\Users\ssallem\.claude\plans\code-melodic-russell.md`

### T1 — Authenticode 코드 서명 (SafeNet USB EV)
- 신규 `installer/windows/lib/Signing.psm1` — `Find-SignTool` / `Test-SignedAndValid` / `Sign-Binary` exports
- `build-setup.ps1` `-Sign` + `-ForceSign` switch 추가, 2단계 서명 (fx-host.exe pre-ISCC + setup.exe post-ISCC)
- timestamp 재시도 chain: DigiCert → Sectigo → GlobalSign
- cert selector: `LOCALFX_SIGN_THUMBPRINT` > `LOCALFX_SIGN_SUBJECT` > `/a` (auto-pick)
- `LOCALFX_SIGN_CSP` env-var override (KSP 강제 지정 시)
- env-var 미설정 시 unsigned dev build 유지 (CI 안전 fallback)

### T2 — GitHub Actions CI/CD (Hybrid 모델)
- 신규 `.github/workflows/release.yml` — tag `v*` 트리거, 4 jobs:
  1. `build-host-windows` (Go 1.22)
  2. `build-extension` (Node 20, npm run build:prod)
  3. `package-windows` (choco innosetup 6.2.2 + build-setup.ps1 unsigned)
  4. `release-draft` (gh release create --draft, idempotent guard)
- 신규 `installer/windows/sign-and-publish.ps1` — 로컬 sign-and-publish:
  - `gh release download` → `Sign-Binary` (SafeNet PIN 1회) → `gh release upload --clobber` → `gh release edit --draft=false`
  - throw-based Fail + outer try/catch/finally → temp dir 누수 방지
  - SHA256SUMS.txt에 .exe + .zip 모두 포함 (regen 시 zip 보존)
  - stable-named alias `localfx-host-setup-windows.exe` 생성 (T3 onboarding URL용)

### T3 — In-extension Onboarding (호스트 미발견 시 1-click 회복)
- 신규 `extension/src/ui/components/HostMissingOnboarding.tsx` (199줄) — 전체 panel:
  - OS 자동 감지 (`navigator.userAgentData` + `navigator.platform` fallback)
  - "다운로드" 버튼 → `window.open(latest_release_url, "_blank", "noopener,noreferrer")` (보안 강화)
  - "재시도" 버튼 → exponential backoff (0/2/5/10s, max 4회), useRef-based re-entry guard
  - 4단계 한국어 가이드 + 영어 부제
- `ErrorBanner.tsx` `error.code === "E_HOST_NOT_FOUND"` 시 onboarding으로 위임
- en/ko locales 18 신규 i18n 키
- App.css `.onboarding-*` 스타일 (~165줄, dark/light 테마)

### T6 — 옵트인 호스트 자동 업데이트 (default OFF)
- 신규 `native-host/internal/ops/update.go` (516줄) — `checkUpdate` op:
  - 24h 캐시 (`%LOCALAPPDATA%\LocalFx\update-cache.json`, ETag 협상)
  - HTTPS GET to `api.github.com/repos/ssallem/local-fx/releases/latest`
  - User-Agent: `local-fx/<ver> (+https://github.com/ssallem/local-fx)`
  - 10s timeout
  - draft/prerelease 응답 거부
  - `LOCALFX_DISABLE_UPDATE_CHECK=1` env var → `E_DISABLED` (defense in depth)
  - 캐시 dir override는 package-level `SetTestCacheDir` (테스트 전용, wire 노출 X)
- 신규 `update_test.go` (321줄) — 8 케이스 (cache hit, 200/304/403, env disable, draft 거부, prerelease 거부, semver edge cases)
- 확장: `UpdateCheckSettings.tsx` (251줄) 모달 + 동의 다이얼로그 + chrome.alarms 스케줄 (1분 후 첫 fire + 24h 주기)
- `UpdateToast.tsx` (73줄) 영구 toast + "다운로드" 버튼 (noopener/noreferrer)
- `background.ts` chrome.alarms.onAlarm + in-flight guard + broadcastToTabs
- `Toolbar.tsx` ⚙ 설정 버튼 (dev panel은 🛠로 변경)
- `docs/PRIVACY.md` 옵트인 업데이트 섹션 (전송 데이터 / 옵트아웃 경로 / 캐시 위치 / 트레이드오프)

### Reviewer 결과
- T1+T2 1차: NEEDS_REVISION (CRITICAL 1: Fail/exit/finally; HIGH 3: SHA256SUMS zip / gh idempotent / pwsh 중첩) → fix 후 PASS
- T3+T6 1차: NEEDS_REVISION (T3 CRITICAL 1: noopener; T6 CRITICAL 2: cache dir wire / draft 필터) → fix 후 PASS
- compareSemver pre-existing test fail 1건 직접 1줄 수정 → 전체 테스트 PASS

### 빌드 검증
- `go build ./...`: PASS
- `go vet ./...`: PASS
- `go test ./internal/ops/...`: PASS (all)
- TS typecheck + vite build: implementer 보고 PASS (node_modules 미설치로 외부 재검증 불가)

### 변경 통계
- 22 파일 (modified 13 + new 9)
- +945 lines insertion, -12 deletion

### 사용자 다음 액션 (이번 세션 외)
1. **T1 검증**: SafeNet USB 꽂고 SAC 설치 → `$env:LOCALFX_SIGN_THUMBPRINT="<thumbprint>"` 설정 → `pwsh installer\windows\build-setup.ps1 -Sign` 실행 → signtool verify 통과 확인
2. **T2 trigger**: `git tag v0.3.0-test && git push origin v0.3.0-test` → Actions 통과 → `pwsh installer\windows\sign-and-publish.ps1 -Tag v0.3.0-test -DryRun` (USB 꽂은 상태)
3. **T3 검증**: dev mode 호스트 미설치 상태로 새 탭 열기 → onboarding panel 확인 → 다운로드 → 설치 → 재시도
4. **T6 검증**: 새 탭 → ⚙ → 토글 ON → 동의 → 1분 후 alarm fire → toast 표시 (또는 cache miss 시)

### 후속 (v0.4.x)
- T4 (WinGet 등록 + winget-pkgs PR 자동화)
- T7 (macOS .pkg signed + notarized + homebrew cask)
- 코드서명 + reputation 누적 모니터링
- 확장 v0.3.0 Web Store 재배포 (onboarding + 업데이트 토글 포함)

