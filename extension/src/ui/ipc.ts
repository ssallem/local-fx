import type {
  Drive,
  EntryMeta,
  ErrorResponse,
  OpArgsMap,
  OpDataMap,
  OpNoArgs,
  ReaddirArgs,
  ReaddirData,
  Request,
  Response
} from "../types/shared";

// PROTOCOL.md §4: Phase 1+ handshake requires the extension to advertise the
// protocol version it speaks on every request. Bumping this constant here is
// the single point of truth on the UI side; the Go host compares against
// hostMaxProtocolVersion and rejects with E_PROTOCOL when we exceed it.
const PROTOCOL_VERSION = 1;

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
