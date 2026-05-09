//go:build integration

// Shared daemon-tick test helper for the bootstrap integration suite.
//
// Multiple integration tests in this package need to drive a single
// daemon-equivalent capture-and-commit pass against a live tmux server
// without coupling to cmd/state_daemon.go (the production daemon path
// the spec § Out of Scope explicitly forbids modifying for these
// regressions). This file holds the single source of truth for that
// simulation; future state.Commit signature changes touch one helper.
//
// The helper mirrors the daemon's per-tick body in
// cmd/state_daemon.go captureAndCommit:
//   - state.ListSkeletonMarkers   — read the skip-save set
//   - state.CaptureStructure      — walk live sessions/windows/panes
//   - per-pane skip-save guard    — when WithSkipGuard() is set
//   - state.CaptureAndHashPane    — production-shape scrollback bytes
//     (or empty bytes when withEmptyScrollback() is set, used by the
//     reboot round-trip which overwrites the byte-compared file with
//     a deterministic ANSI fixture afterwards)
//   - state.WriteScrollbackIfChanged
//   - state.Commit                — atomically persist sessions.json
//
// PrevIndex is intentionally nil — Fix Component A's merge filter only
// kicks in when both prev and skipSet are non-empty, and the callers
// of this helper focus on single-tick flows, not the merge filter
// (covered separately by tasks 2-1 and 2-2).

package bootstrap_test

import (
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// daemonTickOpts is the internal shape configured by daemonTickOption
// values passed to runDaemonTick.
type daemonTickOpts struct {
	// skipGuard, when true, mirrors the daemon's per-pane skip-save
	// guard (cmd/state_daemon.go:131-133): panes whose paneKey is in
	// the skipSet returned by ListSkeletonMarkers have their
	// scrollback save suppressed.
	skipGuard bool
	// emptyScrollback, when true, writes empty bytes for every pane
	// instead of calling state.CaptureAndHashPane. Used by the reboot
	// round-trip which overwrites the one byte-compared scrollback
	// file with a deterministic ANSI fixture after the commit.
	emptyScrollback bool
}

// daemonTickOption configures runDaemonTick. Functional options keep
// the call sites readable when only one knob is needed.
type daemonTickOption func(*daemonTickOpts)

// withoutSkipGuard disables the per-pane skip-save guard. The default
// matches the production daemon at cmd/state_daemon.go:131-133 — markers
// in skipSet block scrollback save for their paneKey. The reboot
// round-trip opts out because its in-line save path was always
// guard-free and switching to skip-guard semantics now would change
// observable behaviour for that test.
func withoutSkipGuard() daemonTickOption {
	return func(o *daemonTickOpts) { o.skipGuard = false }
}

// withEmptyScrollback writes empty bytes for every pane instead of
// invoking state.CaptureAndHashPane. The reboot round-trip uses this
// because it overwrites the hook pane's scrollback file with a known
// ANSI fixture immediately after — capture-pane output is timing- and
// terminal-dependent and would make a byte-compare flaky.
func withEmptyScrollback() daemonTickOption {
	return func(o *daemonTickOpts) { o.emptyScrollback = true }
}

// runDaemonTick drives a single daemon-equivalent capture-and-commit
// against the live tmux server backing client. It returns the captured
// state.Index so callers can sanity-check topology.
//
// Default behaviour (no options) matches the production daemon's tick:
// skip guard ON, scrollback bytes captured via state.CaptureAndHashPane.
// Pass withEmptyScrollback() to suppress real capture (reboot round-trip).
// Default is intentionally production-shape so future call sites get
// the spec-compliant behaviour without thinking; the reboot round-trip
// is the only known caller that needs the empty-bytes shape.
func runDaemonTick(
	t *testing.T,
	client *tmux.Client,
	stateDir string,
	options ...daemonTickOption,
) state.Index {
	t.Helper()

	opts := daemonTickOpts{skipGuard: true}
	for _, apply := range options {
		apply(&opts)
	}

	skipSet, err := state.ListSkeletonMarkers(client)
	if err != nil {
		t.Fatalf("ListSkeletonMarkers: %v", err)
	}

	idx, err := state.CaptureStructure(client, skipSet, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}

	hm := state.HashMap{}
	anyChanged := false
	for _, sess := range idx.Sessions {
		for _, win := range sess.Windows {
			for _, pane := range win.Panes {
				key := state.SanitizePaneKey(sess.Name, win.Index, pane.Index)
				if opts.skipGuard {
					if _, skipped := skipSet[key]; skipped {
						continue
					}
				}

				var (
					data []byte
					hash uint64
				)
				if !opts.emptyScrollback {
					target := tmux.PaneTarget(sess.Name, win.Index, pane.Index)
					data, hash, err = state.CaptureAndHashPane(client, target)
					if err != nil {
						t.Fatalf("CaptureAndHashPane %s: %v", target, err)
					}
				}

				written, err := state.WriteScrollbackIfChanged(stateDir, key, data, hash, hm)
				if err != nil {
					t.Fatalf("WriteScrollbackIfChanged %s: %v", key, err)
				}
				if written {
					anyChanged = true
				}
			}
		}
	}

	if err := state.Commit(stateDir, idx, anyChanged, nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	return idx
}
