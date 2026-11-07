package migrate_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/migrate"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// migrating is a plan of two tmux sessions, one of two panes, ready to build
// into a fresh state dir and to rewrite hooks.json by: the tmux version's
// state dir; the state dir to build, not yet made; hooks.json's backup, its
// keys the plan's; and hooks.json, holding a key the backup doesn't.
type migrating struct {
	plan      migrate.Plan
	legacyDir string
	dir       string
	backup    string
	store     *hooks.Store
	hooksPath string
}

// migratingBackup holds a token key in the string form, one in the object form
// with an attribute the codec doesn't model, a key of no shape Portal wrote
// named as the first token's session will be, a pane address and a token of no
// saved pane.
var migratingBackup = `{
  "tokLan": {"on-resume": "claude --resume lantern"},
  "tokTd2": {"on-resume": {"command": "claude --resume tidepool", "resume": "eager", "nickname": "kept"}},
  "lantern": {"on-resume": "set by hand"},
  "tidepool/web · flaky test:0.0": {"on-resume": "claude --resume coords"},
  "tokGon": {"on-resume": "claude --resume gone"}
}`

func newMigrating(t *testing.T) migrating {
	t.Helper()
	sessions := []v1Session{
		session("lantern", pane(0, orchard).token("tokLan").scrollback("scrollback/pane-tokLan.bin")),
		session("tidepool/web · flaky test",
			pane(0, orchard).scrollback("scrollback/tidepool___flaky_test-0b1c2d3e__0.0.bin"),
			pane(1, orchard+"/web").token("tokTd2").scrollback("scrollback/gone.bin")),
	}
	legacy := legacyDir(t, sessions, map[string]string{
		"scrollback/pane-tokLan.bin":                         "\x1b[1mlantern\x1b[0m\r\n$ ",
		"scrollback/tidepool___flaky_test-0b1c2d3e__0.0.bin": "tidepool transcript\n",
	})
	backupStore, backup := hookstest.StageStore(t, hookstest.Staging{Seed: migratingBackup, SidecarAbsent: true})
	store, hooksPath := hookstest.StageStore(t, hookstest.Staging{Seed: `{"seed-live": {"on-resume": "live only"}}`})
	return migrating{
		plan:      planOf(t, legacy, hookKeys(t, backupStore), naming(t)),
		legacyDir: legacy,
		dir:       filepath.Join(t.TempDir(), "state"),
		backup:    backup,
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

func (m migrating) build(t *testing.T) {
	t.Helper()
	if err := migrate.Build(m.plan, m.dir); err != nil {
		t.Fatalf("Build: %v", err)
	}
}

func (m migrating) rewriteHooks(t *testing.T) {
	t.Helper()
	if err := migrate.RewriteHooks(m.plan, m.backup, m.store); err != nil {
		t.Fatalf("RewriteHooks: %v", err)
	}
}

func scrollbackFile(t *testing.T, name string) string {
	t.Helper()
	file, err := state.SessionScrollbackFile(name, savedAt, 0)
	if err != nil {
		t.Fatalf("SessionScrollbackFile: %v", err)
	}
	return file
}

func TestBuild(t *testing.T) {
	t.Run("it writes each session's entry owed a restore, recording no creation, resuming lazily", func(t *testing.T) {
		m := newMigrating(t)
		m.build(t)

		want := []sessionindex.Entry{
			{Name: "lantern", Dir: orchard, Cwd: orchard, ScrollbackFile: scrollbackFile(t, "lantern"), Owed: true, ResumeLazy: true},
			{Name: "tidepool-web · flaky test", Label: "tidepool/web · flaky test", Dir: orchard, Cwd: orchard, ScrollbackFile: scrollbackFile(t, "tidepool-web · flaky test"), Owed: true, ResumeLazy: true},
			{Name: "tidepool-web · flaky test-2", Label: "tidepool/web · flaky test-2", Dir: orchard, Cwd: orchard + "/web", Owed: true, ResumeLazy: true},
		}
		if got := readIndex(t, m.dir).Entries(); !slices.Equal(got, want) {
			t.Errorf("entries = %+v\nwant %+v", got, want)
		}
	})

	t.Run("it copies each scrollback file byte for byte to the file its entry names, which clean and Forget both own", func(t *testing.T) {
		m := newMigrating(t)
		m.build(t)

		idx := readIndex(t, m.dir)
		for name, source := range map[string]string{
			"lantern":                   "scrollback/pane-tokLan.bin",
			"tidepool-web · flaky test": "scrollback/tidepool___flaky_test-0b1c2d3e__0.0.bin",
		} {
			entry, _ := idx.Lookup(name)
			path, err := state.ScrollbackPath(m.dir, entry)
			if err != nil {
				t.Errorf("%s: Forget's ScrollbackPath refuses %q: %v", name, entry.ScrollbackFile, err)
				continue
			}
			if !state.IsSessionScrollbackFile(filepath.Base(path)) {
				t.Errorf("%s: the saver's clean doesn't own %s", name, path)
			}
			if got, want := readBytes(t, path), readBytes(t, filepath.Join(m.legacyDir, source)); !bytes.Equal(got, want) {
				t.Errorf("%s's copy = %q, want %q", name, got, want)
			}
		}
	})

	t.Run("a pane whose scrollback file is missing gets an entry naming none, and nothing is copied for it", func(t *testing.T) {
		m := newMigrating(t)
		if missing := m.plan.Sessions[2].Transcript.Missing; !errors.Is(missing, fs.ErrNotExist) {
			t.Fatalf("tidepool-web · flaky test-2's transcript missing for %v, want an absent file", missing)
		}
		m.build(t)

		if files := dirFiles(t, state.ScrollbackDir(m.dir)); len(files) != 2 {
			t.Errorf("scrollback dir holds %v, want the two copies alone", slices.Sorted(maps.Keys(files)))
		}
	})

	t.Run("it makes the state dir and its scrollback dir 0700", func(t *testing.T) {
		m := newMigrating(t)
		m.build(t)

		for _, dir := range []string{m.dir, state.ScrollbackDir(m.dir)} {
			info, err := os.Stat(dir)
			if err != nil || info.Mode().Perm() != 0o700 {
				t.Errorf("%s: %v, %v; want a 0700 directory", dir, info, err)
			}
		}
	})

	t.Run("building again changes nothing", func(t *testing.T) {
		m := newMigrating(t)
		m.build(t)
		once := dirFiles(t, m.dir)

		m.build(t)
		if again := dirFiles(t, m.dir); !maps.EqualFunc(again, once, bytes.Equal) {
			t.Errorf("state dir after a second build = %v, want %v", slices.Sorted(maps.Keys(again)), slices.Sorted(maps.Keys(once)))
		}
	})

	t.Run("a copy that fails writes no sessions.json", func(t *testing.T) {
		m := newMigrating(t)
		if err := os.Remove(filepath.Join(m.legacyDir, "scrollback", "tidepool___flaky_test-0b1c2d3e__0.0.bin")); err != nil {
			t.Fatalf("remove a planned scrollback file: %v", err)
		}

		if err := migrate.Build(m.plan, m.dir); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("Build = %v, want the copy's failure", err)
		}
		if _, err := os.Stat(filepath.Join(m.dir, "sessions.json")); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("sessions.json after a failed copy: %v, want none", err)
		}
	})

	t.Run("a scrollback dir swapped since planning for a link out of the state dir isn't read", func(t *testing.T) {
		m := newMigrating(t)
		scrollback, outside := filepath.Join(m.legacyDir, "scrollback"), filepath.Join(t.TempDir(), "scrollback")
		if err := os.Rename(scrollback, outside); err != nil {
			t.Fatalf("move the scrollback dir out: %v", err)
		}
		if err := os.Symlink(outside, scrollback); err != nil {
			t.Fatalf("link the scrollback dir out: %v", err)
		}

		if err := migrate.Build(m.plan, m.dir); err == nil {
			t.Fatal("Build read a scrollback file through a link out of the state dir")
		}
		if _, err := os.Stat(filepath.Join(m.dir, "sessions.json")); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("sessions.json after a refused copy: %v, want none", err)
		}
	})

	for what, sessions := range map[string][]v1Session{
		"no session":            nil,
		"only skipped sessions": {{Name: "windowless"}, sessionOf("paneless", window(0))},
	} {
		t.Run("a plan of "+what+" writes a sessions.json holding none", func(t *testing.T) {
			plan := planOf(t, legacyDir(t, sessions, nil), nil, naming(t))
			dir := filepath.Join(t.TempDir(), "state")

			if err := migrate.Build(plan, dir); err != nil {
				t.Fatalf("Build: %v", err)
			}
			idx, err := sessionindex.Decode(readBytes(t, sessionindex.Path(dir)))
			if err != nil || idx.Version != sessionindex.SchemaVersion || idx.Len() != 0 {
				t.Errorf("sessions.json = %+v, %v; want a version 2 index of no entries", idx, err)
			}
		})
	}

	t.Run("an entry under a planned name that isn't migrate's is refused, its sessions.json unchanged", func(t *testing.T) {
		m := newMigrating(t)
		if _, err := sessionindex.Update(mkdir(t, m.dir), "cli", func(idx *sessionindex.Index) error {
			return idx.SetLabel("tidepool-web · flaky test", "made since the plan")
		}); err != nil {
			t.Fatalf("seed an entry: %v", err)
		}
		before := readBytes(t, filepath.Join(m.dir, "sessions.json"))

		if err := migrate.Build(m.plan, m.dir); !errors.Is(err, sessionindex.ErrNameHeld) {
			t.Fatalf("Build = %v, want ErrNameHeld", err)
		}
		if after := readBytes(t, filepath.Join(m.dir, "sessions.json")); !bytes.Equal(after, before) {
			t.Errorf("sessions.json after a refused build:\n%s\nwant\n%s", after, before)
		}
	})
}

func TestRewriteHooks(t *testing.T) {
	t.Run("it writes the backup with each token key under its session's name, every value byte for byte, and nothing else", func(t *testing.T) {
		m := newMigrating(t)
		backup := rawHooks(t, m.backup)
		m.rewriteHooks(t)

		after := rawHooks(t, m.hooksPath)
		if got := slices.Sorted(maps.Keys(after)); !slices.Equal(got, []string{"lantern", "tidepool-web · flaky test-2"}) {
			t.Fatalf("keys = %v, want the two sessions' names alone", got)
		}
		for from, to := range map[string]string{"tokLan": "lantern", "tokTd2": "tidepool-web · flaky test-2"} {
			if got, want := compact(t, after[to]), compact(t, backup[from]); got != want {
				t.Errorf("%s = %s, want %s as %s held it", to, got, want, from)
			}
		}
	})

	t.Run("rewriting again writes the same file", func(t *testing.T) {
		m := newMigrating(t)
		m.rewriteHooks(t)
		once := readBytes(t, m.hooksPath)

		m.rewriteHooks(t)
		hookstest.AssertHooksFileUnchanged(t, m.hooksPath, once, "changed by a second rewrite")
	})

	t.Run("no backup writes nothing", func(t *testing.T) {
		m := newMigrating(t)
		before := readBytes(t, m.hooksPath)

		if err := migrate.RewriteHooks(m.plan, filepath.Join(t.TempDir(), "hooks.json.tmux"), m.store); err != nil {
			t.Fatalf("RewriteHooks = %v, want nothing to do", err)
		}
		hookstest.AssertHooksFileUnchanged(t, m.hooksPath, before, "written with no backup")
	})

	t.Run("a malformed backup fails, writing nothing", func(t *testing.T) {
		m := newMigrating(t)
		writeFile(t, m.backup, `{"tokLan": `)
		before := readBytes(t, m.hooksPath)

		if err := migrate.RewriteHooks(m.plan, m.backup, m.store); !errors.Is(err, hooks.ErrMalformed) {
			t.Fatalf("RewriteHooks = %v, want the backup refused as malformed", err)
		}
		hookstest.AssertHooksFileUnchanged(t, m.hooksPath, before, "written from a malformed backup")
	})

	for what, backup := range map[string]string{
		"holding a key the plan doesn't name": strings.Replace(migratingBackup, "{\n", "{\n  \"tokNew\": {\"on-resume\": \"set since the plan\"},\n", 1),
		"lacking a key the plan names":        strings.Replace(migratingBackup, ",\n  \"tokGon\": {\"on-resume\": \"claude --resume gone\"}", "", 1),
	} {
		t.Run("a backup "+what+" is refused, writing nothing, with one WARN", func(t *testing.T) {
			m := newMigrating(t)
			if backup == migratingBackup {
				t.Fatal("the backup is unchanged")
			}
			writeFile(t, m.backup, backup)
			before := readBytes(t, m.hooksPath)
			sink := logtest.Install(t)

			if err := migrate.RewriteHooks(m.plan, m.backup, m.store); !errors.Is(err, hooks.ErrUnaccountedKeys) {
				t.Fatalf("RewriteHooks = %v, want ErrUnaccountedKeys", err)
			}
			hookstest.AssertHooksFileUnchanged(t, m.hooksPath, before, "written for keys the plan didn't account for")
			sink.Records().AtOrAboveLevel(slog.LevelWarn).Matching("hooks", "rekey").Only(t, "rekey WARN")
		})
	}
}

func readIndex(t *testing.T, dir string) *sessionindex.Index {
	t.Helper()
	idx, err := sessionindex.Read(dir)
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	return &idx
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

// rawHooks is a hooks file's every key's value as the file holds it.
func rawHooks(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	var held map[string]json.RawMessage
	if err := json.Unmarshal(readBytes(t, path), &held); err != nil {
		t.Fatalf("decode %s: %v", path, err)
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
