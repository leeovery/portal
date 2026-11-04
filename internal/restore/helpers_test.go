package restore_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// exe is the hydrate helper unit tests restore with. The Fake backend runs
// nothing, so it need not exist.
const exe = "/opt/portal/bin/portal"

// saved is the creation the entries a test saves record, before any boot a
// test makes.
var saved = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

// world is a restore over a Fake backend and a state directory of the test's
// own, its log captured.
type world struct {
	dir     string
	fake    *backendtest.Fake
	witness *flagWitness
	sink    *logtest.Sink
	r       *restore.Restorer
}

func newWorld(t *testing.T, live ...backend.Session) *world {
	t.Helper()
	w := &world{dir: t.TempDir(), fake: backendtest.New(live...)}
	w.witness = &flagWitness{Backend: w.fake, dir: w.dir, raised: map[string]bool{}}
	logger, sink := logtest.NewCaptureLogger(t)
	r, err := restore.New(restore.Config{Backend: w.witness, StateDir: w.dir, Exe: exe, Logger: logger})
	if err != nil {
		t.Fatalf("restore.New: %v", err)
	}
	w.sink, w.r = sink, r
	return w
}

// flagWitness records, as each Create is made, whether the session's
// hydrating flag was raised.
type flagWitness struct {
	backend.Backend
	dir    string
	raised map[string]bool
}

func (f *flagWitness) Create(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
	up, err := state.Hydrating.Exists(f.dir, name)
	f.raised[name] = up && err == nil
	return f.Backend.Create(ctx, name, dir, command)
}

// entry is a saved session's entry that starts in a directory of the test's
// own.
func entry(t *testing.T, name string) sessionindex.Entry {
	t.Helper()
	return sessionindex.Entry{Name: name, Label: name + " label", Dir: t.TempDir(), CreatedAt: saved, PID: 42}
}

// save writes sessions.json holding entries, as a save before the reboot left
// it.
func (w *world) save(t *testing.T, entries ...sessionindex.Entry) {
	t.Helper()
	doc := struct {
		Version  int                  `json:"version"`
		Sessions []sessionindex.Entry `json:"sessions"`
	}{sessionindex.SchemaVersion, entries}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	w.writeIndex(t, string(data))
}

func (w *world) writeIndex(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(sessionindex.Path(w.dir), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// saveTranscript gives e a saved transcript, written where its entry names it.
func (w *world) saveTranscript(t *testing.T, e sessionindex.Entry) (sessionindex.Entry, string) {
	t.Helper()
	file, err := state.SessionScrollbackFile(e.Name, e.CreatedAt, e.PID)
	if err != nil {
		t.Fatal(err)
	}
	e.ScrollbackFile = file
	path := filepath.Join(w.dir, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("$ make\r\nok\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return e, path
}

func (w *world) lookup(t *testing.T, name string) (sessionindex.Entry, bool) {
	t.Helper()
	idx, err := sessionindex.Read(w.dir)
	if err != nil {
		t.Fatalf("read the index: %v", err)
	}
	return idx.Lookup(name)
}

func (w *world) hydrating(t *testing.T, name string) bool {
	t.Helper()
	up, err := state.Hydrating.Exists(w.dir, name)
	if err != nil {
		t.Fatalf("look for %s's hydrating flag: %v", name, err)
	}
	return up
}

// created is the names Create was asked to make, in order.
func (w *world) created() []string {
	var names []string
	for _, c := range w.fake.CallsTo(backendtest.OpCreate) {
		names = append(names, c.Name)
	}
	return names
}

// onlyCreate is the one Create call restore made.
func (w *world) onlyCreate(t *testing.T) backendtest.Call {
	t.Helper()
	calls := w.fake.CallsTo(backendtest.OpCreate)
	if len(calls) != 1 {
		t.Fatalf("Create calls = %+v, want one", calls)
	}
	return calls[0]
}
