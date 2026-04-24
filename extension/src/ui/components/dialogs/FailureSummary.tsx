import { useEffect, useRef } from "react";
import type { FailureInfo } from "../../../types/shared";
import { t } from "../../utils/i18n";

/**
 * Phase 2.4 — terminal-state results modal for streaming copy/move jobs.
 *
 * Shown automatically (App.tsx subscribes to the jobs store) when a job
 * settles into done/failed/canceled with at least one entry in `failures`.
 * The toast already shows a collapsed `<details>` summary; this dialog
 * surfaces the full table for batches large enough that scanning the toast
 * is impractical.
 *
 * Single OK button: this is informational only, the failures themselves
 * have already happened and there's nothing to "confirm". onClose triggers
 * via Esc / backdrop click / OK.
 */

interface Props {
  title: string;
  totalAttempted: number;
  failures: FailureInfo[];
  onClose: () => void;
}

export function FailureSummary({
  title,
  totalAttempted,
  failures,
  onClose
}: Props): JSX.Element {
  const okBtnRef = useRef<HTMLButtonElement | null>(null);

  // autoFocus on the button gives Enter a sensible default. Mirrors the
  // ConfirmDialog pattern where focus = primary action.
  useEffect(() => {
    okBtnRef.current?.focus();
  }, []);

  useEffect(() => {
    function onKey(e: KeyboardEvent): void {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
      }
    }
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [onClose]);

  // Defensive arithmetic: the Host's per-failure list may exceed
  // totalAttempted in pathological cases (e.g. a directory-walk explosion
  // counted as one attempt but yielding many failures). Clamp at zero so the
  // summary never reads "성공: -3건".
  const successCount = Math.max(0, totalAttempted - failures.length);

  return (
    <div
      className="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      aria-labelledby="failure-summary-title"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        className="dialog"
        onMouseDown={(e) => e.stopPropagation()}
        onClick={(e) => e.stopPropagation()}
      >
        <h2 id="failure-summary-title">{title}</h2>
        <p className="dialog-message">
          {t("dialog_failure_summary_stats", [successCount, failures.length])}
        </p>
        <div className="failure-list">
          <table>
            <thead>
              <tr>
                <th>{t("dialog_failure_col_path")}</th>
                <th>{t("dialog_failure_col_code")}</th>
                <th>{t("dialog_failure_col_message")}</th>
              </tr>
            </thead>
            <tbody>
              {failures.map((f, i) => (
                // Failures don't carry a stable id; index is fine because
                // the list is built once per modal open and never reordered.
                // eslint-disable-next-line react/no-array-index-key
                <tr key={i}>
                  <td title={f.path}>
                    <code>{shortenPath(f.path)}</code>
                  </td>
                  <td>{f.code}</td>
                  <td>{f.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="dialog-buttons">
          <button ref={okBtnRef} type="button" onClick={onClose}>
            {t("common_ok")}
          </button>
        </div>
      </div>
    </div>
  );
}

// Shorten from the left so the trailing filename — usually the disambiguating
// part — stays visible. Mirrors the helper inside ProgressToasts but kept
// local to avoid a cross-component utility import for one call site.
const SHORTEN_MAX = 64;
function shortenPath(s: string): string {
  if (s.length <= SHORTEN_MAX) return s;
  return "…" + s.slice(-(SHORTEN_MAX - 1));
}
