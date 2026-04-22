//go:build windows

package safety

import (
	"errors"
	"testing"
)

func TestIsSystemPath_Windows(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		// positive — direct matches
		{"Windows dir itself", `C:\Windows`, true},
		{"Windows with trailing sep", `C:\Windows\`, true},
		{"inside Windows", `C:\Windows\System32`, true},
		{"deeply inside Windows", `C:\Windows\System32\drivers\etc\hosts`, true},
		{"Program Files", `C:\Program Files`, true},
		{"Program Files (x86)", `C:\Program Files (x86)\App`, true},
		{"ProgramData exact", `C:\ProgramData`, true},
		{"ProgramData child", `C:\ProgramData\Foo`, true},

		// positive — case insensitive
		{"lowercase drive", `c:\windows\foo`, true},
		{"UPPERCASE", `C:\WINDOWS\SYSTEM32`, true},
		{"MixedCase", `C:\wInDoWs\FOO`, true},

		// negative — boundary edge (ProgramDataX must NOT match ProgramData)
		{"ProgramDataX boundary", `C:\ProgramDataX`, false},
		{"ProgramDataX child", `C:\ProgramDataX\sub`, false},
		{"WindowsSomething boundary", `C:\WindowsApps`, false},

		// negative — user dirs
		{"Users subtree", `C:\Users\me\Documents`, false},
		{"D drive", `D:\random\file.txt`, false},
		{"tempdir", `C:\Temp`, false},
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

func TestCheckMutatingOp_Windows(t *testing.T) {
	// System path + no confirm -> require confirm.
	if err := CheckMutatingOp(`C:\Windows\foo`, false); !errors.Is(err, ErrSystemPathConfirmRequired) {
		t.Errorf("system+noconfirm: got %v, want ErrSystemPathConfirmRequired", err)
	}
	// System path + explicit confirm -> allowed.
	if err := CheckMutatingOp(`C:\Windows\foo`, true); err != nil {
		t.Errorf("system+confirm: got %v, want nil", err)
	}
	// Non-system path -> allowed regardless.
	if err := CheckMutatingOp(`C:\Users\me\x`, false); err != nil {
		t.Errorf("user path: got %v, want nil", err)
	}
	if err := CheckMutatingOp(`D:\foo`, false); err != nil {
		t.Errorf("D drive: got %v, want nil", err)
	}
}
