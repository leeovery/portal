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
	b := mustNew(t, zmx.Config{Dir: "/tmp/zx-unit"})

	t.Run("it accepts the names Portal makes and odd ones zmx holds", func(t *testing.T) {
		for _, name := range []string{
			"dotfiles-PpaTER", "_portal-saver", "logs-1", "logs.d", "a.b", "a:b", "has space", "a=b", "x*y", "mid-dash", "ünï", "...", "$x",
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
			"logs", "Logs", "LOGS", // zmx keeps its log directory beside the sockets; macOS folds case
			"-h", "--help", "--labels=x", "--vt", "--force", // zmx reads these as flags
			"dotfiles*", "*", // zmx kill reads a trailing * as a prefix match
			"a\tb", "a\nb", "a\x00b", "a\x7fb", "a\u0085b", // zmx's own listing prints them raw
			"bad\xffname", "\xc3", // not UTF-8
		} {
			if err := b.ValidateName(name); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("ValidateName(%q) = %v, want ErrInvalidName", name, err)
			}
		}
	})

	t.Run("it refuses a name whose socket path would pass the platform limit", func(t *testing.T) {
		for _, dir := range []string{"/tmp/zx-unit", "/Users/lee/.local/state/zmx"} {
			b := mustNew(t, zmx.Config{Dir: dir})
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

	t.Run("it refuses a name too long as ErrNameTooLong, and no other", func(t *testing.T) {
		if err := b.ValidateName(strings.Repeat("a", b.MaxNameLen()+1)); !errors.Is(err, backend.ErrNameTooLong) {
			t.Errorf("ValidateName of %d bytes = %v, want ErrNameTooLong", b.MaxNameLen()+1, err)
		}
		for _, name := range []string{"", "logs", "-x", "x*", "a/b", "a\tb"} {
			if err := b.ValidateName(name); errors.Is(err, backend.ErrNameTooLong) {
				t.Errorf("ValidateName(%q) = %v, refused as too long", name, err)
			}
		}
	})
}

func TestRefusesRune(t *testing.T) {
	t.Run("it refuses each character a name may never hold, and no other", func(t *testing.T) {
		for _, r := range []rune{'/', 0, '\t', '\n', '\x1b', '\x7f', '\u0085'} {
			if !zmx.RefusesRune(r) {
				t.Errorf("RefusesRune(%q) = false, want true", r)
			}
		}
		for _, r := range []rune{'a', ' ', '-', '*', '.', ':', '=', '$', '·', 'é', '\u202e'} {
			if zmx.RefusesRune(r) {
				t.Errorf("RefusesRune(%q) = true, want false", r)
			}
		}
	})

	t.Run("it refuses a character exactly when CheckName refuses a plain name holding it", func(t *testing.T) {
		for r := range rune(0x100) {
			refused := zmx.CheckName("a"+string(r)+"a") != nil
			if refused != zmx.RefusesRune(r) {
				t.Errorf("CheckName of a name holding %q refuses: %v; RefusesRune: %v", r, refused, zmx.RefusesRune(r))
			}
		}
	})
}

func TestCheckName(t *testing.T) {
	t.Run("it applies every rule but the length", func(t *testing.T) {
		b := mustNew(t, zmx.Config{Dir: "/tmp/zx-unit"})
		for _, name := range []string{"", "logs", "-x", "x*", "a\nb", strings.Repeat("a", b.MaxNameLen())} {
			if got, want := zmx.CheckName(name), b.ValidateName(name); !errors.Is(got, backend.ErrInvalidName) && want != nil {
				t.Errorf("CheckName(%q) = %v, where ValidateName refuses it: %v", name, got, want)
			}
		}
		if err := zmx.CheckName(strings.Repeat("a", 500)); err != nil {
			t.Errorf("CheckName of 500 bytes = %v, want nil: length is the directory's to judge", err)
		}
	})
}

func TestCheckLookup(t *testing.T) {
	t.Run("it accepts every name a session can have, whatever made it", func(t *testing.T) {
		for _, name := range []string{"-x", "--vt", "-h", "x*", "logs", "a\nb", "bad\xffname", "...", strings.Repeat("a", 500)} {
			if err := zmx.CheckLookup(name); err != nil {
				t.Errorf("CheckLookup(%q) = %v, want nil", name, err)
			}
		}
	})

	t.Run("it refuses a name no entry in the directory can have", func(t *testing.T) {
		for _, name := range []string{"", ".", "..", "a/b", "/x", "a\x00b"} {
			if err := zmx.CheckLookup(name); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("CheckLookup(%q) = %v, want ErrInvalidName", name, err)
			}
		}
	})
}

func TestCheckAttach(t *testing.T) {
	t.Run("it refuses what zmx attach reads as its own flags, and what no session can have", func(t *testing.T) {
		for _, name := range []string{"-h", "--help", "--labels", "--labels=a=1", "--labels=", "", "a/b"} {
			if err := zmx.CheckAttach(name); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("CheckAttach(%q) = %v, want ErrInvalidName", name, err)
			}
		}
	})

	t.Run("it passes every other name, as zmx attach takes it for the session's", func(t *testing.T) {
		for _, name := range []string{"-x", "--vt", "-help", "--labelsx", "--labels-x", "x*", "logs"} {
			if err := zmx.CheckAttach(name); err != nil {
				t.Errorf("CheckAttach(%q) = %v, want nil", name, err)
			}
		}
	})
}

func TestNew(t *testing.T) {
	t.Run("it refuses a directory that is empty or relative", func(t *testing.T) {
		for _, dir := range []string{"", ".", "zmx", "./zmx", "../zmx"} {
			if b, err := zmx.New(zmx.Config{Dir: dir}); err == nil {
				t.Errorf("New(%q) bound %s, want an error", dir, b.Dir())
			}
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
			b := mustNew(t, zmx.Config{Dir: dir})
			if got := len(dir) + 1 + b.MaxNameLen(); got != limit {
				t.Errorf("socket path for the longest name in %s = %d bytes, want %d", dir, got, limit)
			}
		}
	})

	t.Run("it measures the directory as zmx is given it", func(t *testing.T) {
		if a, b := mustNew(t, zmx.Config{Dir: "/tmp/zx/"}), mustNew(t, zmx.Config{Dir: "/tmp/zx"}); a.MaxNameLen() != b.MaxNameLen() || a.Dir() != "/tmp/zx" {
			t.Errorf("trailing slash: Dir %q, MaxNameLen %d vs %d", a.Dir(), a.MaxNameLen(), b.MaxNameLen())
		}
	})
}
