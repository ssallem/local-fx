// Phase 2.3 — bottom-right toast stack visualising the jobs store.
//
// Each running / canceling / terminal Job in `useJobs` renders a card. Cards
// in a terminal state (done / canceled / failed) auto-dismiss 3s after their
// `finishedAt` timestamp, giving the user a beat to read the result before
// the toast disappears. Manual dismiss via the × button is always available.
//
// The component is purely presentational over the jobs store — the store
// owns all state transitions, this file only reads + dispatches `removeJob`
// and `markCanceling`.

import { useEffect } from "react";
import { useJobs, type Job } from "../store/jobs";
import { formatBytes } from "../utils/format";
import { t } from "../utils/i18n";

const AUTO_DISMISS_MS = 3000;

export function ProgressToasts(): JSX.Element | null {
  const jobs = useJobs((s) => s.jobs);
  const order = useJobs((s) => s.order);
  const removeJob = useJobs((s) => s.removeJob);

  // Auto-dismiss terminal toasts after AUTO_DISMISS_MS. We re-arm timers on
  // every render where order/jobs change; cleanup clears any pending timers
  // so a fast-completing/replaced job doesn't leak a stale removeJob call.
  useEffect(() => {
    const timers: number[] = [];
    const now = Date.now();
    for (const id of order) {
      const j = jobs[id];
      if (!j || !j.finishedAt) continue;
      const remaining = AUTO_DISMISS_MS - (now - j.finishedAt);
      if (remaining <= 0) {
        // Already past the dismiss window — drop on the next tick so we
        // don't synchronously mutate state during render.
        timers.push(window.setTimeout(() => removeJob(id), 0));
      } else {
        timers.push(window.setTimeout(() => removeJob(id), remaining));
      }
    }
    return () => {
      for (const t of timers) window.clearTimeout(t);
    };
  }, [jobs, order, removeJob]);

  if (order.length === 0) return null;

  return (
    <div className="progress-toasts" role="status" aria-live="polite">
      {order.map((id) => {
        const j = jobs[id];
        if (!j) return null;
        return <JobCard key={id} job={j} />;
      })}
    </div>
  );
}

function JobCard({ job }: { job: Job }): JSX.Element {
  const markCanceling = useJobs((s) => s.markCanceling);
  const removeJob = useJobs((s) => s.removeJob);

  // Avoid divide-by-zero before the first progress frame arrives.
  const pct =
    job.bytesTotal > 0
      ? Math.min(100, Math.round((job.bytesDone / job.bytesTotal) * 100))
      : 0;

  const onCancel = async (): Promise<void> => {
    if (!job.cancel || job.state !== "running") return;
    markCanceling(job.id);
    try {
      await job.cancel();
    } catch {
      // The terminal "done" event (or the StreamHandle.promise) will resolve
      // the canonical state. We've already flipped the badge to "canceling";
      // a thrown cancel shouldn't surface its own error toast.
    }
  };

  let stateLabel: string;
  switch (job.state) {
    case "running":
      stateLabel = `${pct}%`;
      break;
    case "canceling":
      stateLabel = t("toast_canceling");
      break;
    case "done":
      stateLabel = t("toast_done");
      break;
    case "canceled":
      stateLabel = t("toast_canceled");
      break;
    case "failed":
      stateLabel =
        job.failures.length > 0
          ? t("toast_failed_count", [job.failures.length])
          : t("toast_failed");
      break;
  }

  const isTerminal =
    job.state === "done" ||
    job.state === "canceled" ||
    job.state === "failed";

  return (
    <div className={`progress-toast state-${job.state}`}>
      <div className="progress-toast-header">
        <span className="progress-toast-label" title={job.label}>
          {job.kind === "copy" ? t("toast_kind_copy") : t("toast_kind_move")}:{" "}
          {job.label}
        </span>
        <span className="progress-toast-state">{stateLabel}</span>
        {isTerminal && (
          <button
            type="button"
            className="progress-toast-close"
            onClick={() => removeJob(job.id)}
            aria-label={t("common_close")}
          >
            ×
          </button>
        )}
      </div>

      {(job.state === "running" || job.state === "canceling") && (
        <div className="progress-toast-body">
          <div
            className="progress-bar"
            role="progressbar"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={pct}
          >
            <div
              className="progress-bar-fill"
              style={{ width: `${pct}%` }}
            />
          </div>
          <div className="progress-toast-meta">
            <span>
              {formatBytes(job.bytesDone)} / {formatBytes(job.bytesTotal)}
            </span>
            {job.fileTotal > 1 && (
              <span>
                {t("toast_file_progress", [job.fileDone, job.fileTotal])}
              </span>
            )}
            {job.rate !== undefined && job.rate > 0 && (
              <span>{formatBytes(job.rate)}/s</span>
            )}
          </div>
          {job.currentPath && (
            <div className="progress-toast-path" title={job.currentPath}>
              {shorten(job.currentPath, 60)}
            </div>
          )}
          {job.state === "running" && job.cancel && (
            <button
              type="button"
              className="progress-toast-cancel"
              onClick={() => {
                void onCancel();
              }}
            >
              {t("toast_cancel_button")}
            </button>
          )}
        </div>
      )}

      {job.state === "failed" && job.errorMessage && (
        <div className="progress-toast-error">{job.errorMessage}</div>
      )}

      {(job.state === "failed" || job.state === "done") &&
        job.failures.length > 0 && (
          <details className="progress-toast-failures">
            <summary>
              {t("toast_failures_summary", [job.failures.length])}
            </summary>
            <ul>
              {job.failures.slice(0, 10).map((f, i) => (
                <li key={i}>
                  <code>{f.path}</code>: {f.code}
                </li>
              ))}
              {job.failures.length > 10 && (
                <li>{t("toast_failures_more", [job.failures.length - 10])}</li>
              )}
            </ul>
          </details>
        )}
    </div>
  );
}

// Shorten a path from the left so the trailing filename stays visible.
// "C:/a/b/c/d/very-long-name.txt" → "…/c/d/very-long-name.txt"
function shorten(s: string, max: number): string {
  if (s.length <= max) return s;
  return "…" + s.slice(-(max - 1));
}
