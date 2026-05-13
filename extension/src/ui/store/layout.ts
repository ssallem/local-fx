import { create } from "zustand";

// 좌측 Sidebar 폭과 하단 StatusBar 높이를 사용자 드래그로 조절할 수 있게
// 영속화하는 store. 본질적으로 UI 전용 prefs라 explorer.ts와 격리하여
// 별도 파일로 둔다. 패턴(localStorage 가드 + 300ms 디바운스 + sanitize) 은
// explorer.ts의 columnWidths 처리 방식을 그대로 따른다.

const LS_SIDEBAR_WIDTH_KEY = "explorer.sidebarWidth";
const LS_STATUSBAR_HEIGHT_KEY = "explorer.statusBarHeight";

const DEFAULT_SIDEBAR_WIDTH = 240;
const MIN_SIDEBAR_WIDTH = 160;
const MAX_SIDEBAR_WIDTH = 480;

const DEFAULT_STATUSBAR_HEIGHT = 28;
const MIN_STATUSBAR_HEIGHT = 28;
const MAX_STATUSBAR_HEIGHT = 240;

/**
 * Sanitize a persisted dimension. `localStorage`는 신뢰할 수 없는 입력 경계 —
 * 다른 탭, 확장, 손상된 쓰기 등이 문자열/`null`/객체/`NaN`을 남겨둘 수 있다.
 * 그대로 `Number()` + clamp 하면 `"NaNpx"` 로 grid template을 오염시켜
 * 레이아웃이 무너진다. 유한한 양의 정수가 아니면 default로 폴백한 뒤
 * [min, max] 범위로 clamp 한다.
 *
 * 참고: explorer.ts의 sanitizeWidth와 동일한 정책 — 단, 여기는 max도 본다.
 * 추출 리팩토링 금지 지시에 따라 본 파일에 인라인 복사해 두었다.
 */
function sanitizeWidth(
  val: unknown,
  defaultVal: number,
  min: number,
  max: number
): number {
  const n = Math.floor(Number(val));
  if (!Number.isFinite(n) || n <= 0) return defaultVal;
  if (n < min) return min;
  if (n > max) return max;
  return n;
}

function loadSidebarWidth(): number {
  if (typeof localStorage === "undefined") return DEFAULT_SIDEBAR_WIDTH;
  try {
    const raw = localStorage.getItem(LS_SIDEBAR_WIDTH_KEY);
    if (raw === null) return DEFAULT_SIDEBAR_WIDTH;
    // 단일 숫자도 JSON 파싱이 안전하고 stringify 한 값을 그대로 받아낼 수 있다.
    const parsed = JSON.parse(raw);
    return sanitizeWidth(
      parsed,
      DEFAULT_SIDEBAR_WIDTH,
      MIN_SIDEBAR_WIDTH,
      MAX_SIDEBAR_WIDTH
    );
  } catch {
    return DEFAULT_SIDEBAR_WIDTH;
  }
}

function loadStatusBarHeight(): number {
  if (typeof localStorage === "undefined") return DEFAULT_STATUSBAR_HEIGHT;
  try {
    const raw = localStorage.getItem(LS_STATUSBAR_HEIGHT_KEY);
    if (raw === null) return DEFAULT_STATUSBAR_HEIGHT;
    const parsed = JSON.parse(raw);
    return sanitizeWidth(
      parsed,
      DEFAULT_STATUSBAR_HEIGHT,
      MIN_STATUSBAR_HEIGHT,
      MAX_STATUSBAR_HEIGHT
    );
  } catch {
    return DEFAULT_STATUSBAR_HEIGHT;
  }
}

// 디바운스 영속화 — splitter drag는 mousemove마다 setter를 때리므로
// 매번 localStorage.setItem 하면 메인 스레드가 막혀 가시적 jank가 생긴다.
// 300ms 합치는 정책은 explorer.ts columnWidths 와 동일.
let sidebarWidthPersistTimer: number | undefined;
function persistSidebarWidthDebounced(v: number): void {
  if (typeof localStorage === "undefined") return;
  if (sidebarWidthPersistTimer !== undefined) {
    window.clearTimeout(sidebarWidthPersistTimer);
  }
  sidebarWidthPersistTimer = window.setTimeout(() => {
    try {
      localStorage.setItem(LS_SIDEBAR_WIDTH_KEY, JSON.stringify(v));
    } catch {
      // Quota / private-mode 실패는 fatal 아님 — 메모리 상태는 유효.
    }
    sidebarWidthPersistTimer = undefined;
  }, 300);
}

let statusBarHeightPersistTimer: number | undefined;
function persistStatusBarHeightDebounced(v: number): void {
  if (typeof localStorage === "undefined") return;
  if (statusBarHeightPersistTimer !== undefined) {
    window.clearTimeout(statusBarHeightPersistTimer);
  }
  statusBarHeightPersistTimer = window.setTimeout(() => {
    try {
      localStorage.setItem(LS_STATUSBAR_HEIGHT_KEY, JSON.stringify(v));
    } catch {
      // ignore — persistSidebarWidthDebounced 주석 참조.
    }
    statusBarHeightPersistTimer = undefined;
  }, 300);
}

export interface LayoutState {
  sidebarWidth: number;
  statusBarHeight: number;

  setSidebarWidth: (v: number) => void;
  setStatusBarHeight: (v: number) => void;
}

export const useLayoutStore = create<LayoutState>((set, get) => ({
  sidebarWidth: loadSidebarWidth(),
  statusBarHeight: loadStatusBarHeight(),

  setSidebarWidth(v: number) {
    // NaN / 비유한 / 0 이하 입력은 조용히 무시 — grid template 오염 차단.
    const n = Math.floor(Number(v));
    if (!Number.isFinite(n) || n <= 0) return;
    const clamped =
      n < MIN_SIDEBAR_WIDTH
        ? MIN_SIDEBAR_WIDTH
        : n > MAX_SIDEBAR_WIDTH
          ? MAX_SIDEBAR_WIDTH
          : n;
    if (get().sidebarWidth === clamped) return;
    set({ sidebarWidth: clamped });
    persistSidebarWidthDebounced(clamped);
  },

  setStatusBarHeight(v: number) {
    const n = Math.floor(Number(v));
    if (!Number.isFinite(n) || n <= 0) return;
    const clamped =
      n < MIN_STATUSBAR_HEIGHT
        ? MIN_STATUSBAR_HEIGHT
        : n > MAX_STATUSBAR_HEIGHT
          ? MAX_STATUSBAR_HEIGHT
          : n;
    if (get().statusBarHeight === clamped) return;
    set({ statusBarHeight: clamped });
    persistStatusBarHeightDebounced(clamped);
  }
}));

// beforeunload flush — 300ms 디바운스 타이머가 fire되기 전에 페이지가 닫히면
// 마지막 드래그 값이 영영 손실된다(특히 drag 직후 곧장 탭/창 종료 시).
// 미flush된 타이머가 있으면 강제로 cleanup 하고 store.getState() 현재 값을
// 동기 쓰기로 localStorage에 박아 넣는다. 안전망일 뿐이라 try/catch 로 감쌈.
if (typeof window !== "undefined") {
  window.addEventListener("beforeunload", () => {
    try {
      if (sidebarWidthPersistTimer !== undefined) {
        window.clearTimeout(sidebarWidthPersistTimer);
        sidebarWidthPersistTimer = undefined;
        const v = useLayoutStore.getState().sidebarWidth;
        localStorage.setItem(LS_SIDEBAR_WIDTH_KEY, JSON.stringify(v));
      }
      if (statusBarHeightPersistTimer !== undefined) {
        window.clearTimeout(statusBarHeightPersistTimer);
        statusBarHeightPersistTimer = undefined;
        const v = useLayoutStore.getState().statusBarHeight;
        localStorage.setItem(LS_STATUSBAR_HEIGHT_KEY, JSON.stringify(v));
      }
    } catch {
      // Quota / private-mode / localStorage 부재 시 silent — 어차피 종료 직전.
    }
  });
}
