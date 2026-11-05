package zmx_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/backend/zmx"
)

func TestCanonicalDir(t *testing.T) {
	// real is a directory with no link in its path, so its canonical form is
	// itself.
	real := func(t *testing.T) string {
		t.Helper()
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return dir
	}

	t.Run("a missing dir under a linked parent has the parent resolved", func(t *testing.T) {
		parent := real(t)
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(parent, link); err != nil {
			t.Fatal(err)
		}

		if got, want := zmx.CanonicalDir(filepath.Join(link, "gone", "zmx")), filepath.Join(parent, "gone", "zmx"); got != want {
			t.Errorf("CanonicalDir = %q, want %q", got, want)
		}
	})

	t.Run("a trailing slash and a .. compare equal, the dir there or not", func(t *testing.T) {
		parent := real(t)
		if err := os.Mkdir(filepath.Join(parent, "x"), 0o700); err != nil {
			t.Fatal(err)
		}
		for _, leaf := range []string{"there", "gone"} {
			if leaf == "there" {
				if err := os.Mkdir(filepath.Join(parent, leaf), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			want := filepath.Join(parent, leaf)
			for _, spelling := range []string{want + "/", filepath.Join(parent, "x") + "/../" + leaf} {
				if got := zmx.CanonicalDir(spelling); got != want {
					t.Errorf("CanonicalDir(%q) = %q, want %q", spelling, got, want)
				}
			}
		}
	})

	// /x/link/../c is c beside link's target, as the OS resolves it, never
	// /x/c, which a lexical clean would make it.
	t.Run("a .. after a link is taken after the link, the dir there or not", func(t *testing.T) {
		x, y := real(t), real(t)
		if err := os.Mkdir(filepath.Join(y, "b"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(y, "b"), filepath.Join(x, "link")); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(x, "c"), 0o700); err != nil {
			t.Fatal(err)
		}
		spelling := filepath.Join(x, "link") + "/../c"

		if got, want := zmx.CanonicalDir(spelling), filepath.Join(y, "c"); got != want {
			t.Errorf("CanonicalDir(%q) = %q, want %q", spelling, got, want)
		}
		if got := zmx.CanonicalDir(spelling); got == zmx.CanonicalDir(filepath.Join(x, "c")) {
			t.Errorf("CanonicalDir(%q) = %q, equal to the existing %s/c", spelling, got, x)
		}
	})

	t.Run("a .. past a missing dir keeps its spelling, equal to nothing else", func(t *testing.T) {
		parent := real(t)
		if err := os.Mkdir(filepath.Join(parent, "c"), 0o700); err != nil {
			t.Fatal(err)
		}
		spelling := filepath.Join(parent, "gone") + "/../c"

		if got := zmx.CanonicalDir(spelling); got != spelling || got == zmx.CanonicalDir(filepath.Join(parent, "c")) {
			t.Errorf("CanonicalDir(%q) = %q, want it as written", spelling, got)
		}
	})

	// The holder proof compares a process's ZMX_DIR with the backend's own,
	// which exists: a missing one must never match it.
	t.Run("a missing dir never compares equal to one that exists", func(t *testing.T) {
		there := real(t)

		if got := zmx.CanonicalDir(filepath.Join(there, "gone")); got == zmx.CanonicalDir(there) {
			t.Errorf("CanonicalDir of a missing child = %q, the existing dir's own", got)
		}
	})

	t.Run("a relative dir that doesn't resolve is as written, cleaned", func(t *testing.T) {
		if got := zmx.CanonicalDir("no/such/zmx/"); got != "no/such/zmx" {
			t.Errorf("CanonicalDir = %q, want no/such/zmx", got)
		}
	})
}
