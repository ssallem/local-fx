// Package protocol implements the Chrome Native Messaging wire format:
// a 4-byte little-endian length prefix followed by a UTF-8 JSON body.
// See https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging
package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
)

// MaxFrameSize is the largest allowed frame body, in bytes.
//
// Chrome limits extension -> host messages to 4KB but allows host -> Chrome
// messages up to 1MB. We apply the more permissive 1MB cap on both directions
// so that the host can never emit frames that Chrome would reject, while still
// tolerating any well-formed input Chrome may ever send.
const MaxFrameSize = 1024 * 1024 // 1 MiB

// ErrFrameTooLarge is returned when a declared or supplied frame body exceeds
// MaxFrameSize. Callers should treat this as fatal for the current session
// because the stream framing is effectively desynchronised at that point.
var ErrFrameTooLarge = errors.New("protocol: frame exceeds 1MB limit")

// ReadFrame reads a single length-prefixed frame body from r.
//
// It returns io.EOF verbatim when the stream ends cleanly on a frame boundary,
// which callers (e.g. main loop) rely on as the "Chrome closed stdin" signal.
// io.ErrUnexpectedEOF indicates the stream was truncated mid-frame.
func ReadFrame(r io.Reader) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(lenBuf[:])
	if n > MaxFrameSize {
		return nil, ErrFrameTooLarge
	}
	if n == 0 {
		return []byte{}, nil
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

// WriteFrame writes body to w preceded by its little-endian uint32 length.
//
// The header and body are assembled into a single buffer and emitted with one
// Write call. This guarantees atomicity when multiple goroutines eventually
// share the same writer (Phase 2 streaming ops: copy/move progress events),
// without requiring a package-level mutex. The extra 4B+len(body) allocation
// is negligible compared to the JSON marshal that precedes every WriteFrame.
func WriteFrame(w io.Writer, body []byte) error {
	if len(body) > MaxFrameSize {
		return ErrFrameTooLarge
	}
	buf := make([]byte, 4+len(body))
	binary.LittleEndian.PutUint32(buf[:4], uint32(len(body)))
	copy(buf[4:], body)
	_, err := w.Write(buf)
	return err
}

// Encode marshals v to JSON suitable for WriteFrame.
func Encode(v any) ([]byte, error) { return json.Marshal(v) }

// Decode unmarshals a frame body into v.
func Decode(body []byte, v any) error { return json.Unmarshal(body, v) }
