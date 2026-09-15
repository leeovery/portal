// Staging helpers shared across the cmd test suites: how a test is set up and
// driven. Subject vocabulary — the domain values a suite asserts on and the
// fakes that answer with them — lives in a file named for its subject instead;
// the hook-key half lives in hookkey_vocabulary_test.go.
package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/tui"
	"github.com/leeovery/portal/internal/warning"
)

// lockBound is the lowered acquisition bound the lock-timeout suites drive the
// timeout through, so no case waits out the production figure.
const lockBound = 60 * time.Millisecond

// withHooksDeps installs deps as the package-level hooks seam for the rest of
// the test and registers the restore in the same breath. The seam outlives the
// test that sets it, so the install and its restore are written together here
// rather than left to each call site to pair.
func withHooksDeps(t *testing.T, deps HooksDeps) {
	t.Helper()
	hooksDeps = &deps
	t.Cleanup(func() { hooksDeps = nil })
}

// withoutHooksDeps leaves the hooks seam unset for the rest of the test, so a
// case whose subject is the production default states that precondition instead
// of inheriting it.
func withoutHooksDeps(t *testing.T) {
	t.Helper()
	hooksDeps = nil
	t.Cleanup(func() { hooksDeps = nil })
}

// The seams below are staged the same way, for the same reason. Each is a
// package-level pointer the production command body reads, so an install
// without its paired restore outlives the test that made it and answers for
// every later test in the package. Writing the install and the restore in one
// place is what makes the pair impossible to separate at a call site.

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
func withDoctorDeps(t *testing.T, deps DoctorDeps) {
	t.Helper()
	doctorDeps = &deps
	t.Cleanup(func() { doctorDeps = nil })
}

// withKillDeps installs deps as the package-level kill seam for the rest of the
// test and registers the restore in the same breath.
func withKillDeps(t *testing.T, deps KillDeps) {
	t.Helper()
	killDeps = &deps
	t.Cleanup(func() { killDeps = nil })
}

// withListDeps installs deps as the package-level list seam for the rest of the
// test and registers the restore in the same breath.
func withListDeps(t *testing.T, deps ListDeps) {
	t.Helper()
	listDeps = &deps
	t.Cleanup(func() { listDeps = nil })
}

// withCommitNowDeps installs deps as the package-level commit-now seam for the
// rest of the test and registers the restore in the same breath.
func withCommitNowDeps(t *testing.T, deps CommitNowDeps) {
	t.Helper()
	commitNowDeps = &deps
	t.Cleanup(func() { commitNowDeps = nil })
}

// withUninstallDeps installs deps as the package-level uninstall seam for the
// rest of the test and registers the restore in the same breath.
func withUninstallDeps(t *testing.T, deps UninstallDeps) {
	t.Helper()
	uninstallDeps = &deps
	t.Cleanup(func() { uninstallDeps = nil })
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

// runHookSet drives `hook set --on-resume command` with both streams captured,
// returning what the command wrote alongside its own error.
func runHookSet(t *testing.T, command string) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"hook", "set", "--on-resume", command})
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

func readHooksJSON(t *testing.T, path string) map[string]map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read hooks file: %v", err)
	}
	var data map[string]map[string]string
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
