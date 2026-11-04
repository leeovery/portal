package saver_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/bootid"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// world is what a poll works over: a state directory of the test's own, the
// Fake behind the race points, the saver's clock, and this machine's boot,
// which the one claim rule reads.
type world struct {
	t     *testing.T
	dir   string
	fake  *backendtest.Fake
	race  *racing
	clock *clock
	boot  time.Time
	hooks *hooks.Store
	saver *saver.Saver
}

func newWorld(t *testing.T) *world {
	t.Helper()
	boot, err := bootid.BootTime()
	if err != nil {
		t.Fatalf("read this machine's boot time: %v", err)
	}
	w := &world{t: t, dir: t.TempDir(), fake: backendtest.New(), boot: boot}
	w.clock = &clock{at: boot.Add(6 * time.Hour)}
	w.fake.Now = w.clock.now
	w.race = &racing{Fake: w.fake}
	w.restart()
	return w
}

// withHooks makes store the hooks store the saver forgets hooks from.
func (w *world) withHooks(store *hooks.Store) *world {
	w.hooks = store
	w.restart()
	return w
}

// restart replaces the saver with a fresh one, remembering nothing, as a
// daemon restarted is.
func (w *world) restart() {
	w.saver = saver.New(saver.Config{Backend: w.race, StateDir: w.dir, Hooks: w.hooks, Now: w.clock.now})
}

// thisBoot is a creation n minutes into the hour after this boot began: past
// the claim rule's leeway, and before the saver's clock.
func (w *world) thisBoot(n int) time.Time {
	return w.boot.Add(time.Hour + time.Duration(n)*time.Minute)
}

// beforeBoot is a creation saved a day before this boot.
func (w *world) beforeBoot() time.Time { return w.boot.Add(-24 * time.Hour) }

// live holds an answering session named name, created n minutes into this
// boot, and returns it as the backend lists it.
func (w *world) live(name string, n int) backend.Session {
	s := backend.Session{Name: name, Answered: true, Dir: "/srv/" + name, PID: 4000 + n, Created: w.thisBoot(n)}
	w.fake.Add(s)
	return s
}

// held is the session the Fake holds as name.
func (w *world) held(name string) backend.Session {
	w.t.Helper()
	s, ok := w.fake.Held(name)
	if !ok {
		w.t.Fatalf("the Fake holds no session %s", name)
	}
	return s
}

// entryOf is the index entry recording s's creation.
func entryOf(s backend.Session) sessionindex.Entry {
	return sessionindex.Entry{Name: s.Name, CreatedAt: s.Created, PID: s.PID}
}

// seed writes sessions.json holding entries.
func (w *world) seed(entries ...sessionindex.Entry) {
	w.t.Helper()
	data, err := json.Marshal(map[string]any{"version": sessionindex.SchemaVersion, "sessions": entries})
	if err != nil {
		w.t.Fatalf("encode sessions.json: %v", err)
	}
	if err := os.WriteFile(sessionindex.Path(w.dir), data, 0o600); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) entries() []sessionindex.Entry {
	w.t.Helper()
	idx, err := sessionindex.Read(w.dir)
	if err != nil {
		w.t.Fatalf("read sessions.json: %v", err)
	}
	return idx.Entries()
}

func (w *world) entry(name string) (sessionindex.Entry, bool) {
	w.t.Helper()
	i := slices.IndexFunc(w.entries(), func(e sessionindex.Entry) bool { return e.Name == name })
	if i < 0 {
		return sessionindex.Entry{}, false
	}
	return w.entries()[i], true
}

func (w *world) names() []string {
	w.t.Helper()
	var names []string
	for _, e := range w.entries() {
		names = append(names, e.Name)
	}
	slices.Sort(names)
	return names
}

// poll runs a poll that must succeed.
func (w *world) poll() {
	w.t.Helper()
	if err := w.saver.Poll(w.t.Context()); err != nil {
		w.t.Fatalf("Poll: %v", err)
	}
}

// after moves the saver's clock on by d, then polls.
func (w *world) after(d time.Duration) {
	w.t.Helper()
	w.clock.at = w.clock.at.Add(d)
	w.poll()
}

// update writes the index as another writer of it would, beside the saver.
func (w *world) update(fn func(*sessionindex.Index)) {
	w.t.Helper()
	if _, err := sessionindex.Update(w.dir, "cli", func(idx *sessionindex.Index) error {
		fn(idx)
		return nil
	}); err != nil {
		w.t.Fatalf("Update: %v", err)
	}
}

// claim claims s's entry as a mint, rename or hook set does.
func (w *world) claim(s backend.Session, dir string) {
	w.t.Helper()
	w.update(func(idx *sessionindex.Index) {
		idx.Claim(s.Name, dir, s.Created, s.PID, state.ClaimRule(w.dir))
	})
}

// scrollbackFile is the file the index names for s's creation, relative to the
// state directory.
func scrollbackFile(t *testing.T, s backend.Session) string {
	t.Helper()
	file, err := state.SessionScrollbackFile(s.Name, s.Created, s.PID)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

// read is the content of the file at rel in the state directory, and whether
// there is one.
func (w *world) read(rel string) (string, bool) {
	w.t.Helper()
	data, err := os.ReadFile(filepath.Join(w.dir, filepath.FromSlash(rel)))
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		w.t.Fatal(err)
	}
	return string(data), true
}

// write puts content at rel in the state directory.
func (w *world) write(rel, content string) {
	w.t.Helper()
	path := filepath.Join(w.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) raise(flag state.SessionFlag, name string) {
	w.t.Helper()
	if err := flag.Set(w.dir, name); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) lower(flag state.SessionFlag, name string) {
	w.t.Helper()
	if _, err := flag.Clear(w.dir, name); err != nil {
		w.t.Fatal(err)
	}
}

// calls counts the calls of op the Fake has taken since mark.
func (w *world) calls(op backendtest.Op, mark int) []backendtest.Call {
	var out []backendtest.Call
	for _, c := range w.fake.Calls()[mark:] {
		if c.Op == op {
			out = append(out, c)
		}
	}
	return out
}

// mark is where the Fake's call log stands now.
func (w *world) mark() int { return len(w.fake.Calls()) }

type clock struct{ at time.Time }

func (c *clock) now() time.Time { return c.at }

// racing is the Fake, running what a test stages at the exact points a poll
// reaches, once each: as List has answered, and as History has. onSession
// sees every Session call's context, as it is made.
type racing struct {
	*backendtest.Fake
	afterList    func()
	afterHistory func(name string)
	onSession    func(ctx context.Context)
}

func (r *racing) Session(ctx context.Context, name string) (backend.Session, error) {
	if r.onSession != nil {
		r.onSession(ctx)
	}
	return r.Fake.Session(ctx, name)
}

func (r *racing) List(ctx context.Context) ([]backend.Session, error) {
	sessions, err := r.Fake.List(ctx)
	if run := r.afterList; run != nil {
		r.afterList = nil
		run()
	}
	return sessions, err
}

func (r *racing) History(ctx context.Context, name string, rows int) ([]byte, error) {
	vt, err := r.Fake.History(ctx, name, rows)
	if run := r.afterHistory; run != nil {
		r.afterHistory = nil
		run(name)
	}
	return vt, err
}
