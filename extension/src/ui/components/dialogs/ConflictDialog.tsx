import { useEffect, useRef, useState } from "react";
import { formatBytes, formatTime } from "../../utils/format";

/**
 * Phase 2.4 — paste-time conflict prompt.
 *
 * Why dialog-side resolution? The Go Host explicitly rejects
 * `conflict: "prompt"` for both copy and move (BadRequest), so the UI is
 * the *only* layer that can decide overwrite/skip/rename. Pre-scanning
 * destinations with stat() and routing each conflict through this modal
 * keeps that responsibility in one place.
 *
 * UX rules borrowed from the platform file managers:
 *   - Default focus on "이름 변경 (자동)" — the safest, non-destructive choice.
 *   - Esc maps to onCancel (abort the whole batch, not just this one).
 *   - "이후 전부 적용" carries the chosen strategy across the rest of the
 *     batch. The caller short-circuits subsequent prompts when set.
 *   - remainingCount is "1 = this is the last" so the header text reads
 *     correctly when there's exactly one conflict left.
 */

export interface ConflictInfo {
  srcPath: string;
  dstPath: string;
  srcSize?: number;
  dstSize?: number;
  srcMtime?: number;
  dstMtime?: number;
}

export type ConflictStrategyChoice = "overwrite" | "skip" | "rename";

interface Props {
  conflict: ConflictInfo;
  /**
   * Number of conflicts left in the batch INCLUDING this one. We surface a
   * "남은 충돌 N건" hint when > 1 so the user understands what "이후 전부
   * 적용" applies to.
   */
  remainingCount: number;
  onResolve: (strategy: ConflictStrategyChoice, applyToAll: boolean) => void;
  onCancel: () => void;
}

// Esc closes; Tab cycles a small focus ring of buttons + the checkbox so the
// modal is fully keyboard-operable. Enter triggers whichever button has focus
// — no global "Enter = rename" shortcut, because that would silently accept
// the safe-but-not-always-correct choice when the user may be reading the
// metadata table.
export function ConflictDialog({
  conflict,
  remainingCount,
  onResolve,
  onCancel
}: Props): JSX.Element {
  const [applyToAll, setApplyToAll] = useState(false);
  const renameBtnRef = useRef<HTMLButtonElement | null>(null);
  const overwriteBtnRef = useRef<HTMLButtonElement | null>(null);
  const skipBtnRef = useRef<HTMLButtonElement | null>(null);
  const checkboxRef = useRef<HTMLInputElement | null>(null);

  // Default focus on the safer choice. Matches RenameDialog's "users hit
  // Enter without thinking" defensive posture.
  useEffect(() => {
    renameBtnRef.current?.focus();
  }, []);

  useEffect(() => {
    function onKey(e: KeyboardEvent): void {
      if (e.key === "Escape") {
        e.preventDefault();
        onCancel();
        return;
      }
      if (e.key === "Tab") {
        const order: Array<HTMLButtonElement | HTMLInputElement> = [];
        if (renameBtnRef.current) order.push(renameBtnRef.current);
        if (overwriteBtnRef.current) order.push(overwriteBtnRef.current);
        if (skipBtnRef.current) order.push(skipBtnRef.current);
        if (checkboxRef.current) order.push(checkboxRef.current);
        if (order.length === 0) return;
        const active = document.activeElement;
        const idx = order.findIndex((el) => el === active);
        e.preventDefault();
        const dir = e.shiftKey ? -1 : 1;
        const next = order[(idx + dir + order.length) % order.length];
        next?.focus();
      }
    }
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [onCancel]);

  const srcName = lastSegment(conflict.srcPath);
  const dstName = lastSegment(conflict.dstPath);

  return (
    <div
      className="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      aria-labelledby="conflict-dialog-title"
      onMouseDown={(e) => {
        // Backdrop click cancels the whole batch — same as Esc. Mousedown
        // (not click) so a drag started inside the dialog and released on
        // the backdrop doesn't trigger an accidental cancel.
        if (e.target === e.currentTarget) onCancel();
      }}
    >
      <div
        className="dialog"
        onMouseDown={(e) => e.stopPropagation()}
        onClick={(e) => e.stopPropagation()}
      >
        <h2 id="conflict-dialog-title">대상 이름이 이미 존재합니다</h2>
        {remainingCount > 1 && (
          <div className="conflict-remaining">
            남은 충돌 {remainingCount}건 — 선택을 일괄 적용하려면 아래
            체크박스를 사용하세요.
          </div>
        )}
        <p className="dialog-message">
          <code>{dstName}</code> 파일/폴더가 대상 위치에 이미 있습니다. 어떻게
          처리할까요?
        </p>

        <div className="conflict-compare">
          <div className="conflict-compare-side">
            <h4>원본 ({srcName})</h4>
            <div
              className="conflict-compare-path"
              title={conflict.srcPath}
            >
              {conflict.srcPath}
            </div>
            <div className="conflict-compare-meta">
              크기: {formatBytes(conflict.srcSize ?? null)}
              <br />
              수정: {formatTime(conflict.srcMtime ?? null)}
            </div>
          </div>
          <div className="conflict-compare-side">
            <h4>대상 ({dstName})</h4>
            <div
              className="conflict-compare-path"
              title={conflict.dstPath}
            >
              {conflict.dstPath}
            </div>
            <div className="conflict-compare-meta">
              크기: {formatBytes(conflict.dstSize ?? null)}
              <br />
              수정: {formatTime(conflict.dstMtime ?? null)}
            </div>
          </div>
        </div>

        <label className="conflict-apply-all">
          <input
            ref={checkboxRef}
            type="checkbox"
            checked={applyToAll}
            onChange={(e) => setApplyToAll(e.target.checked)}
          />
          이후 충돌에도 같은 선택 적용 ({Math.max(0, remainingCount - 1)}건)
        </label>

        <div className="dialog-buttons">
          <button
            ref={skipBtnRef}
            type="button"
            onClick={() => onResolve("skip", applyToAll)}
          >
            건너뛰기
          </button>
          <button
            ref={overwriteBtnRef}
            type="button"
            className="danger"
            onClick={() => onResolve("overwrite", applyToAll)}
          >
            덮어쓰기
          </button>
          <button
            ref={renameBtnRef}
            type="button"
            onClick={() => onResolve("rename", applyToAll)}
          >
            이름 변경 (자동)
          </button>
        </div>
      </div>
    </div>
  );
}

// Same logic as utils/format.basename, inlined to keep the dialog dependency
// surface flat (basename has Windows-vs-POSIX root edge cases that ConflictInfo
// paths never hit because they always include a filename segment).
function lastSegment(path: string): string {
  if (!path) return "";
  const idx = Math.max(path.lastIndexOf("\\"), path.lastIndexOf("/"));
  if (idx < 0) return path;
  return path.slice(idx + 1) || path;
}
