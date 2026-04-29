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

## 옵트인 업데이트 확인 (T6)

기본값은 **OFF**. 사용자가 설정에서 명시적으로 켰을 때만 동작합니다. 켜기 전 동의 다이얼로그로 다음 항목을 안내하고, 사용자가 "동의"를 누른 경우에만 활성화됩니다.

### 동작
- 활성화 시 24시간마다 1회 GitHub Releases API에 `GET https://api.github.com/repos/ssallem/local-fx/releases/latest` 요청.
- 응답 ETag를 디스크에 캐시하여 다음 요청은 `If-None-Match` 헤더로 보냅니다 → 보통 `304 Not Modified`로 응답되어 본문 트래픽이 발생하지 않습니다.
- Chrome `chrome.alarms`로 24시간 주기 스케줄. 새 버전이 발견되면 토스트로 알림 + "다운로드" 버튼.

### 외부로 전송되는 데이터
- HTTP 요청의 `User-Agent`: `local-fx/<버전> (+https://github.com/ssallem/local-fx)`
- 발신 IP 주소 (모든 HTTP 요청과 동일)
- 그 외 본문 없음 (`GET` 요청)

### **전송되지 않는 데이터**
- 사용자의 파일 경로, 파일 내용, 디렉터리 목록 — 일절 전송되지 않습니다.
- 확장 사용 통계, 텔레메트리, 익명화된 지표 — 없습니다.
- 사용자 식별자, Chrome 프로필 정보, 광고 ID — 없습니다.

### 옵트아웃 방법
1. **확장 UI에서 끄기**: 툴바의 ⚙ 아이콘 → 설정 패널 → "업데이트 자동 확인" 토글 OFF. 이후 어떤 외부 호출도 발생하지 않으며, `chrome.alarms` 스케줄도 즉시 해제됩니다.
2. **호스트 측 강제 비활성화** (관리자/고급 사용자용): 환경 변수 `LOCALFX_DISABLE_UPDATE_CHECK=1` 설정. 이 경우 호스트는 캐시 조회조차 하지 않고 즉시 `E_DISABLED`를 반환합니다 — 확장 토글이 켜져 있어도 네트워크 호출이 차단됩니다 (defense in depth).

### 캐시 파일
- 위치: `%LOCALAPPDATA%\LocalFx\update-cache.json` (Windows), `~/Library/Caches/LocalFx/update-cache.json` (macOS).
- 내용: 최근 확인 시각 (unix ms), 최신 태그명, ETag, 상태 코드. 어떤 사용자 데이터도 포함하지 않습니다.

### 설계 트레이드오프
업데이트 확인을 켜는 순간 "외부 호출 0건" 보장이 깨집니다 — 24시간마다 1회의 GitHub HTTPS 요청이 발생하며, GitHub 서버가 IP를 로그할 수 있습니다. 이는 **새 버전 알림 vs 완전 무외부통신** 사이의 사용자 선택입니다. 기본값은 후자입니다.

## 연락

- GitHub Issues: (레포 URL)
