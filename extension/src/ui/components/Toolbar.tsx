import { useExplorerStore } from "../store/explorer";
import { t } from "../utils/i18n";
import { Breadcrumb } from "./Breadcrumb";

interface Props {
  onToggleDevPanel: () => void;
  onCreateFolder: () => void;
}

export function Toolbar({
  onToggleDevPanel,
  onCreateFolder
}: Props): JSX.Element {
  const currentPath = useExplorerStore((s) => s.currentPath);
  const historyIndex = useExplorerStore((s) => s.historyIndex);
  const history = useExplorerStore((s) => s.history);
  const goBack = useExplorerStore((s) => s.goBack);
  const goForward = useExplorerStore((s) => s.goForward);
  const goUp = useExplorerStore((s) => s.goUp);
  const reload = useExplorerStore((s) => s.reload);

  const canBack = historyIndex >= 0; // at 0 → step back to home
  const canForward = historyIndex < history.length - 1;
  const canUp = currentPath !== null;
  const canCreate = currentPath !== null;

  return (
    <header className="toolbar">
      <div className="toolbar-buttons">
        <button
          type="button"
          onClick={() => {
            void goBack();
          }}
          disabled={!canBack}
          title={t("toolbar_back_title")}
          aria-label={t("toolbar_back")}
        >
          ←
        </button>
        <button
          type="button"
          onClick={() => {
            void goForward();
          }}
          disabled={!canForward}
          title={t("toolbar_forward_title")}
          aria-label={t("toolbar_forward")}
        >
          →
        </button>
        <button
          type="button"
          onClick={() => {
            void goUp();
          }}
          disabled={!canUp}
          title={t("toolbar_up_title")}
          aria-label={t("toolbar_up")}
        >
          ↑
        </button>
        <button
          type="button"
          onClick={() => {
            void reload();
          }}
          title={t("toolbar_reload_title")}
          aria-label={t("toolbar_reload")}
        >
          ⟳
        </button>
        <button
          type="button"
          onClick={onCreateFolder}
          disabled={!canCreate}
          title={t("toolbar_new_folder")}
          aria-label={t("toolbar_new_folder")}
        >
          {t("toolbar_new_folder_button")}
        </button>
      </div>
      <Breadcrumb path={currentPath} />
      <div className="toolbar-spacer" />
      <button
        type="button"
        className="toolbar-dev"
        onClick={onToggleDevPanel}
        title={t("toolbar_dev_panel_title")}
        aria-label={t("toolbar_dev_panel")}
      >
        ⚙
      </button>
    </header>
  );
}
