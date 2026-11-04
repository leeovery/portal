// The hook-staleness cycle's own test vocabulary: the world a cycle reads —
// the backend's listing, a state directory holding sessions.json and the
// restoring flag, and a hooks.json — and the bodies a fixture stages. The seed
// keys themselves are named once in internal/hookstest, so no suite can
// re-point a name at a different key.
package hooksweep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// hooksBody renders a hooks.json body registering one on-resume entry per key,
// so a fixture that cares only about which keys are present says exactly that.
// With no keys it renders the empty registry: a file that exists and holds
// nothing.
func hooksBody(keys ...string) string {
	if len(keys) == 0 {
		return "{}"
	}
	entries := make([]string, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, fmt.Sprintf("  %q: {\"on-resume\": \"echo hi\"}", key))
	}
	return "{\n" + strings.Join(entries, ",\n") + "\n}"
}

// world is what one cycle judges against: the backend's sessions and a state
// directory of the test's own. during, when set, runs as the listing is
// taken, so a test can land a concurrent writer's mutation in the window
// between the cycle's snapshot and its deletion; lists counts the listings.
type world struct {
	backend *backendtest.Fake
	dir     string
	during  func()
	lists   int
}

// newWorld lists the named sessions, each answering, over an empty state
// directory.
func newWorld(t *testing.T, listed ...string) *world {
	t.Helper()
	w := &world{backend: backendtest.New(), dir: t.TempDir()}
	for _, name := range listed {
		w.backend.Add(backend.Session{Name: name})
	}
	return w
}

func (w *world) list(ctx context.Context) ([]backend.Session, error) {
	w.lists++
	if w.during != nil {
		w.during()
	}
	return w.backend.List(ctx)
}

func (w *world) sources() Sources {
	return Sources{List: w.list, StateDir: w.dir}
}

// index writes a sessions.json holding an entry for each of names.
func (w *world) index(t *testing.T, names ...string) {
	t.Helper()
	entries := make([]map[string]string, 0, len(names))
	for _, name := range names {
		entries = append(entries, map[string]string{"name": name, "dir": "/srv/" + name})
	}
	w.writeIndex(t, map[string]any{"version": sessionindex.SchemaVersion, "sessions": entries})
}

// writeIndex writes body as sessions.json, whatever it says.
func (w *world) writeIndex(t *testing.T, body any) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode sessions.json: %v", err)
	}
	w.writeIndexBytes(t, data)
}

func (w *world) writeIndexBytes(t *testing.T, data []byte) {
	t.Helper()
	if err := os.WriteFile(sessionindex.Path(w.dir), data, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}
}

func (w *world) raiseRestoring(t *testing.T) {
	t.Helper()
	if err := state.Restoring.Set(w.dir); err != nil {
		t.Fatalf("raise the restoring flag: %v", err)
	}
}

// makeUnreadableAt replaces whatever is at path with a directory no read can
// get through, so a file read there fails rather than reading as absent.
func makeUnreadableAt(t *testing.T, path string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root bypasses 0o000 directory permissions")
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatalf("clear %s: %v", path, err)
	}
	if err := os.Mkdir(path, 0o000); err != nil {
		t.Fatalf("stage an unreadable directory at %s: %v", path, err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o700) })
}

// keysOf names the entries a loaded store holds, for a failure message that
// says which keys survived a cycle rather than dumping the whole file.
func keysOf(snapshot hooks.Snapshot) []string {
	keys := make([]string, 0, len(snapshot))
	for key := range snapshot {
		keys = append(keys, key)
	}
	return keys
}

// runErr drives one cycle and keeps only its error, for the cases that assert
// on hooks.json or on the log rather than on what the cycle reported.
func runErr(t *testing.T, src Sources, store *hooks.Store) error {
	t.Helper()
	_, err := Run(t.Context(), src, store)
	return err
}

// assertStandDown pins the stand-down breadcrumb, at DEBUG whatever its
// reason, as the cycle's only record at or above DEBUG beside the degraded
// pre-read's own breadcrumb, and returns it for per-case attr assertions.
func assertStandDown(t *testing.T, sink *logtest.Sink, reason Reason) logtest.Record {
	t.Helper()

	for _, r := range sink.Records().AtOrAboveLevel(slog.LevelInfo) {
		t.Errorf("stand-down cycle emitted at %v: %+v", r.Level, r)
	}
	rec := sink.Records().WithMessage(standDownMsg).Only(t, "stand-down record")

	logtest.AssertRecord(t, rec, logtest.RecordWant{
		Level:     slog.LevelDebug,
		Msg:       standDownMsg,
		Component: "hooks",
		Op:        standDownMsg,
		Via:       "internal",
	})
	if got := rec.AttrString(t, "reason"); got != string(reason) {
		t.Errorf("reason = %q, want %q", got, reason)
	}
	return rec
}

// lockBound is the lowered sidecar timeout a contention fixture runs at: long
// enough to be a real wait, short enough that a suite can afford one.
const lockBound = 60 * time.Millisecond

// errZmxDown is the listing failure a backend is armed with when the case is
// about what the cycle does with a failed read, not about which read failed.
var errZmxDown = errors.New("zmx directory unreadable")

// dirListing is the directory's entry names, so a test can assert a cycle that
// had nothing to do added nothing to it.
func dirListing(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
