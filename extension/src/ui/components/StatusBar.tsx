import { useExplorerStore } from "../store/explorer";
import { t } from "../utils/i18n";

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
        <span>{t("statusbar_drives_count", [driveCount])}</span>
      </footer>
    );
  }

  const showWarn = total > LARGE_DIR_WARN_THRESHOLD;
  // Build the items summary as a single translated string with placeholders so
  // translators can reorder shown/total/page without English-biased glue text.
  const itemsLine = t("statusbar_items", [entries.length, total, page]);
  const selectedSuffix =
    selectionCount > 0
      ? ` · ${t("statusbar_selected", [selectionCount])}`
      : "";

  return (
    <footer className="statusbar">
      <span>
        {itemsLine}
        {selectedSuffix}
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
          {loading ? t("common_loading") : t("statusbar_load_more")}
        </button>
      )}
      {showWarn && (
        <span className="statusbar-warn">
          {t("statusbar_large_dir", [total])}
        </span>
      )}
    </footer>
  );
}
