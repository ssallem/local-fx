import { create } from "zustand";

/**
 * Clipboard state for the explorer's copy/cut/paste UX.
 *
 * Phase 2.2-B scope: this store only tracks *intent*. The actual copy / move
 * IPC wiring lives in Phase 2.3-T (background streaming) and Phase 2.4-U
 * (conflict resolution). Keeping the intent in its own store — not
 * `useExplorerStore` — means:
 *  - FileList can subscribe to clipPaths without re-rendering on every
 *    navigate()/readdir() churn.
 *  - P2.4-U can introduce an in-flight progress slice without touching the
 *    intent slice.
 *
 * `sourcePath` is bookkeeping-only (we remember which directory the user was
 * in when they cut/copied) so a future "paste into same folder" conflict
 * check has a source-dir reference without walking `paths`.
 */
export type ClipboardMode = "copy" | "cut";

interface ClipboardState {
  mode: ClipboardMode | null;
  paths: string[]; // absolute paths
  sourcePath: string | null; // directory where the copy/cut was issued
  setClipboard: (
    mode: ClipboardMode,
    paths: string[],
    sourcePath: string | null
  ) => void;
  clearClipboard: () => void;
}

export const useClipboard = create<ClipboardState>((set) => ({
  mode: null,
  paths: [],
  sourcePath: null,
  // Always copy the array so callers that mutate their own local list
  // (e.g. Array.prototype.push) can't retroactively alter our snapshot.
  setClipboard: (mode, paths, sourcePath) =>
    set({ mode, paths: [...paths], sourcePath }),
  clearClipboard: () => set({ mode: null, paths: [], sourcePath: null })
}));
