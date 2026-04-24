import { useEffect, useRef } from "react";
import { t } from "../../utils/i18n";

export type ConfirmVariant = "default" | "danger" | "warning";

interface Props {
  title: string;
  message: string;
  variant?: ConfirmVariant;
  confirmLabel?: string;
  cancelLabel?: string;
  onConfirm: () => void;
  onCancel: () => void;
}

/**
 * Shared confirm modal — used for system-path guard, trash confirm, and
 * permanent-delete confirm. Esc = cancel, Tab cycles between the two buttons
 * while focus is trapped inside the dialog.
 *
 * Enter is NOT globally intercepted: we rely on the browser default where
 * pressing Enter on a focused <button> fires its click handler. That way
 * danger/warning variants (which put initial focus on Cancel) correctly
 * cancel on a stray Enter instead of confirming a destructive action, while
 * default variant (initial focus on Confirm) still accepts on Enter.
 */
export function ConfirmDialog({
  title,
  message,
  variant = "default",
  confirmLabel,
  cancelLabel,
  onConfirm,
  onCancel
}: Props): JSX.Element {
  // Defaults are resolved at render time (not in the destructuring) so they
  // pick up the active locale every invocation. Module-load-time evaluation
  // would freeze the first-seen translation.
  const resolvedConfirmLabel = confirmLabel ?? t("common_ok");
  const resolvedCancelLabel = cancelLabel ?? t("common_cancel");
  const confirmBtnRef = useRef<HTMLButtonElement | null>(null);
  const cancelBtnRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    // Danger/warning confirms default focus to Cancel so hitting Enter on a
    // stray keypress doesn't nuke the user's files. Default variant puts
    // focus on Confirm for quick acceptance.
    if (variant === "default") confirmBtnRef.current?.focus();
    else cancelBtnRef.current?.focus();
  }, [variant]);

  useEffect(() => {
    function onKey(e: KeyboardEvent): void {
      if (e.key === "Escape") {
        e.preventDefault();
        onCancel();
        return;
      }
      if (e.key === "Tab") {
        // Two focusable elements — cycle between them explicitly.
        const a = cancelBtnRef.current;
        const b = confirmBtnRef.current;
        if (!a || !b) return;
        const active = document.activeElement;
        e.preventDefault();
        if (active === a) b.focus();
        else a.focus();
      }
      // Enter is intentionally NOT handled here. Letting the browser's
      // default <button> behaviour fire (focused button → click) means
      // danger/warning variants (initial focus on Cancel) safely cancel on
      // Enter, while default variant (initial focus on Confirm) accepts.
    }
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [onCancel]);

  const confirmClass =
    variant === "danger"
      ? "danger"
      : variant === "warning"
      ? "warning"
      : "";

  return (
    <div
      className="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      aria-labelledby="confirm-dialog-title"
      onClick={(e) => {
        if (e.target === e.currentTarget) onCancel();
      }}
    >
      <div className="dialog">
        <h2 id="confirm-dialog-title">{title}</h2>
        <p className="dialog-message">{message}</p>
        <div className="dialog-buttons">
          <button
            ref={cancelBtnRef}
            type="button"
            onClick={onCancel}
          >
            {resolvedCancelLabel}
          </button>
          <button
            ref={confirmBtnRef}
            type="button"
            className={confirmClass}
            onClick={onConfirm}
          >
            {resolvedConfirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
