import { useState } from "react";
import { request } from "../ipc";
import type { PingData, Response } from "../../types/shared";

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
    <div className="devpanel" role="dialog" aria-label="Developer panel">
      <div className="devpanel-header">
        <strong>Dev panel</strong>
        <button type="button" onClick={onClose} aria-label="Close">
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
          {busy ? "Pinging…" : "Ping Host"}
        </button>
        {result && result.ok && (
          <pre className="devpanel-pre">{JSON.stringify(result, null, 2)}</pre>
        )}
        {result && !result.ok && (
          <div className="devpanel-err">
            <strong>{result.error.code}</strong>: {result.error.message}
            {result.error.retryable ? " (retryable)" : ""}
          </div>
        )}
      </div>
    </div>
  );
}
