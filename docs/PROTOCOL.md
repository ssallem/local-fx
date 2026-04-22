# PROTOCOL.md — Native Messaging IPC 스펙

> Chrome 확장(extension)과 Go Native Host(`com.local.fx`) 간의 권위있는 IPC 프로토콜 명세.
> 권위있는 원천: `C:\Users\mellass\.claude\plans\harmonic-chasing-narwhal.md` — "아키텍처" / "IPC 프로토콜" / "핵심 op 목록" / "에러 & 복구 맵" 섹션.

이 문서가 계획 파일과 충돌하면 이 문서가 우선한다. 계획 파일은 전략, 이 문서는 구현 계약(contract)이다.

---

## 1. 와이어 포맷 (Wire Format)

Chrome Native Messaging 표준을 그대로 따른다.

```
┌────────────────────────┬──────────────────────────────┐
│ uint32 LE length (4 B) │ UTF-8 encoded JSON body      │
└────────────────────────┴──────────────────────────────┘
```

- 길이 필드는 **리틀 엔디언 부호 없는 32비트 정수**.
- 본문은 UTF-8 JSON. **최대 1 MB (1,048,576 바이트)** — Chrome의 하드 리밋이다. 초과 시 Chrome이 프레임을 drop하거나 호스트를 죽인다.
- 한 stdin/stdout stream 위에 여러 프레임이 순서대로 흐른다.
- 프레임 경계 = JSON 객체 경계. 한 프레임 = 한 메시지.

**스트리밍 응답이 필요한 op**(`copy`, `move`, `search`)는 한 요청에 대해 여러 프레임으로 응답한다 ([§6](#6-streaming-규약) 참조).

---

## 2. 메시지 종류

네 종류의 프레임이 존재한다. 모든 프레임은 최상위 필드로 `id`를 갖는다(매칭용).

| 종류 | 방향 | 용도 |
|------|------|-----|
| `Request` | Extension → Host | op 호출 |
| `Response` | Host → Extension | 최종 성공 응답 |
| `Error` | Host → Extension | 최종 실패 응답 |
| `Event` | Host → Extension | 스트리밍 op의 중간/완료 이벤트 |

`id`는 확장이 UUIDv4로 생성한다. Host는 `id`를 절대 발급하지 않는다. Host는 수신한 `id`를 그대로 에코한다.

---

## 3. 타입 정의

### 3.1 TypeScript (확장 / `extension/src/types/shared.ts`)

```typescript
export type OpName =
  | "ping"
  | "listDrives"
  | "readdir"
  | "stat"
  | "mkdir"
  | "writeFile"
  | "readFile"
  | "rename"
  | "remove"
  | "copy"
  | "move"
  | "open"
  | "revealInOsExplorer"
  | "search"
  | "cancel";

export interface Request<A = unknown> {
  id: string;              // UUIDv4 생성 (확장 측)
  op: OpName;
  args: A;
  stream?: boolean;        // true면 Event 프레임이 흐를 수 있음
  protocolVersion?: number; // Phase 1부터 handshake 시 포함
}

export interface Response<D = unknown> {
  id: string;
  ok: true;
  data?: D;
}

export interface ErrorFrame {
  id: string;
  ok: false;
  error: {
    code: ErrorCode;
    message: string;
    retryable: boolean;
    details?: Record<string, unknown>;
  };
}

export type EventType = "progress" | "item" | "done";

export interface EventFrame<P = unknown> {
  id: string;
  event: EventType;
  payload: P;
}

export type HostFrame = Response | ErrorFrame | EventFrame;
```

### 3.2 Go (Host / `native-host/internal/protocol/types.go`)

Go 구현은 성공/실패를 하나의 `Response` 타입으로 통일한다(별도 `ErrorFrame` 없이 `OK` 필드로 분기). `OpName` 타입은 쓰지 않고 `Op`를 `string` 필드로 두며, op 이름 상수는 필요 시 파일별/핸들러별로 `const` 선언한다. TS `ErrorFrame`의 `error` 객체는 Go에서 `ErrorPayload`로 매핑된다.

```go
package protocol

import "encoding/json"

// Phase 0/1/2/3의 모든 op는 string 리터럴로 쓰고, 디스패처가 등록된 테이블과 대조한다.
// "ping" | "listDrives" | "readdir" | "stat" | "mkdir" | "writeFile" | "readFile"
// | "rename" | "remove" | "copy" | "move" | "open" | "revealInOsExplorer"
// | "search" | "cancel"

type Request struct {
    ID              string          `json:"id"`
    Op              string          `json:"op"`
    Args            json.RawMessage `json:"args,omitempty"`
    Stream          bool            `json:"stream,omitempty"`
    ProtocolVersion int             `json:"protocolVersion,omitempty"`
}

// Response는 성공/실패를 하나의 타입으로 표현한다.
// 성공:  OK=true,  Data 세팅, Error=nil
// 실패:  OK=false, Error 세팅, Data=nil
type Response struct {
    ID    string        `json:"id"`
    OK    bool          `json:"ok"`
    Data  any           `json:"data,omitempty"`
    Error *ErrorPayload `json:"error,omitempty"`
}

type ErrorPayload struct {
    Code      string                 `json:"code"`
    Message   string                 `json:"message"`
    Retryable bool                   `json:"retryable"`
    Details   map[string]interface{} `json:"details,omitempty"`
}

// Event 프레임은 스트리밍 op(copy/move/search) 전용이며 별도 타입으로 선언된다
// (framing 계층에서 encoder가 Response와 동일한 경로로 내보낸다).
type EventType string

const (
    EventProgress EventType = "progress"
    EventItem     EventType = "item"
    EventDone     EventType = "done"
)

type EventFrame struct {
    ID      string      `json:"id"`
    Event   EventType   `json:"event"`
    Payload interface{} `json:"payload"`
}
```

> **TS ↔ Go 매핑 참고:** TS의 `ErrorFrame.error` ↔ Go `ErrorPayload`. TS `Response | ErrorFrame` 유니온 ↔ Go `Response`(성공/실패 단일 타입, `OK`로 분기). JSON 와이어 형식은 양쪽이 동일하다.

---

## 4. 버전 관리 (`protocolVersion`)

- **Phase 0:** 필드 미사용. Host는 `protocolVersion`이 없어도 허용. 하위호환.
- **Phase 1부터:** 확장은 첫 요청(`ping` 또는 `listDrives`)에 `protocolVersion` 정수 값을 포함해야 한다. 값은 다음 표를 따른다.

| Version | 도입 Phase | 포함 op |
|---------|-----------|--------|
| `1` | Phase 1 | `ping`, `listDrives`, `readdir`, `stat` |
| `2` | Phase 2 | + `mkdir`, `writeFile`, `readFile`, `rename`, `remove`, `copy`, `move`, `open`, `revealInOsExplorer`, `cancel` |
| `3` | Phase 3 | + `search` |

Host 동작:
- 확장이 Host가 지원하는 최대 버전보다 **높은** `protocolVersion`을 보내면 `E_PROTOCOL`로 즉시 거절. `details.hostMaxVersion` 필드로 Host 최대값 안내.
- **낮은** 버전은 허용하나 해당 버전에 없는 op 호출 시 `E_PROTOCOL`.

---

## 5. Phase별 op 도입 시기

| Phase | op | 비고 |
|-------|-----|-----|
| 0 | `ping` | 왕복 지연 < 50ms 검증용. 실제 FS 접근 없음. |
| 1 | `listDrives`, `readdir`, `stat` | 읽기 전용. 에러 맵 read-path 전부 구현. |
| 2 | `mkdir`, `writeFile`, `readFile`, `rename`, `remove`, `copy`, `move`, `open`, `revealInOsExplorer`, `cancel` | 변경 작업. 시스템 allowlist confirm 적용. 휴지통 기본. |
| 3 | `search` | 스트리밍, 글로브/정규식, depth 기본 10. |

Phase 0 시점에 확장은 `ping` **외의** op를 호출해서는 안 된다. Host는 미지원 op에 `E_PROTOCOL`로 응답한다.

---

## 6. Streaming 규약

요청 측:
```json
{ "id": "uuid", "op": "copy", "args": {...}, "stream": true }
```

Host 응답 시퀀스:
```
Event   { id, event: "progress", payload: { bytesDone, bytesTotal, currentPath } }
Event   { id, event: "item",     payload: { path, status: "ok"|"failed", error? } }
... (반복, 0.1초 미만 debounce) ...
Event   { id, event: "done",     payload: { successCount, failCount, failures: [...] } }
```

**규칙:**
- 최종 프레임은 반드시 `event: "done"` 또는 `Error` 프레임 하나. 이후 같은 `id`로 더 이상 프레임 없음.
- 취소: 확장이 같은 `id`를 참조하는 `cancel` op 요청을 별도 프레임으로 보낸다.
  ```json
  { "id": "new-uuid", "op": "cancel", "args": { "targetId": "uuid-of-copy" } }
  ```
  Host는 타겟 op를 중단하고, 해당 op의 `id`로 `event: "done"` with `payload.canceled: true`를 발송한다. `cancel` op 자체는 즉시 `Response`(`ok: true`)를 돌려받는다.
- `stream: true`가 아닌 op 요청에서 Event 프레임을 보내면 프로토콜 위반(`E_PROTOCOL`).
- `progress`는 debounce 0.1초. `item`은 개별 항목마다(복사/검색). `done`은 정확히 1회.

---

## 7. op 카탈로그 (args / data 스키마)

각 op의 `args` 와 `data` JSON 예시. 모든 경로는 플랫폼 절대 경로(Windows `C:\...`, macOS `/...`).

### 7.1 `ping` (Phase 0)

Request:
```json
{
  "id": "8f3e1a2c-1111-4aaa-9999-000000000001",
  "op": "ping",
  "args": { "clientTs": 1713787200000 }
}
```

Response:
```json
{
  "id": "8f3e1a2c-1111-4aaa-9999-000000000001",
  "ok": true,
  "data": {
    "pong": true,
    "hostVersion": "0.1.0",
    "hostMaxProtocolVersion": 1,
    "serverTs": 1713787200003
  }
}
```

### 7.2 `listDrives` (Phase 1)

Request:
```json
{
  "id": "8f3e1a2c-1111-4aaa-9999-000000000002",
  "op": "listDrives",
  "args": {},
  "protocolVersion": 1
}
```

Response (Windows):
```json
{
  "id": "8f3e1a2c-1111-4aaa-9999-000000000002",
  "ok": true,
  "data": {
    "drives": [
      { "path": "C:\\", "label": "System",      "fsType": "NTFS",  "totalBytes": 512000000000, "freeBytes": 120000000000, "readOnly": false },
      { "path": "D:\\", "label": "Data",        "fsType": "NTFS",  "totalBytes": 2000000000000, "freeBytes": 800000000000, "readOnly": false }
    ]
  }
}
```

Response (macOS):
```json
{
  "id": "8f3e1a2c-1111-4aaa-9999-000000000002",
  "ok": true,
  "data": {
    "drives": [
      { "path": "/",                 "label": "Macintosh HD", "fsType": "APFS", "totalBytes": 994662584320, "freeBytes": 123456789012, "readOnly": false },
      { "path": "/Volumes/Backup",   "label": "Backup",        "fsType": "APFS", "totalBytes": 500000000000, "freeBytes":  50000000000, "readOnly": false }
    ]
  }
}
```

### 7.3 `readdir` (Phase 1)

Request:
```json
{
  "id": "8f3e1a2c-1111-4aaa-9999-000000000003",
  "op": "readdir",
  "args": {
    "path": "C:\\Users\\mellass\\Documents",
    "page": 0,
    "pageSize": 1000,
    "sort": { "field": "name", "order": "asc" },
    "includeHidden": false
  },
  "protocolVersion": 1
}
```

Response:
```json
{
  "id": "8f3e1a2c-1111-4aaa-9999-000000000003",
  "ok": true,
  "data": {
    "entries": [
      { "name": "report.pdf", "path": "C:\\Users\\mellass\\Documents\\report.pdf", "type": "file",      "sizeBytes": 1048576, "modifiedTs": 1713700000000, "readOnly": false, "hidden": false },
      { "name": "photos",     "path": "C:\\Users\\mellass\\Documents\\photos",     "type": "directory", "sizeBytes": null,    "modifiedTs": 1713600000000, "readOnly": false, "hidden": false }
    ],
    "nextCursor": null,
    "total": 2
  }
}
```

`sort.field` ∈ `"name" | "size" | "modified" | "type"`. `order` ∈ `"asc" | "desc"`.
`page`는 0-based (첫 페이지가 `0`, 생략 시 기본값 `0`). Host (`native-host/internal/ops/readdir.go`)가 `offset = page * pageSize`로 계산한다.
`nextCursor`는 다음 `readdir` 호출의 `args.cursor` 로 그대로 전달. `null`이면 끝.

### 7.4 `stat` (Phase 1)

Request:
```json
{ "id": "...", "op": "stat", "args": { "path": "C:\\Users\\mellass\\Documents\\report.pdf" }, "protocolVersion": 1 }
```

Response:
```json
{
  "id": "...",
  "ok": true,
  "data": {
    "path": "C:\\Users\\mellass\\Documents\\report.pdf",
    "type": "file",
    "sizeBytes": 1048576,
    "modifiedTs": 1713700000000,
    "createdTs": 1713000000000,
    "accessedTs": 1713700000000,
    "readOnly": false,
    "hidden": false,
    "symlink": false
  }
}
```

### 7.5 `mkdir` (Phase 2)

```json
{ "id": "...", "op": "mkdir", "args": { "path": "C:\\Users\\mellass\\Documents\\new-folder", "recursive": true }, "protocolVersion": 2 }
```
응답: `{ "ok": true, "data": {} }`.

### 7.6 `writeFile` (Phase 2)

```json
{
  "id": "...",
  "op": "writeFile",
  "args": {
    "path": "C:\\tmp\\a.txt",
    "contentB64": "aGVsbG8K",
    "overwrite": false,
    "explicitConfirm": false
  },
  "protocolVersion": 2
}
```
`contentB64`는 base64 인코딩된 바이트. 최대 약 700KB(프레임 1MB - 오버헤드). 큰 파일은 Phase 3+의 청크 API를 사용하라. Phase 2 기간 1MB 초과 시 `E_FRAME_TOO_LARGE`.

### 7.7 `readFile` (Phase 2)

```json
{ "id": "...", "op": "readFile", "args": { "path": "C:\\tmp\\a.txt", "range": { "offset": 0, "length": 1024 } } }
```
Response `data.contentB64` = base64 바이트. `range` 생략 시 전체(단 1MB 이내).

### 7.8 `rename` (Phase 2)

```json
{ "id": "...", "op": "rename", "args": { "src": "C:\\old.txt", "dst": "C:\\new.txt" } }
```

### 7.9 `remove` (Phase 2)

```json
{ "id": "...", "op": "remove", "args": { "path": "C:\\tmp\\a.txt", "mode": "trash", "explicitConfirm": false } }
```
`mode` ∈ `"trash" | "permanent"`. 시스템 allowlist 경로는 `explicitConfirm: true` 필수 (SECURITY.md §5 참조).

### 7.10 `copy` / `move` (Phase 2, 스트리밍)

Request:
```json
{
  "id": "8f3e1a2c-1111-4aaa-9999-000000000010",
  "op": "copy",
  "args": {
    "src": "C:\\big-folder",
    "dst": "D:\\backup\\big-folder",
    "overwrite": false,
    "conflict": "prompt",
    "explicitConfirm": false
  },
  "stream": true,
  "protocolVersion": 2
}
```
`conflict` ∈ `"prompt" | "overwrite" | "skip" | "rename"`. `prompt` 선택 시 충돌 발생하면 Host는 `event: "progress"`와 함께 `payload.awaitingResolution: true`를 보내고 대기; 확장은 별도 `cancel`-유사 프레임(현재 id에 대한 `resolve` 구조) 대신 `args.conflict`를 미리 정해 다시 호출해야 한다. (Phase 2 초기 구현은 `prompt` 대신 UI가 충돌을 사전 해소해 `overwrite|skip|rename` 중 하나로 내려보낸다.)

Event 프레임 순서:
```json
{ "id": "...", "event": "progress", "payload": { "bytesDone": 1048576,  "bytesTotal": 10485760, "currentPath": "C:\\big-folder\\1.bin" } }
{ "id": "...", "event": "item",     "payload": { "path": "C:\\big-folder\\1.bin", "status": "ok" } }
{ "id": "...", "event": "item",     "payload": { "path": "C:\\big-folder\\locked.bin", "status": "failed", "error": { "code": "ERROR_SHARING_VIOLATION", "message": "파일 사용 중", "retryable": false } } }
{ "id": "...", "event": "done",     "payload": { "successCount": 12, "failCount": 1, "failures": [ { "path": "C:\\big-folder\\locked.bin", "code": "ERROR_SHARING_VIOLATION" } ], "canceled": false } }
```

### 7.11 `open` / `revealInOsExplorer` (Phase 2)

```json
{ "id": "...", "op": "open", "args": { "path": "C:\\Users\\mellass\\Documents\\report.pdf" } }
{ "id": "...", "op": "revealInOsExplorer", "args": { "path": "C:\\Users\\mellass\\Documents\\report.pdf" } }
```

### 7.12 `search` (Phase 3, 스트리밍)

```json
{
  "id": "...",
  "op": "search",
  "args": {
    "root": "C:\\Users\\mellass",
    "pattern": "*.pdf",
    "patternType": "glob",
    "depth": 10,
    "maxResults": 5000
  },
  "stream": true,
  "protocolVersion": 3
}
```
Event payload:
```json
{ "event": "item", "payload": { "path": "C:\\Users\\mellass\\Documents\\report.pdf", "sizeBytes": 1048576, "modifiedTs": 1713700000000 } }
{ "event": "progress", "payload": { "scanned": 12345, "matched": 27 } }
{ "event": "done", "payload": { "matched": 27, "scanned": 98765, "truncated": false, "canceled": false } }
```

### 7.13 `cancel` (Phase 2)

```json
{ "id": "cancel-uuid", "op": "cancel", "args": { "targetId": "copy-uuid" } }
```
Response: `{ "ok": true, "data": { "accepted": true } }` 또는 `accepted: false`(이미 종료된 id).

---

## 8. 에러 코드 카탈로그

계획 파일의 에러 맵을 확장하여 모든 코드에 `retryable`을 명시한다. `retryable: true`는 **동일 args로 재시도하면 성공할 수 있음**을 뜻한다(일시적 I/O, 네트워크 공유 지연 등). `false`는 상태/정책 원인으로 재시도 무의미.

총 **20개 코드** (Transport 4 + Dispatch 4 + Filesystem/op-level 12). 추가는 반드시 이 표 + Go `native-host/internal/protocol/errors.go` + TS `extension/src/types/shared.ts`의 `ErrorCode` 유니온 세 곳을 한 커밋에서 동기화해야 한다.

| Code | 의미 | retryable | 기본 HTTP 유사 등급 | 복구 액션 |
|------|-----|-----------|---------------------|-----------|
| `EACCES` | 권한 없음 | `false` | 403 | 항목 스킵, UI 자물쇠 아이콘 |
| `ENOENT` | 경로 없음 | `false` | 404 | 부모로 이동, 토스트 |
| `EIO` | 저수준 I/O 오류 | `true` | 500 | Host가 내부 2회 재시도 후 실패 보고 |
| `E_TOO_LARGE` | 디렉터리 항목 > 임계치 | `false` | 413 | 페이징 강제 |
| `EEXIST` | 대상 존재 | `false` | 409 | 충돌 다이얼로그 |
| `ENOSPC` | 디스크 공간 부족 | `false` | 507 | 중단, 여유 용량 표시 |
| `ERROR_SHARING_VIOLATION` | Windows 파일 사용 중 | `true` | 423 | 잠근 앱 추정 표시, 재시도 버튼 |
| `E_TRASH_UNAVAILABLE` | 휴지통 사용 불가 | `false` | 409 | 영구 삭제 재유도 |
| `EINVAL` | 인자 유효성(잘못된 문자 등) | `false` | 400 | 입력 검증 UI |
| `E_NO_HANDLER` | 연결된 기본 앱 없음 | `false` | 404 | 앱 설정 유도 |
| `E_HOST_NOT_FOUND` | Host 실행 파일/manifest 부재 | `false` | 503 | 설치관리자 다운로드 화면 |
| `E_FRAME_TOO_LARGE` | 1MB 초과 프레임 | `false` | 413 | 스트리밍/청크 op 재설계 |
| `E_HOST_CRASH` | Host 비정상 종료 | `true` | 502 | 확장이 1회 자동 재연결 |
| `E_PROTOCOL` | JSON 파싱/스키마/버전 불일치 | `false` | 400 | 해당 요청만 실패, 개발자 로그 |
| `E_PATH_REJECTED` | 경로 정제 실패(트래버설, 미지원 루트) | `false` | 403 | SECURITY.md §5 참조 |
| `E_CANCELED` | 사용자가 `cancel` 요청 | `false` | 499 | 정상 종료로 취급 |
| `E_SYSTEM_PATH_CONFIRM_REQUIRED` | 시스템 allowlist 경로에 `explicitConfirm` 누락 | `false` | 412 | UI 2단계 confirm 재요청 |
| `E_UNKNOWN_OP` | dispatch에 등록되지 않은 op 이름 수신 | `false` | 404 | 확장/Host 버전 불일치 확인, 개발자 로그 |
| `E_BAD_REQUEST` | 요청 args 스키마 검증 실패 또는 JSON 파싱 실패 | `false` | 400 | 해당 요청만 실패, 개발자 로그 |
| `E_INTERNAL` | Host 내부 예외 (panic recover 포함) | `false` | 500 | 개발자 로그, 필요 시 Host 재시작 |

> **extension-local 추가 2개** (Go Host는 절대 발신하지 않음, 확장 SW/UI 내부 코드):
> - `E_TIMEOUT` : 확장이 `REQUEST_TIMEOUT_MS` 경과 시 자체 생성.
> - `E_UNKNOWN` : 위 카탈로그 어느 것에도 매칭되지 않을 때의 fallback.
>
> 따라서 TS `ErrorCode` 유니온 총 개수는 **22** (20 §8 + 2 transport-local).

**확장 측 규칙:** `retryable: true`인 에러에 대해서만 자동 재시도(최대 1회). 그 외는 사용자에게 결정 위임.

**Host 측 규칙:** catch-all 금지. Go 측은 `errors.Is/As`로 sentinel 에러를 코드에 매핑한다. 미식별 오류는 `EIO` (retryable: `true`) + `details.wrapped` 원문 메시지로 감싼다.

---

## 9. 세션 수명 주기

1. 확장 서비스 워커가 `chrome.runtime.connectNative("com.local.fx")` 호출.
2. OS가 manifest를 찾아 Host 프로세스 스폰. stdin/stdout 파이프 연결.
3. (Phase 1+) 확장은 즉시 `ping` 요청에 `protocolVersion`을 포함해 보낸다. Host가 `hostMaxProtocolVersion`을 응답 → 확장은 호환성 검증.
4. 일반 요청/응답 교환.
5. 확장 측 `port.disconnect()` 또는 서비스 워커 종료 시 Host는 stdin EOF 감지 → 진행 중 op 정리 후 **0** 코드로 종료.
6. Host crash(비정상 종료) 시 확장은 `E_HOST_CRASH`로 인지, 1회 자동 재연결 시도. 실패 시 UI 에러.

---

## 10. 변경 이력 (Protocol Changelog)

| Version | Phase | 변경 요약 |
|---------|-------|----------|
| 1 | Phase 1 | 최초 공식 버전. `ping`, `listDrives`, `readdir`, `stat`. `protocolVersion` 필드 도입. |
| 2 | Phase 2 | mutating op 추가. `explicitConfirm` 플래그. 스트리밍 규약(`copy`, `move`). `cancel` op. |
| 3 | Phase 3 | `search` op. |

새 버전 추가 시 이 표와 [§4](#4-버전-관리-protocolversion), [§5](#5-phase별-op-도입-시기) 를 같은 커밋에서 갱신한다.
