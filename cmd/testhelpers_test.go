// Staging helpers shared across the cmd test suites: how a test is set up and
// driven. Subject vocabulary — the domain values a suite asserts on and the
// fakes that answer with them — lives in a file named for its subject instead;
// the hook-key half lives in hookkey_vocabulary_test.go.
package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/leeovery/portal/internal/warning"
)

// lockBound is the lowered acquisition bound the lock-timeout suites drive the
// timeout through, so no case waits out the production figure.
const lockBound = 60 * time.Millisecond

// Each seam below is a package-level pointer the production command body
// reads, so an install without its paired restore outlives the test that made
// it and answers for every later test in the package. Writing the install and
// the restore in one place is what makes the pair impossible to separate at a
// call site.

// withBootstrapDeps installs deps as the package-level bootstrap seam for the
// rest of the test and registers the restore in the same breath.
func withBootstrapDeps(t *testing.T, deps BootstrapDeps) {
	t.Helper()
	bootstrapDeps = &deps
	t.Cleanup(func() { bootstrapDeps = nil })
}

// withOpenDeps installs deps as the package-level open seam for the rest of the
// test and registers the restore in the same breath.
func withOpenDeps(t *testing.T, deps OpenDeps) {
	t.Helper()
	openDeps = &deps
	t.Cleanup(func() { openDeps = nil })
}

// withOpenBurstDeps installs deps as the package-level open-burst seam for the
// rest of the test and registers the restore in the same breath.
func withOpenBurstDeps(t *testing.T, deps OpenBurstDeps) {
	t.Helper()
	openBurstDeps = &deps
	t.Cleanup(func() { openBurstDeps = nil })
}

// withDoctorDeps installs deps as the package-level doctor seam for the rest of
// the test and registers the restore in the same breath.
// Doctor reaches tmux only to detect the host terminal, so its tests answer
// that detection as a machine running no tmux server would. The returned fake
// records every tmux command doctor sent.
func withDoctorDeps(t *testing.T, deps DoctorDeps) *commandertest.Scripted {
	t.Helper()
	doctorDeps = &deps
	t.Cleanup(func() { doctorDeps = nil })
	noServer := commandertest.Quiet().AllowingUnmatched("", errors.New("no server running"))
	withFuncSeam(t, &defaultTmuxClient, func() *tmux.Client { return tmux.NewClient(noServer) })
	return noServer
}

// withListDeps installs deps as the package-level list seam for the rest of the
// test and registers the restore in the same breath.
func withListDeps(t *testing.T, deps ListDeps) {
	t.Helper()
	listDeps = &deps
	t.Cleanup(func() { listDeps = nil })
}

// requireStagedBackend refuses a nil Backend at the staging site, where it
// would otherwise surface later as a nil-interface panic in a command body.
func requireStagedBackend(t harnesstest.TestingT, deps BackendDeps) {
	t.Helper()
	if deps.Backend == nil {
		t.Fatalf("withBackendDeps: BackendDeps.Backend is nil; stage a backend, such as backendtest.New()")
	}
}

// withBackendDeps installs deps as the package-level backend seam for the rest
// of the test and registers the restore in the same breath.
func withBackendDeps(t *testing.T, deps BackendDeps) {
	t.Helper()
	requireStagedBackend(t, deps)
	backendDeps = &deps
	t.Cleanup(func() { backendDeps = nil })
}

// withCommitNowDeps installs deps as the package-level commit-now seam for the
// rest of the test and registers the restore in the same breath.
func withCommitNowDeps(t *testing.T, deps CommitNowDeps) {
	t.Helper()
	commitNowDeps = &deps
	t.Cleanup(func() { commitNowDeps = nil })
}

// withFuncSeam installs replacement as the package-level function seam at seam
// for the rest of the test and registers the restore in the same breath. The
// family it serves is staged through one helper rather than one per seam because
// the type parameter already carries what a per-seam helper would spell out.
//
// A function seam's production default is a real value, not nil, so the restore
// puts back what the install captured; restoring the zero value would leave a
// nil func for every later test in the package to call.
func withFuncSeam[F any](t *testing.T, seam *F, replacement F) {
	t.Helper()
	original := *seam
	*seam = replacement
	t.Cleanup(func() { *seam = original })
}

// readFileBytes returns nil on ENOENT, so callers can distinguish "file absent"
// from "file empty". The rule itself lives in hookstest, which is already this
// package's home for the hooks.json assertions the read feeds; it is
// path-generic, so projects.json reads through it too.
func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	return hookstest.HooksFileBytes(t, path)
}

// TestReadFileBytes pins the read rule the hooks/projects assertion pairs both
// depend on: exact bytes when the file is there, nil when it is not, whichever
// of the two config files is named.
func TestReadFileBytes(t *testing.T) {
	t.Run("it reads projects.json by the same rule", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "projects.json")

		if got := readFileBytes(t, path); got != nil {
			t.Errorf("readFileBytes of an absent projects.json = %q, want nil", got)
		}

		body := []byte(`{"projects":[{"path":"/tmp/proj","name":"proj0"}]}`)
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatalf("seed projects.json: %v", err)
		}
		if got := readFileBytes(t, path); !bytes.Equal(got, body) {
			t.Errorf("readFileBytes = %s, want %s", got, body)
		}
	})
}

// hooksFileInTempDir points PORTAL_HOOKS_FILE at a hooks.json holding body,
// staged through hookstest.StageStore in a fresh temp directory with its
// sidecar beside it. A nil body stages no file, leaving the absent hooks.json a
// first run starts from. It adds the env pointing to the shared stager rather
// than staging anything itself, so a case that hands a store straight to a seam
// calls hookstest.StageStore directly and both describe the same file the same
// way. A case whose subject is the sidecar's absence asks the stager for it.
//
// The directory is returned alongside the file because several callers stage
// siblings of the hooks file in it.
func hooksFileInTempDir(t *testing.T, body map[string]map[string]string) (dir, hooksFile string) {
	t.Helper()
	dir = t.TempDir()
	_, hooksFile = hookstest.StageStore(t, hookstest.Staging{Dir: dir, Body: body})
	t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
	return dir, hooksFile
}

// runHookSet drives `hook set --on-resume command [extra…]` with both streams
// captured, returning what the command wrote alongside its own error.
func runHookSet(t *testing.T, command string, extra ...string) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(append([]string{"hook", "set", "--on-resume", command}, extra...))
	err := rootCmd.Execute()
	return buf.String(), err
}

// runHookRm drives `hook rm --on-resume [extra…]` with both streams captured,
// returning what the command wrote alongside its own error.
func runHookRm(t *testing.T, extra ...string) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(append([]string{"hook", "rm", "--on-resume"}, extra...))
	err := rootCmd.Execute()
	return buf.String(), err
}

// runHookList drives `hook list`, failing the test on a non-zero exit and
// returning what the command wrote to stdout.
func runHookList(t *testing.T) string {
	t.Helper()
	buf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(buf)
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"hook", "list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return buf.String()
}

// readHooksJSON decodes hooks.json through the store's own types, so an entry
// of either stored shape reads as the registration it holds.
func readHooksJSON(t *testing.T, path string) hooks.Snapshot {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read hooks file: %v", err)
	}
	var data hooks.Snapshot
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("failed to unmarshal hooks JSON: %v", err)
	}
	return data
}

// assertHooksFileUnchanged names the shared byte-identity assertion in this
// package's own vocabulary, so a cmd test reads the same as its neighbours.
func assertHooksFileUnchanged(t *testing.T, path string, before []byte, context ...string) {
	t.Helper()
	hookstest.AssertHooksFileUnchanged(t, path, before, context...)
}

// landingShape is a pickerLanding reduced to values a test can compare: its
// filter text, the search form's term, whether a search form reached it at all,
// and whether that form carried a decision closure. A pickerLanding compares by
// search-form pointer identity, which says nothing about what the form carries.
type landingShape struct {
	filter  string
	term    string
	search  bool
	decided bool
}

func shapeOfLanding(l pickerLanding) landingShape {
	shape := landingShape{filter: l.filter, search: l.search != nil}
	if l.search != nil {
		shape.term = l.search.Term
		shape.decided = l.search.Decide != nil
	}
	return shape
}

// driveLoadingGates satisfies both of the picker's loading gates and, when the
// model answers by dispatching its search decision rather than dismissing,
// runs that command and delivers its answer — the step a running Bubble Tea
// program takes and a hand-driven model otherwise skips.
func driveLoadingGates(t *testing.T, model tea.Model, warnings []warning.Warning) tea.Model {
	t.Helper()

	var dispatched tea.Cmd
	for _, msg := range []tea.Msg{tui.LoadingMinElapsedMsg{}, tui.BootstrapCompleteMsg{Warnings: warnings}} {
		var cmd tea.Cmd
		model, cmd = model.Update(msg)
		m, ok := model.(tui.Model)
		if !ok {
			t.Fatalf("model type = %T, want tui.Model", model)
		}
		// A command returned with the page still loading is the decision going
		// out; one returned alongside the dismissal belongs to the picker.
		if cmd != nil && m.ActivePage() == tui.PageLoading {
			dispatched = cmd
		}
	}
	if dispatched == nil {
		return model
	}
	answer := dispatched()
	if answer == nil {
		return model
	}
	model, _ = model.Update(answer)
	return model
}
