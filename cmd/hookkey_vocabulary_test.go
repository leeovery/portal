// The hook-key subject vocabulary shared across the cmd test suites: the
// hooks.json bodies the seed keys are written into, the enumeration rows they
// arrive in, and the seam fakes that answer with them. The seed keys themselves
// — and the stale-beside-live body — are named once in internal/hookstest, so
// no suite can re-point a name at a different key. Staging — how a test is set
// up and driven, including the store and file fixtures these bodies are staged
// into — lives in testhelpers_test.go.
package cmd

import (
	"fmt"
	"strings"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hooksweep"
	"github.com/leeovery/portal/internal/tmux"
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

// tokenRows models the enumeration's answer for stamped panes. The location
// half is display-only, so it fabricates a distinct one per row rather than
// asserting on it.
func tokenRows(tokens ...string) []tmux.PaneHookRow {
	rows := make([]tmux.PaneHookRow, 0, len(tokens))
	for i, token := range tokens {
		rows = append(rows, tmux.PaneHookRow{Token: token, Location: fmt.Sprintf("stamped%d:0.0", i)})
	}
	return rows
}

// restoringOption models the @portal-restoring read for the sweep's seam fakes:
// absent by default, so a fake that says nothing about a restore is not
// restoring.
func restoringOption(restoring bool, readErr error) (string, bool, error) {
	if readErr != nil {
		return "", false, readErr
	}
	if !restoring {
		return "", false, nil
	}
	return "1", true, nil
}

// recordingPaneHookLister answers the pane-token enumeration with a fixed set
// of rows (or a fixed failure) and records every read it is asked for, so a
// test can assert on the read count as well as the rows.
type recordingPaneHookLister struct {
	rows  []tmux.PaneHookRow
	err   error
	calls int
}

func (r *recordingPaneHookLister) ListAllPaneHookKeys() ([]tmux.PaneHookRow, error) {
	r.calls++
	return r.rows, r.err
}

var _ hooksweep.PaneHookLister = (*recordingPaneHookLister)(nil)

// stubStaleSweepReader answers the sweep's two seams with fixed values: a fixed
// row set (or failure) for the pane-token enumeration, and a fixed
// @portal-restoring read. It counts the enumeration reads, so a test can assert
// the sweep stood down before enumerating as well as what it read.
type stubStaleSweepReader struct {
	rows         []tmux.PaneHookRow
	err          error
	restoring    bool
	restoringErr error
	calls        int
}

func (s *stubStaleSweepReader) ListAllPaneHookKeys() ([]tmux.PaneHookRow, error) {
	s.calls++
	return s.rows, s.err
}

func (s *stubStaleSweepReader) TryGetServerOption(string) (string, bool, error) {
	return restoringOption(s.restoring, s.restoringErr)
}

var _ hooksweep.Reader = (*stubStaleSweepReader)(nil)

// sweepErr drives one cycle and keeps only its error, for the cases that assert
// on hooks.json or on the log rather than on what the cycle reported.
func sweepErr(reader hooksweep.Reader, store *hooks.Store) error {
	_, err := hooksweep.Run(reader, store)
	return err
}

func keysOf(m hooks.Snapshot) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
