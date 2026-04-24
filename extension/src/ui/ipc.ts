import type {
  CancelArgs,
  CopyArgs,
  Drive,
  EmptyData,
  EntryMeta,
  ErrorResponse,
  MkdirArgs,
  MoveArgs,
  OpArgsMap,
  OpDataMap,
  OpNoArgs,
  ReaddirArgs,
  ReaddirData,
  RemoveArgs,
  RenameArgs,
  Request,
  Response,
  StreamEvent
} from "../types/shared";

// PROTOCOL.md §4: Phase 1+ handshake requires the extension to advertise the
// protocol version it speaks on every request. Bumping this constant here is
// the single point of truth on the UI side; the Go host compares against
// hostMaxProtocolVersion and rejects with E_PROTOCOL when we exceed it.
//
// Phase 2.1 bumped this to 2 alongside the mutation ops (mkdir / rename /
// remove / open / revealInOsExplorer). The Go Host's hostMaxProtocolVersion
// is also 2 — do NOT advance this past 2 until the Host follows.
const PROTOCOL_VERSION = 2;

// F-8: MV3 service worker / modern browser UI always exposes
// crypto.randomUUID(). No fallback needed — call it directly so every
// id is a real UUIDv4 per PROTOCOL.md §2 ("확장이 UUIDv4로 생성한다").
function newId(): string {
  return crypto.randomUUID();
}

// F-6: one automatic retry when the SW disappears mid-flight. Matches
// PROTOCOL.md §9.6 ("확장이 1회 자동 재연결 시도"). We retry only for
// transient transport failures where a fresh SW can plausibly succeed.
function shouldRetry(resp: Response<unknown>): resp is ErrorResponse {
  if (resp.ok) return false;
  const code = resp.error.code;
  return code === "E_HOST_CRASH" || code === "E_TIMEOUT";
}

async function sendOnce<O extends keyof OpArgsMap>(
  req: Request
): Promise<Response<OpDataMap[O]>> {
  return new Promise((resolve) => {
    try {
      chrome.runtime.sendMessage(req, (resp: unknown) => {
        const err = chrome.runtime.lastError;
        if (err) {
          // SW was torn down before the response arrived — surface as
          // E_HOST_CRASH so the outer retry layer picks it up.
          resolve({
            id: req.id,
            ok: false,
            error: {
              code: "E_HOST_CRASH",
              message: err.message ?? "runtime error",
              retryable: true
            }
          });
          return;
        }
        resolve(resp as Response<OpDataMap[O]>);
      });
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e);
      resolve({
        id: req.id,
        ok: false,
        error: { code: "E_PROTOCOL", message, retryable: false }
      });
    }
  });
}

// buildRequest centralises the wire-frame shape so both first-try and retry
// produce identical envelopes (modulo a fresh id). `args === undefined` is
// omitted entirely to match Go's `omitempty` on Args — avoids sending a
// literal `"args": null` that the dispatcher would have to tolerate.
function buildRequest<O extends keyof OpArgsMap>(
  op: O,
  args: OpArgsMap[O]
): Request {
  const base = {
    id: newId(),
    op,
    protocolVersion: PROTOCOL_VERSION
  } satisfies Omit<Request, "args">;
  return args === undefined ? base : { ...base, args };
}

// Overloads give each call site the narrowest possible type:
//   - no-args ops (ping, listDrives) may omit the 2nd parameter
//   - readdir/stat require their own args shape
// Implementation signature below is the permissive union.

export async function request<O extends OpNoArgs>(
  op: O
): Promise<Response<OpDataMap[O]>>;
export async function request<O extends keyof OpArgsMap>(
  op: O,
  args: OpArgsMap[O]
): Promise<Response<OpDataMap[O]>>;
export async function request<O extends keyof OpArgsMap>(
  op: O,
  args?: OpArgsMap[O]
): Promise<Response<OpDataMap[O]>> {
  const req = buildRequest(op, args as OpArgsMap[O]);
  const first = await sendOnce<O>(req);
  if (!shouldRetry(first)) return first;

  // F-6: single retry with a fresh id so the SW treats it as a new
  // request. The old id may still be tracked in a revived pending Map.
  const retryReq = buildRequest(op, args as OpArgsMap[O]);
  return sendOnce<O>(retryReq);
}

// -----------------------------------------------------------------------------
// Convenience helpers — unwrap Response<T> into T on success, throw otherwise.
//
// UI layers that want raw access to `ok: false` error payloads should keep
// calling request() directly. These helpers exist for the common case where
// a component already wraps the call in try/catch and just wants the data.
// -----------------------------------------------------------------------------

// IpcError preserves the full wire-level error shape (code/message/retryable
// /details) so consumers can switch on `err.code` without re-parsing the
// response. Matches PROTOCOL.md §8 ErrorCode catalog.
export class IpcError extends Error {
  readonly code: ErrorResponse["error"]["code"];
  readonly retryable: boolean;
  readonly details?: Record<string, unknown>;

  constructor(err: ErrorResponse["error"]) {
    super(err.message);
    this.name = "IpcError";
    this.code = err.code;
    this.retryable = err.retryable;
    if (err.details !== undefined) this.details = err.details;
  }
}

function unwrap<T>(resp: Response<T>): T {
  if (resp.ok) return resp.data;
  throw new IpcError(resp.error);
}

// Void-returning variant for Phase 2.1 mutation ops whose success payload is
// the empty object `{}`. Sharing the error path with unwrap() keeps IpcError
// construction consistent across all helpers; we just drop the empty data on
// the floor instead of returning it.
function unwrapVoid(resp: Response<EmptyData>): void {
  if (!resp.ok) throw new IpcError(resp.error);
}

export async function listDrives(): Promise<Drive[]> {
  const resp = await request("listDrives");
  return unwrap(resp).drives;
}

export async function readdir(args: ReaddirArgs): Promise<ReaddirData> {
  const resp = await request("readdir", args);
  return unwrap(resp);
}

export async function stat(path: string): Promise<EntryMeta> {
  const resp = await request("stat", { path });
  return unwrap(resp);
}

// --- Phase 2.1 mutation helpers ---------------------------------------------
//
// These wrap request() the same way listDrives/readdir/stat do: build the
// envelope, throw IpcError on failure, resolve `void` on success. Callers
// that need the raw Response<EmptyData> (e.g. to branch on
// E_SYSTEM_PATH_CONFIRM_REQUIRED without catch) should keep using
// request("mkdir", ...) directly.

export async function mkdir(args: MkdirArgs): Promise<void> {
  const resp = await request("mkdir", args);
  unwrapVoid(resp);
}

export async function rename(args: RenameArgs): Promise<void> {
  const resp = await request("rename", args);
  unwrapVoid(resp);
}

export async function remove(args: RemoveArgs): Promise<void> {
  const resp = await request("remove", args);
  unwrapVoid(resp);
}

// Named openEntry / revealEntry rather than open / reveal to avoid clashing
// with the DOM globals `window.open` and future reveal-style APIs when
// consumers do `import * as ipc`.
export async function openEntry(path: string): Promise<void> {
  const resp = await request("open", { path });
  unwrapVoid(resp);
}

export async function revealEntry(path: string): Promise<void> {
  const resp = await request("revealInOsExplorer", { path });
  unwrapVoid(resp);
}

// -----------------------------------------------------------------------------
// Phase 2.3 — streaming op helpers (copy / cancel) + EventFrame routing.
//
// background.ts broadcasts every Host EventFrame as
// `chrome.runtime.sendMessage({ kind: "host-event", event })`. Each open
// extension page receives that message; we register a single onMessage
// listener here and dispatch by request id to per-id callbacks registered
// via requestStream(). The terminal "done" event clears the listener;
// callers may also clear it explicitly when their stream resolves with an
// error before any "done" frame arrives.
// -----------------------------------------------------------------------------

type StreamListener = (event: StreamEvent) => void;
const streamListeners = new Map<string, StreamListener>();

// One-time install: chrome.runtime.onMessage stays registered for the page's
// lifetime, which matches the SW's lifetime for our purposes (the SW restarts
// the listener side). The listener is idempotent — registering twice would
// just no-op the second time, but we guard with a module-level flag so hot
// reloads in dev don't pile up handlers.
let streamListenerInstalled = false;
function ensureStreamListenerInstalled(): void {
  if (streamListenerInstalled) return;
  streamListenerInstalled = true;
  chrome.runtime.onMessage.addListener((msg: unknown) => {
    if (
      typeof msg !== "object" ||
      msg === null ||
      (msg as { kind?: unknown }).kind !== "host-event"
    ) {
      return false;
    }
    const evt = (msg as { event?: unknown }).event as StreamEvent | undefined;
    if (!evt || typeof evt.id !== "string") return false;
    const listener = streamListeners.get(evt.id);
    if (!listener) return false;
    try {
      listener(evt);
    } catch (e) {
      // Defensive: a throwing listener should not poison the dispatch loop.
      // eslint-disable-next-line no-console
      console.error("[ipc] stream listener threw", e);
    }
    if (evt.event === "done") {
      streamListeners.delete(evt.id);
    }
    return false;
  });
}

// StreamHandle bundles the request id (so the caller can correlate with later
// telemetry / cancel calls), the resolution promise (the terminal Response),
// and an idempotent cancel() that fires the §7.13 cancel op.
export interface StreamHandle<T = unknown> {
  id: string;
  promise: Promise<Response<T>>;
  cancel: () => Promise<void>;
}

/**
 * Issue a streaming op, routing every EventFrame for its id to onEvent until
 * the terminal Response arrives. Caller owns the lifetime of onEvent — once
 * the returned promise settles (or the "done" event fires), the listener is
 * removed automatically.
 *
 * Streaming requests intentionally bypass the F-6 retry path: a half-emitted
 * stream cannot be safely re-driven from scratch without coordinating with
 * the Host on which frames have already been delivered. The single-shot
 * sendOnce path is adequate; transient failures surface as a normal error
 * Response and the caller decides whether to restart.
 */
export function requestStream<O extends keyof OpArgsMap>(
  op: O,
  args: OpArgsMap[O],
  onEvent: StreamListener
): StreamHandle<OpDataMap[O]> {
  ensureStreamListenerInstalled();
  const req = buildRequest(op, args);
  // Mark stream:true so the SW/Host treat it as a streaming op (PROTOCOL §6).
  const streamReq: Request = { ...req, stream: true };
  streamListeners.set(req.id, onEvent);

  const promise = sendOnce<O>(streamReq).finally(() => {
    // Defensive cleanup: if the Host returned ok:false BEFORE emitting "done"
    // (e.g. EACCES at open-time), there is no terminal event to drop the
    // listener. Always clear here.
    streamListeners.delete(req.id);
  });

  const cancelHandle = async (): Promise<void> => {
    await cancel(req.id);
  };

  return { id: req.id, promise, cancel: cancelHandle };
}

/**
 * Convenience wrapper for the most common streaming op. Mirrors readdir/stat
 * style: pass typed args + a callback, get back a StreamHandle.
 */
export function copyFile(
  args: CopyArgs,
  onEvent: StreamListener
): StreamHandle<EmptyData> {
  return requestStream("copy", args, onEvent);
}

/**
 * Phase 2.4 — streaming move. Same shape as copyFile; the Host treats
 * intra-volume moves as a fast OS rename and falls back to copy+remove
 * across volumes. Conflict resolution MUST be settled UI-side: the Host
 * rejects MoveArgs.conflict === "prompt" with E_BAD_REQUEST.
 */
export function moveFile(
  args: MoveArgs,
  onEvent: StreamListener
): StreamHandle<EmptyData> {
  return requestStream("move", args, onEvent);
}

/**
 * Cancel an in-flight streaming op by its request id (PROTOCOL §7.13).
 * Resolves to `accepted`: true when the Host acknowledged a known target,
 * false when the id was unknown (already finished, never seen, etc.).
 *
 * Errors at the transport layer (E_TIMEOUT / E_HOST_CRASH on the cancel
 * call itself) collapse to `false` so callers can treat the boolean as
 * "did the cancel actually take effect". The original op's terminal "done"
 * event remains the source of truth for `canceled: true`.
 */
export async function cancel(targetId: string): Promise<boolean> {
  const args: CancelArgs = { targetId };
  const resp = await request("cancel", args);
  if (!resp.ok) return false;
  return resp.data.accepted === true;
}
