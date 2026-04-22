import { useCallback, useEffect, useState } from "react";
import "./App.css";
import { useExplorerStore } from "./store/explorer";
import { Toolbar } from "./components/Toolbar";
import { Sidebar } from "./components/Sidebar";
import { FileList } from "./components/FileList";
import { StatusBar } from "./components/StatusBar";
import { ErrorBanner } from "./components/ErrorBanner";
import { DevPanel } from "./components/DevPanel";
import { ConfirmDialog, RenameDialog } from "./components/dialogs";
import { basename, formatBytes } from "./utils/format";
import type { Drive, RemoveMode } from "../types/shared";

/**
 * Home screen (drive grid). Shown when currentPath === null so first-launch
 * users see a list of their disks without any sidebar/list chrome.
 */
function HomeScreen(): JSX.Element {
  const drives = useExplorerStore((s) => s.drives);
  const navigate = useExplorerStore((s) => s.navigate);
  const loading = useExplorerStore((s) => s.loading);

  if (loading && drives.length === 0) {
    return <div className="home-title">Loading drives…</div>;
  }

  return (
    <div style={{ width: "100%", maxWidth: 960 }}>
      <div className="home-title">Local Explorer</div>
      <div className="home-grid">
        {drives.length === 0 ? (
          <div className="sidebar-empty">
            No drives detected. Open the dev panel (⚙) to ping the host.
          </div>
        ) : (
          drives.map((d: Drive) => (
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
                {d.totalBytes > 0
                  ? ` • ${formatBytes(d.freeBytes)} free / ${formatBytes(d.totalBytes)}`
                  : ""}
                {d.readOnly ? " • read-only" : ""}
              </div>
            </button>
          ))
        )}
      </div>
    </div>
  );
}

type RenameDialogState =
  | { mode: "create"; initialName: string; title: string }
  | { mode: "rename"; path: string; initialName: string; title: string };

type DeleteConfirmState = { path: string; mode: RemoveMode };

export function App(): JSX.Element {
  const currentPath = useExplorerStore((s) => s.currentPath);
  const loadDrives = useExplorerStore((s) => s.loadDrives);
  const goUp = useExplorerStore((s) => s.goUp);
  const goBack = useExplorerStore((s) => s.goBack);
  const goForward = useExplorerStore((s) => s.goForward);
  const reload = useExplorerStore((s) => s.reload);
  const entries = useExplorerStore((s) => s.entries);
  const selectedIndex = useExplorerStore((s) => s.selectedIndex);
  const setSelectedIndex = useExplorerStore((s) => s.setSelectedIndex);
  const navigate = useExplorerStore((s) => s.navigate);
  const openEntry = useExplorerStore((s) => s.openEntry);
  const createFolder = useExplorerStore((s) => s.createFolder);
  const renameEntry = useExplorerStore((s) => s.renameEntry);
  const deleteEntry = useExplorerStore((s) => s.deleteEntry);
  const pendingConfirm = useExplorerStore((s) => s.pendingConfirm);
  const resolvePendingConfirm = useExplorerStore(
    (s) => s.resolvePendingConfirm
  );
  const cancelPendingConfirm = useExplorerStore((s) => s.cancelPendingConfirm);

  const [devOpen, setDevOpen] = useState(false);
  const toggleDev = useCallback(() => setDevOpen((v) => !v), []);

  const [renameDialog, setRenameDialog] = useState<RenameDialogState | null>(
    null
  );
  const [deleteConfirm, setDeleteConfirm] = useState<DeleteConfirmState | null>(
    null
  );

  const openCreateFolder = useCallback(() => {
    setRenameDialog({
      mode: "create",
      initialName: "새 폴더",
      title: "새 폴더 만들기"
    });
  }, []);

  // Bootstrap
  useEffect(() => {
    void loadDrives();
  }, [loadDrives]);

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

      // Suppress keyboard nav while any modal is open — ConfirmDialog and
      // RenameDialog own Enter/Escape/Tab inside their own listeners.
      const modalOpen =
        pendingConfirm !== null ||
        renameDialog !== null ||
        deleteConfirm !== null;
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
      if (e.key === "F2") {
        const entry = entries[selectedIndex];
        if (!entry) return;
        e.preventDefault();
        setRenameDialog({
          mode: "rename",
          path: entry.path,
          initialName: entry.name,
          title: "이름 변경"
        });
        return;
      }
      if (e.key === "Delete") {
        const entry = entries[selectedIndex];
        if (!entry) return;
        e.preventDefault();
        setDeleteConfirm({
          path: entry.path,
          mode: e.shiftKey ? "permanent" : "trash"
        });
        return;
      }
      if (e.key === "Enter") {
        const entry = entries[selectedIndex];
        if (!entry) return;
        e.preventDefault();
        if (entry.type === "directory") {
          void navigate(entry.path);
        } else {
          void openEntry(entry.path);
        }
        return;
      }
      if (e.key === "ArrowDown") {
        if (entries.length === 0) return;
        e.preventDefault();
        const next = Math.min(
          entries.length - 1,
          selectedIndex < 0 ? 0 : selectedIndex + 1
        );
        setSelectedIndex(next);
        return;
      }
      if (e.key === "ArrowUp") {
        if (entries.length === 0) return;
        e.preventDefault();
        const next = Math.max(0, selectedIndex < 0 ? 0 : selectedIndex - 1);
        setSelectedIndex(next);
        return;
      }
      if (e.key === "Home") {
        if (entries.length === 0) return;
        e.preventDefault();
        setSelectedIndex(0);
        return;
      }
      if (e.key === "End") {
        if (entries.length === 0) return;
        e.preventDefault();
        setSelectedIndex(entries.length - 1);
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
    selectedIndex,
    setSelectedIndex,
    navigate,
    openEntry,
    pendingConfirm,
    renameDialog,
    deleteConfirm
  ]);

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
          <FileList />
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
          confirmLabel="계속"
          cancelLabel="취소"
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

      {deleteConfirm && (
        <ConfirmDialog
          title={
            deleteConfirm.mode === "permanent"
              ? "영구 삭제"
              : "휴지통으로 이동"
          }
          message={
            deleteConfirm.mode === "permanent"
              ? `'${basename(deleteConfirm.path)}'를 영구 삭제합니다. 이 작업은 되돌릴 수 없습니다.`
              : `'${basename(deleteConfirm.path)}'를 휴지통으로 이동합니다.`
          }
          variant="danger"
          confirmLabel={
            deleteConfirm.mode === "permanent"
              ? "영구 삭제"
              : "휴지통으로 이동"
          }
          cancelLabel="취소"
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
