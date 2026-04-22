import { useCallback, useEffect, useState } from "react";
import "./App.css";
import { useExplorerStore } from "./store/explorer";
import { Toolbar } from "./components/Toolbar";
import { Sidebar } from "./components/Sidebar";
import { FileList } from "./components/FileList";
import { StatusBar } from "./components/StatusBar";
import { ErrorBanner } from "./components/ErrorBanner";
import { DevPanel } from "./components/DevPanel";
import { formatBytes } from "./utils/format";
import type { Drive } from "../types/shared";

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

  const [devOpen, setDevOpen] = useState(false);
  const toggleDev = useCallback(() => setDevOpen((v) => !v), []);

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
      if (e.key === "Enter") {
        const entry = entries[selectedIndex];
        if (entry && entry.type === "directory") {
          e.preventDefault();
          void navigate(entry.path);
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
    navigate
  ]);

  return (
    <div className="app">
      <Toolbar onToggleDevPanel={toggleDev} />
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
    </div>
  );
}
