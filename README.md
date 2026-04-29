# Tab Explorer

> Chrome 새 탭에서 로컬 드라이브를 탐색·조작하는 MV3 확장 — File Manager in a New Tab. Native Messaging Host로 로컬 파일에 접근. Windows 11 + macOS 지원. **v0.3.0**
>
> (내부 코드명: **LocalFx**. 바이너리·레지스트리·zip 파일명에서만 사용)

브라우저를 나가지 않고 `C:\`, `D:\`, `/Volumes/...` 같은 로컬 드라이브에 접근해 파일을 만들고, 이름을 바꾸고, 휴지통에 버리고, 복사하고, 이동할 수 있습니다.

### v0.3.0 하이라이트

- **1-클릭 설치 복구** — 호스트가 없으면 새 탭에서 온보딩 패널이 자동으로 떠 설치 파일을 안내. (T3)
- **옵트인 업데이트 알림** — 24시간마다 GitHub Releases API에 1회 요청해 새 버전을 알림. 기본 OFF. (T6, [docs/PRIVACY.md](docs/PRIVACY.md))
- **태그-드리븐 CI/CD 하이브리드** — 태그 푸시 → GitHub Actions가 드래프트 릴리즈 + 미서명 자산 생성 → 운영자가 SafeNet USB로 서명·발행. (T1+T2, [docs/PUBLISHING.md](docs/PUBLISHING.md))

## 설치 (사용자)

1. **Chrome Web Store에서 Tab Explorer 확장 설치.**
2. **새 탭을 엽니다.** Native Host가 없으면 온보딩 패널이 자동으로 표시되며 "설치 파일 다운로드" 버튼이 나타납니다.
3. **`localfx-host-setup-windows.exe`를 실행.** (T1 코드 서명이 적용되기 전 v0.3.0에서는 OV 평판 빌드 단계 — SmartScreen "추가 정보 → 실행" 우회 필요. macOS는 별도 설치 스크립트.)
4. **"다시 시도" 버튼 클릭.** 호스트가 등록되면 탐색기가 즉시 활성화됩니다.

> 설치 파일의 stable-named alias `localfx-host-setup-windows.exe`는 릴리즈마다 동일한 URL로 유지되므로 확장의 온보딩 다운로드 링크가 깨지지 않습니다. 자세한 내용은 [docs/NATIVE_HOST_DISTRIBUTION.md](docs/NATIVE_HOST_DISTRIBUTION.md).

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
extension/                # Chrome MV3 확장 (Vite + React + TS + Zustand) — Web Store 배포
native-host/              # Go Native Messaging Host (stdlib만, 크로스플랫폼) — GitHub Releases 배포
installer/windows/        # Inno Setup 래퍼 + sign-and-publish.ps1 + Signing.psm1
installer/macos/          # bash 설치 스크립트
.github/workflows/        # release.yml — tag → 드래프트 릴리즈
docs/                     # PROTOCOL, SECURITY, DEV, PRIVACY, PUBLISHING, NATIVE_HOST_DISTRIBUTION
store-assets/             # Chrome Web Store 스크린샷·아이콘
```

## 개발 (Develop)

```powershell
# 호스트 빌드 + 로컬 등록 (Windows)
cd native-host
go build -o bin\fx-host.exe .\cmd\fx-host
cd ..\installer\windows
powershell -ExecutionPolicy Bypass -File install.ps1
```

```bash
# 확장 dev 빌드 + 로드
cd extension
npm install
npm run build
# Chrome: chrome://extensions → 개발자 모드 ON → "압축해제된 확장 프로그램 로드" → extension/dist
```

세부 가이드:

- 상세 개발: [docs/DEV.md](docs/DEV.md)
- IPC 프로토콜: [docs/PROTOCOL.md](docs/PROTOCOL.md)
- 보안 모델: [docs/SECURITY.md](docs/SECURITY.md)

## 릴리즈 프로세스 (Release process)

태그 푸시 → CI 빌드 + 드래프트 생성 → 운영자가 로컬에서 SafeNet USB 토큰으로 서명·발행하는 **하이브리드 모델**입니다.

```powershell
# 1) 태그 푸시 — GitHub Actions가 드래프트 릴리즈와 미서명 자산을 생성
git tag v0.3.0
git push origin v0.3.0

# 2) 운영자 워크스테이션 (SafeNet USB + SAC 로그인 상태)
pwsh installer\windows\sign-and-publish.ps1 -Tag v0.3.0
```

전체 절차·확장 Web Store 동기화·릴리즈 노트 템플릿은 [docs/PUBLISHING.md](docs/PUBLISHING.md), Native Host 배포 세부는 [docs/NATIVE_HOST_DISTRIBUTION.md](docs/NATIVE_HOST_DISTRIBUTION.md).

## 개인정보 (Privacy)

기본 상태에서 외부 네트워크 호출 **0건**. 옵트인 업데이트 확인을 켠 경우에만 24시간마다 1회 `api.github.com/repos/ssallem/local-fx/releases/latest`를 호출하며, ETag 캐시로 응답을 절약합니다. 상세: [docs/PRIVACY.md](docs/PRIVACY.md).

## 라이선스

사용자가 결정 (placeholder: MIT 권장).

## 상태

- [완료] Phase 0~2: 스캐폴딩·읽기 전용·CRUD + 스트리밍 + 충돌 해소
- [완료] Phase 3 일부: 검색·속성·미리보기·DnD·즐겨찾기
- [완료] v0.3.0 자동화: T1 서명 / T2 CI/CD 하이브리드 / T3 온보딩 / T6 옵트인 업데이트
- [진행] Phase 4: macOS .pkg + notarization
