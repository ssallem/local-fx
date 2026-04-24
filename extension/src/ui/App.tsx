import { useCallback, useEffect, useRef, useState } from "react";
import "./App.css";
import { useExplorerStore } from "./store/explorer";
import {
  useClipboard,
  type ClipboardMode
} from "./store/clipboard";
import { useJobs, startCopyJob, startMoveJob } from "./store/jobs";
import { Toolbar } from "./components/Toolbar";
import { Sidebar } from "./components/Sidebar";
import { FileList } from "./components/FileList";
import { StatusBar } from "./components/StatusBar";
import { ErrorBanner } from "./components/ErrorBanner";
import { DevPanel } from "./components/DevPanel";
import {
  ContextMenu,
  type ContextMenuItem
} from "./components/ContextMenu";
import {
  ConfirmDialog,
  ConflictDialog,
  FailureSummary,
  RenameDialog,
  type ConflictInfo,
  type ConflictStrategyChoice
} from "./components/dialogs";
import { ProgressToasts } from "./components/ProgressToasts";
import { basename, formatBytes, joinPath } from "./utils/format";
import { t } from "./utils/i18n";
import { IpcError, stat as ipcStat } from "./ipc";
import type {
  CopyArgs,
  Drive,
  Entry,
  FailureInfo,
  MoveArgs,
  RemoveMode
} from "../types/shared";

/**
 * Home screen (drive grid). Shown when currentPath === null so first-launch
 * users see a list of their disks without any sidebar/list chrome.
 */
function HomeScreen(): JSX.Element {
  const drives = useExplorerStore((s) => s.drives);
  const navigate = useExplorerStore((s) => s.navigate);
  const loading = useExplorerStore((s) => s.loading);

  if (loading && drives.length === 0) {
    return <div className="home-title">{t("app_loading_drives")}</div>;
  }

  return (
    <div style={{ width: "100%", maxWidth: 960 }}>
      <div className="home-title">{t("app_home_title")}</div>
      <div className="home-grid">
        {drives.length === 0 ? (
          <div className="sidebar-empty">{t("app_no_drives_hint")}</div>
        ) : (
          drives.map((d: Drive) => {
            // Build the drive meta line piecewise so translators get complete
            // sentences for capacity / read-only and the `•` separators stay
            // locale-agnostic glue.
            const capacity =
              d.totalBytes > 0
                ? ` • ${t("app_drive_capacity", [formatBytes(d.freeBytes), formatBytes(d.totalBytes)])}`
                : "";
            const roSuffix = d.readOnly
              ? ` • ${t("app_drive_read_only_suffix")}`
              : "";
            return (
              <button
                key={d.path}
                type="button"
                className="home-drive-card"
                onClick={() => {
                  void navigate(d.path);
                }}
              >
                <div className="home-drive-title">
                  💽 {d.label ? `${d.label} (${d.path})` : d.path}
                </div>
                <div className="home-drive-meta">
                  {d.fsType}
                  {capacity}
                  {roSuffix}
                </div>
              </button>
            );
          })
        )}
      </div>
    </div>
  );
}

// Pure helper — local to App because the store doesn't need it, and pulling
// it from utils for one call site is overkill.
function clamp(v: number, lo: number, hi: number): number {
  if (v < lo) return lo;
  if (v > hi) return hi;
  return v;
}

type RenameDialogState =
  | { mode: "create"; initialName: string; title: string }
  | { mode: "rename"; path: string; initialName: string; title: string };

type DeleteConfirmState = { path: string; mode: RemoveMode };

// Two visual/behavioural variants. "row" anchors to a specific entry; "blank"
// is the empty-space context menu with create-folder / paste / refresh only.
type ContextMenuState =
  | { kind: "row"; entry: Entry; x: number; y: number }
  | { kind: "blank"; x: number; y: number };

// Phase 2.4 — paste flow plumbing.
//
// ConflictDialogState bridges the imperative pasteClipboard() loop with the
// declarative ConflictDialog modal: the loop awaits a Promise that the
// dialog resolves on user choice. Storing the resolver in component state
// (instead of a ref) means the dialog re-renders if the conflict batch
// changes mid-flight (it currently doesn't, but the shape is future-proof).
interface ConflictDialogState {
  conflict: ConflictInfo;
  remaining: number;
  resolve: (
    result: { strategy: ConflictStrategyChoice; applyToAll: boolean } | null
  ) => void;
}

// FailureSummaryState is the inverse: an opened summary modal is parameterised
// purely on the job snapshot at the moment it terminated. We snapshot
// title/totalAttempted/failures/jobId rather than re-deriving from the store
// so the dialog is stable even if the user dismisses the underlying job toast.
interface FailureSummaryState {
  jobId: string;
  title: string;
  totalAttempted: number;
  failures: FailureInfo[];
}

export function App(): JSX.Element {
  const currentPath = useExplorerStore((s) => s.currentPath);
  const loadDrives = useExplorerStore((s) => s.loadDrives);
  const goUp = useExplorerStore((s) => s.goUp);
  const goBack = useExplorerStore((s) => s.goBack);
  const goForward = useExplorerStore((s) => s.goForward);
  const reload = useExplorerStore((s) => s.reload);
  const entries = useExplorerStore((s) => s.entries);
  const selectedIndices = useExplorerStore((s) => s.selectedIndices);
  const lastAnchorIndex = useExplorerStore((s) => s.lastAnchorIndex);
  const selectOnly = useExplorerStore((s) => s.selectOnly);
  const selectRange = useExplorerStore((s) => s.selectRange);
  const selectAll = useExplorerStore((s) => s.selectAll);
  const clearSelection = useExplorerStore((s) => s.clearSelection);
  const navigate = useExplorerStore((s) => s.navigate);
  const openEntry = useExplorerStore((s) => s.openEntry);
  const revealEntry = useExplorerStore((s) => s.revealEntry);
  const createFolder = useExplorerStore((s) => s.createFolder);
  const renameEntry = useExplorerStore((s) => s.renameEntry);
  const deleteEntry = useExplorerStore((s) => s.deleteEntry);
  const pendingConfirm = useExplorerStore((s) => s.pendingConfirm);
  const resolvePendingConfirm = useExplorerStore(
    (s) => s.resolvePendingConfirm
  );
  const cancelPendingConfirm = useExplorerStore((s) => s.cancelPendingConfirm);

  // Clipboard mode is used both for the key-handler guard (Ctrl+V no-ops when
  // nothing to paste) and for disabling the menu items, so we subscribe.
  const clipboardMode = useClipboard((s) => s.mode);

  const [devOpen, setDevOpen] = useState(false);
  const toggleDev = useCallback(() => setDevOpen((v) => !v), []);

  const [renameDialog, setRenameDialog] = useState<RenameDialogState | null>(
    null
  );
  const [deleteConfirm, setDeleteConfirm] = useState<DeleteConfirmState | null>(
    null
  );
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);

  // Phase 2.4 dialog states. conflictDialog is set during the paste pre-scan;
  // failureSummary is auto-opened by the jobs subscription (below) when a
  // streaming op terminates with a non-empty failures list.
  const [conflictDialog, setConflictDialog] =
    useState<ConflictDialogState | null>(null);
  const [failureSummary, setFailureSummary] =
    useState<FailureSummaryState | null>(null);
  // Tracks job ids whose FailureSummary has already been shown so the same
  // terminal event can't pop the modal twice (the jobs subscription fires on
  // every store mutation, including dismissJob).
  const shownFailuresRef = useRef<Set<string>>(new Set());

  // CRITICAL-P2-R4-2 — re-entry guard for pasteClipboard. A user pressing
  // Ctrl+V rapidly (or clicking the context-menu "붙여넣기" mid-flight) used
  // to fire two concurrent pastes that read the same clipboard paths and
  // raced on the same temp file names. The ref is checked+set at the very
  // top of pasteClipboard and cleared in the finally block so keybinding and
  // onClick entry points are both gated.
  const isPastingRef = useRef(false);

  const openCreateFolder = useCallback(() => {
    setRenameDialog({
      mode: "create",
      initialName: t("dialog_rename_default_name_new_folder"),
      title: t("dialog_rename_title_new_folder")
    });
  }, []);

  // Copy the selected entries' absolute paths into the clipboard store. We
  // always read the live `selectedIndices` / `entries` via useCallback's deps
  // so shortcuts invoked from a stale closure don't send the wrong set.
  const setClipboardFromSelection = useCallback(
    (mode: ClipboardMode) => {
      if (selectedIndices.size === 0) return;
      const paths = Array.from(selectedIndices)
        .sort((a, b) => a - b)
        .map((i) => entries[i])
        .filter((e): e is Entry => !!e)
        .map((e) => e.path);
      if (paths.length === 0) return;
      useClipboard.getState().setClipboard(mode, paths, currentPath);
    },
    [selectedIndices, entries, currentPath]
  );

  // Phase 2.4 — real paste. Three phases:
  //   1. Pre-scan: stat() each destination. ENOENT → no conflict; existing
  //      target → push onto a conflict list. Same-path noop is skipped.
  //   2. Resolve: walk the conflict list, opening ConflictDialog for each.
  //      "Apply to all" short-circuits subsequent prompts. Cancel aborts the
  //      whole batch (no partial work).
  //   3. Dispatch: kick off startCopyJob / startMoveJob per resolved entry.
  //      Cut mode clears the clipboard once the jobs are queued — completion
  //      lives on the jobs store.
  //
  // The Host rejects `conflict: "prompt"` for both copy and move, so this
  // pre-scan is the only valid bridge between user intent and an issuable
  // streaming request.
  const promptConflict = useCallback(
    (
      c: ConflictInfo,
      remaining: number
    ): Promise<{
      strategy: ConflictStrategyChoice;
      applyToAll: boolean;
    } | null> =>
      new Promise((resolve) => {
        setConflictDialog({ conflict: c, remaining, resolve });
      }),
    []
  );

  const pasteClipboard = useCallback(async () => {
    // CRITICAL-P2-R4-2 — re-entrance guard. Both the Ctrl+V keybinding and
    // the context-menu onClick dispatch into this callback; without the ref
    // a double-tap would read the clipboard twice and race on the same
    // temp files.
    if (isPastingRef.current) return;
    isPastingRef.current = true;
    try {
      const { mode, paths } = useClipboard.getState();
      if (!mode || paths.length === 0 || currentPath === null) return;

      // ── Phase 1: pre-scan ────────────────────────────────────────────────
      const conflicts: ConflictInfo[] = [];
      const noConflict: string[] = [];
      for (const src of paths) {
        const dst = joinPath(currentPath, basename(src));
        // Same-folder same-name paste: skip silently. Copying onto self would
        // collide; moving onto self is a no-op. Either way, nothing useful to do.
        if (src === dst) continue;
        try {
          const dstMeta = await ipcStat(dst);
          // Best-effort source metadata for the comparison panel; failure is
          // non-fatal — the dialog renders "—" placeholders.
          const srcMeta = await ipcStat(src).catch(() => null);
          const info: ConflictInfo = { srcPath: src, dstPath: dst };
          if (srcMeta?.sizeBytes !== undefined && srcMeta.sizeBytes !== null) {
            info.srcSize = srcMeta.sizeBytes;
          }
          if (dstMeta.sizeBytes !== undefined && dstMeta.sizeBytes !== null) {
            info.dstSize = dstMeta.sizeBytes;
          }
          if (srcMeta?.modifiedTs !== undefined) {
            info.srcMtime = srcMeta.modifiedTs;
          }
          if (dstMeta.modifiedTs !== undefined) {
            info.dstMtime = dstMeta.modifiedTs;
          }
          conflicts.push(info);
        } catch (e) {
          if (e instanceof IpcError && e.code === "ENOENT") {
            noConflict.push(src);
          } else {
            // Pre-scan stat failed for non-ENOENT reasons (EACCES, EIO, etc.).
            // Skip the pre-scan branch and let the actual op surface the error
            // through its terminal "done" failure list — this avoids blocking
            // the whole paste on an inscrutable stat error.
            noConflict.push(src);
          }
        }
      }

      // ── Phase 2: resolve conflicts interactively ────────────────────────
      const resolved: Array<{
        src: string;
        dst: string;
        strategy: ConflictStrategyChoice;
      }> = [];
      let appliedToAll: ConflictStrategyChoice | null = null;
      for (let i = 0; i < conflicts.length; i++) {
        const c = conflicts[i];
        if (!c) continue;
        let strategy: ConflictStrategyChoice;
        if (appliedToAll !== null) {
          strategy = appliedToAll;
        } else {
          const result = await promptConflict(c, conflicts.length - i);
          if (!result) {
            // User canceled mid-batch. Abort everything — partial paste with
            // some conflicts unresolved would be confusing.
            return;
          }
          strategy = result.strategy;
          if (result.applyToAll) appliedToAll = strategy;
        }
        if (strategy !== "skip") {
          resolved.push({ src: c.srcPath, dst: c.dstPath, strategy });
        }
      }

      // ── Phase 3: dispatch ───────────────────────────────────────────────
      const kind: "copy" | "move" = mode === "cut" ? "move" : "copy";
      for (const src of noConflict) {
        const dst = joinPath(currentPath, basename(src));
        if (kind === "copy") {
          const args: CopyArgs = { src, dst };
          startCopyJob(args, basename(src));
        } else {
          const args: MoveArgs = { src, dst };
          startMoveJob(args, basename(src));
        }
      }
      for (const r of resolved) {
        if (kind === "copy") {
          const args: CopyArgs = { src: r.src, dst: r.dst, conflict: r.strategy };
          startCopyJob(args, basename(r.src));
        } else {
          // MoveArgs.conflict excludes "prompt" by construction — r.strategy is
          // already "overwrite" | "skip" | "rename" so the assignment is sound.
          const args: MoveArgs = { src: r.src, dst: r.dst, conflict: r.strategy };
          startMoveJob(args, basename(r.src));
        }
      }

      // Cut semantics: the originals are about to disappear. Clearing the
      // clipboard now (rather than after the jobs finish) prevents the user
      // from re-pasting stale paths into another folder while the move runs.
      if (mode === "cut") {
        useClipboard.getState().clearClipboard();
      }
    } finally {
      isPastingRef.current = false;
    }
  }, [currentPath, promptConflict]);

  // Bootstrap
  useEffect(() => {
    void loadDrives();
  }, [loadDrives]);

  // Phase 2.4 — auto-open FailureSummary for any job that terminates with
  // failures. We subscribe to the jobs store imperatively so the modal
  // pops exactly once per terminal transition, not once per re-render.
  // shownFailuresRef debounces against repeated subscriptions firing on
  // unrelated store mutations (e.g. dismissJob).
  useEffect(() => {
    const unsub = useJobs.subscribe((s) => {
      for (const id of s.order) {
        const j = s.jobs[id];
        if (!j) continue;
        const terminal =
          j.state === "done" ||
          j.state === "failed" ||
          j.state === "canceled";
        if (!terminal) continue;
        if (j.failures.length === 0) continue;
        if (shownFailuresRef.current.has(id)) continue;
        shownFailuresRef.current.add(id);
        // Reload the current directory so the user immediately sees the
        // post-paste state (new files, missing originals from move). Best
        // effort; if reload fails the explorer's own error path surfaces it.
        void useExplorerStore.getState().reload();
        // Open the summary modal. fileTotal is the Host's per-stream total
        // and is a reasonable proxy for "how many things did we attempt".
        setFailureSummary({
          jobId: id,
          title:
            j.kind === "copy"
              ? t("toast_result_copy_title")
              : t("toast_result_move_title"),
          totalAttempted: Math.max(j.fileTotal, j.failures.length),
          failures: j.failures
        });
        // Only one modal at a time — break so subsequent jobs queue behind
        // the user dismissing this one. The next subscription fire will pick
        // them up because shownFailuresRef gates on per-id show state.
        break;
      }
    });
    return unsub;
  }, []);

  // Keyboard shortcuts.
  // Scoped to window because the explorer owns the whole new-tab viewport.
  useEffect(() => {
    function onKey(e: KeyboardEvent): void {
      // Avoid hijacking keys while the user is typing in an input field.
      const tgt = e.target as HTMLElement | null;
      const inEditable =
        tgt &&
        (tgt.tagName === "INPUT" ||
          tgt.tagName === "TEXTAREA" ||
          tgt.isContentEditable);

      // Ctrl+Shift+P — toggle dev panel (always works, even in inputs)
      if (e.ctrlKey && e.shiftKey && (e.key === "P" || e.key === "p")) {
        e.preventDefault();
        toggleDev();
        return;
      }

      if (inEditable) return;

      // Suppress keyboard nav while any modal is open — ConfirmDialog,
      // RenameDialog, ConflictDialog, FailureSummary all own Enter/Escape/Tab
      // inside their own listeners.
      const modalOpen =
        pendingConfirm !== null ||
        renameDialog !== null ||
        deleteConfirm !== null ||
        conflictDialog !== null ||
        failureSummary !== null;

      // Primary entry for single-target actions (F2/Del/Enter). Anchor wins,
      // else smallest selected index, else nothing.
      const primaryIndex =
        lastAnchorIndex >= 0
          ? lastAnchorIndex
          : selectedIndices.size > 0
            ? Math.min(...selectedIndices)
            : -1;
      const primaryEntry =
        primaryIndex >= 0 ? (entries[primaryIndex] ?? null) : null;

      // Ctrl+A — select all. Blocked on home screen (no file list yet) and
      // while any input is focused (handled by inEditable above).
      if ((e.ctrlKey || e.metaKey) && (e.key === "a" || e.key === "A")) {
        if (currentPath === null) return;
        if (modalOpen) return;
        e.preventDefault();
        selectAll();
        return;
      }

      // Ctrl+C / Ctrl+X / Ctrl+V — clipboard ops. Plain Ctrl chord only;
      // Shift/Alt combos belong to future shortcuts (e.g. Ctrl+Shift+C to
      // "copy as path"), so we explicitly reject them here instead of letting
      // them silently trigger copy.
      if (
        (e.ctrlKey || e.metaKey) &&
        !e.shiftKey &&
        !e.altKey &&
        !modalOpen
      ) {
        if (e.key === "c" || e.key === "C") {
          if (currentPath === null || selectedIndices.size === 0) return;
          e.preventDefault();
          setClipboardFromSelection("copy");
          return;
        }
        if (e.key === "x" || e.key === "X") {
          if (currentPath === null || selectedIndices.size === 0) return;
          e.preventDefault();
          setClipboardFromSelection("cut");
          return;
        }
        if (e.key === "v" || e.key === "V") {
          if (currentPath === null || clipboardMode === null) return;
          e.preventDefault();
          void pasteClipboard();
          return;
        }
      }

      if (modalOpen) return;

      if (e.key === "Backspace") {
        e.preventDefault();
        void goUp();
        return;
      }
      if (e.altKey && e.key === "ArrowLeft") {
        e.preventDefault();
        void goBack();
        return;
      }
      if (e.altKey && e.key === "ArrowRight") {
        e.preventDefault();
        void goForward();
        return;
      }
      if (e.key === "F5") {
        e.preventDefault();
        void reload();
        return;
      }
      if (e.key === "Escape") {
        // Only clear selection if there's something to clear; avoid
        // needlessly stomping state on unrelated Esc presses.
        if (selectedIndices.size === 0 && lastAnchorIndex < 0) return;
        e.preventDefault();
        clearSelection();
        return;
      }
      if (e.key === "F2") {
        // Rename operates on the primary target only; multi-rename is not
        // in scope for Phase 2.2.
        if (!primaryEntry) return;
        e.preventDefault();
        setRenameDialog({
          mode: "rename",
          path: primaryEntry.path,
          initialName: primaryEntry.name,
          title: t("dialog_rename_title_rename")
        });
        return;
      }
      if (e.key === "Delete") {
        // Primary target only for now. Bulk delete comes with
        // clipboard/context-menu work in Phase 2.2-B.
        if (!primaryEntry) return;
        e.preventDefault();
        setDeleteConfirm({
          path: primaryEntry.path,
          mode: e.shiftKey ? "permanent" : "trash"
        });
        return;
      }
      if (e.key === "Enter") {
        if (!primaryEntry) return;
        e.preventDefault();
        if (primaryEntry.type === "directory") {
          void navigate(primaryEntry.path);
        } else {
          void openEntry(primaryEntry.path);
        }
        return;
      }
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        if (entries.length === 0) return;
        e.preventDefault();
        // Steer from the anchor; fall back to the first selected row, then
        // 0 for a cold start. Matches what FileList scrolls into view.
        const pivot =
          lastAnchorIndex >= 0
            ? lastAnchorIndex
            : selectedIndices.size > 0
              ? Math.min(...selectedIndices)
              : 0;
        const delta = e.key === "ArrowDown" ? 1 : -1;
        const nextTarget = clamp(pivot + delta, 0, entries.length - 1);
        if (e.shiftKey) {
          selectRange(nextTarget);
        } else {
          selectOnly(nextTarget);
        }
        return;
      }
      if (e.key === "Home") {
        if (entries.length === 0) return;
        e.preventDefault();
        if (e.shiftKey) {
          selectRange(0);
        } else {
          selectOnly(0);
        }
        return;
      }
      if (e.key === "End") {
        if (entries.length === 0) return;
        e.preventDefault();
        if (e.shiftKey) {
          selectRange(entries.length - 1);
        } else {
          selectOnly(entries.length - 1);
        }
      }
    }

    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [
    goUp,
    goBack,
    goForward,
    reload,
    toggleDev,
    entries,
    selectedIndices,
    lastAnchorIndex,
    selectOnly,
    selectRange,
    selectAll,
    clearSelection,
    navigate,
    openEntry,
    currentPath,
    pendingConfirm,
    renameDialog,
    deleteConfirm,
    conflictDialog,
    failureSummary,
    clipboardMode,
    setClipboardFromSelection,
    pasteClipboard
  ]);

  // Build the row-variant menu. Entries differ slightly for directories
  // (activate = "열기") vs files/symlinks ("기본 앱으로 열기"). Disabled
  // states are computed eagerly — the menu item list is re-created on every
  // open so no stale snapshots.
  function buildRowMenu(entry: Entry): ContextMenuItem[] {
    const canPaste = clipboardMode !== null && currentPath !== null;
    return [
      {
        label:
          entry.type === "directory"
            ? t("context_open")
            : t("context_open_default"),
        shortcut: "Enter",
        onClick: () => {
          if (entry.type === "directory") {
            void navigate(entry.path);
          } else {
            void openEntry(entry.path);
          }
        }
      },
      {
        label: t("context_reveal"),
        onClick: () => {
          void revealEntry(entry.path);
        }
      },
      { label: "", separator: true, onClick: () => {} },
      {
        label: t("context_copy"),
        shortcut: "Ctrl+C",
        onClick: () => setClipboardFromSelection("copy")
      },
      {
        label: t("context_cut"),
        shortcut: "Ctrl+X",
        onClick: () => setClipboardFromSelection("cut")
      },
      {
        label: t("context_paste"),
        shortcut: "Ctrl+V",
        disabled: !canPaste,
        onClick: () => {
          void pasteClipboard();
        }
      },
      { label: "", separator: true, onClick: () => {} },
      {
        label: t("context_rename"),
        shortcut: "F2",
        onClick: () =>
          setRenameDialog({
            mode: "rename",
            path: entry.path,
            initialName: entry.name,
            title: t("dialog_rename_title_rename")
          })
      },
      {
        label: t("context_trash"),
        shortcut: "Del",
        danger: true,
        onClick: () =>
          setDeleteConfirm({ path: entry.path, mode: "trash" })
      },
      {
        label: t("context_permanent_delete"),
        shortcut: "Shift+Del",
        danger: true,
        onClick: () =>
          setDeleteConfirm({ path: entry.path, mode: "permanent" })
      }
    ];
  }

  // Blank-area menu. Paste needs both a current directory AND something in
  // the clipboard; create-folder only needs a current directory (guard is
  // inside store.createFolder anyway — we just disable proactively).
  function buildBlankMenu(): ContextMenuItem[] {
    const canPaste = clipboardMode !== null && currentPath !== null;
    return [
      {
        label: t("context_new_folder"),
        disabled: currentPath === null,
        onClick: openCreateFolder
      },
      { label: "", separator: true, onClick: () => {} },
      {
        label: t("context_paste"),
        shortcut: "Ctrl+V",
        disabled: !canPaste,
        onClick: () => {
          void pasteClipboard();
        }
      },
      {
        label: t("context_refresh"),
        shortcut: "F5",
        onClick: () => {
          void reload();
        }
      }
    ];
  }

  return (
    <div className="app">
      <Toolbar
        onToggleDevPanel={toggleDev}
        onCreateFolder={openCreateFolder}
      />
      <ErrorBanner />
      {currentPath === null ? (
        <div className="main-home">
          <HomeScreen />
        </div>
      ) : (
        <div className="main">
          <Sidebar />
          <FileList
            onContextMenuRow={(entry, _index, x, y) =>
              setContextMenu({ kind: "row", entry, x, y })
            }
            onContextMenuBlank={(x, y) =>
              setContextMenu({ kind: "blank", x, y })
            }
          />
        </div>
      )}
      <StatusBar />
      <DevPanel open={devOpen} onClose={() => setDevOpen(false)} />

      {pendingConfirm && (
        <ConfirmDialog
          title={pendingConfirm.title}
          message={pendingConfirm.message}
          variant={
            pendingConfirm.kind === "system-path" ? "warning" : "danger"
          }
          confirmLabel={t("common_continue")}
          cancelLabel={t("common_cancel")}
          onConfirm={() => {
            void resolvePendingConfirm();
          }}
          onCancel={cancelPendingConfirm}
        />
      )}

      {renameDialog && (
        <RenameDialog
          initialName={renameDialog.initialName}
          title={renameDialog.title}
          onConfirm={(name) => {
            if (renameDialog.mode === "create") {
              void createFolder(name);
            } else {
              void renameEntry(renameDialog.path, name);
            }
            setRenameDialog(null);
          }}
          onCancel={() => setRenameDialog(null)}
        />
      )}

      {contextMenu && (
        <ContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          items={
            contextMenu.kind === "row"
              ? buildRowMenu(contextMenu.entry)
              : buildBlankMenu()
          }
          onClose={() => setContextMenu(null)}
        />
      )}

      <ProgressToasts />

      {conflictDialog && (
        <ConflictDialog
          conflict={conflictDialog.conflict}
          remainingCount={conflictDialog.remaining}
          onResolve={(strategy, applyToAll) => {
            const { resolve } = conflictDialog;
            setConflictDialog(null);
            resolve({ strategy, applyToAll });
          }}
          onCancel={() => {
            const { resolve } = conflictDialog;
            setConflictDialog(null);
            // null = batch canceled. pasteClipboard short-circuits the rest.
            resolve(null);
          }}
        />
      )}

      {failureSummary && (
        <FailureSummary
          title={failureSummary.title}
          totalAttempted={failureSummary.totalAttempted}
          failures={failureSummary.failures}
          onClose={() => {
            setFailureSummary(null);
            // After dismissal, scan for the next still-unshown failed job —
            // the subscription only fires on store mutations, so a job that
            // already terminated while this modal was open would otherwise
            // never surface. We pull the live state synchronously here.
            const s = useJobs.getState();
            for (const id of s.order) {
              const j = s.jobs[id];
              if (!j) continue;
              const terminal =
                j.state === "done" ||
                j.state === "failed" ||
                j.state === "canceled";
              if (!terminal) continue;
              if (j.failures.length === 0) continue;
              if (shownFailuresRef.current.has(id)) continue;
              shownFailuresRef.current.add(id);
              setFailureSummary({
                jobId: id,
                title:
                  j.kind === "copy"
                    ? t("toast_result_copy_title")
                    : t("toast_result_move_title"),
                totalAttempted: Math.max(j.fileTotal, j.failures.length),
                failures: j.failures
              });
              break;
            }
          }}
        />
      )}

      {deleteConfirm && (
        <ConfirmDialog
          title={
            deleteConfirm.mode === "permanent"
              ? t("dialog_delete_title_permanent")
              : t("dialog_delete_title_trash")
          }
          message={
            deleteConfirm.mode === "permanent"
              ? t("dialog_delete_message_permanent", [
                  basename(deleteConfirm.path)
                ])
              : t("dialog_delete_message_trash", [
                  basename(deleteConfirm.path)
                ])
          }
          variant="danger"
          confirmLabel={
            deleteConfirm.mode === "permanent"
              ? t("dialog_delete_title_permanent")
              : t("dialog_delete_title_trash")
          }
          cancelLabel={t("common_cancel")}
          onConfirm={() => {
            void deleteEntry(deleteConfirm.path, deleteConfirm.mode);
            setDeleteConfirm(null);
          }}
          onCancel={() => setDeleteConfirm(null)}
        />
      )}
    </div>
  );
}
