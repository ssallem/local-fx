package ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"local-fx-host/internal/protocol"
)

func TestMove_SameDir_Success(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "a.txt")
	dst := filepath.Join(tmp, "b.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(MoveArgs{Src: src, Dst: dst})
	req := protocol.Request{ID: "m1", Op: "move", Args: args, Stream: true}
	var events []protocol.EventFrame
	emit := func(e protocol.EventFrame) error {
		events = append(events, e)
		return nil
	}

	resp := Move(context.Background(), req, emit)
	if !resp.OK {
		t.Fatalf("expected OK, got error: %+v", resp.Error)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("src still exists after move: err=%v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("dst missing: %v", err)
	}
	// Expect a terminal "done" frame with empty payload.
	if len(events) == 0 {
		t.Fatal("expected at least one event frame")
	}
	last := events[len(events)-1]
	if last.Event != "done" {
		t.Fatalf("expected final event=done, got %q", last.Event)
	}
}

func TestMove_SamePath_Rejected(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "a.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(MoveArgs{Src: p, Dst: p})
	req := protocol.Request{ID: "m2", Op: "move", Args: args, Stream: true}
	emit := func(protocol.EventFrame) error { return nil }
	resp := Move(context.Background(), req, emit)
	if resp.OK {
		t.Fatal("expected error for same path")
	}
	if resp.Error == nil || resp.Error.Code != protocol.ErrCodeEINVAL {
		t.Fatalf("expected EINVAL, got %+v", resp.Error)
	}
}

func TestMove_DstUnderSrc_Rejected(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(src, "sub")

	args, _ := json.Marshal(MoveArgs{Src: src, Dst: dst})
	req := protocol.Request{ID: "m3", Op: "move", Args: args, Stream: true}
	emit := func(protocol.EventFrame) error { return nil }
	resp := Move(context.Background(), req, emit)
	if resp.OK {
		t.Fatal("expected cycle rejection")
	}
	if resp.Error == nil || resp.Error.Code != protocol.ErrCodeEINVAL {
		t.Fatalf("expected EINVAL, got %+v", resp.Error)
	}
}

func TestMove_DirectoryRename_Success(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "f.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(MoveArgs{Src: src, Dst: dst})
	req := protocol.Request{ID: "m4", Op: "move", Args: args, Stream: true}
	emit := func(protocol.EventFrame) error { return nil }
	resp := Move(context.Background(), req, emit)
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("src still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "f.txt")); err != nil {
		t.Fatalf("dst/f.txt missing: %v", err)
	}
}

func TestMove_BadArgs_Rejected(t *testing.T) {
	req := protocol.Request{ID: "m5", Op: "move", Args: json.RawMessage("not json"), Stream: true}
	emit := func(protocol.EventFrame) error { return nil }
	resp := Move(context.Background(), req, emit)
	if resp.OK {
		t.Fatal("expected bad request error")
	}
	if resp.Error == nil || resp.Error.Code != protocol.ErrCodeBadRequest {
		t.Fatalf("expected E_BAD_REQUEST, got %+v", resp.Error)
	}
}

func TestMove_ConflictPrompt_Rejected(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "a.txt")
	dst := filepath.Join(tmp, "b.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(MoveArgs{Src: src, Dst: dst, Conflict: "prompt"})
	req := protocol.Request{ID: "m6", Op: "move", Args: args, Stream: true}
	emit := func(protocol.EventFrame) error { return nil }
	resp := Move(context.Background(), req, emit)
	if resp.OK {
		t.Fatal("expected E_BAD_REQUEST for conflict=prompt")
	}
	if resp.Error == nil || resp.Error.Code != protocol.ErrCodeBadRequest {
		t.Fatalf("expected E_BAD_REQUEST, got %+v", resp.Error)
	}
}
