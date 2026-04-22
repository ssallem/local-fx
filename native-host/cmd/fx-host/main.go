// Command fx-host is the Chrome Native Messaging host binary.
//
// Lifecycle:
//  1. Chrome spawns this process and writes length-prefixed JSON frames to stdin.
//  2. We read one frame at a time, dispatch via ops.Lookup, and write the
//     response frame to stdout.
//  3. Chrome closing stdin (EOF) signals clean shutdown.
//
// All diagnostic logs go to stderr, which Chrome forwards to its own log but
// does not treat as part of the protocol.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"

	"local-fx-host/internal/ops"
	"local-fx-host/internal/protocol"
)

func main() {
	logger := log.New(os.Stderr, "fx-host ", log.LstdFlags|log.Lmicroseconds)
	logger.Printf("starting fx-host v%s", ops.Version)

	if err := run(context.Background(), os.Stdin, os.Stdout, logger); err != nil {
		logger.Printf("fatal: %v", err)
		os.Exit(1)
	}
	logger.Printf("stdin closed, exiting cleanly")
}

// run is main's body in testable form: stdin/stdout/logger are injected so
// that integration tests can drive the full loop over io.Pipe pairs.
func run(ctx context.Context, in io.Reader, out io.Writer, logger *log.Logger) error {
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

		resp := dispatch(ctx, body, logger)

		encoded, err := protocol.Encode(resp)
		if err != nil {
			// Very unlikely (Response is always marshalable), but fall
			// back to a hand-rolled error frame so we can keep going.
			logger.Printf("encode response: %v", err)
			encoded = fallbackEncoded()
		}
		if err := protocol.WriteFrame(out, encoded); err != nil {
			return fmt.Errorf("write frame: %w", err)
		}
	}
}

// dispatch parses one frame body and routes it to the appropriate handler.
// Any panic inside a handler is recovered and converted to an E_INTERNAL
// response so that one misbehaving op cannot crash the whole host.
func dispatch(ctx context.Context, body []byte, logger *log.Logger) (resp protocol.Response) {
	var req protocol.Request
	if err := protocol.Decode(body, &req); err != nil {
		return protocol.ErrorResponse("", protocol.ErrCodeBadRequest,
			fmt.Sprintf("invalid JSON: %v", err), false)
	}
	if req.Op == "" {
		return protocol.ErrorResponse(req.ID, protocol.ErrCodeBadRequest,
			"missing required field: op", false)
	}

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
