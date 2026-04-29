# Native Host Distribution

Chrome Web Store는 확장만 배포. Native Host는 별도 채널(GitHub Releases)로 배포합니다. v0.3.0부터 태그 푸시 → CI 드래프트 → 운영자 서명 발행의 **하이브리드 모델**로 자동화되었습니다.

> 사용자 관점 설치 흐름은 [README.md](../README.md#설치-사용자), Web Store 측 발행은 [PUBLISHING.md](PUBLISHING.md), 업데이트 확인 프라이버시는 [PRIVACY.md](PRIVACY.md) 참조.

## 배포 자산 (v0.3.0)

GitHub Releases (`https://github.com/ssallem/local-fx/releases`)에 다음 자산이 게시됩니다:

- `localfx-host-setup-v<ver>.exe` — 버전별 Inno Setup 인스톨러 (서명됨)
- **`localfx-host-setup-windows.exe` — stable-named alias** (확장 온보딩 패널이 다운로드하는 URL)
- `fx-host-windows-amd64.exe` — 단일 호스트 바이너리 (수동 등록·디버깅용)
- `SHA256SUMS.txt` — 모든 자산의 해시
- macOS: `fx-host-darwin-arm64`, `fx-host-darwin-amd64`, `install.sh` (Phase 4에서 .pkg + notarization으로 전환 예정)

### Stable-named alias의 역할

`localfx-host-setup-windows.exe`는 **항상 최신 release에서 동일한 이름으로 다운로드 가능한 별칭**입니다. 확장 안의 온보딩 패널(T3)은 이 URL을 하드코딩하므로 — 새 버전이 나와도 확장 자체를 재배포하지 않고 호스트만 업그레이드시킬 수 있습니다. `sign-and-publish.ps1`이 `localfx-host-setup-v<ver>.exe`를 업로드한 뒤 같은 파일을 별칭 이름으로 한 번 더 업로드해 alias를 갱신합니다.

## CI/CD 하이브리드 모델 (T2)

태그(`vX.Y.Z`) 푸시 → `.github/workflows/release.yml`이 다음을 수행합니다:

1. Windows runner에서 `fx-host.exe` 빌드 + 확장 빌드 + Inno Setup으로 `localfx-host-setup-v<ver>.exe` 패키징.
2. 모든 자산을 **DRAFT release**에 첨부. (자산은 모두 미서명.)
3. SafeNet UCert USB 토큰은 GitHub-hosted runner에 꽂을 수 없으므로 서명·발행은 운영자 워크스테이션에서 수동.

워크플로우 정의는 [`.github/workflows/release.yml`](../.github/workflows/release.yml) 참조 — YAML을 여기에 다시 옮기지 않습니다.

## 운영자 서명·발행 단계 (T1)

```powershell
# 운영자 워크스테이션 — SafeNet USB 토큰 + SAC 로그인 상태
pwsh installer\windows\sign-and-publish.ps1 -Tag v0.3.0
```

스크립트가 수행하는 일:

1. `gh release download v0.3.0 -p localfx-host-setup-v0.3.0.exe` — 드래프트의 미서명 인스톨러 가져오기.
2. `installer\windows\lib\Signing.psm1`을 통해 인증서 thumbprint로 Authenticode 서명 (`signtool /tr <RFC3161 TSA> /td sha256 /fd sha256`).
3. 서명된 자산을 동일 release에 재업로드(덮어쓰기).
4. **stable-named alias** `localfx-host-setup-windows.exe`로 한 번 더 업로드 — 확장 온보딩 다운로드 링크가 새 버전을 가리키도록.
5. `gh release edit v0.3.0 --draft=false` — 드래프트 해제, 공개 발행.

> 이전 버전 문서에 있던 "GitHub Releases 웹 UI에서 수동 업로드" 절차는 v0.3.0 시점에 폐기되었습니다. 모든 발행은 `sign-and-publish.ps1`을 거칩니다 — 서명되지 않은 자산이 공개 release에 노출되는 것을 막기 위함.

## 옵트인 업데이트 확인 (T6)

확장의 설정 패널 → "업데이트 자동 확인" 토글이 **ON**일 때만 동작합니다. 기본값 OFF.

- 호출: `GET https://api.github.com/repos/ssallem/local-fx/releases/latest`
- 빈도: **24시간에 1회** (`chrome.alarms` 스케줄)
- 캐시: ETag 기반 — 변경 없으면 304, 본문 미전송. 캐시 위치 `%LOCALAPPDATA%\LocalFx\update-cache.json`.
- User-Agent: `local-fx/<버전> (+https://github.com/ssallem/local-fx)`
- 새 버전 감지 시 확장 토스트로 알림만 — **자동 설치하지 않습니다**. 사용자가 직접 위의 stable alias URL에서 받아 설치.
- 끄는 방법·환경 변수 `LOCALFX_DISABLE_UPDATE_CHECK=1` 등 상세는 [PRIVACY.md](PRIVACY.md#옵트인-업데이트-확인-t6).

## 확장에서 호스트 미감지 시 흐름

확장이 첫 실행 시 호스트 미등록(`E_HOST_NOT_FOUND`) → 새 탭에 온보딩 패널이 자동 표시됩니다. 패널의 "설치 파일 다운로드" 버튼이 위의 stable alias로 이동, 사용자가 인스톨러 실행 후 "다시 시도"를 누르면 즉시 활성화됩니다. (T3)

## 서명 부재 시 사용자 경험 (현재 v0.3.0)

T1 OV 코드 서명 평판이 충분히 누적되기 전까지는:

- Windows: SmartScreen "게시자를 확인할 수 없음" 경고 — 사용자가 "추가 정보" → "실행". 온보딩 패널 안내 카피에 이 우회법이 명시되어 있습니다.
- macOS: Gatekeeper "확인되지 않은 개발자" — 시스템 환경설정 → 보안에서 "확인 없이 열기" (Phase 4에서 notarization으로 해결 예정).

## Phase 4 향후 전략

- **Windows**: 현재 OV → EV 코드 서명으로 업그레이드(SmartScreen 즉시 신뢰), MSI 변환 검토.
- **macOS**: `productbuild` .pkg + Developer ID 서명 + Apple 공증(notarization).
- 자동 업데이트: Sparkle(macOS) / WinSparkle(Windows) 검토 — 현재 T6는 알림만, 자동 설치 없음.

## 릴리즈 운영자 첫 설정 (First-time setup for the release operator)

신규 릴리즈 운영자 워크스테이션 준비물:

- **SafeNet UCert USB 토큰** — DigiCert/Sectigo 등에서 발급한 OV/EV 코드 서명 인증서.
- **SafeNet Authentication Client (SAC)** 설치 + 토큰 PIN 등록. 서명 시 SAC가 PIN을 묻습니다.
- **인증서 thumbprint 확인** — `Get-ChildItem Cert:\CurrentUser\My | Format-List Subject, Thumbprint`. `Signing.psm1`이 이 thumbprint로 인증서를 선택합니다.
- **`gh` CLI** — `gh auth login`으로 GitHub repo `ssallem/local-fx`에 `repo` scope 권한.
- **Inno Setup 6** 로컬 설치 — `build-setup.ps1 -Sign` 등 로컬 테스트 빌드 시 필요. CI에선 runner가 자동 설치하므로 운영자 PC에는 검증용으로만.
- (선택) **RFC3161 timestamp authority URL** 환경 변수 — 기본값은 `Signing.psm1`에 하드코딩되어 있으나 만료 대비 override 가능.

> 운영자 PC에 비밀키가 없습니다. SafeNet 토큰을 뽑으면 `signtool`이 즉시 실패하므로 서명 자산이 유출 위험에 노출되지 않습니다.
