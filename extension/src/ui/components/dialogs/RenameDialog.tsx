import { useEffect, useMemo, useRef, useState } from "react";

interface Props {
  initialName: string;
  title?: string;
  confirmLabel?: string;
  onConfirm: (name: string) => void;
  onCancel: () => void;
}

// Windows-invalid chars. POSIX is more permissive but the host runs on
// Windows today; this is the stricter common subset, so enforcing it is safe
// cross-platform.
const FORBIDDEN = /[\\/:*?"<>|]/;

function validate(name: string): string | null {
  if (name.length === 0) return "이름을 입력하세요";
  if (name !== name.trim()) return "앞뒤 공백은 사용할 수 없습니다";
  if (FORBIDDEN.test(name)) return "\\ / : * ? \" < > | 는 사용할 수 없습니다";
  if (name === "." || name === "..") return "'.' 또는 '..'는 사용할 수 없습니다";
  return null;
}

/**
 * Name-entry modal. Drives both "create folder" and "rename" flows. The
 * input auto-selects the stem (everything before the last dot) so typing
 * replaces the name but preserves the extension on rename.
 */
export function RenameDialog({
  initialName,
  title = "이름",
  confirmLabel = "확인",
  onConfirm,
  onCancel
}: Props): JSX.Element {
  const [name, setName] = useState(initialName);
  const inputRef = useRef<HTMLInputElement | null>(null);

  const error = useMemo(() => validate(name), [name]);
  const canSubmit = error === null;

  useEffect(() => {
    const el = inputRef.current;
    if (!el) return;
    el.focus();
    // Select the stem (up to the last dot) so the extension is preserved
    // when the user starts typing. Names without a dot, dotfiles (".foo"),
    // and trailing-dot names all fall through to select-all.
    const dot = initialName.lastIndexOf(".");
    if (dot > 0 && dot < initialName.length - 1) {
      el.setSelectionRange(0, dot);
    } else {
      el.select();
    }
  }, [initialName]);

  function submit(): void {
    if (!canSubmit) return;
    onConfirm(name);
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLInputElement>): void {
    if (e.key === "Enter") {
      e.preventDefault();
      submit();
    } else if (e.key === "Escape") {
      e.preventDefault();
      onCancel();
    }
  }

  return (
    <div
      className="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      aria-labelledby="rename-dialog-title"
      onClick={(e) => {
        if (e.target === e.currentTarget) onCancel();
      }}
    >
      <div className="dialog">
        <h2 id="rename-dialog-title">{title}</h2>
        <input
          ref={inputRef}
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          onKeyDown={onKeyDown}
          className={error ? "invalid" : ""}
          aria-invalid={error !== null}
          aria-describedby={error ? "rename-dialog-error" : undefined}
        />
        {error && (
          <div id="rename-dialog-error" className="dialog-error">
            {error}
          </div>
        )}
        <div className="dialog-buttons">
          <button type="button" onClick={onCancel}>
            취소
          </button>
          <button type="button" onClick={submit} disabled={!canSubmit}>
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
