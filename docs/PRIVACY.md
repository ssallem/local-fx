# Privacy Policy

> 최신 수정: 2026-04-24 (Phase 2 release v0.2.0)

## 요약

**Tab Explorer는 사용자의 로컬 파일을 수집·전송하지 않습니다.**
- 외부 서버로 데이터 전송 없음
- 통신은 **동일 기기 내** Native Messaging Host 프로세스와의 stdio만 사용
- 원격 URL 요청 없음, 광고·분석 SDK 없음

## 처리되는 데이터

### 1. 로컬 파일 경로
사용자가 UI에서 탐색·조작한 파일 경로. Native Host가 OS 파일 시스템 호출에만 사용.

### 2. 파일 내용
copy/move 작업 시 파일 내용이 임시 버퍼(64KB)를 통과. 브라우저/원격 서버로 전송되지 않음.

### 3. 설정
- 컬럼 너비/순서 → `localStorage` (브라우저 내부)
- 정렬 상태 → 세션 내 메모리

## 저장 위치

- Native Host integrity 기록: `%LOCALAPPDATA%\LocalFx\integrity.json` (Windows), `~/Library/Application Support/LocalFx/integrity.json` (macOS)
- Native Host 로그(Phase 3+): `%LOCALAPPDATA%\LocalFx\host.log` / `~/Library/Logs/LocalFx/host.log`

## 권한 설명

- **nativeMessaging** — Native Host와 stdio 통신
- **alarms** — Service Worker keepalive (스트리밍 중 연결 유지)

## Native Host 보안

별도 설치된 fx-host 바이너리의 manifest `allowed_origins`는 **본 확장 ID 1개**로만 제한됨. 타 확장/웹사이트에서 호출 불가.

## 연락

- GitHub Issues: (레포 URL)
