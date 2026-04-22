import { splitPath } from "../utils/format";
import { useExplorerStore } from "../store/explorer";

interface Props {
  path: string | null;
}

export function Breadcrumb({ path }: Props): JSX.Element {
  const navigate = useExplorerStore((s) => s.navigate);
  const goHome = useExplorerStore((s) => s.goHome);

  const segments = path === null ? [] : splitPath(path);

  return (
    <nav className="breadcrumb" aria-label="path">
      <button
        type="button"
        className="breadcrumb-home"
        onClick={goHome}
        title="Drives"
      >
        Drives
      </button>
      {segments.map((seg, i) => (
        <span key={seg.path} className="breadcrumb-seg">
          <span className="breadcrumb-sep">›</span>
          <button
            type="button"
            className="breadcrumb-link"
            onClick={() => {
              void navigate(seg.path);
            }}
            disabled={i === segments.length - 1}
            title={seg.path}
          >
            {seg.label}
          </button>
        </span>
      ))}
    </nav>
  );
}
