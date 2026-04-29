import {
  HOST_NAME,
  type CheckUpdateData,
  type ErrorCode,
  type Request,
  type Response,
  type StreamEvent
} from "./types/shared";

type Pending = {
  resolve: (value: Response) => void;
  timeout: ReturnType<typeof setTimeout>;
};

const REQUEST_TIMEOUT_MS = 10_000;
// WARNING-P2-R4-1 — streaming ops (copy/move/search) can take minutes for
// large payloads. A 10s cap would surface a bogus E_TIMEOUT while the Host
// is still streaming progress events and the job completes normally via
// the Event channel. 24h is effectively unlimited for user-initiated
// paste/move; the cancel op provides the real escape hatch.
const STREAM_TIMEOUT_MS = 24 * 60 * 60 * 1000;

// F-6: MV3 Service Worker goes idle after 30s. A chrome.alarms-based
// keepalive wakes the SW so pending requests don't silently disappear.
// Chrome 116+ clamps periodInMinutes to a minimum of 0.5 (30s); using that
// exact value keeps the SW warm without being rejected.
const KEEPALIVE_ALARM = "local-explorer-keepalive";
const KEEPALIVE_PERIOD_MIN = 0.5;

// T6 — opt-in update check. Constants are duplicated (rather than imported
// from UpdateCheckSettings.tsx) so the SW bundle doesn't pull in the React
// component graph. A rename in either place would be caught by the
// alarm-name mismatch in Chrome's behaviour, surfacing fast.
const UPDATE_CHECK_ALARM = "checkUpdate";
const UPDATE_CHECKS_ENABLED_KEY = "updateChecksEnabled";
const UPDATE_PROTOCOL_VERSION = 2;

let port: chrome.runtime.Port | null = null;
const pending = new Map<string, Pending>();

chrome.runtime.onInstalled.addListener((details) => {
  console.info("[local-explorer] onInstalled", details.reason);
});

// F-6: alarm handler is a no-op — its only job is to wake the SW so
// setTimeout timers and the Port message listeners stay scheduled.
//
// T6: when the alarm name is UPDATE_CHECK_ALARM, dispatch the opt-in
// update check. The handler is async-aware via void promise to keep the
// addListener callback synchronous (Chrome's alarm contract).
chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === KEEPALIVE_ALARM) {
    // intentionally empty: wake-up side-effect is the whole point.
    return;
  }
  if (alarm.name === UPDATE_CHECK_ALARM) {
    void runUpdateCheck();
    return;
  }
});

// T6 — read the opt-in toggle from chrome.storage.sync. Treats absent /
// failed reads as `false` so a sync hiccup can never accidentally enable
// the network path (defense in depth alongside the host's env-var gate).
async function isUpdateCheckEnabled(): Promise<boolean> {
  return new Promise((resolve) => {
    try {
      chrome.storage.sync.get(UPDATE_CHECKS_ENABLED_KEY, (items: unknown) => {
        if (chrome.runtime.lastError) {
          resolve(false);
          return;
        }
        const obj = items as Record<string, unknown>;
        resolve(obj[UPDATE_CHECKS_ENABLED_KEY] === true);
      });
    } catch {
      resolve(false);
    }
  });
}

// T6 — module-level in-flight guard. Two alarms can fire close together
// (e.g. user toggles off → on quickly, or Chrome catches up missed alarms
// after an SW wake) — without this guard each fire would issue its own
// IPC and broadcast a duplicate update toast. console.warn (NOT console.log)
// is intentional: warnings are visible in chrome://serviceworker-internals
// for diagnosing skipped fires without polluting normal-path logs.
let updateCheckInFlight = false;

// T6 — fire a checkUpdate IPC and broadcast the result to UI tabs.
// Reuses ensurePort()/pending machinery so the alarm-driven request rides
// the same SW lifecycle plumbing as user-initiated ops.
async function runUpdateCheck(): Promise<void> {
  if (updateCheckInFlight) {
    console.warn(
      "[fx] runUpdateCheck: previous run still in flight, skipping this alarm fire"
    );
    return;
  }
  updateCheckInFlight = true;
  try {
    await runUpdateCheckImpl();
  } finally {
    updateCheckInFlight = false;
  }
}

async function runUpdateCheckImpl(): Promise<void> {
  if (!(await isUpdateCheckEnabled())) {
    // Toggle is OFF — never reach the network. The alarm should already
    // be cleared by UpdateCheckSettings.writeEnabled(false), but a stale
    // alarm could survive a settings-store rollback; this guard makes
    // the property "no toggle = no traffic" hold regardless.
    return;
  }
  const active = ensurePort();
  if (!active) return; // Host unavailable — silently retry on next tick.

  const id = crypto.randomUUID();
  const req: Request = {
    id,
    op: "checkUpdate",
    protocolVersion: UPDATE_PROTOCOL_VERSION
  };

  // 30s ceiling — generous because the host's own httpClient timeout is
  // 10s and we want to give it room to retry once before declaring
  // failure SW-side. We don't surface this timeout to UI; a missed check
  // just means the user waits another 24h.
  const TIMEOUT_MS = 30_000;
  const timeout = setTimeout(() => {
    const entry = pending.get(id);
    if (!entry) return;
    pending.delete(id);
    if (pending.size === 0) stopKeepalive();
    entry.resolve({
      id,
      ok: false,
      error: { code: "E_TIMEOUT", message: "checkUpdate timed out", retryable: true }
    });
  }, TIMEOUT_MS);

  const respPromise = new Promise<Response>((resolve) => {
    pending.set(id, { resolve, timeout });
  });
  startKeepalive();

  try {
    active.postMessage(req);
  } catch {
    const entry = pending.get(id);
    if (entry) {
      clearTimeout(entry.timeout);
      pending.delete(id);
      if (pending.size === 0) stopKeepalive();
    }
    return;
  }

  const resp = (await respPromise) as Response<CheckUpdateData>;
  if (!resp.ok) {
    if (resp.error.code === "E_DISABLED") {
      // Host respects LOCALFX_DISABLE_UPDATE_CHECK=1 — surface a
      // separate broadcast so the UI can show an info message rather
      // than silently doing nothing.
      broadcastToTabs({ kind: "update-host-disabled" });
    }
    return;
  }
  if (!resp.data.hasUpdate) return; // No new version — nothing to surface.
  broadcastToTabs({ kind: "update-available", payload: resp.data });
}

// T6 — fire-and-forget broadcast helper. chrome.runtime.sendMessage with
// no tab target reaches every extension page; if no listeners are open
// the promise rejects with "Could not establish connection" which we
// silently swallow — same pattern as broadcastEvent below.
function broadcastToTabs(message: unknown): void {
  try {
    const maybePromise = chrome.runtime.sendMessage(message);
    if (
      maybePromise &&
      typeof (maybePromise as Promise<unknown>).catch === "function"
    ) {
      (maybePromise as Promise<unknown>).catch(() => {
        /* no listeners — expected when no extension tab is open */
      });
    }
  } catch {
    /* ignore */
  }
}

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
      // Phase 2.3: streaming ops (copy / future move / search) interleave
      // mid-stream Event frames with their terminal Response. Event frames
      // are identified by the presence of a string `event` field and
      // broadcast to every extension page so any open tab's ipc.ts listener
      // can correlate by request id. The final Response still flows through
      // the pending Map below — unchanged from Phase 2.1 semantics.
      if (isEventFrame(raw)) {
        broadcastEvent(raw);
        return;
      }
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

// Phase 2.3 — EventFrame carries { id, event, payload } and never an `ok`
// field (PROTOCOL.md §6). The Go side emits these via WriteFrame on the
// same stdout pipe as the final Response, serialised by SafeWriter. We key
// detection on the string `event` discriminator so a malformed frame lacking
// both `ok` and `event` falls through to the "malformed host message" warn.
function isEventFrame(value: unknown): value is StreamEvent {
  if (typeof value !== "object" || value === null) return false;
  const v = value as Record<string, unknown>;
  return (
    typeof v.id === "string" &&
    typeof v.event === "string" &&
    (v.event === "progress" || v.event === "item" || v.event === "done")
  );
}

// Phase 2.3 — broadcast Host Event frames to every extension page so a
// UI-side `chrome.runtime.onMessage` listener (registered in ipc.ts) can
// route by request id. `chrome.runtime.sendMessage` with no tab target
// delivers to all open extension pages + popups + the SW itself; we wrap
// the Event in `{ kind: "host-event" }` so ipc.ts can distinguish it from
// a UI-originated Request that would collide on this same channel. The
// sendMessage promise rejects with "Could not establish connection" when
// no listeners are registered yet — swallow it; it is the normal state
// during early startup before the newtab page mounts.
function broadcastEvent(evt: StreamEvent): void {
  try {
    const maybePromise = chrome.runtime.sendMessage({
      kind: "host-event",
      event: evt
    });
    // @types/chrome models sendMessage as returning a Promise in MV3. Guard
    // defensively in case a runtime returns void (older typings / tests).
    if (maybePromise && typeof (maybePromise as Promise<unknown>).catch === "function") {
      (maybePromise as Promise<unknown>).catch(() => {
        /* no listeners — expected during early startup */
      });
    }
  } catch {
    /* sendMessage threw synchronously — no receivers, ignore */
  }
}

function isRequest(value: unknown): value is Request {
  if (typeof value !== "object" || value === null) return false;
  const v = value as Record<string, unknown>;
  return typeof v.id === "string" && typeof v.op === "string";
}

chrome.runtime.onMessage.addListener((message: unknown, _sender, sendResponse) => {
  // Phase 2.3: host-event broadcasts originate from THIS service worker
  // (broadcastEvent above). chrome.runtime.sendMessage delivers to every
  // extension context including the sender, so the SW receives its own
  // broadcast here — drop silently and do NOT call sendResponse, letting
  // the real UI-side listener in ipc.ts handle routing.
  if (
    typeof message === "object" &&
    message !== null &&
    (message as { kind?: unknown }).kind === "host-event"
  ) {
    return false;
  }

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

  const timeoutMs = req.stream ? STREAM_TIMEOUT_MS : REQUEST_TIMEOUT_MS;
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
        message: `request timed out after ${timeoutMs}ms`,
        retryable: true
      }
    });
  }, timeoutMs);

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
