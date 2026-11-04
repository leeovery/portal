package bootstrap

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/state"
)

// owner pairs a fresh state directory with zmxDir, compared as the zmx
// backend compares directories.
func owner(t *testing.T, zmxDir string) ZmxDirOwner {
	t.Helper()
	return ZmxDirOwner{StateDir: t.TempDir(), ZmxDir: zmxDir, Canonical: zmx.CanonicalDir}
}

// recordBytes is the state directory's zmx directory record as it stands.
func recordBytes(t *testing.T, o ZmxDirOwner) []byte {
	t.Helper()
	data, err := os.ReadFile(state.ZmxDirRecordPath(o.StateDir))
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	return data
}

// decision 76: a state directory belongs to the one zmx directory the first
// bootstrap that finds no record ran against.
func TestZmxDirOwner(t *testing.T) {
	t.Run("the first claim records the zmx directory, canonical", func(t *testing.T) {
		zmxDir := t.TempDir()
		o := owner(t, zmxDir)

		if err := o.Claim(); err != nil {
			t.Fatalf("Claim = %v, want nil", err)
		}
		if got, err := state.ReadZmxDir(o.StateDir); err != nil || got != zmx.CanonicalDir(zmxDir) {
			t.Errorf("record = %q, %v; want %q", got, err, zmx.CanonicalDir(zmxDir))
		}
	})

	t.Run("a claim against the zmx directory recorded passes, the record untouched", func(t *testing.T) {
		o := owner(t, t.TempDir())
		if err := o.Claim(); err != nil {
			t.Fatal(err)
		}
		before := recordBytes(t, o)

		if err := o.Claim(); err != nil {
			t.Errorf("Claim = %v, want nil", err)
		}
		if after := recordBytes(t, o); !bytes.Equal(before, after) {
			t.Errorf("record = %s, want %s untouched", after, before)
		}
	})

	t.Run("another zmx directory is refused, naming both, with the record untouched", func(t *testing.T) {
		recorded := owner(t, t.TempDir())
		if err := recorded.Claim(); err != nil {
			t.Fatal(err)
		}
		before := recordBytes(t, recorded)
		other := t.TempDir()
		o := ZmxDirOwner{StateDir: recorded.StateDir, ZmxDir: other, Canonical: zmx.CanonicalDir}

		for name, judge := range map[string]func() error{"Claim": o.Claim, "Check": o.Check} {
			err := judge()
			if refused, ok := errors.AsType[*OtherZmxDirError](err); !ok || refused.Recorded != zmx.CanonicalDir(recorded.ZmxDir) || refused.Using != other || refused.StateDir != o.StateDir {
				t.Errorf("%s = %v, want an *OtherZmxDirError naming %s and %s", name, err, recorded.ZmxDir, other)
			}
		}
		if after := recordBytes(t, o); !bytes.Equal(before, after) {
			t.Errorf("record = %s, want %s untouched", after, before)
		}
	})

	t.Run("a symbolic link to the zmx directory recorded passes", func(t *testing.T) {
		o := owner(t, t.TempDir())
		if err := o.Claim(); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(t.TempDir(), "zmx")
		if err := os.Symlink(o.ZmxDir, link); err != nil {
			t.Fatal(err)
		}

		if err := (ZmxDirOwner{StateDir: o.StateDir, ZmxDir: link, Canonical: zmx.CanonicalDir}).Claim(); err != nil {
			t.Errorf("Claim through a link = %v, want nil", err)
		}
	})

	t.Run("/tmp and /private/tmp are one zmx directory", func(t *testing.T) {
		if real, err := filepath.EvalSymlinks("/tmp"); err != nil || real != "/private/tmp" {
			t.Skip("/tmp is not a link to /private/tmp here")
		}
		zmxDir, err := os.MkdirTemp("/tmp", "zx-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(zmxDir) })
		o := owner(t, zmxDir)
		if err := o.Claim(); err != nil {
			t.Fatal(err)
		}

		if err := (ZmxDirOwner{StateDir: o.StateDir, ZmxDir: "/private" + zmxDir, Canonical: zmx.CanonicalDir}).Claim(); err != nil {
			t.Errorf("Claim as /private%s = %v, want nil", zmxDir, err)
		}
	})

	t.Run("Check records nothing", func(t *testing.T) {
		o := owner(t, t.TempDir())

		if err := o.Check(); err != nil {
			t.Errorf("Check = %v, want nil", err)
		}
		if _, err := os.Stat(state.ZmxDirRecordPath(o.StateDir)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat the record = %v, want none written", err)
		}
	})

	t.Run("a record that doesn't decode is an error, never a pass", func(t *testing.T) {
		o := owner(t, t.TempDir())
		if err := os.WriteFile(state.ZmxDirRecordPath(o.StateDir), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := o.Claim(); err == nil || errors.As(err, new(*OtherZmxDirError)) {
			t.Errorf("Claim = %v, want the read's error", err)
		}
	})
}
