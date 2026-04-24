import { create } from "zustand";
import type {
  Drive,
  Entry,
  ReaddirArgs,
  RemoveMode
} from "../../types/shared";

// Column-level UI state kept in-store so FileList can be purely presentational.
// Sort is server-driven (Go readdir handles the ordering); columnWidths and
// columnOrder are pure UI prefs persisted to localStorage.
export type SortableField = "name" | "size" | "modified";
export type ColumnWidths = { name: number; size: number; modified: number };
export type ColumnOrder = ("name" | "size" | "modified")[];

const LS_WIDTHS_KEY = "explorer.columnWidths";
const LS_ORDER_KEY = "explorer.columnOrder";
const DEFAULT_COLUMN_WIDTHS: ColumnWidths = {
  name: 400,
  size: 100,
  modified: 200
};
const DEFAULT_COLUMN_ORDER: ColumnOrder = ["name", "size", "modified"];
// min widths mirror the handle-drag clamp in FileList.tsx. Duplicated
// intentionally so the store can defensively clamp persisted values on load
// without importing UI-layer constants.
const MIN_COLUMN_WIDTHS: ColumnWidths = { name: 80, size: 60, modified: 120 };

/**
 * Sanitize a persisted width value. `localStorage` is untrusted input —
 * another tab, an extension, or a corrupted write can leave us with strings,
 * `null`, objects, or `NaN`. Any of those coerced through `Number()` +
 * `Math.max` would poison `gridTemplateColumns` with `"NaNpx"` and collapse
 * the layout. We reject anything that isn't a finite positive integer and
 * fall back to the default before the floor / min clamp.
 */
function sanitizeWidth(
  val: unknown,
  defaultVal: number,
  min: number
): number {
  const n = Math.floor(Number(val));
  if (!Number.isFinite(n) || n <= 0) return defaultVal;
  return Math.max(min, n);
}

function loadColumnWidths(): ColumnWidths {
  if (typeof localStorage === "undefined") return { ...DEFAULT_COLUMN_WIDTHS };
  try {
    const raw = localStorage.getItem(LS_WIDTHS_KEY);
    if (!raw) return { ...DEFAULT_COLUMN_WIDTHS };
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object") {
      return { ...DEFAULT_COLUMN_WIDTHS };
    }
    const p = parsed as Record<string, unknown>;
    return {
      name: sanitizeWidth(
        p.name,
        DEFAULT_COLUMN_WIDTHS.name,
        MIN_COLUMN_WIDTHS.name
      ),
      size: sanitizeWidth(
        p.size,
        DEFAULT_COLUMN_WIDTHS.size,
        MIN_COLUMN_WIDTHS.size
      ),
      modified: sanitizeWidth(
        p.modified,
        DEFAULT_COLUMN_WIDTHS.modified,
        MIN_COLUMN_WIDTHS.modified
      )
    };
  } catch {
    return { ...DEFAULT_COLUMN_WIDTHS };
  }
}

function loadColumnOrder(): ColumnOrder {
  if (typeof localStorage === "undefined") return [...DEFAULT_COLUMN_ORDER];
  try {
    const raw = localStorage.getItem(LS_ORDER_KEY);
    if (!raw) return [...DEFAULT_COLUMN_ORDER];
    const parsed = JSON.parse(raw);
    const required: SortableField[] = ["name", "size", "modified"];
    const valid =
      Array.isArray(parsed) &&
      parsed.length === 3 &&
      required.every((k) => parsed.includes(k)) &&
      parsed.every((v) => required.includes(v as SortableField));
    if (!valid) return [...DEFAULT_COLUMN_ORDER];
    return parsed as ColumnOrder;
  } catch {
    return [...DEFAULT_COLUMN_ORDER];
  }
}

// Debounced persistence for column widths. `setColumnWidth` fires on every
// mousemove during a resize drag — that's dozens to hundreds of synchronous
// `localStorage.setItem` calls per second, which blocks the main thread and
// causes visible jank. Coalescing to a single write 300ms after the drag
// settles keeps the gesture smooth. A refresh within 300ms of mouseup loses
// the very last pixel-level update, which is an acceptable trade; the
// in-memory state is already correct for the rest of the session.
let columnWidthsPersistTimer: number | undefined;
function persistColumnWidthsDebounced(w: ColumnWidths): void {
  if (typeof localStorage === "undefined") return;
  if (columnWidthsPersistTimer !== undefined) {
    window.clearTimeout(columnWidthsPersistTimer);
  }
  columnWidthsPersistTimer = window.setTimeout(() => {
    try {
      localStorage.setItem(LS_WIDTHS_KEY, JSON.stringify(w));
    } catch {
      // Quota / private-mode failures are non-fatal: state still works in-memory.
    }
    columnWidthsPersistTimer = undefined;
  }, 300);
}

function persistColumnOrder(o: ColumnOrder): void {
  if (typeof localStorage === "undefined") return;
  try {
    localStorage.setItem(LS_ORDER_KEY, JSON.stringify(o));
  } catch {
    // ignore — see persistColumnWidthsDebounced note.
  }
}
import {
  listDrives,
  readdir,
  IpcError,
  mkdir as ipcMkdir,
  rename as ipcRename,
  remove as ipcRemove,
  openEntry as ipcOpenEntry,
  revealEntry as ipcRevealEntry
} from "../ipc";
import { joinPath, parentPath } from "../utils/format";

/**
 * Explorer state machine.
 *
 * `currentPath === null` means we're on the drive-list screen (the "home"
 * of the app). Any other string means we're inside a directory and should
 * render <Sidebar /> + <FileList />.
 *
 * History is a linear stack. `navigate()` truncates forward history when
 * the user branches off, matching browser semantics.
 */

// PROTOCOL.md §10 system-path guard surfaces as a pending confirm. A
// permanent-delete prompt uses the same modal so we funnel both through
// pendingConfirm rather than growing one modal flavour per mutation kind.
export type PendingConfirmKind = "system-path" | "permanent-delete";

export interface PendingConfirm {
  kind: PendingConfirmKind;
  title: string;
  message: string;
  // The closure holds the original args with explicitConfirm flipped to true
  // so the resume path is a single `await onConfirm()` — no re-plumbing.
  onConfirm: () => Promise<void>;
}

export interface ExplorerState {
  drives: Drive[];
  currentPath: string | null;
  entries: Entry[];
  total: number;
  page: number;
  nextCursor: string | null;
  hasMore: boolean;
  loading: boolean;
  error: IpcError | null;
  history: string[]; // each entry is a path string; `null` (root) not stored
  historyIndex: number; // -1 = no history, else pointer into `history`
  // Multi-selection. Set chosen over Array for O(1) has/add/delete and
  // size-as-"count". Treat as immutable — always `new Set(prev)` on mutation
  // so Zustand's reference-equality subscribers re-render.
  selectedIndices: Set<number>; // empty = nothing selected
  lastAnchorIndex: number; // -1 = no anchor. Shift+Click range pivot.
  pendingConfirm: PendingConfirm | null;

  // --- Phase 2.2 UI polish — sort + column layout ------------------------
  sortField: SortableField;
  sortOrder: "asc" | "desc";
  columnWidths: ColumnWidths;
  columnOrder: ColumnOrder;

  // actions
  loadDrives: () => Promise<void>;
  navigate: (path: string) => Promise<void>;
  goUp: () => Promise<void>;
  goBack: () => Promise<void>;
  goForward: () => Promise<void>;
  loadMore: () => Promise<void>;
  reload: () => Promise<void>;
  goHome: () => void;
  // --- Selection actions ---
  // Replace selection with {i} and set anchor = i. Mirrors the old
  // setSelectedIndex() semantics — callers that just want "pick this row".
  selectOnly: (i: number) => void;
  // Extend selection from anchor to toIndex (inclusive). Replaces any prior
  // selection. No-op anchor stays put (Shift+Click keeps the pivot).
  // If no anchor is set, behaves like selectOnly(toIndex).
  selectRange: (toIndex: number) => void;
  // Toggle membership of i and move anchor to i. Ctrl+Click semantics.
  toggleSelect: (i: number) => void;
  // Select every current entry; anchor = 0. No-op if entries empty.
  selectAll: () => void;
  // Clear selection and anchor.
  clearSelection: () => void;
  clearError: () => void;

  // Phase 2.2 UI polish
  setSort: (field: SortableField, order?: "asc" | "desc") => Promise<void>;
  setColumnWidth: (col: SortableField, px: number) => void;
  setColumnOrder: (order: ColumnOrder) => void;

  // Phase 2.1 — mutation actions
  createFolder: (name: string) => Promise<void>;
  renameEntry: (oldPath: string, newName: string) => Promise<void>;
  deleteEntry: (path: string, mode: RemoveMode) => Promise<void>;
  openEntry: (path: string) => Promise<void>;
  revealEntry: (path: string) => Promise<void>;
  resolvePendingConfirm: () => Promise<void>;
  cancelPendingConfirm: () => void;
}

const DEFAULT_PAGE_SIZE = 1000;

function toIpcError(e: unknown): IpcError {
  if (e instanceof IpcError) return e;
  const message = e instanceof Error ? e.message : String(e);
  return new IpcError({ code: "E_UNKNOWN", message, retryable: false });
}

/**
 * Load one page of a directory. Does not touch history — callers decide.
 */
async function fetchDir(
  path: string,
  args?: Partial<ReaddirArgs>
): Promise<Awaited<ReturnType<typeof readdir>>> {
  const req: ReaddirArgs = {
    path,
    pageSize: DEFAULT_PAGE_SIZE,
    ...args
  };
  return readdir(req);
}

export const useExplorerStore = create<ExplorerState>((set, get) => ({
  drives: [],
  currentPath: null,
  entries: [],
  total: 0,
  page: 1,
  nextCursor: null,
  hasMore: false,
  loading: false,
  error: null,
  history: [],
  historyIndex: -1,
  selectedIndices: new Set<number>(),
  lastAnchorIndex: -1,
  pendingConfirm: null,

  sortField: "name",
  sortOrder: "asc",
  columnWidths: loadColumnWidths(),
  columnOrder: loadColumnOrder(),

  async loadDrives() {
    set({ loading: true, error: null });
    try {
      const drives = await listDrives();
      set({ drives, loading: false });
    } catch (e) {
      set({ loading: false, error: toIpcError(e) });
    }
  },

  async navigate(path: string) {
    set({
      loading: true,
      error: null,
      selectedIndices: new Set<number>(),
      lastAnchorIndex: -1
    });
    try {
      const { sortField, sortOrder } = get();
      const data = await fetchDir(path, {
        sort: { field: sortField, order: sortOrder }
      });
      const { history, historyIndex } = get();
      // Truncate forward history and append the new path.
      const truncated = history.slice(0, historyIndex + 1);
      const nextHistory = [...truncated, path];
      set({
        currentPath: path,
        entries: data.entries,
        total: data.total,
        page: 1,
        nextCursor: data.nextCursor ?? null,
        hasMore: data.nextCursor != null,
        history: nextHistory,
        historyIndex: nextHistory.length - 1,
        loading: false
      });
    } catch (e) {
      set({ loading: false, error: toIpcError(e) });
    }
  },

  async goUp() {
    const { currentPath } = get();
    if (currentPath === null) return; // already at home
    const parent = parentPath(currentPath);
    if (parent === null) {
      // At a root (C:\ or /) — flip to drive-list screen.
      get().goHome();
      return;
    }
    await get().navigate(parent);
  },

  async goBack() {
    const { history, historyIndex } = get();
    if (historyIndex <= 0) {
      // Step back from first entry → home screen.
      if (historyIndex === 0) {
        set({
          currentPath: null,
          entries: [],
          total: 0,
          historyIndex: -1,
          selectedIndices: new Set<number>(),
          lastAnchorIndex: -1,
          error: null
        });
      }
      return;
    }
    const prev = history[historyIndex - 1];
    if (!prev) return;
    set({
      loading: true,
      error: null,
      selectedIndices: new Set<number>(),
      lastAnchorIndex: -1
    });
    try {
      const { sortField, sortOrder } = get();
      const data = await fetchDir(prev, {
        sort: { field: sortField, order: sortOrder }
      });
      set({
        currentPath: prev,
        entries: data.entries,
        total: data.total,
        page: 1,
        nextCursor: data.nextCursor ?? null,
        hasMore: data.nextCursor != null,
        historyIndex: historyIndex - 1,
        loading: false
      });
    } catch (e) {
      set({ loading: false, error: toIpcError(e) });
    }
  },

  async goForward() {
    const { history, historyIndex } = get();
    if (historyIndex >= history.length - 1) return;
    const next = history[historyIndex + 1];
    if (!next) return;
    set({
      loading: true,
      error: null,
      selectedIndices: new Set<number>(),
      lastAnchorIndex: -1
    });
    try {
      const { sortField, sortOrder } = get();
      const data = await fetchDir(next, {
        sort: { field: sortField, order: sortOrder }
      });
      set({
        currentPath: next,
        entries: data.entries,
        total: data.total,
        page: 1,
        nextCursor: data.nextCursor ?? null,
        hasMore: data.nextCursor != null,
        historyIndex: historyIndex + 1,
        loading: false
      });
    } catch (e) {
      set({ loading: false, error: toIpcError(e) });
    }
  },

  async loadMore() {
    const { currentPath, nextCursor, entries, page, loading } = get();
    if (loading) return;
    if (currentPath === null || nextCursor === null) return;
    set({ loading: true, error: null });
    try {
      const { sortField, sortOrder } = get();
      const data = await fetchDir(currentPath, {
        cursor: nextCursor,
        sort: { field: sortField, order: sortOrder }
      });
      // Deduplicate by path: the underlying directory may have changed
      // between pages (file renamed/added/removed), so the Host can return
      // an entry we've already seen. Keeping the first occurrence preserves
      // the sort order the Host picked for page N.
      const existing = new Set(entries.map((e) => e.path));
      const newEntries = data.entries.filter((e) => !existing.has(e.path));
      set({
        entries: [...entries, ...newEntries],
        total: data.total,
        page: page + 1,
        nextCursor: data.nextCursor ?? null,
        hasMore: data.nextCursor != null,
        loading: false
      });
    } catch (e) {
      set({ loading: false, error: toIpcError(e) });
    }
  },

  async reload() {
    const { currentPath } = get();
    if (currentPath === null) {
      await get().loadDrives();
      return;
    }
    set({
      loading: true,
      error: null,
      selectedIndices: new Set<number>(),
      lastAnchorIndex: -1
    });
    try {
      const { sortField, sortOrder } = get();
      const data = await fetchDir(currentPath, {
        sort: { field: sortField, order: sortOrder }
      });
      set({
        entries: data.entries,
        total: data.total,
        page: 1,
        nextCursor: data.nextCursor ?? null,
        hasMore: data.nextCursor != null,
        loading: false
      });
    } catch (e) {
      set({ loading: false, error: toIpcError(e) });
    }
  },

  goHome() {
    set({
      currentPath: null,
      entries: [],
      total: 0,
      page: 1,
      nextCursor: null,
      hasMore: false,
      selectedIndices: new Set<number>(),
      lastAnchorIndex: -1,
      error: null
    });
  },

  // --- Selection actions ---
  // We always allocate a fresh Set so Zustand's ref-equality check fires
  // subscribers; mutating the existing Set in place would look identical to
  // React and skip the re-render.

  selectOnly(i: number) {
    set({ selectedIndices: new Set<number>([i]), lastAnchorIndex: i });
  },

  selectRange(toIndex: number) {
    const { lastAnchorIndex } = get();
    // No anchor → fall back to single-select so the first Shift+Arrow /
    // Shift+Click from an empty selection still behaves reasonably.
    if (lastAnchorIndex < 0) {
      set({
        selectedIndices: new Set<number>([toIndex]),
        lastAnchorIndex: toIndex
      });
      return;
    }
    const lo = Math.min(lastAnchorIndex, toIndex);
    const hi = Math.max(lastAnchorIndex, toIndex);
    const next = new Set<number>();
    for (let i = lo; i <= hi; i += 1) next.add(i);
    // anchor stays put — that's the whole point of a range pivot.
    set({ selectedIndices: next });
  },

  toggleSelect(i: number) {
    const { selectedIndices } = get();
    const next = new Set(selectedIndices);
    if (next.has(i)) {
      next.delete(i);
    } else {
      next.add(i);
    }
    // Ctrl+Click re-seats the anchor on the clicked row regardless of
    // whether it was added or removed; matches native file-manager UX.
    set({ selectedIndices: next, lastAnchorIndex: i });
  },

  selectAll() {
    const { entries } = get();
    if (entries.length === 0) {
      set({ selectedIndices: new Set<number>(), lastAnchorIndex: -1 });
      return;
    }
    const next = new Set<number>();
    for (let i = 0; i < entries.length; i += 1) next.add(i);
    set({ selectedIndices: next, lastAnchorIndex: 0 });
  },

  clearSelection() {
    set({ selectedIndices: new Set<number>(), lastAnchorIndex: -1 });
  },

  clearError() {
    set({ error: null });
  },

  // --- Phase 2.1 mutation actions ------------------------------------------

  async createFolder(name: string) {
    const parent = get().currentPath;
    if (parent === null) {
      set({
        error: new IpcError({
          code: "EINVAL",
          message: "홈 화면에서는 폴더를 만들 수 없습니다",
          retryable: false
        })
      });
      return;
    }
    const path = joinPath(parent, name);
    const runner = async (explicit: boolean): Promise<void> => {
      await ipcMkdir({ path, explicitConfirm: explicit });
      await get().reload();
    };
    try {
      await runner(false);
    } catch (e) {
      if (
        e instanceof IpcError &&
        e.code === "E_SYSTEM_PATH_CONFIRM_REQUIRED"
      ) {
        set({
          pendingConfirm: {
            kind: "system-path",
            title: "시스템 경로 경고",
            message: `'${path}'는 시스템 경로입니다. 계속 진행하시겠습니까?`,
            onConfirm: async () => {
              await runner(true);
            }
          }
        });
      } else {
        set({ error: toIpcError(e) });
      }
    }
  },

  async renameEntry(oldPath: string, newName: string) {
    const parent = parentPath(oldPath);
    if (parent === null) {
      set({
        error: new IpcError({
          code: "EINVAL",
          message: "루트 경로는 이름을 변경할 수 없습니다",
          retryable: false
        })
      });
      return;
    }
    const dst = joinPath(parent, newName);
    const runner = async (explicit: boolean): Promise<void> => {
      await ipcRename({ src: oldPath, dst, explicitConfirm: explicit });
      await get().reload();
    };
    try {
      await runner(false);
    } catch (e) {
      if (
        e instanceof IpcError &&
        e.code === "E_SYSTEM_PATH_CONFIRM_REQUIRED"
      ) {
        set({
          pendingConfirm: {
            kind: "system-path",
            title: "시스템 경로 경고",
            message: `'${oldPath}' 또는 '${dst}'는 시스템 경로입니다. 계속 진행하시겠습니까?`,
            onConfirm: async () => {
              await runner(true);
            }
          }
        });
      } else {
        set({ error: toIpcError(e) });
      }
    }
  },

  async deleteEntry(path: string, mode: RemoveMode) {
    const runner = async (explicit: boolean): Promise<void> => {
      await ipcRemove({ path, mode, explicitConfirm: explicit });
      await get().reload();
    };
    try {
      await runner(false);
    } catch (e) {
      if (
        e instanceof IpcError &&
        e.code === "E_SYSTEM_PATH_CONFIRM_REQUIRED"
      ) {
        set({
          pendingConfirm: {
            kind: "system-path",
            title: "시스템 경로 경고",
            message: `'${path}'는 시스템 경로입니다. 계속 진행하시겠습니까?`,
            onConfirm: async () => {
              await runner(true);
            }
          }
        });
      } else {
        set({ error: toIpcError(e) });
      }
    }
  },

  async openEntry(path: string) {
    try {
      await ipcOpenEntry(path);
    } catch (e) {
      set({ error: toIpcError(e) });
    }
  },

  async revealEntry(path: string) {
    try {
      await ipcRevealEntry(path);
    } catch (e) {
      set({ error: toIpcError(e) });
    }
  },

  async resolvePendingConfirm() {
    const pending = get().pendingConfirm;
    if (!pending) return;
    set({ pendingConfirm: null });
    try {
      await pending.onConfirm();
    } catch (e) {
      set({ error: toIpcError(e) });
    }
  },

  cancelPendingConfirm() {
    set({ pendingConfirm: null });
  },

  // --- Phase 2.2 UI polish -------------------------------------------------

  async setSort(field, order) {
    const current = get();
    const nextOrder: "asc" | "desc" =
      order ??
      (current.sortField === field && current.sortOrder === "asc"
        ? "desc"
        : "asc");
    // Short-circuit a no-op click (same field + same resolved order).
    if (current.sortField === field && current.sortOrder === nextOrder) {
      return;
    }
    set({ sortField: field, sortOrder: nextOrder });
    // Reload applies the new sort. On the home screen there's nothing to
    // reload — the state still persists for the next navigate().
    if (current.currentPath !== null) {
      await get().reload();
    }
  },

  setColumnWidth(col, px) {
    // Defend against NaN / non-finite input: a stale closure, a rogue
    // extension, or arithmetic on undefined could call us with garbage.
    // Silently drop instead of poisoning the grid template.
    const n = Math.floor(Number(px));
    if (!Number.isFinite(n) || n <= 0) return;
    const current = get().columnWidths;
    const clamped = Math.max(MIN_COLUMN_WIDTHS[col], n);
    if (current[col] === clamped) return;
    const next: ColumnWidths = { ...current, [col]: clamped };
    set({ columnWidths: next });
    persistColumnWidthsDebounced(next);
  },

  setColumnOrder(order) {
    // Defensive validation: caller may pass a mutated array. Reject anything
    // that doesn't cover exactly the three fields without duplicates.
    const required: SortableField[] = ["name", "size", "modified"];
    const ok =
      order.length === 3 &&
      required.every((k) => order.includes(k)) &&
      new Set(order).size === 3;
    if (!ok) return;
    set({ columnOrder: [...order] });
    persistColumnOrder(order);
  }
}));
