# Native Host Distribution

Chrome Web Store는 확장만 배포. Native Host는 별도 채널 필요.

## 현재 전략 (Phase 2)

GitHub Releases로 Native Host 바이너리 + 설치 스크립트 배포:
- `fx-host-windows-amd64.exe`
- `fx-host-darwin-arm64`
- `fx-host-darwin-amd64`
- `install.ps1` / `install.sh` / `generate-dev-key.ps1/.sh`

## Phase 4 프로덕션 전략

- **Windows**: WiX Toolset v4로 MSI 빌드 + Authenticode 서명 ($~400/yr)
- **macOS**: `productbuild` .pkg + Developer ID 서명 ($99/yr Apple Developer) + Apple 공증(notarization)
- 자동 업데이트: Sparkle(macOS) / WinSparkle(Windows) 또는 수동

## 확장 리스팅에서 Host 안내

확장 스토어 페이지 "상세 설명"에 다음 문구 포함:

> This extension requires a local Native Messaging Host application.
> After installing the extension, download and run the installer from:
> https://github.com/example/local-fx/releases

또는 확장이 첫 실행 시 Host 미등록 감지(`E_HOST_NOT_FOUND`) → UI에 다운로드 링크 표시. 현재 구현의 ErrorBanner가 `details.mayNeedInstall` 플래그로 표시.

## 서명 부재 시 사용자 경험

- Windows: SmartScreen "게시자를 확인할 수 없음" 경고 — 사용자가 "추가 정보" → "실행"
- macOS: Gatekeeper "확인되지 않은 개발자" — 시스템 환경설정 → 보안에서 "확인 없이 열기"
- 문서에 해당 우회법 포함 권장.
