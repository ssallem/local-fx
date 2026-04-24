import { useState } from "react";
import { request } from "../ipc";
import type { PingData, Response } from "../../types/shared";
import { t } from "../utils/i18n";

interface Props {
  open: boolean;
  onClose: () => void;
}

export function DevPanel({ open, onClose }: Props): JSX.Element | null {
  const [result, setResult] = useState<Response<PingData> | null>(null);
  const [busy, setBusy] = useState(false);

  async function onPing(): Promise<void> {
    setBusy(true);
    setResult(null);
    try {
      const resp = await request("ping");
      setResult(resp);
    } finally {
      setBusy(false);
    }
  }

  if (!open) return null;

  return (
    <div className="devpanel" role="dialog" aria-label={t("devpanel_aria")}>
      <div className="devpanel-header">
        <strong>{t("devpanel_title")}</strong>
        <button type="button" onClick={onClose} aria-label={t("common_close")}>
          ✕
        </button>
      </div>
      <div className="devpanel-body">
        <button
          type="button"
          onClick={() => {
            void onPing();
          }}
          disabled={busy}
        >
          {busy ? t("devpanel_pinging") : t("devpanel_ping_host")}
        </button>
        {result && result.ok && (
          <pre className="devpanel-pre">{JSON.stringify(result, null, 2)}</pre>
        )}
        {result && !result.ok && (
          <div className="devpanel-err">
            <strong>{result.error.code}</strong>: {result.error.message}
            {result.error.retryable ? t("devpanel_retryable_suffix") : ""}
          </div>
        )}
      </div>
    </div>
  );
}
