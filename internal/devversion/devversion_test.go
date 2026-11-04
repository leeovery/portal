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
