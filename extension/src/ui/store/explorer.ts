import { create } from "zustand";
import type {
  Drive,
  Entry,
  ReaddirArgs,
  RemoveMode
} from "../../types/shared";
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
  selectedIndex: number; // -1 = no row selected
  pendingConfirm: PendingConfirm | null;

  // actions
  loadDrives: () => Promise<void>;
  navigate: (path: string) => Promise<void>;
  goUp: () => Promise<void>;
  goBack: () => Promise<void>;
  goForward: () => Promise<void>;
  loadMore: () => Promise<void>;
  reload: () => Promise<void>;
  goHome: () => void;
  setSelectedIndex: (i: number) => void;
  clearError: () => void;

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
  selectedIndex: -1,
  pendingConfirm: null,

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
    set({ loading: true, error: null, selectedIndex: -1 });
    try {
      const data = await fetchDir(path);
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
          selectedIndex: -1,
          error: null
        });
      }
      return;
    }
    const prev = history[historyIndex - 1];
    if (!prev) return;
    set({ loading: true, error: null, selectedIndex: -1 });
    try {
      const data = await fetchDir(prev);
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
    set({ loading: true, error: null, selectedIndex: -1 });
    try {
      const data = await fetchDir(next);
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
      const data = await fetchDir(currentPath, { cursor: nextCursor });
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
    set({ loading: true, error: null, selectedIndex: -1 });
    try {
      const data = await fetchDir(currentPath);
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
      selectedIndex: -1,
      error: null
    });
  },

  setSelectedIndex(i: number) {
    set({ selectedIndex: i });
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
  }
}));
