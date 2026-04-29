// T6 — UI store slice for the "new version available" toast.
//
// Kept in its own store (not bolted onto the jobs store) because:
//   - Update toasts are persistent (no auto-dismiss) while job toasts are
//     ephemeral. Mixing the two state machines made the jobs reducers
//     more complex than the feature warranted.
//   - The background SW pushes update toasts via chrome.runtime.sendMessage;
//     keeping a separate store means only one component needs the listener
//     and the jobs slice stays purely UI-driven.

import { create } from "zustand";

/**
 * Snapshot of a discovered release to surface in the toast. Mirrors
 * CheckUpdateData's user-facing fields plus a `dismissed` flag so the
 * toast can be hidden without losing the metadata (helpful if a future
 * "view release notes" link wants to pop the same payload back).
 */
export interface UpdateAvailable {
  latestVersion: string;
  currentVersion: string;
  // Empty string when the host couldn't find a matching installer asset;
  // the toast falls back to a release-page URL in that case.
  downloadUrl: string;
  // Empty when no notes (host returns omitempty); UI may render as small
  // grey caption.
  releaseNotes: string;
  // Unix milliseconds when the host completed the underlying check. UI
  // uses this for diagnostics ("checked: <local time>").
  checkedAtMs: number;
}

interface UpdateState {
  /** Null when no update is currently surfaced. */
  available: UpdateAvailable | null;
  /** True when host returned E_DISABLED — UI may render an info note. */
  hostDisabled: boolean;

  setAvailable: (u: UpdateAvailable) => void;
  setHostDisabled: (v: boolean) => void;
  dismiss: () => void;
}

export const useUpdate = create<UpdateState>((set) => ({
  available: null,
  hostDisabled: false,
  setAvailable: (u) => set({ available: u, hostDisabled: false }),
  setHostDisabled: (v) => set({ hostDisabled: v }),
  // Dismiss only clears the toast — the background SW still has the
  // chrome.alarms entry, so the next 24h tick can re-surface a newer
  // version. We deliberately do NOT clear the available payload from
  // chrome.storage because there isn't one — the toast is in-memory.
  dismiss: () => set({ available: null })
}));
