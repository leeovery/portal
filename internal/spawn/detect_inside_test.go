package spawn

import (
	"errors"
	"testing"
)

// fakeClientLister is a map-free fake clientLister: it returns a fabricated
// ClientActivity slice (or an error) and records the sessions it was asked
// about so a test can assert the session passthrough.
type fakeClientLister struct {
	clients []ClientActivity
	err     error
	calls   []string
}

func (f *fakeClientLister) ListClients(session string) ([]ClientActivity, error) {
	f.calls = append(f.calls, session)
	return f.clients, f.err
}

// ghosttyProc/terminalProc are single-hop ancestries that resolve to a .app.
var (
	ghosttyCommand  = "/Applications/Ghostty.app/Contents/MacOS/ghostty"
	ghosttyAppPath  = "/Applications/Ghostty.app"
	terminalCommand = "/Applications/Terminal.app/Contents/MacOS/Terminal"
	terminalAppPath = "/Applications/Terminal.app"
)

func localWalkSeams() (*fakeWalker, *fakeReader) {
	walker := &fakeWalker{procs: map[int]fakeProc{
		501: {ppid: 1, command: ghosttyCommand},
		502: {ppid: 1, command: terminalCommand},
		// A remote/mosh client walks to NULL.
		601: {ppid: 1, command: "mosh-server"},
		602: {ppid: 1, command: "mosh-server"},
	}}
	reader := &fakeReader{bundles: map[string]fakeBundle{
		ghosttyAppPath:  {bundleID: "com.mitchellh.ghostty", name: "Ghostty"},
		terminalAppPath: {bundleID: "com.apple.Terminal", name: "Terminal"},
	}}
	return walker, reader
}

func TestDetectInsideTmux(t *testing.T) {
	t.Run("it returns NULL when every client is remote or mosh", func(t *testing.T) {
		lister := &fakeClientLister{clients: []ClientActivity{
			{PID: 601, Activity: 100},
			{PID: 602, Activity: 200},
		}}
		walker, reader := localWalkSeams()

		got, err := detectInsideTmux("dev", lister, walker, reader)
		if err != nil {
			t.Fatalf("detectInsideTmux returned error: %v, want nil", err)
		}
		if !got.IsNull() {
			t.Errorf("identity = %+v, want NULL (no host-local terminal)", got)
		}
		if len(lister.calls) != 1 || lister.calls[0] != "dev" {
			t.Errorf("lister calls = %v, want exactly [dev]", lister.calls)
		}
	})

	t.Run("it returns the single local client's identity without a tiebreak", func(t *testing.T) {
		lister := &fakeClientLister{clients: []ClientActivity{
			{PID: 501, Activity: 0}, // zero activity must not matter for a sole local client
		}}
		walker, reader := localWalkSeams()

		got, err := detectInsideTmux("dev", lister, walker, reader)
		if err != nil {
			t.Fatalf("detectInsideTmux returned error: %v, want nil", err)
		}
		if got.BundleID != "com.mitchellh.ghostty" {
			t.Errorf("BundleID = %q, want %q", got.BundleID, "com.mitchellh.ghostty")
		}
		if got.Name != "Ghostty" {
			t.Errorf("Name = %q, want %q", got.Name, "Ghostty")
		}
	})

	t.Run("it picks the highest-client_activity local client among 2+ locals", func(t *testing.T) {
		// Higher-activity client listed SECOND so a passing test proves a
		// max-by-activity comparison, not merely last-wins.
		lister := &fakeClientLister{clients: []ClientActivity{
			{PID: 501, Activity: 100}, // Ghostty
			{PID: 502, Activity: 200}, // Terminal — higher
		}}
		walker, reader := localWalkSeams()

		got, err := detectInsideTmux("dev", lister, walker, reader)
		if err != nil {
			t.Fatalf("detectInsideTmux returned error: %v, want nil", err)
		}
		if got.BundleID != "com.apple.Terminal" {
			t.Errorf("BundleID = %q, want the higher-activity %q", got.BundleID, "com.apple.Terminal")
		}
	})

	t.Run("it picks the highest activity when the higher-activity client is listed first", func(t *testing.T) {
		lister := &fakeClientLister{clients: []ClientActivity{
			{PID: 502, Activity: 200}, // Terminal — higher, listed first
			{PID: 501, Activity: 100}, // Ghostty
		}}
		walker, reader := localWalkSeams()

		got, err := detectInsideTmux("dev", lister, walker, reader)
		if err != nil {
			t.Fatalf("detectInsideTmux returned error: %v, want nil", err)
		}
		if got.BundleID != "com.apple.Terminal" {
			t.Errorf("BundleID = %q, want the higher-activity %q", got.BundleID, "com.apple.Terminal")
		}
	})

	t.Run("it prefers the first local client on an exact activity tie", func(t *testing.T) {
		lister := &fakeClientLister{clients: []ClientActivity{
			{PID: 501, Activity: 150}, // Ghostty — first
			{PID: 502, Activity: 150}, // Terminal — equal activity
		}}
		walker, reader := localWalkSeams()

		got, err := detectInsideTmux("dev", lister, walker, reader)
		if err != nil {
			t.Fatalf("detectInsideTmux returned error: %v, want nil", err)
		}
		if got.BundleID != "com.mitchellh.ghostty" {
			t.Errorf("BundleID = %q, want the first-listed %q on an exact tie", got.BundleID, "com.mitchellh.ghostty")
		}
	})

	t.Run("it returns NULL when the most-active client is remote even with a local bystander", func(t *testing.T) {
		// The remote client is the most-active — it is the triggering client.
		// Under winner-first locality gating it is selected and walked, and its
		// ancestry resolves a clean NULL, so detection is an honest no-op even
		// though a lower-activity local client is also attached to the session.
		// This is the reported bug's shape: before the fix the local bystander
		// was (wrongly) driven, spawning windows on a machine the triggering
		// user is not at.
		lister := &fakeClientLister{clients: []ClientActivity{
			{PID: 601, Activity: 9999}, // remote/mosh, most active — the trigger
			{PID: 501, Activity: 1},    // local Ghostty, idle bystander
		}}
		walker, reader := localWalkSeams()

		got, err := detectInsideTmux("dev", lister, walker, reader)
		if err != nil {
			t.Fatalf("detectInsideTmux returned error: %v, want nil (clean NULL no-op)", err)
		}
		if !got.IsNull() {
			t.Errorf("identity = %+v, want NULL — the most-active client is remote", got)
		}
	})

	t.Run("it returns a transient error when list-clients fails", func(t *testing.T) {
		listFailure := errors.New("list-clients: server not found")
		lister := &fakeClientLister{err: listFailure}
		walker, reader := localWalkSeams()

		got, err := detectInsideTmux("dev", lister, walker, reader)
		if err == nil {
			t.Fatalf("detectInsideTmux returned nil error, want a transient error")
		}
		if !errors.Is(err, ErrDetectTransient) {
			t.Errorf("errors.Is(err, ErrDetectTransient) = false, want true; err = %v", err)
		}
		if !errors.Is(err, listFailure) {
			t.Errorf("underlying list-clients failure not preserved in the chain; err = %v", err)
		}
		if !got.IsNull() {
			t.Errorf("identity = %+v, want NULL alongside the transient error", got)
		}
	})

	t.Run("it returns a transient error when a walk fails and nothing local resolves", func(t *testing.T) {
		psFailure := errors.New("ps: operation not permitted")
		lister := &fakeClientLister{clients: []ClientActivity{
			{PID: 501, Activity: 100},
		}}
		walker := &fakeWalker{procs: map[int]fakeProc{
			501: {err: psFailure},
		}}
		reader := &fakeReader{bundles: map[string]fakeBundle{}}

		got, err := detectInsideTmux("dev", lister, walker, reader)
		if err == nil {
			t.Fatalf("detectInsideTmux returned nil error, want a transient error")
		}
		if !errors.Is(err, ErrDetectTransient) {
			t.Errorf("errors.Is(err, ErrDetectTransient) = false, want true; err = %v", err)
		}
		if !errors.Is(err, psFailure) {
			t.Errorf("underlying ps failure not preserved in the chain; err = %v", err)
		}
		if !got.IsNull() {
			t.Errorf("identity = %+v, want NULL alongside the transient error", got)
		}
	})

	t.Run("it fails safe to NULL when the most-active winner walk transiently fails", func(t *testing.T) {
		// Under walk-only-the-winner the flaky high-activity client IS the
		// winner, so a transient walk failure on it fails safe to NULL + an
		// ErrDetectTransient-wrapped error (which Detect() folds to a spawn
		// WARN) rather than falling back to the resolvable lower-activity local.
		// This is the deliberately-dropped walk-resilience property — never
		// spawn on uncertainty — locked in on purpose.
		psFailure := errors.New("ps: operation not permitted")
		lister := &fakeClientLister{clients: []ClientActivity{
			{PID: 601, Activity: 100}, // most active — winner — transient walk failure
			{PID: 501, Activity: 50},  // local Ghostty resolves, but is never walked
		}}
		walker := &fakeWalker{procs: map[int]fakeProc{
			601: {err: psFailure},
			501: {ppid: 1, command: ghosttyCommand},
		}}
		reader := &fakeReader{bundles: map[string]fakeBundle{
			ghosttyAppPath: {bundleID: "com.mitchellh.ghostty", name: "Ghostty"},
		}}

		got, err := detectInsideTmux("dev", lister, walker, reader)
		if err == nil {
			t.Fatalf("detectInsideTmux returned nil error, want an ErrDetectTransient failure")
		}
		if !errors.Is(err, ErrDetectTransient) {
			t.Errorf("errors.Is(err, ErrDetectTransient) = false, want true; err = %v", err)
		}
		if !errors.Is(err, psFailure) {
			t.Errorf("underlying ps failure not preserved in the chain; err = %v", err)
		}
		if !got.IsNull() {
			t.Errorf("identity = %+v, want NULL alongside the transient error", got)
		}
	})

	t.Run("it returns clean NULL for zero clients", func(t *testing.T) {
		lister := &fakeClientLister{clients: nil}
		walker, reader := localWalkSeams()

		got, err := detectInsideTmux("dev", lister, walker, reader)
		if err != nil {
			t.Fatalf("detectInsideTmux returned error: %v, want nil", err)
		}
		if !got.IsNull() {
			t.Errorf("identity = %+v, want NULL for a session with no clients", got)
		}
	})
}
