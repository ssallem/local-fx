import {
  HOST_NAME,
  type ErrorCode,
  type Request,
  type Response
} from "./types/shared";

type Pending = {
  resolve: (value: Response) => void;
  timeout: ReturnType<typeof setTimeout>;
};

const REQUEST_TIMEOUT_MS = 10_000;

// F-6: MV3 Service Worker goes idle after 30s. A chrome.alarms-based
// keepalive wakes the SW so pending requests don't silently disappear.
// Chrome 116+ clamps periodInMinutes to a minimum of 0.5 (30s); using that
// exact value keeps the SW warm without being rejected.
const KEEPALIVE_ALARM = "local-explorer-keepalive";
const KEEPALIVE_PERIOD_MIN = 0.5;

let port: chrome.runtime.Port | null = null;
const pending = new Map<string, Pending>();

chrome.runtime.onInstalled.addListener((details) => {
  console.info("[local-explorer] onInstalled", details.reason);
});

// F-6: alarm handler is a no-op — its only job is to wake the SW so
// setTimeout timers and the Port message listeners stay scheduled.
chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name !== KEEPALIVE_ALARM) return;
  // intentionally empty: wake-up side-effect is the whole point.
  return;
});

function startKeepalive(): void {
  // Creating the same-named alarm replaces the existing one, which is cheap.
  chrome.alarms.create(KEEPALIVE_ALARM, {
    periodInMinutes: KEEPALIVE_PERIOD_MIN
  });
}

function stopKeepalive(): void {
  // clear() is best-effort. Use the callback form so it works regardless
  // of whether @types/chrome reports a Promise or void return type.
  try {
    chrome.alarms.clear(KEEPALIVE_ALARM, () => {
      // discard lastError — absent alarm is not an error we care about.
      void chrome.runtime.lastError;
    });
  } catch {
    /* ignore */
  }
}

// F-6: onSuspend fires right before SW shutdown. The message channel is
// already closing so sendResponse may not actually reach the UI, but we
// still invoke failAll to (a) log and (b) give a last-ditch chance for
// the callback to flush. UI-side retry (ipc.ts) is the real recovery.
chrome.runtime.onSuspend.addListener(() => {
  if (pending.size === 0) return;
  console.warn(
    "[local-explorer] onSuspend with",
    pending.size,
    "pending — firing E_HOST_CRASH"
  );
  failAll(
    "E_HOST_CRASH",
    "Service worker suspended with pending requests",
    { mayNeedInstall: false, suspended: true }
  );
});

function failAll(
  code: ErrorCode,
  message: string,
  details?: Record<string, unknown>
): void {
  for (const [id, entry] of pending) {
    clearTimeout(entry.timeout);
    entry.resolve({
      id,
      ok: false,
      error: {
        code,
        message,
        retryable: true,
        ...(details ? { details } : {})
      }
    });
  }
  pending.clear();
  // Pending map is now empty — no reason to keep the SW alive.
  stopKeepalive();
}

function ensurePort(): chrome.runtime.Port | null {
  if (port) return port;
  try {
    const next = chrome.runtime.connectNative(HOST_NAME);
    next.onMessage.addListener((raw: unknown) => {
      if (!isResponse(raw)) {
        console.warn("[local-explorer] malformed host message", raw);
        return;
      }
      const entry = pending.get(raw.id);
      if (!entry) return;
      clearTimeout(entry.timeout);
      pending.delete(raw.id);
      if (pending.size === 0) stopKeepalive();
      entry.resolve(raw);
    });
    next.onDisconnect.addListener(() => {
      const err = chrome.runtime.lastError;
      const message = err?.message ?? "native host disconnected";
      // F-9: keyword match is brittle under Chrome locale changes. Keep
      // the E_HOST_NOT_FOUND fast-path for English, but fall back to
      // E_HOST_CRASH with details.mayNeedInstall so UI can still surface
      // an install hint even when we can't confidently classify.
      const looksMissing =
        /not found|not installed|Specified native messaging host not found/i.test(
          message
        );
      port = null;
      if (looksMissing) {
        failAll("E_HOST_NOT_FOUND", message, { mayNeedInstall: true });
      } else {
        failAll("E_HOST_CRASH", message, { mayNeedInstall: true });
      }
    });
    port = next;
    return next;
  } catch (e) {
    const message = e instanceof Error ? e.message : String(e);
    console.error("[local-explorer] connectNative failed", message);
    return null;
  }
}

function isResponse(value: unknown): value is Response {
  if (typeof value !== "object" || value === null) return false;
  const v = value as Record<string, unknown>;
  return typeof v.id === "string" && typeof v.ok === "boolean";
}

function isRequest(value: unknown): value is Request {
  if (typeof value !== "object" || value === null) return false;
  const v = value as Record<string, unknown>;
  return typeof v.id === "string" && typeof v.op === "string";
}

chrome.runtime.onMessage.addListener((message: unknown, _sender, sendResponse) => {
  if (!isRequest(message)) {
    sendResponse({
      id: "unknown",
      ok: false,
      error: {
        code: "E_PROTOCOL",
        message: "malformed request from UI",
        retryable: false
      }
    } satisfies Response);
    return false;
  }

  const req = message;
  // F-6: ensurePort BEFORE registering pending so a failed connect
  // returns immediately without leaving an orphaned entry behind.
  const active = ensurePort();
  if (!active) {
    sendResponse({
      id: req.id,
      ok: false,
      error: {
        code: "E_HOST_NOT_FOUND",
        message: "native host not available",
        retryable: true,
        details: { mayNeedInstall: true }
      }
    } satisfies Response);
    return false;
  }

  const timeout = setTimeout(() => {
    const entry = pending.get(req.id);
    if (!entry) return;
    pending.delete(req.id);
    if (pending.size === 0) stopKeepalive();
    entry.resolve({
      id: req.id,
      ok: false,
      error: {
        code: "E_TIMEOUT",
        message: `request timed out after ${REQUEST_TIMEOUT_MS}ms`,
        retryable: true
      }
    });
  }, REQUEST_TIMEOUT_MS);

  pending.set(req.id, {
    resolve: (value) => sendResponse(value),
    timeout
  });
  // F-6: pending Map is non-empty — ensure SW stays alive until resolved.
  startKeepalive();

  try {
    active.postMessage(req);
  } catch (e) {
    const entry = pending.get(req.id);
    if (entry) {
      clearTimeout(entry.timeout);
      pending.delete(req.id);
      if (pending.size === 0) stopKeepalive();
    }
    const message = e instanceof Error ? e.message : String(e);
    port = null;
    sendResponse({
      id: req.id,
      ok: false,
      error: {
        code: "E_HOST_CRASH",
        message,
        retryable: true,
        details: { mayNeedInstall: true }
      }
    } satisfies Response);
    return false;
  }

  return true;
});
