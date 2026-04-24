package ops

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"local-fx-host/internal/protocol"
)

// copyReq builds a streaming copy Request with a deterministic ID so tests
// can refer back to it for cancellation via CancelJob.
func copyReq(t *testing.T, id string, args map[string]any) protocol.Request {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return protocol.Request{ID: id, Op: "copy", Args: raw, Stream: true}
}

// collector is a thread-safe EventFrame accumulator for stream tests.
type collector struct {
	mu     sync.Mutex
	events []protocol.EventFrame
}

func (c *collector) emit(evt protocol.EventFrame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, evt)
	return nil
}

func (c *collector) snapshot() []protocol.EventFrame {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]protocol.EventFrame, len(c.events))
	copy(out, c.events)
	return out
}

// writeRandFile creates a file of `size` random bytes. Random content
// defeats any filesystem compression and makes sure the copy loop actually
// moves bytes (vs. zero-fill sparse files on some platforms).
func writeRandFile(t *testing.T, path string, size int) {
	t.Helper()
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand: %v", err)
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestCopy_SmallFileHappyPath copies a 200KB file, verifies the destination
// matches byte-for-byte, and confirms progress+done events were emitted.
func TestCopy_SmallFileHappyPath(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src.bin")
	dst := filepath.Join(base, "dst.bin")
	writeRandFile(t, src, 200*1024)

	col := &collector{}
	resp := Copy(context.Background(), copyReq(t, "cp-ok", map[string]any{
		"src": src, "dst": dst,
	}), col.emit)

	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	srcBytes, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read src: %v", err)
	}
	dstBytes, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if !bytes.Equal(srcBytes, dstBytes) {
		t.Errorf("dst content mismatch: got %d bytes want %d", len(dstBytes), len(srcBytes))
	}

	events := col.snapshot()
	if len(events) == 0 {
		t.Fatal("no events emitted")
	}
	last := events[len(events)-1]
	if last.Event != "done" {
		t.Errorf("last event: got %q want done", last.Event)
	}
	donePayload, _ := last.Payload.(protocol.DonePayload)
	if donePayload.Canceled {
		t.Errorf("done.canceled: got true, want false on happy path")
	}

	// Temp file must be gone after rename. os.CreateTemp uses a
	// ".<base>.part-<reqID>-*" pattern so we glob the directory instead of
	// stat'ing a fixed path.
	matches, _ := filepath.Glob(filepath.Join(base, ".dst.bin.part-cp-ok-*"))
	if len(matches) != 0 {
		t.Errorf("temp file(s) left behind: %v", matches)
	}
}

// TestCopy_LargeFileEmitsProgress writes a file big enough to trigger at
// least one progress event inside the 100ms debounce window. We slow the
// copy down indirectly by making the file large relative to typical SSD
// throughput — 8MB with 64KB buffer is 128 iterations, easily spanning
// a debounce interval on even fast disks.
func TestCopy_LargeFileEmitsProgress(t *testing.T) {
	if testing.Short() {
		t.Skip("large file test skipped in -short mode")
	}
	base := t.TempDir()
	src := filepath.Join(base, "large.bin")
	dst := filepath.Join(base, "large.out")
	const size = 8 * 1024 * 1024 // 8MB — roomy enough for a progress tick
	writeRandFile(t, src, size)

	col := &collector{}
	resp := Copy(context.Background(), copyReq(t, "cp-large", map[string]any{
		"src": src, "dst": dst,
	}), col.emit)

	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}

	events := col.snapshot()
	gotProgress := false
	for _, ev := range events {
		if ev.Event == "progress" {
			p, ok := ev.Payload.(protocol.ProgressPayload)
			if !ok {
				t.Fatalf("progress payload type: got %T", ev.Payload)
			}
			if p.BytesTotal != int64(size) {
				t.Errorf("BytesTotal: got %d want %d", p.BytesTotal, size)
			}
			if p.BytesDone <= 0 || p.BytesDone > int64(size) {
				t.Errorf("BytesDone: got %d, want in (0, %d]", p.BytesDone, size)
			}
			gotProgress = true
		}
	}
	if !gotProgress {
		// Not fatal: a very fast disk might finish an 8MB copy in
		// <100ms and emit no progress. Log for observability but accept.
		t.Logf("no progress events emitted (disk was fast enough); done-only path is still correct")
	}
}

// TestCopy_CancelMidway starts a copy of a large file on a goroutine, then
// fires CancelJob before it can finish. We verify:
//
//  1. the final Response is OK:true (cancellation is not an error)
//  2. a "done" event with canceled=true was emitted
//  3. the destination file does NOT exist (temp file was cleaned up)
//  4. no .part-* file is left behind
func TestCopy_CancelMidway(t *testing.T) {
	if testing.Short() {
		t.Skip("cancel race test skipped in -short mode")
	}
	base := t.TempDir()
	src := filepath.Join(base, "cancel-src.bin")
	dst := filepath.Join(base, "cancel-dst.bin")
	// Large enough that the copy takes materially longer than the cancel
	// race we set up below. 64MB is safe on CI VMs.
	const size = 64 * 1024 * 1024
	writeRandFile(t, src, size)

	col := &collector{}
	const jobID = "cp-cancel"
	req := copyReq(t, jobID, map[string]any{"src": src, "dst": dst})

	// main.go (runStreamHandler) is responsible for RegisterJob + the
	// context.WithCancel wrapper; Copy itself no longer registers. The
	// test mirrors that lifecycle here so CancelJob(jobID) can still reach
	// this job.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	RegisterJob(jobID, cancel)
	defer UnregisterJob(jobID)

	done := make(chan protocol.Response, 1)
	go func() {
		done <- Copy(ctx, req, col.emit)
	}()

	// Give the handler a head start so it's actually inside the copy
	// loop (not stuck on args parse) when we cancel. The sleep is short
	// because we want to cancel well before the copy finishes.
	time.Sleep(30 * time.Millisecond)
	if !CancelJob(jobID) {
		// Copy may have completed on a very fast disk. Wait for resp
		// and decide whether to accept the race as a flake.
		resp := <-done
		if !resp.OK {
			t.Fatalf("copy failed unexpectedly: %+v", resp.Error)
		}
		t.Skip("cancel lost race against fast disk; skipping assertion")
	}

	resp := <-done
	if !resp.OK {
		t.Fatalf("expected OK after cancel, got %+v", resp.Error)
	}

	events := col.snapshot()
	if len(events) == 0 {
		t.Fatal("no events emitted")
	}
	last := events[len(events)-1]
	if last.Event != "done" {
		t.Errorf("last event: got %q want done", last.Event)
	}
	d, _ := last.Payload.(protocol.DonePayload)
	if !d.Canceled {
		t.Errorf("done.canceled: got false, want true after cancel")
	}

	// Destination must not exist; temp must be cleaned up. os.CreateTemp uses
	// a ".<base>.part-<reqID>-*" pattern, so glob the directory rather than
	// stat a fixed path.
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("dst should not exist after cancel, got err=%v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(base, ".cancel-dst.bin.part-"+jobID+"-*"))
	if len(matches) != 0 {
		t.Errorf("temp file(s) leaked after cancel: %v", matches)
	}
}

// TestCopy_DstExistsDefaultSkipped matches the historical Phase 2.1 default:
// no overwrite, no conflict field → "skip" strategy → EEXIST.
func TestCopy_DstExistsDefaultSkipped(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src.txt")
	dst := filepath.Join(base, "dst.txt")
	writeRandFile(t, src, 64)
	if err := os.WriteFile(dst, []byte("existing"), 0o644); err != nil {
		t.Fatalf("WriteFile dst: %v", err)
	}

	col := &collector{}
	resp := Copy(context.Background(), copyReq(t, "cp-skip", map[string]any{
		"src": src, "dst": dst,
	}), col.emit)

	if resp.OK {
		t.Fatalf("expected OK=false on skip conflict")
	}
	if resp.Error.Code != protocol.ErrCodeEEXIST {
		t.Errorf("code: got %q want EEXIST", resp.Error.Code)
	}
	// No "done" event should fire on a setup error.
	for _, ev := range col.snapshot() {
		if ev.Event == "done" {
			t.Errorf("unexpected done event on setup error: %+v", ev)
		}
	}
	// dst content must be unchanged.
	got, _ := os.ReadFile(dst)
	if string(got) != "existing" {
		t.Errorf("dst content changed: got %q", string(got))
	}
}

// TestCopy_DstExistsOverwrite confirms conflict="overwrite" replaces the
// destination atomically.
func TestCopy_DstExistsOverwrite(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src.txt")
	dst := filepath.Join(base, "dst.txt")
	srcData := []byte("new payload")
	if err := os.WriteFile(src, srcData, 0o644); err != nil {
		t.Fatalf("WriteFile src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile dst: %v", err)
	}

	col := &collector{}
	resp := Copy(context.Background(), copyReq(t, "cp-over", map[string]any{
		"src": src, "dst": dst, "conflict": "overwrite",
	}), col.emit)

	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, srcData) {
		t.Errorf("dst content: got %q want %q", got, srcData)
	}
}

// TestCopy_DstExistsOverwriteLegacyBool exercises the legacy `overwrite:true`
// field (pre-conflict API) for backwards compat with callers that haven't
// migrated yet.
func TestCopy_DstExistsOverwriteLegacyBool(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src.txt")
	dst := filepath.Join(base, "dst.txt")
	srcData := []byte("legacy replaces")
	_ = os.WriteFile(src, srcData, 0o644)
	_ = os.WriteFile(dst, []byte("old"), 0o644)

	col := &collector{}
	resp := Copy(context.Background(), copyReq(t, "cp-legacy", map[string]any{
		"src": src, "dst": dst, "overwrite": true,
	}), col.emit)

	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, srcData) {
		t.Errorf("dst content: got %q want %q", got, srcData)
	}
}

// TestCopy_SrcMissing surfaces ENOENT without emitting any events.
func TestCopy_SrcMissing(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "no-such-file.bin")
	dst := filepath.Join(base, "dst.bin")

	col := &collector{}
	resp := Copy(context.Background(), copyReq(t, "cp-noent", map[string]any{
		"src": src, "dst": dst,
	}), col.emit)

	if resp.OK {
		t.Fatalf("expected OK=false, got OK")
	}
	if resp.Error.Code != protocol.ErrCodeENOENT {
		t.Errorf("code: got %q want ENOENT", resp.Error.Code)
	}
	if len(col.snapshot()) != 0 {
		t.Errorf("unexpected events on setup error: %+v", col.snapshot())
	}
}

// TestCopy_SrcIsDirectory confirms directory copy is supported in Phase 2.4:
// a recursive walk mirrors src into dst. Previously (Phase 2.3) this case
// was rejected with EINVAL; the expectation flipped once recursiveCopy landed.
func TestCopy_SrcIsDirectory(t *testing.T) {
	base := t.TempDir()
	srcDir := filepath.Join(base, "srcdir")
	if err := os.Mkdir(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Put a sentinel file inside so we can assert the walk actually ran.
	if err := os.WriteFile(filepath.Join(srcDir, "inside.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(base, "dst")

	col := &collector{}
	resp := Copy(context.Background(), copyReq(t, "cp-dir", map[string]any{
		"src": srcDir, "dst": dst,
	}), col.emit)

	if !resp.OK {
		t.Fatalf("expected OK=true on dir src, got %+v", resp.Error)
	}
	if _, err := os.Stat(filepath.Join(dst, "inside.txt")); err != nil {
		t.Fatalf("dst/inside.txt missing: %v", err)
	}
}

// TestCopy_RelativePathRejected confirms safety.CleanPath gate fires.
func TestCopy_RelativePathRejected(t *testing.T) {
	col := &collector{}
	resp := Copy(context.Background(), copyReq(t, "cp-rel", map[string]any{
		"src": "relative/src",
		"dst": "relative/dst",
	}), col.emit)
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodePathRejected {
		t.Errorf("code: got %q want E_PATH_REJECTED", resp.Error.Code)
	}
}

// TestCopy_BadJSON confirms malformed args return E_BAD_REQUEST.
func TestCopy_BadJSON(t *testing.T) {
	col := &collector{}
	req := protocol.Request{ID: "x", Op: "copy", Args: json.RawMessage(`{not json`), Stream: true}
	resp := Copy(context.Background(), req, col.emit)
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodeBadRequest {
		t.Errorf("code: got %q want E_BAD_REQUEST", resp.Error.Code)
	}
}

// TestCopy_InvalidConflictRejected covers the enum validation.
func TestCopy_InvalidConflictRejected(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src")
	dst := filepath.Join(base, "dst")
	writeRandFile(t, src, 16)

	col := &collector{}
	resp := Copy(context.Background(), copyReq(t, "cp-badconf", map[string]any{
		"src": src, "dst": dst, "conflict": "overwrite-please",
	}), col.emit)

	if resp.OK {
		t.Fatalf("expected OK=false on invalid conflict")
	}
	if resp.Error.Code != protocol.ErrCodeBadRequest {
		t.Errorf("code: got %q want E_BAD_REQUEST", resp.Error.Code)
	}
}

// TestCopy_ConflictRename confirms conflict=rename now picks a "(N)" suffix
// when the destination exists (Phase 2.4 UniqueName). Previously deferred.
func TestCopy_ConflictRename(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src.txt")
	dst := filepath.Join(base, "dst.txt")
	writeRandFile(t, src, 16)
	// Pre-create the destination so the rename strategy has to find a fresh slot.
	if err := os.WriteFile(dst, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	col := &collector{}
	resp := Copy(context.Background(), copyReq(t, "cp-rename", map[string]any{
		"src": src, "dst": dst, "conflict": "rename",
	}), col.emit)

	if !resp.OK {
		t.Fatalf("expected OK=true on conflict=rename, got %+v", resp.Error)
	}
	// Original dst should be untouched; the copy should have landed on dst (1).txt.
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != "existing" {
		t.Fatalf("original dst was clobbered: %q", string(got))
	}
	alt := filepath.Join(base, "dst (1).txt")
	if _, err := os.Stat(alt); err != nil {
		t.Fatalf("expected rename target %q: %v", alt, err)
	}
}

// TestCopy_SystemPathDstRequiresConfirm confirms the mutating-op gate fires
// when dst targets a system allowlist path without explicitConfirm.
func TestCopy_SystemPathDstRequiresConfirm(t *testing.T) {
	var sysPath string
	switch runtime.GOOS {
	case "windows":
		sysPath = `C:\Windows\fx-host-copy-should-not-land`
	case "darwin":
		sysPath = "/System/fx-host-copy-should-not-land"
	default:
		t.Skip("no system allowlist on this OS")
	}
	base := t.TempDir()
	src := filepath.Join(base, "src")
	writeRandFile(t, src, 16)

	col := &collector{}
	resp := Copy(context.Background(), copyReq(t, "cp-sys", map[string]any{
		"src": src, "dst": sysPath,
	}), col.emit)

	if resp.OK {
		t.Fatalf("expected OK=false on system dst without confirm")
	}
	if resp.Error.Code != protocol.ErrCodeSystemPathConfirmRequired {
		t.Errorf("code: got %q want E_SYSTEM_PATH_CONFIRM_REQUIRED", resp.Error.Code)
	}
	if _, err := os.Stat(sysPath); !os.IsNotExist(err) {
		t.Errorf("side effect: system dst should still be missing, err=%v", err)
	}
}

// TestCopy_RegisteredInStreamRegistry confirms the dispatcher has it.
func TestCopy_RegisteredInStreamRegistry(t *testing.T) {
	if LookupStream("copy") == nil {
		t.Fatal("copy stream handler not registered")
	}
}
