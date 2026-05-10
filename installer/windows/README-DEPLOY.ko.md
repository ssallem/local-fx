# 탭 탐색기 — 네이티브 호스트 설치 안내

Chrome 확장 "탭 탐색기" 가 작동하려면 PC에 작은 프로그램(네이티브 호스트)을
한 번 설치해야 합니다. 보안상 Chrome이 자동 설치할 수 없어 별도 인스톨러를
사용합니다.

## 설치 (30초 소요)

1. `localfx-host-setup-v0.3.1.exe` 더블클릭
2. **"Windows의 PC 보호" 경고가 뜨면**: "추가 정보" → "실행" 클릭
   (이 인스톨러는 JCG Inc. 코드 서명 인증서로 Authenticode 서명되어 있어
   SmartScreen 경고가 표시되지 않는 것이 정상입니다. 다만 인증서 평판이
   누적되기 전 환경에서는 일시적으로 경고가 뜰 수 있습니다.)
3. 더블클릭하면 자동으로 설치가 진행되며, 진행률 표시 후 자동으로 닫힙니다.
   (언어 선택·환영·완료 페이지 없음 — 클릭 1~2회로 끝납니다.)
4. Chrome 완전히 닫았다가 다시 열기

## 확인

새 탭을 열면 "탭 탐색기" 화면이 뜨고 좌측에 드라이브(C:, D: 등) 목록이
보이면 정상입니다.

## 제거

제어판 → "프로그램 추가/제거" → "LocalFx Native Host" 제거.

## 문제 해결

- **"E_HOST_NOT_FOUND" 오류**: Chrome 재시작 안 했을 가능성. 모든 Chrome 창
  닫고 다시 열기.
- **확장 ID가 다른 경우**: 이 인스톨러는 공식 Web Store 확장 ID 와 개발 ID 두
  가지만 등록합니다. 만약 사내 배포된 다른 ID 의 확장을 사용 중이라면, 직접
  해결할 수 없습니다. `chrome://extensions` 에서 확장 ID(32자) 를 복사해
  관리자에게 전달하세요. 새 인스톨러 빌드가 필요합니다.

## 데이터/권한

- 설치 위치: `%LOCALAPPDATA%\LocalFx\` (사용자 폴더, 관리자 권한 불필요)
- 레지스트리: `HKCU\Software\Google\Chrome\NativeMessagingHosts\com.local.fx`
  (현재 사용자만)
- 외부 통신 없음. 모든 작업은 로컬 PC에서만 발생.
  (검증: native-host/ 소스에 net/http, net.Dial, websocket, grpc 사용 0건 — 2026-04-29)
