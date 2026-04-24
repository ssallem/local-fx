package ops

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUniqueName_NonexistentReturnsInput(t *testing.T) {
	tmp := t.TempDir()
	got, err := UniqueName(tmp, "new.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(tmp, "new.txt")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestUniqueName_CollisionAppendsNumber(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "x.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := UniqueName(tmp, "x.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(tmp, "x (1).txt")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestUniqueName_ChainCollisions(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "y.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "y (1).txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := UniqueName(tmp, "y.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(tmp, "y (2).txt")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestUniqueName_NoExtension(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "README"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := UniqueName(tmp, "README")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(tmp, "README (1)")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPathsEqual(t *testing.T) {
	if !pathsEqual("/a/b", "/a/b") {
		t.Fatal("identical paths should be equal")
	}
	if pathsEqual("/a/b", "/a/c") {
		t.Fatal("different paths should not be equal")
	}
	// Case sensitivity is platform-dependent.
	got := pathsEqual("C:\\Foo\\Bar", "C:\\foo\\bar")
	want := runtime.GOOS == "windows"
	if got != want {
		t.Fatalf("case compare: got %v want %v on %s", got, want, runtime.GOOS)
	}
}
