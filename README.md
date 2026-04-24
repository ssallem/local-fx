# Local Explorer

> Chrome 새 탭에서 로컬 드라이브를 탐색·조작하는 확장. Windows 11 + macOS 지원. v0.2.0 (Phase 2)

브라우저를 나가지 않고 C:\, D:\, /Volumes/... 같은 로컬 드라이브에 접근해 파일을 만들고, 이름을 바꾸고, 휴지통에 버리고, 복사하고, 이동할 수 있습니다.

## 설치

### 1. Native Host

```powershell
# Windows
cd installer\shared
powershell -ExecutionPolicy Bypass -File generate-dev-key.ps1
cd ..\..\native-host
go build -o bin\fx-host.exe .\cmd\fx-host
cd ..\installer\windows
powershell -ExecutionPolicy Bypass -File install.ps1
```

```bash
# macOS
cd installer/shared && ./generate-dev-key.sh
cd ../../native-host && go build -o bin/fx-host-darwin-$(uname -m) ./cmd/fx-host
cd ../installer/macos && ./install.sh
```

### 2. 확장 빌드 & 로드

```bash
cd extension
npm install
npm run build
```

Chrome: `chrome://extensions` → 개발자 모드 ON → "압축해제된 확장 프로그램 로드" → `extension/dist` 선택.

새 탭을 열면 Local Explorer가 표시됩니다.

## 기능

- 드라이브·폴더 탐색 (트리 + 가상 스크롤)
- 파일 열기 (OS 기본 앱) / 탐색기에서 보기
- 새 폴더 / 이름 변경(F2) / 삭제(Del 휴지통, Shift+Del 영구)
- 복사·잘라내기·붙여넣기 (Ctrl+C/X/V) — 스트리밍 진행률·취소
- 충돌 해소 (덮어쓰기/건너뛰기/이름변경) + "이후 전부 적용"
- 디렉터리 재귀 복사·이동, cross-volume move fallback
- 부분 실패 요약 다이얼로그
- 컬럼 정렬·리사이즈·재정렬 (localStorage 보존)
- 헤더 컨텍스트 메뉴 (우클릭)
- 시스템 경로 보호 (`C:\Windows`, `/System` 등에 쓰기 시 2단계 확인)

## 프로젝트 구조

```
extension/       # Chrome MV3 확장 (Vite + React + TS + Zustand)
native-host/     # Go Native Messaging Host (stdlib만, 크로스플랫폼)
installer/       # 설치 스크립트 (Win PS1 + macOS bash)
docs/            # PROTOCOL, SECURITY, DEV, PRIVACY, PUBLISHING
```

## 개발

- 상세 개발 가이드: [docs/DEV.md](docs/DEV.md)
- IPC 프로토콜: [docs/PROTOCOL.md](docs/PROTOCOL.md)
- 보안 모델: [docs/SECURITY.md](docs/SECURITY.md)
- 개인정보: [docs/PRIVACY.md](docs/PRIVACY.md)
- Chrome Web Store 등록: [docs/PUBLISHING.md](docs/PUBLISHING.md)
- Native Host 배포: [docs/NATIVE_HOST_DISTRIBUTION.md](docs/NATIVE_HOST_DISTRIBUTION.md)

## 라이선스

사용자가 결정 (placeholder: MIT 권장).

## 상태

- [완료] Phase 0: 스캐폴딩 + ping
- [완료] Phase 1: 읽기 전용 탐색기
- [완료] Phase 2: CRUD + 스트리밍 copy/move + 충돌 해소 + 취소 + 실패 요약
- [진행] Phase 3: 검색·속성·미리보기·DnD·즐겨찾기
- [진행] Phase 4: 프로덕션 서명 (MSI/.pkg + notarization)
