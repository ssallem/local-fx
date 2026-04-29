// T6 — opt-in update check settings panel.
//
// Renders as a modal (reusing the existing .dialog-backdrop / .dialog
// styles from App.css) so it inherits the same focus-trap + escape-to-
// close behaviour as ConfirmDialog. The toggle is bound to
// chrome.storage.sync so the preference roams across the user's signed-in
// Chrome profiles — they only have to opt in once.
//
// First-enable consent flow:
//   1. User clicks the toggle from OFF → consent panel slides in
//   2. User clicks "동의 / Agree" → toggle persists + alarm scheduled
//   3. User clicks "취소 / Cancel" → toggle stays OFF
//
// Disabling never requires consent — turning OFF is always one click.

import { useEffect, useState } from "react";
import { t } from "../utils/i18n";

interface Props {
  open: boolean;
  onClose: () => void;
}

// chrome.storage.sync key name. Single source of truth — the background SW
// alarm wiring also imports this constant so a rename here propagates
// automatically.
export const UPDATE_CHECKS_ENABLED_KEY = "updateChecksEnabled";

// Alarm name used by chrome.alarms.create. Matched in background.ts so a
// single rename here keeps the alarm fire path coherent.
export const UPDATE_CHECK_ALARM = "checkUpdate";

// 24h in minutes — the chrome.alarms API requires periodInMinutes.
const UPDATE_CHECK_PERIOD_MIN = 60 * 24;
// First-fire delay after opt-in. Without this, chrome.alarms only fires the
// first instance after a full periodInMinutes (24h) — the user agrees, sees
// nothing for a day, and assumes the toggle is broken. 1 minute is the
// minimum Chrome accepts for non-developer-loaded extensions in production.
const UPDATE_CHECK_FIRST_FIRE_MIN = 1;

/**
 * Read the persisted toggle from chrome.storage.sync. Treats `undefined`
 * as `false` per the project's "default OFF" privacy posture: a brand-new
 * install must not silently enable network calls.
 */
async function readEnabled(): Promise<boolean> {
  return new Promise((resolve) => {
    try {
      chrome.storage.sync.get(UPDATE_CHECKS_ENABLED_KEY, (items: unknown) => {
        // chrome.runtime.lastError → treat as "not enabled" so a sync
        // failure can never accidentally activate the network path.
        if (chrome.runtime.lastError) {
          resolve(false);
          return;
        }
        const obj = items as Record<string, unknown>;
        const v = obj[UPDATE_CHECKS_ENABLED_KEY];
        resolve(v === true);
      });
    } catch {
      resolve(false);
    }
  });
}

/**
 * Persist the toggle and (on enable) schedule the recurring alarm. Caller
 * is responsible for confirming the consent dialog BEFORE invoking this
 * with `next === true` for the first time.
 */
async function writeEnabled(next: boolean): Promise<void> {
  return new Promise((resolve) => {
    try {
      chrome.storage.sync.set(
        { [UPDATE_CHECKS_ENABLED_KEY]: next },
        () => {
          // Schedule / clear the alarm in lock-step with the toggle so a
          // background SW restart can rely on the toggle alone to decide
          // whether the alarm should be live.
          if (next) {
            // delayInMinutes pulls the FIRST fire forward to ~1 min after
            // opt-in so the user gets immediate feedback that the toggle
            // is wired up. periodInMinutes governs every subsequent fire.
            chrome.alarms.create(UPDATE_CHECK_ALARM, {
              delayInMinutes: UPDATE_CHECK_FIRST_FIRE_MIN,
              periodInMinutes: UPDATE_CHECK_PERIOD_MIN
            });
          } else {
            try {
              chrome.alarms.clear(UPDATE_CHECK_ALARM, () => {
                // discard lastError — absent alarm is fine.
                void chrome.runtime.lastError;
              });
            } catch {
              /* ignore */
            }
          }
          resolve();
        }
      );
    } catch {
      resolve();
    }
  });
}

export function UpdateCheckSettings({ open, onClose }: Props): JSX.Element | null {
  // `null` = still loading from chrome.storage; renders the toggle in a
  // disabled state so a fast clicker can't flip it before the persisted
  // value is known.
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [showConsent, setShowConsent] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    void readEnabled().then((v) => {
      if (!cancelled) setEnabled(v);
    });
    return () => {
      cancelled = true;
    };
  }, [open]);

  // Esc closes the panel — matches ConfirmDialog's behaviour. Capture
  // phase so we beat any in-flight global handler in App.tsx.
  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent): void {
      if (e.key === "Escape") {
        e.preventDefault();
        // Closing the consent sub-dialog takes priority over closing the
        // outer settings panel — give the user a "back out of just the
        // consent step" affordance.
        if (showConsent) {
          setShowConsent(false);
          return;
        }
        onClose();
      }
    }
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [open, onClose, showConsent]);

  if (!open) return null;

  const handleToggleClick = async (): Promise<void> => {
    if (enabled === null || busy) return;
    if (!enabled) {
      // OFF → ON: gate behind consent dialog. We do NOT optimistically
      // flip the toggle here — only after the user clicks 동의.
      setShowConsent(true);
      return;
    }
    // ON → OFF: no consent needed; flipping off is always safe.
    setBusy(true);
    try {
      await writeEnabled(false);
      setEnabled(false);
    } finally {
      setBusy(false);
    }
  };

  const handleConsentAgree = async (): Promise<void> => {
    setBusy(true);
    try {
      await writeEnabled(true);
      setEnabled(true);
      setShowConsent(false);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      className="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      aria-labelledby="update-settings-title"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="dialog">
        <h2 id="update-settings-title">{t("update_settings_title")}</h2>
        <p className="dialog-message">{t("update_settings_description")}</p>

        <div className="update-settings-toggle">
          <label>
            <input
              type="checkbox"
              checked={enabled === true}
              disabled={enabled === null || busy}
              onChange={() => {
                void handleToggleClick();
              }}
            />
            <span>{t("update_settings_toggle_label")}</span>
          </label>
        </div>

        <p className="dialog-message">
          {t("update_settings_env_var_hint")}
        </p>

        <div className="dialog-buttons">
          <button type="button" onClick={onClose}>
            {t("common_close")}
          </button>
        </div>

        {showConsent && (
          <div
            className="dialog-backdrop"
            role="dialog"
            aria-modal="true"
            aria-labelledby="update-consent-title"
            onClick={(e) => {
              if (e.target === e.currentTarget) setShowConsent(false);
            }}
          >
            <div className="dialog">
              <h2 id="update-consent-title">{t("update_consent_title")}</h2>
              <p className="dialog-message">{t("update_consent_body")}</p>
              <ul className="dialog-message">
                <li>{t("update_consent_bullet_what")}</li>
                <li>{t("update_consent_bullet_sent")}</li>
                <li>{t("update_consent_bullet_not_sent")}</li>
                <li>{t("update_consent_bullet_optout")}</li>
              </ul>
              <div className="dialog-buttons">
                <button
                  type="button"
                  onClick={() => setShowConsent(false)}
                  disabled={busy}
                >
                  {t("common_cancel")}
                </button>
                <button
                  type="button"
                  className="warning"
                  onClick={() => {
                    void handleConsentAgree();
                  }}
                  disabled={busy}
                >
                  {t("update_consent_agree")}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
