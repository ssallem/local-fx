package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestWriteReadFrame_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	body := []byte(`{"id":"1","op":"ping"}`)
	if err := WriteFrame(&buf, body); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("round trip: got %q want %q", got, body)
	}
}

func TestReadFrame_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, nil); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty frame: got %d bytes, want 0", len(got))
	}
}

func TestWriteFrame_AtMaxBoundary(t *testing.T) {
	var buf bytes.Buffer
	body := bytes.Repeat([]byte{'a'}, MaxFrameSize)
	if err := WriteFrame(&buf, body); err != nil {
		t.Fatalf("WriteFrame at max: %v", err)
	}
	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame at max: %v", err)
	}
	if len(got) != MaxFrameSize {
		t.Errorf("at max: got %d bytes, want %d", len(got), MaxFrameSize)
	}
}

func TestWriteFrame_OverMaxRejected(t *testing.T) {
	var buf bytes.Buffer
	body := bytes.Repeat([]byte{'a'}, MaxFrameSize+1)
	err := WriteFrame(&buf, body)
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("WriteFrame over max: err=%v, want ErrFrameTooLarge", err)
	}
	if buf.Len() != 0 {
		t.Errorf("over-max write should not emit bytes, got %d", buf.Len())
	}
}

func TestReadFrame_OverMaxRejected(t *testing.T) {
	// Hand-craft a header declaring MaxFrameSize+1 so we don't have to
	// buffer a 1MB+ payload in memory just to reject it.
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], MaxFrameSize+1)
	r := bytes.NewReader(header[:])
	_, err := ReadFrame(r)
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("ReadFrame over max: err=%v, want ErrFrameTooLarge", err)
	}
}

func TestReadFrame_EOFOnBoundary(t *testing.T) {
	var buf bytes.Buffer
	_, err := ReadFrame(&buf)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("empty stream: err=%v, want io.EOF", err)
	}
}

func TestReadFrame_TruncatedHeader(t *testing.T) {
	r := bytes.NewReader([]byte{0x01, 0x02}) // only 2 of 4 header bytes
	_, err := ReadFrame(r)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated header: err=%v, want io.ErrUnexpectedEOF", err)
	}
}

func TestReadFrame_TruncatedBody(t *testing.T) {
	var buf bytes.Buffer
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], 10) // claim 10 bytes
	buf.Write(header[:])
	buf.WriteString("abc") // only write 3
	_, err := ReadFrame(&buf)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated body: err=%v, want io.ErrUnexpectedEOF", err)
	}
}

// chunkedReader delivers data one byte at a time to simulate a pipe that
// returns partial reads. io.ReadFull should loop until it has the full frame.
type chunkedReader struct{ src *bytes.Reader }

func (c *chunkedReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return c.src.Read(p[:1])
}

func TestReadFrame_ChunkedReads(t *testing.T) {
	var buf bytes.Buffer
	body := []byte(strings.Repeat("x", 500))
	if err := WriteFrame(&buf, body); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	r := &chunkedReader{src: bytes.NewReader(buf.Bytes())}
	got, err := ReadFrame(r)
	if err != nil {
		t.Fatalf("ReadFrame chunked: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("chunked: got %d bytes, want %d", len(got), len(body))
	}
}

func TestEncodeDecode_RoundTrip(t *testing.T) {
	in := Response{
		ID: "abc",
		OK: true,
		Data: map[string]any{
			"pong": true,
		},
	}
	raw, err := Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var out Response
	if err := Decode(raw, &out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out.ID != in.ID || !out.OK {
		t.Errorf("decode mismatch: %+v", out)
	}
}
