package migrate_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/migrate"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// applying is a plan of two tmux sessions, one of two panes, ready to apply:
// the tmux version's state dir, a new state dir not yet made, and a hooks.json
// holding a token key in the string form, a pre-token key in the object form
// with an attribute the codec doesn't model, and a key mapping to nothing.
type applying struct {
	plan      migrate.Plan
	legacyDir string
	dir       string
	store     *hooks.Store
	hooksPath string
}

const applyingHooks = `{
  "tokLan": {"on-resume": "claude --resume lantern"},
  "tidepool · flaky test:0.1": {"on-resume": {"command": "claude --resume tidepool", "resume": "eager", "nickname": "kept"}},
  "tokGone": {"on-resume": "claude --resume gone"}
}`

func newApplying(t *testing.T) applying {
	t.Helper()
	sessions := []v1Session{
		session("lantern", pane(0, orchard).token("tokLan").scrollback("scrollback/pane-tokLan.bin")),
		session("tidepool · flaky test",
			pane(0, orchard).scrollback("scrollback/tidepool___flaky_test-0b1c2d3e__0.0.bin"),
			pane(1, orchard+"/web").scrollback("scrollback/gone.bin")),
	}
	legacy := legacyDir(t, sessions, map[string]string{
		"scrollback/pane-tokLan.bin":                         "\x1b[1mlantern\x1b[0m\r\n$ ",
		"scrollback/tidepool___flaky_test-0b1c2d3e__0.0.bin": "tidepool transcript\n",
	})
	store, hooksPath := hookstest.StageStore(t, hookstest.Staging{Seed: applyingHooks})
	return applying{
		plan:      planOf(t, legacy, hookKeys(t, store), naming(t)),
		legacyDir: legacy,
		dir:       filepath.Join(t.TempDir(), "state"),
		store:     store,
		hooksPath: hooksPath,
	}
}

func hookKeys(t *testing.T, store *hooks.Store) []string {
	t.Helper()
	held, err := store.LoadStrict(hooks.ViaCLI)
	if err != nil {
		t.Fatalf("load hooks.json: %v", err)
	}
	return slices.Collect(maps.Keys(held))
}

func (a applying) apply(t *testing.T) {
	t.Helper()
	if err := migrate.Apply(a.plan, a.dir, a.store); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}

func TestApply(t *testing.T) {
	t.Run("it writes each session's entry owed a restore and recording no creation", func(t *testing.T) {
		a := newApplying(t)
		a.apply(t)

		idx, err := sessionindex.Read(a.dir)
		if err != nil {
			t.Fatalf("read sessions.json: %v", err)
		}
		scrollback := func(name string) string {
			file, err := state.SessionScrollbackFile(name, savedAt, 0)
			if err != nil {
				t.Fatalf("SessionScrollbackFile: %v", err)
			}
			return file
		}
		want := []sessionindex.Entry{
			{Name: "lantern", Dir: orchard, Cwd: orchard, ScrollbackFile: scrollback("lantern"), Owed: true},
			{Name: "tidepool", Label: "tidepool · flaky test", Dir: orchard, Cwd: orchard, ScrollbackFile: scrollback("tidepool"), Owed: true},
			{Name: "tidepool-2", Label: "tidepool · flaky test-2", Dir: orchard + "/web", Cwd: orchard + "/web", Owed: true},
		}
		if got := idx.Entries(); !slices.Equal(got, want) {
			t.Errorf("entries = %+v\nwant %+v", got, want)
		}
	})

	t.Run("it copies each scrollback file byte for byte to the file its entry names, which clean and Forget both own", func(t *testing.T) {
		a := newApplying(t)
		a.apply(t)

		idx, err := sessionindex.Read(a.dir)
		if err != nil {
			t.Fatalf("read sessions.json: %v", err)
		}
		for name, source := range map[string]string{
			"lantern":  "scrollback/pane-tokLan.bin",
			"tidepool": "scrollback/tidepool___flaky_test-0b1c2d3e__0.0.bin",
		} {
			entry, _ := idx.Lookup(name)
			path, err := state.ScrollbackPath(a.dir, entry)
			if err != nil {
				t.Errorf("%s: Forget's ScrollbackPath refuses %q: %v", name, entry.ScrollbackFile, err)
				continue
			}
			if !state.IsSessionScrollbackFile(filepath.Base(path)) {
				t.Errorf("%s: the saver's clean doesn't own %s", name, path)
			}
			if got, want := readBytes(t, path), readBytes(t, filepath.Join(a.legacyDir, source)); !bytes.Equal(got, want) {
				t.Errorf("%s's copy = %q, want %q", name, got, want)
			}
		}
	})

	t.Run("a pane whose scrollback file is missing gets an entry naming none, and nothing is copied for it", func(t *testing.T) {
		a := newApplying(t)
		if missing := a.plan.Sessions[2].Transcript.Missing; !errors.Is(missing, fs.ErrNotExist) {
			t.Fatalf("tidepool-2's transcript missing for %v, want an absent file", missing)
		}
		a.apply(t)

		if files := dirFiles(t, state.ScrollbackDir(a.dir)); len(files) != 2 {
			t.Errorf("scrollback dir holds %v, want the two copies alone", slices.Sorted(maps.Keys(files)))
		}
	})

	t.Run("it makes the state dir and its scrollback dir 0700", func(t *testing.T) {
		a := newApplying(t)
		a.apply(t)

		for _, dir := range []string{a.dir, state.ScrollbackDir(a.dir)} {
			info, err := os.Stat(dir)
			if err != nil || info.Mode().Perm() != 0o700 {
				t.Errorf("%s: %v, %v; want a 0700 directory", dir, info, err)
			}
		}
	})

	t.Run("it moves each mapped key to its new name, each value byte for byte, and removes the rest", func(t *testing.T) {
		a := newApplying(t)
		before := rawHooks(t, a.hooksPath)
		a.apply(t)

		after := rawHooks(t, a.hooksPath)
		if got := slices.Sorted(maps.Keys(after)); !slices.Equal(got, []string{"lantern", "tidepool-2"}) {
			t.Fatalf("keys = %v, want the two new names alone", got)
		}
		for from, to := range map[string]string{"tokLan": "lantern", "tidepool · flaky test:0.1": "tidepool-2"} {
			if got, want := compact(t, after[to]), compact(t, before[from]); got != want {
				t.Errorf("%s = %s, want %s as %s held it", to, got, want, from)
			}
		}
	})

	t.Run("applying the plan again changes nothing", func(t *testing.T) {
		a := newApplying(t)
		a.apply(t)
		once, hooksFile := dirFiles(t, a.dir), readBytes(t, a.hooksPath)

		a.apply(t)
		if again := dirFiles(t, a.dir); !maps.EqualFunc(again, once, bytes.Equal) {
			t.Errorf("state dir after a second apply = %v, want %v", slices.Sorted(maps.Keys(again)), slices.Sorted(maps.Keys(once)))
		}
		hookstest.AssertHooksFileUnchanged(t, a.hooksPath, hooksFile, "changed by a second apply")
	})

	t.Run("an entry under a planned name that isn't migrate's is refused, writing nothing", func(t *testing.T) {
		a := newApplying(t)
		if _, err := sessionindex.Update(mkdir(t, a.dir), "cli", func(idx *sessionindex.Index) error {
			return idx.SetLabel("tidepool", "made since the plan")
		}); err != nil {
			t.Fatalf("seed an entry: %v", err)
		}
		before, hooksFile := dirFiles(t, a.dir), readBytes(t, a.hooksPath)

		if err := migrate.Apply(a.plan, a.dir, a.store); !errors.Is(err, sessionindex.ErrNameHeld) {
			t.Fatalf("Apply = %v, want ErrNameHeld", err)
		}
		if after := dirFiles(t, a.dir); !maps.EqualFunc(after, before, bytes.Equal) {
			t.Errorf("state dir after a refused apply = %v, want %v", slices.Sorted(maps.Keys(after)), slices.Sorted(maps.Keys(before)))
		}
		hookstest.AssertHooksFileUnchanged(t, a.hooksPath, hooksFile, "changed by a refused apply")
	})
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func mkdir(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("make %s: %v", dir, err)
	}
	return dir
}

// dirFiles is every regular file under dir, by its path relative to dir, with
// its content.
func dirFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err == nil {
			files[rel], err = os.ReadFile(path)
		}
		return err
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return files
}

// rawHooks is hooks.json's every key's value as the file holds it.
func rawHooks(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	var held map[string]json.RawMessage
	if err := json.Unmarshal(readBytes(t, path), &held); err != nil {
		t.Fatalf("decode hooks.json: %v", err)
	}
	return held
}

// compact strips the indentation the hooks store applies to the whole file,
// leaving a value's own content and member order to compare.
func compact(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatalf("compact %s: %v", raw, err)
	}
	return buf.String()
}
