package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
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

// slowWriter delays every Write so that two concurrent goroutines hitting
// the same wrapped io.Writer overlap in time. Without SafeWriter the header
// bytes from one frame can land between the header and body of another.
type slowWriter struct {
	buf   bytes.Buffer
	delay time.Duration
}

func (sw *slowWriter) Write(p []byte) (int, error) {
	if sw.delay > 0 {
		time.Sleep(sw.delay)
	}
	return sw.buf.Write(p)
}

func TestSafeWriter_SerialisesConcurrentWrites(t *testing.T) {
	sw := &slowWriter{delay: 2 * time.Millisecond}
	safe := NewSafeWriter(sw)

	bodies := [][]byte{
		[]byte(`{"a":1}`),
		[]byte(`{"b":2}`),
		[]byte(`{"c":3}`),
		[]byte(`{"d":4}`),
		[]byte(`{"e":5}`),
	}

	var wg sync.WaitGroup
	for _, body := range bodies {
		wg.Add(1)
		go func(b []byte) {
			defer wg.Done()
			if err := safe.WriteFrame(b); err != nil {
				t.Errorf("WriteFrame: %v", err)
			}
		}(body)
	}
	wg.Wait()

	// Pull all frames back out; they should decode cleanly regardless of
	// the random write order. Interleaved header+body bytes would fail
	// the length-prefix read here.
	reader := bytes.NewReader(sw.buf.Bytes())
	seen := map[string]bool{}
	for i := 0; i < len(bodies); i++ {
		got, err := ReadFrame(reader)
		if err != nil {
			t.Fatalf("ReadFrame[%d]: %v", i, err)
		}
		seen[string(got)] = true
	}
	for _, body := range bodies {
		if !seen[string(body)] {
			t.Errorf("missing body on wire: %s", body)
		}
	}
}

func TestSafeWriter_RoundTripSingleFrame(t *testing.T) {
	var buf bytes.Buffer
	safe := NewSafeWriter(&buf)
	body := []byte(`{"id":"x","ok":true}`)
	if err := safe.WriteFrame(body); err != nil {
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

func TestSafeWriter_OverMaxRejected(t *testing.T) {
	var buf bytes.Buffer
	safe := NewSafeWriter(&buf)
	body := bytes.Repeat([]byte{'a'}, MaxFrameSize+1)
	if err := safe.WriteFrame(body); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("SafeWriter over max: err=%v, want ErrFrameTooLarge", err)
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
