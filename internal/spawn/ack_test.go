package spawn

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
)

func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ackChannel is a channel in a state directory of the test's own.
func ackChannel(t *testing.T) *FileAckChannel {
	t.Helper()
	return NewFileAckChannel(t.TempDir())
}

func mustWrite(t *testing.T, c *FileAckChannel, batch, token string) {
	t.Helper()
	if err := c.Write(batch, token); err != nil {
		t.Fatalf("Write(%s, %s): %v", batch, token, err)
	}
}

// ackFiles lists the names in the channel's directory.
func ackFiles(t *testing.T, c *FileAckChannel) []string {
	t.Helper()
	entries, err := os.ReadDir(c.dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// backdate makes the ack old enough to be stale.
func backdate(t *testing.T, c *FileAckChannel, name string) {
	t.Helper()
	old := time.Now().Add(-staleAckAge - time.Minute)
	if err := os.Chtimes(filepath.Join(c.dir, name), old, old); err != nil {
		t.Fatal(err)
	}
}

func TestFileAckChannel_Write(t *testing.T) {
	t.Run("it writes one file per ack in the state directory's spawn-acks, only its owner able to read it", func(t *testing.T) {
		stateDir := t.TempDir()
		c := NewFileAckChannel(stateDir)

		mustWrite(t, c, "b1", "t1")
		mustWrite(t, c, "b1", "t1")

		if c.dir != filepath.Join(stateDir, "spawn-acks") {
			t.Errorf("Dir = %s, want spawn-acks in the state directory", c.dir)
		}
		if names := ackFiles(t, c); !slices.Equal(names, []string{"b1-t1"}) {
			t.Errorf("acks = %q, want b1-t1 once", names)
		}
		for path, want := range map[string]fs.FileMode{c.dir: 0o700, filepath.Join(c.dir, "b1-t1"): 0o600} {
			if info, err := os.Stat(path); err != nil || info.Mode().Perm() != want {
				t.Errorf("%s mode = %v, %v; want %v", path, info.Mode().Perm(), err, want)
			}
		}
	})

	t.Run("it tightens a directory that already exists", func(t *testing.T) {
		c := ackChannel(t)
		if err := os.MkdirAll(c.dir, 0o755); err != nil {
			t.Fatal(err)
		}

		mustWrite(t, c, "b1", "t1")

		if info, err := os.Stat(c.dir); err != nil || info.Mode().Perm() != 0o700 {
			t.Errorf("dir mode = %v, %v; want 0700", info.Mode().Perm(), err)
		}
	})

	t.Run("it refuses a batch or token no burst minted, making nothing", func(t *testing.T) {
		for _, pair := range [][2]string{{"../b1", "t1"}, {"b1", "../../t1"}, {"b1", "t1/x"}, {"", "t1"}, {"b-1", "t1"}} {
			c := ackChannel(t)

			if err := c.Write(pair[0], pair[1]); err == nil {
				t.Errorf("Write(%q, %q) = nil, want a refusal", pair[0], pair[1])
			}
			if _, err := os.Stat(c.dir); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("Write(%q, %q) made the directory: %v", pair[0], pair[1], err)
			}
		}
	})
}

func TestFileAckChannel_Collect(t *testing.T) {
	t.Run("it returns the batch's tokens and nothing of any other batch or file", func(t *testing.T) {
		c := ackChannel(t)
		mustWrite(t, c, "b1", "t1")
		mustWrite(t, c, "b1", "t2")
		mustWrite(t, c, "b2", "t9")
		for _, name := range []string{"b1-x.tmp", "README"} {
			if err := os.WriteFile(filepath.Join(c.dir, name), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Mkdir(filepath.Join(c.dir, "b1-t3"), 0o700); err != nil {
			t.Fatal(err)
		}

		got, err := c.Collect("b1")

		if want := []string{"t1", "t2"}; err != nil || !slices.Equal(sortedKeys(got), want) {
			t.Errorf("Collect(b1) = %v, %v; want %v", sortedKeys(got), err, want)
		}
	})

	t.Run("a batch with no acks, or a directory never made, is an empty set", func(t *testing.T) {
		c := ackChannel(t)
		if got, err := c.Collect("b1"); err != nil || got == nil || len(got) != 0 {
			t.Errorf("Collect before any ack = %v, %v; want a non-nil empty set", got, err)
		}
		mustWrite(t, c, "b2", "t1")
		if got, err := c.Collect("b1"); err != nil || got == nil || len(got) != 0 {
			t.Errorf("Collect(b1) = %v, %v; want a non-nil empty set", got, err)
		}
	})

	t.Run("a directory it can't read is an error, never an empty set", func(t *testing.T) {
		c := ackChannel(t)
		if err := os.WriteFile(c.dir, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		if got, err := c.Collect("b1"); err == nil || got != nil {
			t.Errorf("Collect = %v, %v; want (nil, the read's error)", got, err)
		}
	})
}

func TestFileAckChannel_Clean(t *testing.T) {
	t.Run("it removes the batch's acks and leaves another burst's fresh ones, idempotently", func(t *testing.T) {
		c := ackChannel(t)
		mustWrite(t, c, "b1", "t1")
		mustWrite(t, c, "b1", "t2")
		mustWrite(t, c, "b2", "t9")

		for range 2 {
			if err := c.Clean("b1"); err != nil {
				t.Fatalf("Clean(b1) = %v", err)
			}
		}

		if names := ackFiles(t, c); !slices.Equal(names, []string{"b2-t9"}) {
			t.Errorf("acks = %q, want only the other burst's", names)
		}
	})

	t.Run("it sweeps any burst's stale acks, as one that ended before cleaning leaves them", func(t *testing.T) {
		sink := logtest.Install(t)
		c := ackChannel(t)
		mustWrite(t, c, "b1", "t1")
		mustWrite(t, c, "crashd", "t1")
		mustWrite(t, c, "live", "t1")
		backdate(t, c, "crashd-t1")

		if err := c.Clean("b1"); err != nil {
			t.Fatalf("Clean(b1) = %v", err)
		}

		if names := ackFiles(t, c); !slices.Equal(names, []string{"live-t1"}) {
			t.Errorf("acks = %q, want only the live burst's", names)
		}
		rec := sink.Records().Matching("spawn", "stale spawn ack removed").AtExactLevel(slog.LevelDebug).Only(t, "stale ack record")
		if got := rec.AttrString(t, "path"); got != filepath.Join(c.dir, "crashd-t1") {
			t.Errorf("path = %q, want the stale ack's", got)
		}
	})

	t.Run("a directory never made is clean", func(t *testing.T) {
		if err := ackChannel(t).Clean("b1"); err != nil {
			t.Errorf("Clean = %v, want nil", err)
		}
	})

	t.Run("a removal that fails is returned", func(t *testing.T) {
		c := ackChannel(t)
		mustWrite(t, c, "b1", "t1")
		mustWrite(t, c, "b1", "t2")
		if err := os.Chmod(c.dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(c.dir, 0o700) })

		if err := c.Clean("b1"); err == nil {
			t.Error("Clean = nil, want the failed removal")
		}
	})
}
