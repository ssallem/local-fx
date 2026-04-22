package ops

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"local-fx-host/internal/platform"
	"local-fx-host/internal/protocol"
)

// TestListDrives_ReturnsAtLeastOneDrive is a sanity check: every CI host we
// care about (Windows agents, macOS runners, dev laptops) mounts at least one
// volume that ListDrives should surface. On unsupported OSes the platform
// layer returns ErrUnsupportedOS which maps to EIO — this test asserts the
// happy path on the two tier-1 targets and skips otherwise.
func TestListDrives_HappyPath(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skipf("listDrives only has a native impl on windows/darwin, got %s", runtime.GOOS)
	}
	resp := ListDrives(context.Background(), protocol.Request{ID: "req-1", Op: "listDrives"})
	if !resp.OK {
		t.Fatalf("OK=false: error=%+v", resp.Error)
	}
	// Round-trip through JSON so we assert the on-wire shape, not just the
	// in-memory struct (catches field tag typos).
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var parsed struct {
		Drives []platform.Drive `json:"drives"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Drives) == 0 {
		t.Fatalf("expected >=1 drive, got 0")
	}
	for i, d := range parsed.Drives {
		if d.Path == "" {
			t.Errorf("drive[%d].Path is empty", i)
		}
		if runtime.GOOS == "windows" {
			// Every Windows path should end with "\" so concatenating a
			// filename produces a valid absolute path.
			if !strings.HasSuffix(d.Path, `\`) {
				t.Errorf("drive[%d].Path %q missing trailing backslash", i, d.Path)
			}
		} else {
			if !strings.HasPrefix(d.Path, "/") {
				t.Errorf("drive[%d].Path %q should be absolute", i, d.Path)
			}
		}
	}
}

func TestListDrives_EmptyArgsAccepted(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("platform-specific")
	}
	// Extension may send either `args: {}` or omit args entirely; both paths
	// should succeed because listDrives ignores the field.
	for _, args := range [][]byte{nil, []byte(`{}`)} {
		req := protocol.Request{ID: "r", Op: "listDrives"}
		if args != nil {
			req.Args = args
		}
		resp := ListDrives(context.Background(), req)
		if !resp.OK {
			t.Errorf("args=%s: OK=false, error=%+v", args, resp.Error)
		}
	}
}

func TestListDrives_RegisteredInRegistry(t *testing.T) {
	if Lookup("listDrives") == nil {
		t.Fatal("listDrives handler not registered")
	}
}
