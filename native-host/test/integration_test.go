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
