// Command fx-host is the Chrome Native Messaging host binary.
//
// Lifecycle:
//  1. Chrome spawns this process and writes length-prefixed JSON frames to stdin.
//  2. We read one frame at a time, dispatch via ops.Lookup (or ops.LookupStream
//     for streaming ops), and write the response frame(s) to stdout.
//  3. Chrome closing stdin (EOF) signals clean shutdown.
//
// All diagnostic logs go to stderr, which Chrome forwards to its own log but
// does not treat as part of the protocol.
//
// Stdout writes go through a protocol.SafeWriter because Phase 2.3 streaming
// ops (copy, move, search) run in their own goroutines and may emit event
// frames concurrently with the main loop's response writes. Without the
// mutex, two overlapping writes would interleave on the wire and desync
// framing for both. See PROTOCOL.md §6.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"

	"local-fx-host/internal/ops"
	"local-fx-host/internal/protocol"
	"local-fx-host/internal/version"
)

func main() {
	logger := log.New(os.Stderr, "fx-host ", log.LstdFlags|log.Lmicroseconds)
	logger.Printf("starting fx-host v%s", version.Version)

	if err := run(context.Background(), os.Stdin, os.Stdout, logger); err != nil {
		logger.Printf("fatal: %v", err)
		os.Exit(1)
	}
	logger.Printf("stdin closed, exiting cleanly")
}

// run is main's body in testable form: stdin/stdout/logger are injected so
// that integration tests can drive the full loop over io.Pipe pairs.
func run(ctx context.Context, in io.Reader, out io.Writer, logger *log.Logger) error {
	safeOut := protocol.NewSafeWriter(out)
	for {
		body, err := protocol.ReadFrame(in)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil // normal termination: Chrome closed stdin
			}
			if errors.Is(err, protocol.ErrFrameTooLarge) {
				// Framing is desynchronised; there's no safe way to
				// skip forward without risking further corruption.
				return fmt.Errorf("oversized frame: %w", err)
			}
			return fmt.Errorf("read frame: %w", err)
		}

		var req protocol.Request
		if derr := protocol.Decode(body, &req); derr != nil {
			resp := protocol.ErrorResponse("", protocol.ErrCodeBadRequest,
				fmt.Sprintf("invalid JSON: %v", derr), false)
			if werr := writeResp(safeOut, resp, logger); werr != nil {
				return werr
			}
			continue
		}
		if req.Op == "" {
			resp := protocol.ErrorResponse(req.ID, protocol.ErrCodeBadRequest,
				"missing required field: op", false)
			if werr := writeResp(safeOut, resp, logger); werr != nil {
				return werr
			}
			continue
		}

		// Streaming dispatch: kick the handler off on its own goroutine
		// so the main loop can keep reading (in particular, keep reading
		// the cancel request that targets this very job).
		if req.Stream {
			streamH := ops.LookupStream(req.Op)
			if streamH == nil {
				resp := protocol.ErrorResponse(req.ID, protocol.ErrCodeUnknownOp,
					fmt.Sprintf("no stream handler for op: %q", req.Op), false)
				if werr := writeResp(safeOut, resp, logger); werr != nil {
					return werr
				}
				continue
			}
			go runStreamHandler(ctx, req, streamH, safeOut, logger)
			continue
		}

		// Regular dispatch (one request -> one response).
		resp := dispatch(ctx, req, logger)
		if werr := writeResp(safeOut, resp, logger); werr != nil {
			return werr
		}
	}
}

// dispatch resolves req to a registered Handler and runs it with panic
// recovery. Unknown ops become E_UNKNOWN_OP; panics become E_INTERNAL.
func dispatch(ctx context.Context, req protocol.Request, logger *log.Logger) (resp protocol.Response) {
	handler := ops.Lookup(req.Op)
	if handler == nil {
		return protocol.ErrorResponse(req.ID, protocol.ErrCodeUnknownOp,
			fmt.Sprintf("unknown op: %q", req.Op), false)
	}

	defer func() {
		if r := recover(); r != nil {
			logger.Printf("panic in op %q: %v\n%s", req.Op, r, debug.Stack())
			resp = protocol.ErrorResponse(req.ID, protocol.ErrCodeInternal,
				fmt.Sprintf("internal error: %v", r), false)
		}
	}()

	return handler(ctx, req)
}

// runStreamHandler is the goroutine body for a streaming op. It sets up a
// cancellable child context, registers it under the job ID so the `cancel`
// op can reach it, feeds `emit` to the handler, and emits the final
// Response frame on return. Panics are recovered and surfaced as
// E_INTERNAL on a best-effort basis.
func runStreamHandler(parentCtx context.Context, req protocol.Request, h ops.StreamHandler, safeOut *protocol.SafeWriter, logger *log.Logger) {
	opCtx, cancel := context.WithCancel(parentCtx)
	defer cancel()
	ops.RegisterJob(req.ID, cancel)
	defer ops.UnregisterJob(req.ID)

	emit := func(evt protocol.EventFrame) error {
		enc, merr := json.Marshal(evt)
		if merr != nil {
			return merr
		}
		return safeOut.WriteFrame(enc)
	}

	defer func() {
		if r := recover(); r != nil {
			logger.Printf("panic in stream op %q (id=%s): %v\n%s", req.Op, req.ID, r, debug.Stack())
			resp := protocol.ErrorResponse(req.ID, protocol.ErrCodeInternal,
				fmt.Sprintf("internal error: %v", r), false)
			enc, merr := json.Marshal(resp)
			if merr != nil {
				enc = fallbackEncoded()
			}
			_ = safeOut.WriteFrame(enc)
		}
	}()

	resp := h(opCtx, req, emit)
	enc, merr := json.Marshal(resp)
	if merr != nil {
		logger.Printf("encode stream response: %v", merr)
		enc = fallbackEncoded()
	}
	if werr := safeOut.WriteFrame(enc); werr != nil {
		logger.Printf("write stream response: %v", werr)
	}
}

// writeResp encodes resp and pushes it through the SafeWriter. It falls
// back to a hand-rolled error frame when encoding fails so that the wire
// never sees a truncated/empty frame.
func writeResp(safeOut *protocol.SafeWriter, resp protocol.Response, logger *log.Logger) error {
	enc, err := json.Marshal(resp)
	if err != nil {
		logger.Printf("encode response: %v", err)
		enc = fallbackEncoded()
	}
	if err := safeOut.WriteFrame(enc); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}
	return nil
}

// fallbackEncodedLiteral is returned when json.Marshal(Response) itself fails
// (a handler returned un-marshalable Data). It is a pre-validated JSON
// literal, so we cannot recursively re-hit the same encoder failure. The id
// is deliberately empty: the originating id is unknown-safe to echo when
// marshalling is broken, and callers that need correlation will observe the
// E_INTERNAL code and surface the raw stderr log.
const fallbackEncodedLiteral = `{"id":"","ok":false,"error":{"code":"E_INTERNAL","message":"internal error serializing response","retryable":false}}`

func fallbackEncoded() []byte {
	return []byte(fallbackEncodedLiteral)
}
