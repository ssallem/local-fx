// Package test contains end-to-end tests that drive the host over real
// io.Pipe pairs rather than calling handlers directly. This catches framing,
// dispatch, and response-encoding regressions that unit tests miss.
package test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"local-fx-host/internal/ops"
	"local-fx-host/internal/protocol"
)

// writeFrameTo mirrors protocol.WriteFrame; we re-implement it here to keep
// the test independent of the code under test's write path.
func writeFrameTo(w io.Writer, body []byte) error {
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

func readFrameFrom(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	body := make([]byte, n)
	_, err := io.ReadFull(r, body)
	return body, err
}

// runHost copies the main loop here so the test does not need to exec the
// compiled binary. It is intentionally a thin wrapper that mirrors main.run,
// including the io.EOF-as-clean-shutdown semantics so tests that close the
// writer observe the same graceful exit the real binary does.
func runHost(ctx context.Context, in io.Reader, out io.Writer) error {
	logger := log.New(io.Discard, "", 0)
	for {
		body, err := protocol.ReadFrame(in)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		var req protocol.Request
		if err := protocol.Decode(body, &req); err != nil {
			resp := protocol.ErrorResponse("", protocol.ErrCodeBadRequest, err.Error(), false)
			enc, _ := protocol.Encode(resp)
			if err := protocol.WriteFrame(out, enc); err != nil {
				return err
			}
			continue
		}
		handler := ops.Lookup(req.Op)
		var resp protocol.Response
		if handler == nil {
			resp = protocol.ErrorResponse(req.ID, protocol.ErrCodeUnknownOp, "unknown op: "+req.Op, false)
		} else {
			resp = handler(ctx, req)
		}
		_ = logger
		enc, err := protocol.Encode(resp)
		if err != nil {
			return err
		}
		if err := protocol.WriteFrame(out, enc); err != nil {
			return err
		}
	}
}

func TestIntegration_Ping(t *testing.T) {
	hostIn, extWrite := io.Pipe()  // extension -> host
	extRead, hostOut := io.Pipe()  // host -> extension

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Expected to end in io.ErrClosedPipe once the test closes the writer.
		_ = runHost(ctx, hostIn, hostOut)
	}()

	// Send a ping request.
	reqBody, err := json.Marshal(protocol.Request{ID: "r1", Op: "ping"})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if err := writeFrameTo(extWrite, reqBody); err != nil {
		t.Fatalf("write frame: %v", err)
	}

	// Read the response.
	respBody, err := readFrameFrom(extRead)
	if err != nil {
		t.Fatalf("read response frame: %v", err)
	}
	var resp protocol.Response
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.ID != "r1" {
		t.Errorf("ID: got %q want %q", resp.ID, "r1")
	}
	if !resp.OK {
		t.Fatalf("OK: false; error=%+v", resp.Error)
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data: got %T, want map", resp.Data)
	}
	if pong, _ := data["pong"].(bool); !pong {
		t.Errorf("pong: got %v, want true", data["pong"])
	}
	if os, _ := data["os"].(string); os != runtime.GOOS {
		t.Errorf("os: got %q want %q", os, runtime.GOOS)
	}

	// Tell the host EOF and wait for it to return.
	_ = extWrite.Close()
	_ = hostOut.Close()
	wg.Wait()
}

func TestIntegration_UnknownOp(t *testing.T) {
	hostIn, extWrite := io.Pipe()
	extRead, hostOut := io.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = runHost(ctx, hostIn, hostOut) }()

	reqBody, _ := json.Marshal(protocol.Request{ID: "r2", Op: "does_not_exist"})
	if err := writeFrameTo(extWrite, reqBody); err != nil {
		t.Fatalf("write: %v", err)
	}
	respBody, err := readFrameFrom(extRead)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var resp protocol.Response
	_ = json.Unmarshal(respBody, &resp)

	if resp.OK {
		t.Fatalf("expected OK=false, got OK=true")
	}
	if resp.Error == nil || resp.Error.Code != protocol.ErrCodeUnknownOp {
		t.Errorf("Error: got %+v, want code=%s", resp.Error, protocol.ErrCodeUnknownOp)
	}
	if resp.ID != "r2" {
		t.Errorf("ID: got %q, want %q", resp.ID, "r2")
	}

	_ = extWrite.Close()
	_ = hostOut.Close()
	wg.Wait()
}

func TestIntegration_BadJSON(t *testing.T) {
	hostIn, extWrite := io.Pipe()
	extRead, hostOut := io.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = runHost(ctx, hostIn, hostOut) }()

	if err := writeFrameTo(extWrite, []byte("{not json")); err != nil {
		t.Fatalf("write: %v", err)
	}
	respBody, err := readFrameFrom(extRead)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var resp protocol.Response
	_ = json.Unmarshal(respBody, &resp)
	if resp.OK || resp.Error == nil || resp.Error.Code != protocol.ErrCodeBadRequest {
		t.Errorf("want E_BAD_REQUEST, got %+v", resp)
	}

	_ = extWrite.Close()
	_ = hostOut.Close()
	wg.Wait()
}

// spawnHost starts the host loop against a pair of io.Pipes and returns a
// sender/receiver plus a cleanup closure. Shared by the Phase 2.1 chain
// tests below so we don't repeat the plumbing in every test.
func spawnHost(t *testing.T) (send func(protocol.Request) protocol.Response, cleanup func()) {
	t.Helper()
	hostIn, extWrite := io.Pipe()
	extRead, hostOut := io.Pipe()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = runHost(ctx, hostIn, hostOut)
	}()

	send = func(req protocol.Request) protocol.Response {
		t.Helper()
		body, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal req: %v", err)
		}
		if err := writeFrameTo(extWrite, body); err != nil {
			t.Fatalf("write frame: %v", err)
		}
		respBody, err := readFrameFrom(extRead)
		if err != nil {
			t.Fatalf("read response: %v", err)
		}
		var resp protocol.Response
		if err := json.Unmarshal(respBody, &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		return resp
	}
	cleanup = func() {
		_ = extWrite.Close()
		_ = hostOut.Close()
		wg.Wait()
		cancel()
	}
	return send, cleanup
}

// mustMarshal is a test helper for inline args construction.
func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestIntegration_Phase21_Chain drives the full mkdir -> stat -> rename ->
// stat -> remove(permanent) lifecycle through the real dispatcher + codec.
// This exercises PROTOCOL.md §§7.4, 7.5, 7.8, 7.9 end to end.
func TestIntegration_Phase21_Chain(t *testing.T) {
	send, cleanup := spawnHost(t)
	defer cleanup()

	base := t.TempDir()
	created := filepath.Join(base, "phase21-chain")
	renamed := filepath.Join(base, "phase21-chain-renamed")

	// 1. mkdir protocolVersion=2
	resp := send(protocol.Request{
		ID:              "mk",
		Op:              "mkdir",
		ProtocolVersion: 2,
		Args:            mustMarshal(t, map[string]any{"path": created}),
	})
	if !resp.OK {
		t.Fatalf("mkdir: %+v", resp.Error)
	}

	// 2. stat — confirm creation.
	resp = send(protocol.Request{
		ID:              "st1",
		Op:              "stat",
		ProtocolVersion: 2,
		Args:            mustMarshal(t, map[string]any{"path": created}),
	})
	if !resp.OK {
		t.Fatalf("stat created: %+v", resp.Error)
	}
	data1, _ := resp.Data.(map[string]any)
	if data1 == nil || data1["type"] != "directory" {
		t.Errorf("stat.data.type: got %v, want directory", data1)
	}

	// 3. rename (same dir).
	resp = send(protocol.Request{
		ID:              "rn",
		Op:              "rename",
		ProtocolVersion: 2,
		Args:            mustMarshal(t, map[string]any{"src": created, "dst": renamed}),
	})
	if !resp.OK {
		t.Fatalf("rename: %+v", resp.Error)
	}

	// 4. stat — old path gone.
	resp = send(protocol.Request{
		ID:              "st2",
		Op:              "stat",
		ProtocolVersion: 2,
		Args:            mustMarshal(t, map[string]any{"path": created}),
	})
	if resp.OK {
		t.Fatalf("stat old path should fail, got OK")
	}
	if resp.Error.Code != protocol.ErrCodeENOENT {
		t.Errorf("expected ENOENT, got %q", resp.Error.Code)
	}

	// 5. stat new path — exists.
	resp = send(protocol.Request{
		ID:              "st3",
		Op:              "stat",
		ProtocolVersion: 2,
		Args:            mustMarshal(t, map[string]any{"path": renamed}),
	})
	if !resp.OK {
		t.Fatalf("stat renamed: %+v", resp.Error)
	}

	// 6. remove permanent (empty dir).
	resp = send(protocol.Request{
		ID:              "rm",
		Op:              "remove",
		ProtocolVersion: 2,
		Args:            mustMarshal(t, map[string]any{"path": renamed, "mode": "permanent"}),
	})
	if !resp.OK {
		t.Fatalf("remove: %+v", resp.Error)
	}

	// 7. stat — gone.
	resp = send(protocol.Request{
		ID:              "st4",
		Op:              "stat",
		ProtocolVersion: 2,
		Args:            mustMarshal(t, map[string]any{"path": renamed}),
	})
	if resp.OK {
		t.Fatalf("stat after remove should fail, got OK")
	}
	if resp.Error.Code != protocol.ErrCodeENOENT {
		t.Errorf("expected ENOENT after remove, got %q", resp.Error.Code)
	}
}

// TestIntegration_Phase21_SystemPathWithoutConfirm confirms mutating ops
// against C:\Windows (or /System) without explicitConfirm are refused with
// the dedicated error code BEFORE any filesystem side effect.
func TestIntegration_Phase21_SystemPathWithoutConfirm(t *testing.T) {
	var sysPath string
	switch runtime.GOOS {
	case "windows":
		sysPath = `C:\Windows\fx-host-should-never-create-this`
	case "darwin":
		sysPath = "/System/fx-host-should-never-create-this"
	default:
		t.Skip("no system allowlist for this OS")
	}

	send, cleanup := spawnHost(t)
	defer cleanup()

	resp := send(protocol.Request{
		ID:              "sys-mk",
		Op:              "mkdir",
		ProtocolVersion: 2,
		Args:            mustMarshal(t, map[string]any{"path": sysPath}),
	})
	if resp.OK {
		t.Fatalf("expected OK=false on system path mkdir without confirm")
	}
	if resp.Error.Code != protocol.ErrCodeSystemPathConfirmRequired {
		t.Errorf("code: got %q, want E_SYSTEM_PATH_CONFIRM_REQUIRED", resp.Error.Code)
	}
}

// TestIntegration_Phase21_SystemPathWithConfirm confirms that with
// explicitConfirm=true, the safety gate opens and the OS-level error
// (usually EACCES when running as a non-admin user) surfaces instead.
// We don't assert EACCES specifically because a privileged test runner
// could actually succeed — we only check the gate was bypassed, i.e. the
// error code is NOT E_SYSTEM_PATH_CONFIRM_REQUIRED and the operation did
// not silently succeed on an unexpected surface.
func TestIntegration_Phase21_SystemPathWithConfirm(t *testing.T) {
	var sysPath string
	switch runtime.GOOS {
	case "windows":
		sysPath = `C:\Windows\fx-host-confirm-test-should-fail`
	case "darwin":
		sysPath = "/System/fx-host-confirm-test-should-fail"
	default:
		t.Skip("no system allowlist for this OS")
	}

	send, cleanup := spawnHost(t)
	defer cleanup()

	resp := send(protocol.Request{
		ID:              "sys-mk-ok",
		Op:              "mkdir",
		ProtocolVersion: 2,
		Args: mustMarshal(t, map[string]any{
			"path":            sysPath,
			"explicitConfirm": true,
		}),
	})
	// In a non-admin session we expect EACCES; an admin session might
	// actually create the dir, which is fine — we just need to confirm
	// the gate didn't short-circuit.
	if !resp.OK {
		if resp.Error.Code == protocol.ErrCodeSystemPathConfirmRequired {
			t.Fatalf("explicitConfirm=true should have opened the gate")
		}
	} else {
		// Rare (admin) path — clean up.
		t.Logf("running as admin? created %s", sysPath)
		_ = os.Remove(sysPath)
	}
}

// TestIntegration_Phase21_PingReportsV2 confirms the version bump shipped.
func TestIntegration_Phase21_PingReportsV2(t *testing.T) {
	send, cleanup := spawnHost(t)
	defer cleanup()

	resp := send(protocol.Request{ID: "p", Op: "ping", ProtocolVersion: 2})
	if !resp.OK {
		t.Fatalf("ping: %+v", resp.Error)
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("data wrong shape: %T", resp.Data)
	}
	switch v := data["hostMaxProtocolVersion"].(type) {
	case float64:
		if int(v) != 2 {
			t.Errorf("hostMaxProtocolVersion: got %v, want 2", v)
		}
	case int:
		if v != 2 {
			t.Errorf("hostMaxProtocolVersion: got %d, want 2", v)
		}
	default:
		t.Fatalf("hostMaxProtocolVersion unexpected type %T", data["hostMaxProtocolVersion"])
	}
}

// sanity: buffer-based path, make sure the real protocol codec can round trip
// a full request/response through a single bytes.Buffer (matches what Chrome
// sees over a pipe more closely than json.Marshal alone).
func TestIntegration_BufferRoundTrip(t *testing.T) {
	var toHost, fromHost bytes.Buffer
	reqBody, _ := json.Marshal(protocol.Request{ID: "buf", Op: "ping"})
	if err := protocol.WriteFrame(&toHost, reqBody); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := protocol.ReadFrame(&toHost)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var req protocol.Request
	_ = json.Unmarshal(got, &req)

	resp := ops.Ping(context.Background(), req)
	enc, _ := protocol.Encode(resp)
	_ = protocol.WriteFrame(&fromHost, enc)

	out, err := protocol.ReadFrame(&fromHost)
	if err != nil {
		t.Fatalf("read resp: %v", err)
	}
	var decoded protocol.Response
	_ = json.Unmarshal(out, &decoded)
	if !decoded.OK || decoded.ID != "buf" {
		t.Errorf("unexpected resp: %+v", decoded)
	}
}
