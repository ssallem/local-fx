import { useExplorerStore } from "../store/explorer";
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
          title="Back (Alt+Left)"
          aria-label="Back"
        >
          ←
        </button>
        <button
          type="button"
          onClick={() => {
            void goForward();
          }}
          disabled={!canForward}
          title="Forward (Alt+Right)"
          aria-label="Forward"
        >
          →
        </button>
        <button
          type="button"
          onClick={() => {
            void goUp();
          }}
          disabled={!canUp}
          title="Up (Backspace)"
          aria-label="Up"
        >
          ↑
        </button>
        <button
          type="button"
          onClick={() => {
            void reload();
          }}
          title="Reload (F5)"
          aria-label="Reload"
        >
          ⟳
        </button>
        <button
          type="button"
          onClick={onCreateFolder}
          disabled={!canCreate}
          title="새 폴더"
          aria-label="새 폴더"
        >
          + 새 폴더
        </button>
      </div>
      <Breadcrumb path={currentPath} />
      <div className="toolbar-spacer" />
      <button
        type="button"
        className="toolbar-dev"
        onClick={onToggleDevPanel}
        title="Dev panel (Ctrl+Shift+P)"
        aria-label="Dev panel"
      >
        ⚙
      </button>
    </header>
  );
}
