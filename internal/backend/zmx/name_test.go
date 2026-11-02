package zmx_test

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
)

func TestValidateName(t *testing.T) {
	b := newBackend(newRunner(t, nil))

	t.Run("it accepts the names Portal makes and odd ones zmx holds", func(t *testing.T) {
		for _, name := range []string{
			"dotfiles-PpaTER", "_portal-saver", "a.b", "a:b", "has space", "a=b", "x*y", "mid-dash", "ünï", "...", "$x",
		} {
			if err := b.ValidateName(name); err != nil {
				t.Errorf("ValidateName(%q) = %v, want nil", name, err)
			}
		}
	})

	t.Run("it refuses names zmx refuses, misreads or cannot list", func(t *testing.T) {
		for _, name := range []string{
			"",        // zmx requires a name
			".", "..", // zmx refuses directory entries
			"a/b", "/x", // zmx refuses path separators
			"-h", "--help", "--labels=x", "--vt", "--force", // zmx reads these as flags
			"dotfiles*", "*", // zmx kill reads a trailing * as a prefix match
			"a\tb", "a\nb", "a\x00b", "a\x7fb", "a\u0085b", // list output cannot carry them
		} {
			if err := b.ValidateName(name); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("ValidateName(%q) = %v, want ErrInvalidName", name, err)
			}
		}
	})

	t.Run("it refuses a name whose socket path would pass the platform limit", func(t *testing.T) {
		for _, dir := range []string{"/tmp/zx-unit", "/Users/lee/.local/state/zmx"} {
			b := zmx.New(dir, fixedEnviron(), newRunner(t, nil))
			limit := b.MaxNameLen()

			if err := b.ValidateName(strings.Repeat("a", limit)); err != nil {
				t.Errorf("ValidateName of %d bytes in %s = %v, want nil", limit, dir, err)
			}
			if err := b.ValidateName(strings.Repeat("a", limit+1)); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("ValidateName of %d bytes in %s = %v, want ErrInvalidName", limit+1, dir, err)
			}
		}
	})

	t.Run("it counts the name in bytes", func(t *testing.T) {
		limit := b.MaxNameLen()
		if err := b.ValidateName(strings.Repeat("é", limit/2+1)); !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("ValidateName of %d two-byte runes = %v, want ErrInvalidName", limit/2+1, err)
		}
	})
}

func TestMaxNameLen(t *testing.T) {
	t.Run("it leaves room for the directory and a separator within the socket path limit", func(t *testing.T) {
		limits := map[string]int{"darwin": 103, "linux": 107}
		limit, ok := limits[runtime.GOOS]
		if !ok {
			t.Skipf("no socket path limit pinned for %s", runtime.GOOS)
		}
		for _, dir := range []string{"/tmp/zx-unit", "/Users/lee/.local/state/zmx"} {
			b := zmx.New(dir, fixedEnviron(), newRunner(t, nil))
			if got := len(dir) + 1 + b.MaxNameLen(); got != limit {
				t.Errorf("socket path for the longest name in %s = %d bytes, want %d", dir, got, limit)
			}
		}
	})

	t.Run("it measures the directory as zmx is given it", func(t *testing.T) {
		if a, b := zmx.New("/tmp/zx/", fixedEnviron(), nil), zmx.New("/tmp/zx", fixedEnviron(), nil); a.MaxNameLen() != b.MaxNameLen() || a.Dir() != "/tmp/zx" {
			t.Errorf("trailing slash: Dir %q, MaxNameLen %d vs %d", a.Dir(), a.MaxNameLen(), b.MaxNameLen())
		}
	})
}
