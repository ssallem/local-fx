export const HOST_NAME = "com.local.fx" as const;

// Phase 1: ping (Phase 0) + listDrives / readdir / stat are live.
// Phase 2.1: non-streaming mutation ops (mkdir, rename, remove, open,
// revealInOsExplorer) added — Host bumped hostMaxProtocolVersion to 2.
// Remaining ops (writeFile, readFile, copy, move, search, cancel) are
// defined in PROTOCOL.md §5/§7 and land in Phase 3. Keep this union in
// lock-step with the Go dispatcher's registered handler table
// (native-host/internal/ops).
export type Op =
  | "ping"
  | "listDrives"
  | "readdir"
  | "stat"
  // Phase 2.1 — non-streaming mutations
  | "mkdir"
  | "rename"
  | "remove"
  | "open"
  | "revealInOsExplorer";

export interface Request<T = unknown> {
  id: string;
  op: Op;
  args?: T;
  // PROTOCOL.md §6: set true on ops that emit Event frames (copy/move/search).
  // Phase 0 ping never sets it; omit to keep the wire payload minimal.
  stream?: boolean;
  // PROTOCOL.md §4: version negotiated on the first handshake request.
  // Phase 0 omits it (Host treats absent field as legacy). Phase 1+ MUST send.
  protocolVersion?: number;
}

export interface SuccessResponse<T = unknown> {
  id: string;
  ok: true;
  data: T;
}

export interface ErrorResponse {
  id: string;
  ok: false;
  error: {
    code: ErrorCode;
    message: string;
    retryable: boolean;
    // Optional structured context from Host or SW (e.g. wrapped: "...",
    // hostMaxVersion: 1, mayNeedInstall: true). Go side mirrors this.
    details?: Record<string, unknown>;
  };
}

export type Response<T = unknown> = SuccessResponse<T> | ErrorResponse;

// ErrorCode catalog — authoritative source is PROTOCOL.md §8 (20 codes).
// Plus two transport-level codes used by the extension SW/UI:
//   - E_TIMEOUT : request exceeded REQUEST_TIMEOUT_MS (extension-local)
//   - E_UNKNOWN : unclassified fallback (extension-local)
// Keep this union in lock-step with PROTOCOL.md §8 + transport additions.
// Total: 22 codes (20 §8 + 2 transport).
export type ErrorCode =
  // --- PROTOCOL.md §8 catalog (20) ---
  | "EACCES"
  | "ENOENT"
  | "EIO"
  | "E_TOO_LARGE"
  | "EEXIST"
  | "ENOSPC"
  | "ERROR_SHARING_VIOLATION"
  | "E_TRASH_UNAVAILABLE"
  | "EINVAL"
  | "E_NO_HANDLER"
  | "E_HOST_NOT_FOUND"
  | "E_FRAME_TOO_LARGE"
  | "E_HOST_CRASH"
  | "E_PROTOCOL"
  | "E_PATH_REJECTED"
  | "E_CANCELED"
  | "E_SYSTEM_PATH_CONFIRM_REQUIRED"
  // --- dispatch-level codes emitted by the Go host (PROTOCOL.md §8) ---
  // All three are retryable:false — they signal programmer/protocol bugs,
  // not transient conditions. Mirrors native-host/internal/protocol/errors.go.
  | "E_UNKNOWN_OP"
  | "E_BAD_REQUEST"
  | "E_INTERNAL"
  // --- extension-local transport codes (not in §8) ---
  | "E_TIMEOUT"
  | "E_UNKNOWN";

// -----------------------------------------------------------------------------
// op payload schemas — authority: docs/PROTOCOL.md §7
// Wire format is fixed by the Go Host (native-host/internal/ops/*.go,
// native-host/internal/platform/platform.go). Fields below mirror those
// structs 1:1; any divergence is a bug on one of the two sides.
// -----------------------------------------------------------------------------

// --- 7.1 ping ---------------------------------------------------------------

export interface PingArgs {
  // Client-side wall clock at send time. Host echoes serverTs in its reply
  // so the extension can compute RTT without relying on the response arrival
  // time, which is quantised by Chrome's message pump.
  clientTs?: number;
}

export interface PingData {
  pong: true;
  // Go host still emits the older shape (version + os) as well as the new
  // hostVersion/hostMaxProtocolVersion/serverTs trio. Both sets of fields
  // are optional here so the type stays robust across Host versions.
  version?: string;
  os?: string;
  hostVersion?: string;
  hostMaxProtocolVersion?: number;
  serverTs?: number;
}

// --- 7.2 listDrives ---------------------------------------------------------

// Authority: PROTOCOL.md §7.2 + native-host/internal/platform/platform.go
// (type Drive struct).
//
// NOTE on totalBytes / freeBytes: the Go struct tags these `omitempty`, so a
// drive whose capacity could not be probed (e.g. offline/optical media) will
// emit no field at all rather than `0`. The authoritative type for Phase 1
// clients keeps them required `number`; if we ever see them dropped at
// runtime, relax to `number | undefined` with a migration note.
export interface Drive {
  path: string;   // "C:\\" on Windows, "/" or "/Volumes/Foo" on macOS
  label: string;  // volume label; may be empty string
  fsType: string; // "NTFS" | "APFS" | "exFAT" | ...
  totalBytes: number;
  freeBytes: number;
  readOnly: boolean;
}

export interface ListDrivesData {
  drives: Drive[];
}

// --- 7.3 readdir ------------------------------------------------------------

// Sort field enum matches Go's sortEntries switch in
// native-host/internal/ops/readdir.go: "name" | "size" | "modified" | "type".
// Note: spec docs sometimes write "modifiedTs" but the wire value is
// "modified" — keep this union in sync with Go to avoid silent fallback.
export type SortField = "name" | "size" | "modified" | "type";
export type SortOrder = "asc" | "desc";

export interface Sort {
  field: SortField;
  order?: SortOrder; // defaults to "asc" on the Host side
}

export interface ReaddirArgs {
  path: string;
  // Page is 0-based (Go host convention); default 0 = first page.
  // pageSize defaults to 1000 and is capped at 5000 by the Host;
  // exceeding either is an EINVAL.
  page?: number;
  pageSize?: number;
  // Opaque cursor returned by a prior readdir call as `nextCursor`. When
  // provided, page/pageSize may be ignored by the Host.
  cursor?: string;
  sort?: Sort;
  includeHidden?: boolean;
}

// Authority: native-host/internal/ops/readdir.go (readdirEntry).
// - sizeBytes is explicitly `number | null` — directories serialise to JSON
//   null rather than 0 so a zero-byte file stays distinguishable.
// - modifiedTs is unix milliseconds (number), NOT an RFC 3339 string.
//   UI should wrap with `new Date(ms)` at render time.
export interface Entry {
  name: string;
  path: string;
  type: "file" | "directory" | "symlink";
  sizeBytes: number | null;
  modifiedTs: number; // unix millis
  readOnly: boolean;
  hidden?: boolean;
  symlink?: boolean;
}

// Authority: native-host/internal/ops/readdir.go (readdirData).
// nextCursor is `*string` on the Go side — JSON `null` when the listing is
// complete, else an opaque string to feed back into args.cursor.
export interface ReaddirData {
  entries: Entry[];
  total: number;
  // PROTOCOL.md §7.3 shows `"nextCursor": null`. Keep the explicit null in
  // the type so callers are forced to acknowledge the terminal case.
  nextCursor?: string | null;
  // hasMore is a convenience flag some Host versions emit alongside
  // nextCursor. Treat `nextCursor != null` as the authoritative signal.
  hasMore?: boolean;
}

// --- 7.4 stat ---------------------------------------------------------------

export interface StatArgs {
  path: string;
}

// Authority: native-host/internal/ops/stat.go (statData).
// Shape is a flattened Entry plus optional createdTs / accessedTs / target /
// permissions fields. Times are unix millis; `target` is populated only for
// symlinks; `permissions` is a "drwxr-xr-x" style string on Unix hosts.
export interface EntryMeta {
  path: string;
  type: "file" | "directory" | "symlink";
  sizeBytes: number | null;
  modifiedTs: number;
  createdTs?: number;
  accessedTs?: number;
  readOnly: boolean;
  hidden?: boolean;
  symlink?: boolean;
  target?: string;
  permissions?: string;
}

// --- 7.5 / 7.8 / 7.9 / 7.11 Phase 2.1 mutation ops --------------------------
//
// All five ops return an empty data object (`{}`) on success — the information
// the caller cares about is "did it succeed?", communicated by `ok: true`.
// Authority: native-host/internal/ops/{mkdir,rename,remove,open,reveal}.go.
//
// `explicitConfirm` is the system-path guard flag (PROTOCOL.md §10): the Host
// refuses to mutate system-protected locations unless the client sets this to
// true after presenting a confirmation UI. Omit for ordinary user paths.

// PROTOCOL.md §7.5 mkdir
export interface MkdirArgs {
  path: string;
  // When true, create missing parent directories (mkdir -p semantics). When
  // false or omitted the Host requires the parent to exist already.
  recursive?: boolean;
  explicitConfirm?: boolean;
}

// PROTOCOL.md §7.8 rename (also covers single-file move within same volume).
export interface RenameArgs {
  src: string;
  dst: string;
  explicitConfirm?: boolean;
}

// PROTOCOL.md §7.9 remove. `mode` discriminates between soft delete
// (OS trash / recycle bin) and hard delete. "trash" is the default UI choice;
// "permanent" should require an extra UI confirmation on top of
// explicitConfirm.
export type RemoveMode = "trash" | "permanent";
export interface RemoveArgs {
  path: string;
  mode: RemoveMode;
  explicitConfirm?: boolean;
}

// PROTOCOL.md §7.11 open — launches the OS default handler for the path
// (double-click equivalent). No confirm flag: opening is non-destructive.
export interface OpenArgs {
  path: string;
}

// PROTOCOL.md §7.11 revealInOsExplorer — opens the parent folder in the OS
// file explorer with `path` highlighted (Windows: explorer /select, ; macOS:
// open -R). Non-destructive, same shape as OpenArgs but kept as a distinct
// type so the two ops cannot be accidentally swapped.
export interface RevealArgs {
  path: string;
}

// Data payload shared by all five mutation ops. Modelled as a string-keyed
// record with `never` values so the object is empty at the type level —
// consumers cannot read fields off of it, matching the wire shape `{}`.
export type EmptyData = Record<string, never>;

// -----------------------------------------------------------------------------
// op → args / data mapping tables.
//
// These power the type-safe `request<O>(op, args)` overloads in ipc.ts so
// that the compiler refuses, e.g., a `readdir` call without a `path`, or
// a `listDrives` invocation with spurious args.
//
// Keep keys in sync with the Op union above.
// -----------------------------------------------------------------------------

export interface OpArgsMap {
  ping: PingArgs | undefined;
  listDrives: undefined;
  readdir: ReaddirArgs;
  stat: StatArgs;
  // Phase 2.1 — non-streaming mutations
  mkdir: MkdirArgs;
  rename: RenameArgs;
  remove: RemoveArgs;
  open: OpenArgs;
  revealInOsExplorer: RevealArgs;
}

export interface OpDataMap {
  ping: PingData;
  listDrives: ListDrivesData;
  readdir: ReaddirData;
  stat: EntryMeta;
  // Phase 2.1 — all mutation ops return an empty success payload
  mkdir: EmptyData;
  rename: EmptyData;
  remove: EmptyData;
  open: EmptyData;
  revealInOsExplorer: EmptyData;
}

// Ops that accept no args (or only an optional args payload). Used by the
// request() overload so callers can omit the second parameter.
export type OpNoArgs = {
  [K in keyof OpArgsMap]: undefined extends OpArgsMap[K] ? K : never;
}[keyof OpArgsMap];
