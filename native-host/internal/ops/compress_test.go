package ops

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"local-fx-host/internal/protocol"
)

// compressReq marshals a CompressArgs-shaped map into a streaming Request so
// tests can stay loose about which optional fields they want to exercise.
func compressReq(t *testing.T, id string, args map[string]any) protocol.Request {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return protocol.Request{ID: id, Op: "compress", Args: raw, Stream: true}
}

// archivePathFromResponse extracts the data.archivePath string from a
// successful compress Response. Returns "" if absent so tests can branch.
func archivePathFromResponse(t *testing.T, resp protocol.Response) string {
	t.Helper()
	if resp.Data == nil {
		return ""
	}
	m, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data: got %T, want map[string]any", resp.Data)
	}
	v, _ := m["archivePath"].(string)
	return v
}

// listZipEntries opens the archive at path and returns the names of every
// entry inside it. Order matches the zip's central directory.
func listZipEntries(t *testing.T, path string) []string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("zip.OpenReader %s: %v", path, err)
	}
	defer r.Close()
	out := make([]string, 0, len(r.File))
	for _, f := range r.File {
		out = append(out, f.Name)
	}
	return out
}

// readZipEntry returns the bytes of a single named entry in the archive.
func readZipEntry(t *testing.T, archive, entry string) []byte {
	t.Helper()
	r, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Name != entry {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("entry.Open: %v", err)
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		return b
	}
	t.Fatalf("entry %q not found in %s", entry, archive)
	return nil
}

// TestCompress_RegisteredInRegistry confirms the dispatcher knows about us.
func TestCompress_RegisteredInRegistry(t *testing.T) {
	if LookupStream("compress") == nil {
		t.Fatal("compress stream handler not registered")
	}
}

// TestCompress_SingleFile_HappyPath: one file, default name, default
// overwrite=false. Archive should land at <destDir>/<basename>.zip.
func TestCompress_SingleFile_HappyPath(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "hello.txt")
	if err := os.WriteFile(src, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(base, "out")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	col := &collector{}
	resp := Compress(context.Background(), compressReq(t, "c-single", map[string]any{
		"paths":   []string{src},
		"destDir": dest,
	}), col.emit)
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}

	got := archivePathFromResponse(t, resp)
	want := filepath.Join(dest, "hello.txt.zip")
	if got != want {
		t.Errorf("archivePath: got %q want %q", got, want)
	}

	entries := listZipEntries(t, got)
	if len(entries) != 1 || entries[0] != "hello.txt" {
		t.Errorf("entries: got %v want [hello.txt]", entries)
	}
	body := readZipEntry(t, got, "hello.txt")
	if string(body) != "hello world" {
		t.Errorf("body: got %q want %q", body, "hello world")
	}

	// Done event must be the final emitted event with no canceled flag.
	events := col.snapshot()
	if len(events) == 0 || events[len(events)-1].Event != "done" {
		t.Fatalf("missing done event, events=%+v", events)
	}
	d, _ := events[len(events)-1].Payload.(protocol.DonePayload)
	if d.Canceled {
		t.Errorf("done.canceled: got true, want false on happy path")
	}
}

// TestCompress_SingleDirectory_HappyPath: one directory with nested files.
// Archive should mirror the layout under <dirname>/...
func TestCompress_SingleDirectory_HappyPath(t *testing.T) {
	base := t.TempDir()
	srcDir := filepath.Join(base, "tree")
	if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sub", "b.txt"), []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(base, "out")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	col := &collector{}
	resp := Compress(context.Background(), compressReq(t, "c-dir", map[string]any{
		"paths":   []string{srcDir},
		"destDir": dest,
	}), col.emit)
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}

	archive := archivePathFromResponse(t, resp)
	entries := listZipEntries(t, archive)
	// Expect: tree/, tree/a.txt, tree/sub/, tree/sub/b.txt. Order within the
	// archive matches walk order, so we just assert set membership.
	want := map[string]bool{
		"tree/":          true,
		"tree/a.txt":     true,
		"tree/sub/":      true,
		"tree/sub/b.txt": true,
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e] = true
	}
	for k := range want {
		if !got[k] {
			t.Errorf("missing entry %q (got %v)", k, entries)
		}
	}
	if string(readZipEntry(t, archive, "tree/a.txt")) != "A" {
		t.Errorf("a.txt content wrong")
	}
	if string(readZipEntry(t, archive, "tree/sub/b.txt")) != "B" {
		t.Errorf("sub/b.txt content wrong")
	}
}

// TestCompress_MultiplePathsMixed: a file + a directory in the same archive.
func TestCompress_MultiplePathsMixed(t *testing.T) {
	base := t.TempDir()
	srcDir := filepath.Join(base, "foo")
	if err := os.Mkdir(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "inside.txt"), []byte("I"), 0o644); err != nil {
		t.Fatal(err)
	}
	srcFile := filepath.Join(base, "bar.txt")
	if err := os.WriteFile(srcFile, []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(base, "out")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	col := &collector{}
	resp := Compress(context.Background(), compressReq(t, "c-mix", map[string]any{
		"paths":       []string{srcDir, srcFile},
		"destDir":     dest,
		"archiveName": "mix.zip",
	}), col.emit)
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	archive := archivePathFromResponse(t, resp)
	entries := listZipEntries(t, archive)
	want := map[string]bool{
		"foo/":           true,
		"foo/inside.txt": true,
		"bar.txt":        true,
	}
	for _, e := range entries {
		delete(want, e)
	}
	if len(want) > 0 {
		t.Errorf("missing entries: %v (got %v)", want, entries)
	}
}

// TestCompress_AutoNamingSingle: empty archiveName + 1 path → <basename>.zip.
func TestCompress_AutoNamingSingle(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "report.txt")
	if err := os.WriteFile(src, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(base, "out")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	resp := Compress(context.Background(), compressReq(t, "c-auto1", map[string]any{
		"paths":   []string{src},
		"destDir": dest,
	}), col.emit)
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	got := archivePathFromResponse(t, resp)
	want := filepath.Join(dest, "report.txt.zip")
	if got != want {
		t.Errorf("archivePath: got %q want %q", got, want)
	}
}

// TestCompress_AutoNamingMultiple: empty archiveName + >=2 paths → timestamped
// archive_YYYYMMDD_HHMMSS.zip.
func TestCompress_AutoNamingMultiple(t *testing.T) {
	base := t.TempDir()
	a := filepath.Join(base, "a.txt")
	b := filepath.Join(base, "b.txt")
	if err := os.WriteFile(a, []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(base, "out")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	resp := Compress(context.Background(), compressReq(t, "c-autoN", map[string]any{
		"paths":   []string{a, b},
		"destDir": dest,
	}), col.emit)
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	got := archivePathFromResponse(t, resp)
	name := filepath.Base(got)
	rx := regexp.MustCompile(`^archive_\d{8}_\d{6}\.zip$`)
	if !rx.MatchString(name) {
		t.Errorf("auto name: got %q want match %s", name, rx)
	}
}

// TestCompress_OverwriteFalseSuffix: existing zip + overwrite=false → " (2).zip".
func TestCompress_OverwriteFalseSuffix(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(base, "out")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-create the canonical archive name so the handler must pick a suffix.
	preexist := filepath.Join(dest, "bundle.zip")
	if err := os.WriteFile(preexist, []byte("not a zip but blocks the name"), 0o644); err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	resp := Compress(context.Background(), compressReq(t, "c-sfx", map[string]any{
		"paths":       []string{src},
		"destDir":     dest,
		"archiveName": "bundle.zip",
	}), col.emit)
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	got := archivePathFromResponse(t, resp)
	want := filepath.Join(dest, "bundle (1).zip")
	if got != want {
		t.Errorf("archivePath: got %q want %q", got, want)
	}
	// Original file must be untouched.
	pre, _ := os.ReadFile(preexist)
	if string(pre) != "not a zip but blocks the name" {
		t.Errorf("pre-existing file was modified: %q", pre)
	}
}

// TestCompress_OverwriteTrueReplaces: overwrite=true clobbers an existing zip.
// We verify the post-call archive is a valid zip containing our source entry,
// which proves the pre-existing junk was replaced.
func TestCompress_OverwriteTrueReplaces(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src.txt")
	if err := os.WriteFile(src, []byte("replaced"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(base, "out")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dest, "bundle.zip")
	if err := os.WriteFile(target, []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	resp := Compress(context.Background(), compressReq(t, "c-over", map[string]any{
		"paths":       []string{src},
		"destDir":     dest,
		"archiveName": "bundle.zip",
		"overwrite":   true,
	}), col.emit)
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	if got := archivePathFromResponse(t, resp); got != target {
		t.Errorf("archivePath: got %q want %q", got, target)
	}
	entries := listZipEntries(t, target)
	if len(entries) != 1 || entries[0] != "src.txt" {
		t.Errorf("entries: got %v want [src.txt]", entries)
	}
	if string(readZipEntry(t, target, "src.txt")) != "replaced" {
		t.Errorf("body mismatch")
	}
}

// TestCompress_EmptyPaths_BadRequest: empty paths → E_BAD_REQUEST.
func TestCompress_EmptyPaths_BadRequest(t *testing.T) {
	col := &collector{}
	resp := Compress(context.Background(), compressReq(t, "c-empty", map[string]any{
		"paths":   []string{},
		"destDir": t.TempDir(),
	}), col.emit)
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodeBadRequest {
		t.Errorf("code: got %q want E_BAD_REQUEST", resp.Error.Code)
	}
	// Setup errors must not emit any events.
	if len(col.snapshot()) != 0 {
		t.Errorf("unexpected events on setup error: %+v", col.snapshot())
	}
}

// TestCompress_ArchiveNameWithSlash_BadRequest: separators in archiveName are
// rejected up front. We try forward slash on every platform and backslash
// too (both are listed as forbidden).
func TestCompress_ArchiveNameWithSlash_BadRequest(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "s.txt")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(base, "out")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, badName := range []string{"foo/bar.zip", "foo\\bar.zip"} {
		col := &collector{}
		resp := Compress(context.Background(), compressReq(t, "c-bad-"+badName, map[string]any{
			"paths":       []string{src},
			"destDir":     dest,
			"archiveName": badName,
		}), col.emit)
		if resp.OK {
			t.Fatalf("expected OK=false for %q", badName)
		}
		if resp.Error.Code != protocol.ErrCodeBadRequest {
			t.Errorf("%q: code got %q want E_BAD_REQUEST", badName, resp.Error.Code)
		}
	}
}

// TestCompress_KoreanFilename_UTF8Flag: Korean filename + UTF-8 flag (0x800)
// on every entry header. Critical for Windows zip extractors that otherwise
// treat names as CP-949.
func TestCompress_KoreanFilename_UTF8Flag(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "한글파일.txt")
	if err := os.WriteFile(src, []byte("안녕"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(base, "out")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	resp := Compress(context.Background(), compressReq(t, "c-utf8", map[string]any{
		"paths":   []string{src},
		"destDir": dest,
	}), col.emit)
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	r, err := zip.OpenReader(archivePathFromResponse(t, resp))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer r.Close()
	if len(r.File) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(r.File))
	}
	f := r.File[0]
	if f.Flags&0x800 == 0 {
		t.Errorf("UTF-8 flag (0x800) not set on header (Flags=0x%x)", f.Flags)
	}
	if f.Name != "한글파일.txt" {
		t.Errorf("entry name: got %q want %q", f.Name, "한글파일.txt")
	}
}

// TestCompress_SubPathDuplicate_Deduped: paths=["A","A/sub"] → only "A" is
// compressed; "A/sub" is dropped to avoid double-encoding the same content.
func TestCompress_SubPathDuplicate_Deduped(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "A")
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "x.txt"), []byte("X"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(base, "out")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	resp := Compress(context.Background(), compressReq(t, "c-dedup", map[string]any{
		"paths":       []string{root, sub},
		"destDir":     dest,
		"archiveName": "A.zip",
	}), col.emit)
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	archive := archivePathFromResponse(t, resp)
	entries := listZipEntries(t, archive)
	// Sort for stable comparison.
	sort.Strings(entries)
	// Should be exactly: A/, A/sub/, A/sub/x.txt — no "sub/" at the root
	// (which would have been the result of NOT deduping).
	want := []string{"A/", "A/sub/", "A/sub/x.txt"}
	got := entries
	if !equalStrings(got, want) {
		t.Errorf("entries: got %v want %v", got, want)
	}
	// Defensive: no entry should start with "sub/" (the orphan that would
	// appear if A/sub had been compressed as a top-level root).
	for _, e := range entries {
		if strings.HasPrefix(e, "sub/") {
			t.Errorf("duplicate entry found: %q", e)
		}
	}
}

// TestCompress_NonexistentLeadingPath_ENOENT: paths[0] missing → fatal setup
// error (ENOENT), no archive written, no done event.
func TestCompress_NonexistentLeadingPath_ENOENT(t *testing.T) {
	base := t.TempDir()
	dest := filepath.Join(base, "out")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	resp := Compress(context.Background(), compressReq(t, "c-noent", map[string]any{
		"paths":   []string{filepath.Join(base, "missing")},
		"destDir": dest,
	}), col.emit)
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodeENOENT {
		t.Errorf("code: got %q want ENOENT", resp.Error.Code)
	}
	for _, ev := range col.snapshot() {
		if ev.Event == "done" {
			t.Errorf("unexpected done event on setup error")
		}
	}
	// destDir must remain empty (no partial archive).
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Errorf("destDir should be empty, got %d entries", len(entries))
	}
}

// TestCompress_RelativePathRejected confirms the safety gate fires on relative
// inputs the same way Copy does.
func TestCompress_RelativePathRejected(t *testing.T) {
	col := &collector{}
	resp := Compress(context.Background(), compressReq(t, "c-rel", map[string]any{
		"paths":   []string{"relative/src"},
		"destDir": "relative/out",
	}), col.emit)
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodePathRejected {
		t.Errorf("code: got %q want E_PATH_REJECTED", resp.Error.Code)
	}
}

// TestCompress_DoubleZipExtensionNotDoubled: if archive auto-name source
// already ends in ".zip" (a folder literally named "x.zip"), the result
// should be "x.zip", not "x.zip.zip".
func TestCompress_DoubleZipExtensionNotDoubled(t *testing.T) {
	base := t.TempDir()
	folder := filepath.Join(base, "weird.zip")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "f.txt"), []byte("F"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(base, "out")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	resp := Compress(context.Background(), compressReq(t, "c-dbl", map[string]any{
		"paths":   []string{folder},
		"destDir": dest,
	}), col.emit)
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp.Error)
	}
	got := archivePathFromResponse(t, resp)
	want := filepath.Join(dest, "weird.zip")
	if got != want {
		t.Errorf("archivePath: got %q want %q", got, want)
	}
}

// TestCompress_DestDirInsidePath_BadRequest: destDir nested under a source
// path must be rejected at setup time with E_BAD_REQUEST. No archive is
// written, no events are emitted.
func TestCompress_DestDirInsidePath_BadRequest(t *testing.T) {
	base := t.TempDir()
	srcDir := filepath.Join(base, "A 폴더")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "x.txt"), []byte("X"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(srcDir, "sub")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	col := &collector{}
	resp := Compress(context.Background(), compressReq(t, "c-destinside", map[string]any{
		"paths":   []string{srcDir},
		"destDir": dest,
	}), col.emit)
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodeBadRequest {
		t.Errorf("code: got %q want E_BAD_REQUEST", resp.Error.Code)
	}
	if len(col.snapshot()) != 0 {
		t.Errorf("unexpected events on setup error: %+v", col.snapshot())
	}
	entries, _ := os.ReadDir(dest)
	for _, e := range entries {
		if strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
			t.Errorf("found unexpected zip in destDir: %q", e.Name())
		}
	}
}

// TestCompress_ArchivePathInsidePath_BadRequest: destDir == a source root
// causes archivePath to land inside that source. The setup guard must
// reject this with E_BAD_REQUEST.
func TestCompress_ArchivePathInsidePath_BadRequest(t *testing.T) {
	base := t.TempDir()
	srcDir := filepath.Join(base, "A")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "x.txt"), []byte("X"), 0o644); err != nil {
		t.Fatal(err)
	}

	col := &collector{}
	resp := Compress(context.Background(), compressReq(t, "c-archiveinside", map[string]any{
		"paths":   []string{srcDir},
		"destDir": srcDir,
	}), col.emit)
	if resp.OK {
		t.Fatalf("expected OK=false")
	}
	if resp.Error.Code != protocol.ErrCodeBadRequest {
		t.Errorf("code: got %q want E_BAD_REQUEST", resp.Error.Code)
	}
	if len(col.snapshot()) != 0 {
		t.Errorf("unexpected events on setup error: %+v", col.snapshot())
	}
}

// equalStrings reports whether two string slices are element-wise equal.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
