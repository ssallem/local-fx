package ops

import (
	"context"
	"sync/atomic"
	"testing"
)

// TestJobs_RegisterAndCancel verifies the happy path: register under an ID,
// CancelJob flips the flag, returns true, and removes the entry so a repeat
// Cancel returns false.
func TestJobs_RegisterAndCancel(t *testing.T) {
	var canceled int32
	_, cancel := context.WithCancel(context.Background())
	wrapped := func() {
		atomic.StoreInt32(&canceled, 1)
		cancel()
	}

	const id = "test-job-cancel-1"
	RegisterJob(id, wrapped)
	t.Cleanup(func() { UnregisterJob(id) })

	if !CancelJob(id) {
		t.Fatal("CancelJob: got false, want true on first invocation")
	}
	if atomic.LoadInt32(&canceled) != 1 {
		t.Error("cancel func was not invoked")
	}
	if CancelJob(id) {
		t.Error("CancelJob: got true on second invocation, want false")
	}
}

// TestJobs_UnregisterPreventsLaterCancel confirms that Unregister removes
// the entry atomically with respect to a subsequent Cancel.
func TestJobs_UnregisterPreventsLaterCancel(t *testing.T) {
	var canceled int32
	wrapped := func() { atomic.StoreInt32(&canceled, 1) }

	const id = "test-job-unregister"
	RegisterJob(id, wrapped)
	UnregisterJob(id)

	if CancelJob(id) {
		t.Error("CancelJob after Unregister: got true, want false")
	}
	if atomic.LoadInt32(&canceled) != 0 {
		t.Error("unregistered cancel func was still invoked")
	}
}

// TestJobs_CancelMissingID confirms no-op behaviour for an unknown ID.
func TestJobs_CancelMissingID(t *testing.T) {
	if CancelJob("this-id-was-never-registered-xyzzy") {
		t.Error("CancelJob: got true on unknown ID, want false")
	}
}

// TestJobs_ActiveJobCount gives a diagnostics-only view of tracked jobs;
// we verify Register increments and Unregister decrements as expected.
func TestJobs_ActiveJobCount(t *testing.T) {
	// The ops package init() may register no persistent jobs, but test
	// parallelism can introduce transient entries. Take a baseline and
	// measure deltas only.
	base := ActiveJobCount()

	RegisterJob("count-a", func() {})
	RegisterJob("count-b", func() {})
	t.Cleanup(func() {
		UnregisterJob("count-a")
		UnregisterJob("count-b")
	})

	got := ActiveJobCount()
	if got-base < 2 {
		t.Errorf("ActiveJobCount after 2 registers: got %d (base %d), want >= base+2", got, base)
	}

	UnregisterJob("count-a")
	UnregisterJob("count-b")
	got = ActiveJobCount()
	if got > base {
		t.Errorf("ActiveJobCount after 2 unregisters: got %d, want <= base %d", got, base)
	}
}

// TestJobs_StoreNilCancel exercises the defensive nil check: a malformed
// value in the sync.Map should not panic CancelJob.
func TestJobs_StoreNilCancel(t *testing.T) {
	const id = "test-nil-cancel"
	jobs.Store(id, context.CancelFunc(nil))
	t.Cleanup(func() { jobs.Delete(id) })

	// CancelJob returns false when the stored func is nil.
	if CancelJob(id) {
		t.Error("CancelJob on nil func: got true, want false")
	}
}
