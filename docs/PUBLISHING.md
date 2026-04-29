# Publishing Guide

> v0.3.0부터 Tab Explorer는 **두 개의 병렬 채널**로 발행됩니다:
> 1. **Chrome Web Store** — 확장 (`extension/`)
> 2. **GitHub Releases** — Native Host 인스톨러 (`native-host/` + `installer/windows/`)
>
> 두 채널은 **반드시 동기화**되어야 합니다. 자세한 호스트 측 자동화는 [NATIVE_HOST_DISTRIBUTION.md](NATIVE_HOST_DISTRIBUTION.md), 사용자 관점 흐름은 [../README.md](../README.md) 참조.

## 권장 발행 순서 (중요)

확장 자동 업데이트와 호스트 수동 설치 사이에 **버전 스큐**가 발생할 수 있습니다. 항상 다음 순서를 지키세요:

1. **호스트를 먼저** GitHub Releases에 발행. (CI 드래프트 → `sign-and-publish.ps1`)
2. **확장을 그다음에** Chrome Web Store에 업로드.

이유: Web Store는 설치된 확장을 자동 업데이트합니다. 확장이 먼저 새 버전이 되고 호스트가 구버전이면 사용자가 새 IPC 메시지에 대해 `E_PROTOCOL_MISMATCH`를 보게 됩니다. 반대 순서(호스트 먼저)면 구 확장 + 신 호스트 조합은 호스트의 하위 호환 보장으로 안전합니다.

> T6 옵트인 업데이트 확인은 사용자에게 호스트 구버전을 **알리기만** 하며 자동 설치하지 않습니다. 따라서 운영자 책임으로 두 채널을 같은 날 발행하는 것이 가장 안전합니다.

## Channel 1 — Chrome Web Store (확장)

### 사전 준비

1. Chrome Web Store Developer 계정 — $5 USD 일회성 등록비
   - https://chrome.google.com/webstore/devconsole
2. Privacy Policy 공개 URL (본 repo의 PRIVACY.md를 GitHub Pages로)
3. 스크린샷 5장 (1280x800 또는 640x400 권장, PNG)
   - 드라이브 목록 / 파일 리스트 / 컨텍스트 메뉴 / 진행률 토스트 / 충돌 다이얼로그
4. 홍보 이미지 (선택)
   - 440x280 "Small promo tile"
   - 1400x560 "Marquee"

### 빌드 순서

```bash
cd extension
npm install
npm run icons        # SVG → PNG 3 사이즈
npm run build:prod   # vite build + key 제거 + zip 생성
```

결과: `extension/dist-prod/localfx-v<ver>.zip` + `SHASUM256.txt` (예: v0.3.0이면 `localfx-v0.3.0.zip`).

> 버전 bump는 `extension/package.json`과 manifest를 동시에 — 빌드 prebuild 훅이 불일치 시 실패합니다.

### 업로드

1. Developer Dashboard → "새 항목" → zip 업로드
2. 스토어 리스팅:
   - **제품 이름**: Tab Explorer: File Manager in a New Tab
   - **짧은 이름(short_name)**: Tab Explorer
   - **요약(132자)**: "브라우저 새 탭에서 로컬 드라이브를 탐색하고 파일을 만들기·이름변경·복사·이동·삭제할 수 있는 확장."
   - **상세 설명**: 기능 목록 + Native Host 설치 안내 링크. 반드시 아래 차별화 문구 포함:

     > ⚠️ 이 확장은 링크 핸들러가 아닙니다. 브라우저 새 탭에서 돌아가는 완전한 파일 탐색기입니다.
     > (유사명 "Local Explorer - Open File"는 링크 클릭 시 파일을 여는 별개 제품입니다.)

   - **카테고리**: Productivity
   - **언어**: 한국어 + 영어 (선택)

   > 참고: 내부 코드명 `LocalFx` / Native Host ID `com.local.fx` / zip 파일명 `localfx-v<ver>.zip`은 리브랜딩과 무관하게 유지됩니다.
3. 개인정보 처리방침 URL 입력 (GitHub Pages 배포된 PRIVACY.md)
4. Native Host 설치 링크: README의 설치 섹션
5. 심사 제출

### 심사 포인트 (Chrome Web Store 정책 대응)

- [O] nativeMessaging 권한 사용 사유 명시 ("로컬 파일 시스템 접근을 위한 Native Host와 통신")
- [O] Host 바이너리 별도 배포 및 manifest `allowed_origins` 제한 문서화
- [X] 원격 코드 실행 없음
- [X] 사용자 데이터 수집·전송 없음
- [X] 광고·분석 미포함

보통 3~5 영업일 소요.

### 업데이트 공개

버전 bump (`extension/package.json` + `manifest.json.version`) → `npm run build:prod` → 동일 Web Store 항목에 새 zip 업로드. **Chrome Web Store가 설치된 모든 사용자에게 자동으로 새 버전을 푸시**합니다 — 보통 24~48시간 이내 전파.

## Channel 2 — GitHub Releases (Native Host)

호스트 인스톨러는 Chrome Web Store에 올릴 수 없으므로 GitHub Releases로 따로 발행합니다. v0.3.0부터 **CI 하이브리드 모델**(태그 → 드래프트 → 운영자 서명)로 자동화. 자산 목록·alias 동작·운영자 사전 준비는 [NATIVE_HOST_DISTRIBUTION.md](NATIVE_HOST_DISTRIBUTION.md).

### 발행 순서

```bash
# 1) 태그 푸시 — 워크플로우가 .github/workflows/release.yml에서 트리거
git tag v0.3.0
git push origin v0.3.0
```

```powershell
# 2) GitHub Actions 완료 대기 후, 운영자 워크스테이션 (SafeNet USB + SAC 로그인)
pwsh installer\windows\sign-and-publish.ps1 -Tag v0.3.0
```

스크립트가 끝나면 release가 draft → published로 전환되며, **stable-named alias** `localfx-host-setup-windows.exe`가 새 버전을 가리키게 됩니다. 확장 안의 온보딩 다운로드 링크는 alias로 하드코딩되어 있으므로 별도 작업 없이 즉시 새 버전을 내려받게 됩니다.

### 검증 체크리스트 (release published 직후)

- `https://github.com/ssallem/local-fx/releases/latest/download/localfx-host-setup-windows.exe` 실제 다운로드 (alias 경로).
- 받은 .exe 우클릭 → 속성 → "디지털 서명" 탭에 인증서 + RFC3161 timestamp 확인.
- `SHA256SUMS.txt`와 비교.
- 깨끗한 VM에서 설치 → 확장 새 탭 → 온보딩 패널이 사라지고 탐색기가 활성화되는지.

## v0.3.0 릴리즈 노트 템플릿

GitHub release 본문에 붙여넣을 수 있는 템플릿입니다 (`gh release edit v0.3.0 --notes-file ...` 또는 웹 UI):

```markdown
Tab Explorer v0.3.0은 **자동화 트랙 4종**을 한꺼번에 도입한 릴리즈입니다.
호스트가 등록되지 않은 새 탭에서 온보딩 패널이 자동으로 떠 1-클릭으로 인스톨러를 안내하고(T3),
태그 푸시가 GitHub Actions 드래프트 빌드 + 운영자 SafeNet USB 서명·발행으로 이어지는 하이브리드 CI/CD 파이프라인이 도입되었으며(T1+T2),
새 옵트인 토글을 켜면 호스트가 24시간마다 1회 GitHub Releases API에 ETag 캐시 요청을 보내 새 버전을 알립니다(T6, 기본 OFF·자동 설치 없음).
사용자 관점 설치 흐름과 프라이버시 정책은 README.md / docs/PRIVACY.md를 참조하세요.
인스톨러 다운로드: https://github.com/ssallem/local-fx/releases/latest/download/localfx-host-setup-windows.exe
```

## 두 채널 동기화 요약

| 단계 | 채널 | 액션 | 자동화 |
|---|---|---|---|
| 1 | Git | `git tag v0.3.0 && git push --tags` | 수동 |
| 2 | GitHub Actions | 호스트 + 인스톨러 빌드 → 드래프트 release | 자동 (release.yml) |
| 3 | 운영자 PC | `sign-and-publish.ps1 -Tag v0.3.0` → 서명 + alias 업로드 + draft 해제 | 반자동 (SafeNet PIN 입력 필요) |
| 4 | Chrome Web Store | `npm run build:prod` → Developer Dashboard에 zip 업로드 → 심사 | 수동 |
| 5 | 사용자 | Web Store 자동 업데이트 + (옵트인 시) T6 호스트 업데이트 알림 | 자동 (확장) / 수동 (호스트) |

Step 3과 Step 4 **사이 간격을 짧게** — 가급적 같은 세션에서 처리.
