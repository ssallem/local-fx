# Chrome Web Store Publishing Guide

## 사전 준비

1. Chrome Web Store Developer 계정 — $5 USD 일회성 등록비
   - https://chrome.google.com/webstore/devconsole
2. Privacy Policy 공개 URL (본 repo의 PRIVACY.md를 GitHub Pages로)
3. 스크린샷 5장 (1280x800 또는 640x400 권장, PNG)
   - 드라이브 목록 / 파일 리스트 / 컨텍스트 메뉴 / 진행률 토스트 / 충돌 다이얼로그
4. 홍보 이미지 (선택)
   - 440x280 "Small promo tile"
   - 1400x560 "Marquee"

## 빌드 순서

```bash
cd extension
npm install
npm run icons      # SVG → PNG 3 사이즈
npm run package    # vite build + key 제거 + zip
```

결과: `extension/dist-prod/localfx-v0.2.0.zip` + `SHASUM256.txt`

## 업로드

1. Developer Dashboard → "새 항목" → zip 업로드
2. 스토어 리스팅:
   - **제품 이름**: Local Explorer
   - **요약(132자)**: "브라우저 새 탭에서 로컬 드라이브를 탐색하고 파일을 만들기·이름변경·복사·이동·삭제할 수 있는 확장."
   - **상세 설명**: 기능 목록 + Native Host 설치 안내 링크
   - **카테고리**: Productivity
   - **언어**: 한국어 + 영어 (선택)
3. 개인정보 처리방침 URL 입력 (GitHub Pages 배포된 PRIVACY.md)
4. Native Host 설치 링크: README의 설치 섹션
5. 심사 제출

## 심사 포인트 (Chrome Web Store 정책 대응)

- [O] nativeMessaging 권한 사용 사유 명시 ("로컬 파일 시스템 접근을 위한 Native Host와 통신")
- [O] Host 바이너리 별도 배포 및 manifest `allowed_origins` 제한 문서화
- [X] 원격 코드 실행 없음
- [X] 사용자 데이터 수집·전송 없음
- [X] 광고·분석 미포함

보통 3~5 영업일 소요.

## 업데이트 공개

버전 bump (`manifest.json.version`) → `npm run package` → 동일 항목에 새 zip 업로드.
