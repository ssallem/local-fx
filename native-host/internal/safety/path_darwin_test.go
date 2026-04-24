//go:build darwin

package safety

import (
	"errors"
	"testing"
)

func TestIsSystemPath_Darwin(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		// positive
		{"/System itself", "/System", true},
		{"/System child", "/System/Library", true},
		{"/usr", "/usr", true},
		{"/usr/bin", "/usr/bin", true},
		{"/Library/Preferences", "/Library/Preferences", true},
		{"/private/var", "/private/var", true},
		{"/bin/sh", "/bin/sh", true},
		{"/sbin/launchd", "/sbin/launchd", true},
		// /Applications (W1-1): third-party .app bundles need the same
		// protection — mutating Contents/MacOS breaks code signing.
		{"/Applications Safari bundle plist", "/Applications/Safari.app/Contents/Info.plist", true},
		{"/Applications third-party bundle", "/Applications/MyCustomApp.app", true},

		// negative — boundary (case-sensitive on macOS)
		{"/usr2", "/usr2", false},
		{"/usr2/foo", "/usr2/foo/bar", false},
		{"/Systems", "/Systems/x", false},
		{"/USR", "/USR/bin", false}, // wrong case, no match
		// /Applications prefix collisions must not match.
		{"/ApplicationsOther boundary", "/ApplicationsOther", false},
		{"/ApplicationsOther/sub boundary", "/ApplicationsOther/sub", false},

		// negative — user data
		{"/Users/me", "/Users/me/Documents", false},
		{"/Volumes/Ext", "/Volumes/Ext", false},
		{"/tmp", "/tmp/foo", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := IsSystemPath(c.path)
			if got != c.want {
				t.Errorf("IsSystemPath(%q) = %v, want %v", c.path, got, c.want)
			}
		})
	}
}

func TestCheckMutatingOp_Darwin(t *testing.T) {
	if err := CheckMutatingOp("/System/Library", false); !errors.Is(err, ErrSystemPathConfirmRequired) {
		t.Errorf("system+noconfirm: got %v, want ErrSystemPathConfirmRequired", err)
	}
	if err := CheckMutatingOp("/System/Library", true); err != nil {
		t.Errorf("system+confirm: got %v, want nil", err)
	}
	if err := CheckMutatingOp("/Users/me/x", false); err != nil {
		t.Errorf("user path: got %v, want nil", err)
	}
}
