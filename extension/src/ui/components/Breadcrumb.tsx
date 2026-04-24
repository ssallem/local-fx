import { splitPath } from "../utils/format";
import { useExplorerStore } from "../store/explorer";
import { t } from "../utils/i18n";

interface Props {
  path: string | null;
}

export function Breadcrumb({ path }: Props): JSX.Element {
  const navigate = useExplorerStore((s) => s.navigate);
  const goHome = useExplorerStore((s) => s.goHome);

  const segments = path === null ? [] : splitPath(path);
  const drivesLabel = t("breadcrumb_drives");

  return (
    <nav className="breadcrumb" aria-label={t("breadcrumb_aria")}>
      <button
        type="button"
        className="breadcrumb-home"
        onClick={goHome}
        title={drivesLabel}
      >
        {drivesLabel}
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
