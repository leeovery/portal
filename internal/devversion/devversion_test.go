package devversion_test

import (
	"os"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/devversion"
)

func TestString(t *testing.T) {
	t.Run("it ends with when the running executable was written", func(t *testing.T) {
		exe, err := os.Executable()
		if err != nil {
			t.Fatalf("os.Executable: %v", err)
		}
		fi, err := os.Stat(exe)
		if err != nil {
			t.Fatalf("stat %s: %v", exe, err)
		}

		got := devversion.String()

		if want := "-" + fi.ModTime().UTC().Format("20060102T150405.000000000Z"); !strings.HasPrefix(got, "dev") || !strings.HasSuffix(got, want) {
			t.Errorf("String() = %q, want dev… ending %q", got, want)
		}
	})

	t.Run("it is the same on every call", func(t *testing.T) {
		if a, b := devversion.String(), devversion.String(); a != b {
			t.Errorf("String() = %q, then %q", a, b)
		}
	})
}

func TestIs(t *testing.T) {
	for _, tc := range []struct {
		version string
		dev     bool
	}{
		{devversion.Unidentified, true},
		{"dev-20261103T231632.195584591Z", true},
		{"dev-754e31b498e0-20261103T231632.195584591Z", true},
		{"dev-754e31b498e0-modified-20261103T231632.195584591Z", true},
		{"0.12.6", false},
		{"1.4.0", false},
		{"0.0.0-20261104001632-754e31b498e0", false},
		{"devel", false},
		{"", false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			if got := devversion.Is(tc.version); got != tc.dev {
				t.Errorf("Is(%q) = %v, want %v", tc.version, got, tc.dev)
			}
		})
	}

	t.Run("it holds for the version this build reports", func(t *testing.T) {
		if v := devversion.String(); !devversion.Is(v) {
			t.Errorf("Is(String()) = false for %q", v)
		}
	})
}
