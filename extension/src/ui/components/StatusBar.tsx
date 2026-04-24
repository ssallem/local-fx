import { useExplorerStore } from "../store/explorer";

const LARGE_DIR_WARN_THRESHOLD = 5000;

export function StatusBar(): JSX.Element {
  const entries = useExplorerStore((s) => s.entries);
  const total = useExplorerStore((s) => s.total);
  const page = useExplorerStore((s) => s.page);
  const hasMore = useExplorerStore((s) => s.hasMore);
  const loading = useExplorerStore((s) => s.loading);
  const loadMore = useExplorerStore((s) => s.loadMore);
  const currentPath = useExplorerStore((s) => s.currentPath);
  const driveCount = useExplorerStore((s) => s.drives.length);
  const selectionCount = useExplorerStore((s) => s.selectedIndices.size);

  if (currentPath === null) {
    return (
      <footer className="statusbar">
        <span>{driveCount} drives</span>
      </footer>
    );
  }

  const showWarn = total > LARGE_DIR_WARN_THRESHOLD;

  return (
    <footer className="statusbar">
      <span>
        {entries.length} of {total} items • Page {page}
        {selectionCount > 0 ? ` • ${selectionCount} selected` : ""}
      </span>
      {hasMore && (
        <button
          type="button"
          className="statusbar-loadmore"
          onClick={() => {
            void loadMore();
          }}
          disabled={loading}
        >
          {loading ? "Loading…" : "Load more"}
        </button>
      )}
      {showWarn && (
        <span className="statusbar-warn">
          Large directory ({total} entries) — performance may degrade; virtual
          scroll comes in Phase 1.5.
        </span>
      )}
    </footer>
  );
}
