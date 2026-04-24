// Phase 2.3 — UI-side jobs store. Tracks streaming copy/move ops launched
// via ipc.copyFile() / future ipc.moveFile(), aggregates their progress /
// item / done events into a single per-job snapshot, and exposes lifecycle
// actions (addJob / updateProgress / completeJob / failJob / removeJob /
// markCanceling / clearFinished) for ProgressToasts and any future job-list
// UI.
//
// Design notes:
//   - One Job entry per streaming request id (matches StreamHandle.id).
//   - `order` is a parallel array of ids so display order is stable when the
//     `jobs` map is mutated (insertion-order on a Record is reliable in
//     practice but not guaranteed; an explicit array makes intent obvious).
//   - The store is intentionally dumb about IPC: it does not know how to
//     start a job. The `startCopyJob` helper at the bottom of this file is
//     the wiring layer that calls ipc.copyFile + populates the store.
//   - `state` is a small finite enum so the UI can switch on it without
//     juggling boolean flags ("isRunning && !isCanceling && ...").

import { create } from "zustand";
import type {
  CopyArgs,
  DonePayload,
  FailureInfo,
  ItemPayload,
  MoveArgs,
  ProgressPayload,
  StreamEvent
} from "../../types/shared";
import { copyFile, moveFile } from "../ipc";

export type JobKind = "copy" | "move";
export type JobState =
  | "running"
  | "canceling"
  | "done"
  | "failed"
  | "canceled";

export interface Job {
  id: string;
  kind: JobKind;
  state: JobState;
  /** Human-friendly label, e.g. "복사 중: file.txt" or a directory name. */
  label: string;
  bytesDone: number;
  bytesTotal: number;
  fileDone: number;
  fileTotal: number;
  currentPath?: string;
  /** bytes/sec, averaged over the recent emit window by the Host. */
  rate?: number;
  failures: FailureInfo[];
  /**
   * Set when the op fails BEFORE emitting "done" (e.g. EACCES on the source).
   * Per-entry failures from the "done" event live in `failures` instead so
   * a partial-success run is distinguishable from a full crash.
   */
  errorMessage?: string;
  startedAt: number;
  finishedAt?: number;
  /**
   * Idempotent cancel handle from the StreamHandle. May be undefined when a
   * job is reconstructed from another source (none today, but reserved).
   */
  cancel?: () => Promise<void>;
}

interface JobsState {
  jobs: Record<string, Job>;
  /** Display order — newest job appended at the end. */
  order: string[];

  // ── lifecycle actions ───────────────────────────────────────────────────
  addJob: (j: Job) => void;
  updateProgress: (
    id: string,
    payload: StreamEvent["payload"],
    event: StreamEvent["event"]
  ) => void;
  markCanceling: (id: string) => void;
  completeJob: (
    id: string,
    canceled: boolean,
    failures?: FailureInfo[]
  ) => void;
  failJob: (id: string, message: string) => void;
  removeJob: (id: string) => void;
  /** Drop every non-running job from the store (handy for a "clear" button). */
  clearFinished: () => void;
}

export const useJobs = create<JobsState>((set, get) => ({
  jobs: {},
  order: [],

  addJob: (j) =>
    set((s) => ({
      jobs: { ...s.jobs, [j.id]: j },
      order: s.order.includes(j.id) ? s.order : [...s.order, j.id]
    })),

  updateProgress: (id, payload, event) => {
    const job = get().jobs[id];
    if (!job) return;
    if (event === "progress") {
      const p = payload as ProgressPayload;
      // exactOptionalPropertyTypes: build the next snapshot conditionally
      // so we don't write `undefined` into optional fields when the Host
      // omits them (which would clobber prior values with no field at all).
      const next: Job = {
        ...job,
        bytesDone: p.bytesDone,
        bytesTotal: p.bytesTotal,
        fileDone: p.fileDone,
        fileTotal: p.fileTotal
      };
      const cp = p.currentPath ?? job.currentPath;
      if (cp !== undefined) next.currentPath = cp;
      const rate = p.rate ?? job.rate;
      if (rate !== undefined) next.rate = rate;
      set((s) => ({ jobs: { ...s.jobs, [id]: next } }));
    } else if (event === "item") {
      // Per-entry frames (status: done | skipped | failed) are not surfaced
      // in Phase 2.3 — copy.go currently emits only progress + done. Reserved
      // for Phase 2.4+ when granular item telemetry lands and the toast can
      // show a per-file failure list incrementally instead of waiting for the
      // terminal done frame.
      void (payload as ItemPayload);
    }
  },

  markCanceling: (id) =>
    set((s) => {
      const j = s.jobs[id];
      if (!j) return s;
      // Guard: only meaningful from running. Once a job is already done /
      // failed / canceled, marking it as canceling is a UI bug — drop
      // silently so the stale state doesn't flip the badge backwards.
      if (j.state !== "running") return s;
      return { jobs: { ...s.jobs, [id]: { ...j, state: "canceling" } } };
    }),

  completeJob: (id, canceled, failures) =>
    set((s) => {
      const j = s.jobs[id];
      if (!j) return s;
      const failList = failures ?? [];
      const state: JobState = canceled
        ? "canceled"
        : failList.length > 0
          ? "failed"
          : "done";
      return {
        jobs: {
          ...s.jobs,
          [id]: {
            ...j,
            state,
            failures: failList,
            finishedAt: Date.now()
          }
        }
      };
    }),

  failJob: (id, message) =>
    set((s) => {
      const j = s.jobs[id];
      if (!j) return s;
      return {
        jobs: {
          ...s.jobs,
          [id]: {
            ...j,
            state: "failed",
            errorMessage: message,
            finishedAt: Date.now()
          }
        }
      };
    }),

  removeJob: (id) =>
    set((s) => {
      if (!(id in s.jobs)) return s;
      const { [id]: _removed, ...rest } = s.jobs;
      return { jobs: rest, order: s.order.filter((x) => x !== id) };
    }),

  clearFinished: () =>
    set((s) => {
      const running: Record<string, Job> = {};
      const order: string[] = [];
      for (const id of s.order) {
        const j = s.jobs[id];
        if (j && (j.state === "running" || j.state === "canceling")) {
          running[id] = j;
          order.push(id);
        }
      }
      return { jobs: running, order };
    })
}));

// -----------------------------------------------------------------------------
// Job-start helpers — bridge ipc.copyFile() into the store.
//
// These live here (rather than in components/ProgressToasts.tsx) so that
// future call sites (paste handler in App.tsx, drag-drop, etc.) share one
// canonical wiring path. The toast component stays purely presentational.
// -----------------------------------------------------------------------------

/**
 * Starts a streaming copy job and registers it with the jobs store. Returns
 * the job id (== StreamHandle.id == request id), which callers can use to
 * correlate later cancel attempts or to remove the toast programmatically.
 *
 * The returned id is also the IPC request id, so it is unique per op invocation
 * and safe to use as a React key or Map key without further hashing.
 */
export function startCopyJob(args: CopyArgs, label: string): string {
  const handle = copyFile(args, (evt) => {
    const s = useJobs.getState();
    if (evt.event === "progress" || evt.event === "item") {
      s.updateProgress(evt.id, evt.payload, evt.event);
    } else if (evt.event === "done") {
      const payload = evt.payload as DonePayload;
      s.completeJob(evt.id, payload.canceled === true, payload.failures);
    }
  });

  useJobs.getState().addJob({
    id: handle.id,
    kind: "copy",
    state: "running",
    label,
    bytesDone: 0,
    bytesTotal: 0,
    fileDone: 0,
    fileTotal: 0,
    failures: [],
    startedAt: Date.now(),
    cancel: handle.cancel
  });

  // The terminal Response only matters when the Host failed BEFORE emitting
  // a "done" event (e.g. ENOENT on src). On the happy path completeJob has
  // already fired from the "done" handler and this `then` is a no-op.
  void handle.promise.then((resp) => {
    const s = useJobs.getState();
    const current = s.jobs[handle.id];
    // If we already transitioned to a terminal state via the done event,
    // don't clobber it with the (also-success) Response.
    if (
      current &&
      (current.state === "done" ||
        current.state === "failed" ||
        current.state === "canceled")
    ) {
      return;
    }
    if (!resp.ok) {
      s.failJob(handle.id, resp.error.message);
    } else {
      // Success Response without a prior done event would be a Host protocol
      // violation for streaming ops, but defensively mark complete so the
      // toast doesn't sit at "running" forever.
      s.completeJob(handle.id, false, []);
    }
  });

  return handle.id;
}

/**
 * Starts a streaming move job. Same wiring as startCopyJob but kind="move"
 * so the toast badge reads "이동" instead of "복사". Move shares CopyArgs
 * semantics on the Host side (intra-volume rename fast path, copy+remove
 * fallback across volumes); the only TS-level difference is that
 * conflict: "prompt" is rejected by MoveArgs.
 */
export function startMoveJob(args: MoveArgs, label: string): string {
  const handle = moveFile(args, (evt) => {
    const s = useJobs.getState();
    if (evt.event === "progress" || evt.event === "item") {
      s.updateProgress(evt.id, evt.payload, evt.event);
    } else if (evt.event === "done") {
      const payload = evt.payload as DonePayload;
      s.completeJob(evt.id, payload.canceled === true, payload.failures);
    }
  });

  useJobs.getState().addJob({
    id: handle.id,
    kind: "move",
    state: "running",
    label,
    bytesDone: 0,
    bytesTotal: 0,
    fileDone: 0,
    fileTotal: 0,
    failures: [],
    startedAt: Date.now(),
    cancel: handle.cancel
  });

  void handle.promise.then((resp) => {
    const s = useJobs.getState();
    const current = s.jobs[handle.id];
    if (
      current &&
      (current.state === "done" ||
        current.state === "failed" ||
        current.state === "canceled")
    ) {
      return;
    }
    if (!resp.ok) {
      s.failJob(handle.id, resp.error.message);
    } else {
      s.completeJob(handle.id, false, []);
    }
  });

  return handle.id;
}
