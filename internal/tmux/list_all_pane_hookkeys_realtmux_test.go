package tmux_test

// Real-tmux round-trip guard for (*Client).ListAllPaneHookKeys.
//
// ListAllPaneHookKeys is the canonical live hook-key enumeration for stale
// cleanup: it lists every live pane across every session via
// `list-panes -a -F HookKeyFormat` and lets tmux pick the id-vs-name branch per
// session (stamped @portal-id → "<id>:w.p", un-stamped → "<name>:w.p"). Whether
// the HookKeyFormat conditional resolves the id branch for a stamped session and
// the name branch for an un-stamped one — evaluated independently per pane row
// within a single -a enumeration — is a property of the live tmux server, not of
// the Go string, so a pure unit test cannot prove the enumeration resolves the
// mixed population correctly end to end.
//
// This test closes that seam by driving ListAllPaneHookKeys through a real
// list-panes -a read on an isolated socket, asserting the resolved keys for a
// stamped session (<id>:w.p), an un-stamped session (<name>:w.p), a
// multi-window/multi-pane stamped session (distinct :w.p suffixes sharing one
// id), and a mixed stamped/un-stamped population (per-session prefixes). It also
// pins the two inherited contracts: an underlying list-panes failure returns
// (nil, err) — NOT an empty slice, which would mass-orphan every hooks.json
// entry — and empty output yields a non-nil empty slice. Like the other
// real-tmux guards in this package it carries NO build tag and is gated only by
// SkipIfNoTmux(t) so tmux-less environments skip cleanly rather than fail.
//
// The harness runs -f /dev/null, so base-index and pane-base-index default to
// 0 — hence the ":0.0" suffixes below.
//
// Spec: .workflows/session-rename-orphans-resume-hook/specification §
// "Hook-Key Derivation → Stage 2 Stale cleanup live keys" and § "Testing
// Requirements → cross-site consistency".

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// TestListAllPaneHookKeys_StampedSession proves ListAllPaneHookKeys enumerates a
// stamped session's pane as "<@portal-id>:w.p" — the rename-immune case. It
// creates a session, stamps @portal-id via the production SetSessionOption, then
// enumerates all live hook keys and asserts the id (not the session name) is the
// prefix of that session's key.
func TestListAllPaneHookKeys_StampedSession(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-hookkeys-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	const sessionName = "lapk-stamped"
	if err := client.NewSession(sessionName, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession(%q): %v", sessionName, err)
	}
	ts.WaitForSession(t, sessionName, 2*time.Second)

	if err := client.SetSessionOption(sessionName, portalIDLiteral, "tok123"); err != nil {
		t.Fatalf("SetSessionOption(%q, %q, %q): %v", sessionName, portalIDLiteral, "tok123", err)
	}

	keys, err := client.ListAllPaneHookKeys()
	if err != nil {
		t.Fatalf("ListAllPaneHookKeys: %v", err)
	}
	if !slices.Contains(keys, "tok123:0.0") {
		t.Errorf("stamped session hook key %q not found in %v (conditional must take the @portal-id branch)", "tok123:0.0", keys)
	}
}

// TestListAllPaneHookKeys_UnstampedSession proves ListAllPaneHookKeys enumerates
// an un-stamped session's pane as "<session_name>:w.p" — the
// legacy/no-migration fallback. It creates a session WITHOUT stamping
// @portal-id and asserts the session name (not an empty prefix) resolves, i.e.
// tmux treats the unset option as the conditional's false branch.
func TestListAllPaneHookKeys_UnstampedSession(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-hookkeys-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	const sessionName = "lapk-unstamped"
	if err := client.NewSession(sessionName, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession(%q): %v", sessionName, err)
	}
	ts.WaitForSession(t, sessionName, 2*time.Second)

	// Deliberately do NOT stamp @portal-id — the conditional must fall back to
	// #{session_name}.
	want := sessionName + ":0.0"
	keys, err := client.ListAllPaneHookKeys()
	if err != nil {
		t.Fatalf("ListAllPaneHookKeys: %v", err)
	}
	if !slices.Contains(keys, want) {
		t.Errorf("un-stamped session hook key %q not found in %v (unset @portal-id must take the #{session_name} branch)", want, keys)
	}
}

// TestListAllPaneHookKeys_MultiWindowMultiPane proves ListAllPaneHookKeys
// enumerates distinct ":w.p" suffixes across multiple windows and panes of one
// stamped session, all sharing the single @portal-id prefix. This confirms the
// id is session-scoped (shared) while window/pane indices vary per pane — the
// property that makes each pane's hook key unique yet rename-immune.
func TestListAllPaneHookKeys_MultiWindowMultiPane(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-hookkeys-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	const sessionName = "lapk-multi"
	if err := client.NewSession(sessionName, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession(%q): %v", sessionName, err)
	}
	ts.WaitForSession(t, sessionName, 2*time.Second)

	if err := client.SetSessionOption(sessionName, portalIDLiteral, "tokMulti"); err != nil {
		t.Fatalf("SetSessionOption(%q, %q, %q): %v", sessionName, portalIDLiteral, "tokMulti", err)
	}

	// Split the initial pane (window 0 now has panes 0 and 1) and add a second
	// window (window 1, pane 0).
	ts.Run(t, "split-window", "-t", sessionName+":0")
	ts.Run(t, "new-window", "-t", sessionName)

	keys, err := client.ListAllPaneHookKeys()
	if err != nil {
		t.Fatalf("ListAllPaneHookKeys: %v", err)
	}

	want := []string{"tokMulti:0.0", "tokMulti:0.1", "tokMulti:1.0"}
	for _, w := range want {
		if !slices.Contains(keys, w) {
			t.Errorf("expected distinct hook key %q under the shared id, not found in %v", w, keys)
		}
	}
}

// TestListAllPaneHookKeys_MixedStampedAndUnstamped proves ListAllPaneHookKeys
// resolves each session's key independently within a single -a enumeration: a
// stamped session yields "<id>:w.p" while an un-stamped session in the same
// server yields "<name>:w.p". The HookKeyFormat conditional is evaluated
// per-pane-row, so a mixed population must not bleed one session's prefix into
// another.
func TestListAllPaneHookKeys_MixedStampedAndUnstamped(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-hookkeys-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	const stamped = "lapk-mix-stamped"
	const unstamped = "lapk-mix-unstamped"
	if err := client.NewSession(stamped, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession(%q): %v", stamped, err)
	}
	ts.WaitForSession(t, stamped, 2*time.Second)
	if err := client.NewSession(unstamped, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession(%q): %v", unstamped, err)
	}
	ts.WaitForSession(t, unstamped, 2*time.Second)

	if err := client.SetSessionOption(stamped, portalIDLiteral, "tokMix"); err != nil {
		t.Fatalf("SetSessionOption(%q, %q, %q): %v", stamped, portalIDLiteral, "tokMix", err)
	}

	keys, err := client.ListAllPaneHookKeys()
	if err != nil {
		t.Fatalf("ListAllPaneHookKeys: %v", err)
	}

	if !slices.Contains(keys, "tokMix:0.0") {
		t.Errorf("stamped session key %q not found in %v (must take the @portal-id branch)", "tokMix:0.0", keys)
	}
	unstampedWant := unstamped + ":0.0"
	if !slices.Contains(keys, unstampedWant) {
		t.Errorf("un-stamped session key %q not found in %v (must take the #{session_name} branch)", unstampedWant, keys)
	}
}

// TestListAllPaneHookKeys_EmptyOutputReturnsNonNilEmptySlice pins the inherited
// empty-output contract: ListAllPaneHookKeys delegates to
// ListAllPanesWithFormat + parsePaneOutput, and parsePaneOutput returns a
// non-nil []string{} (len 0) on empty output, which ListAllPaneHookKeys returns
// verbatim as ([]string{}, nil).
//
// A live empty enumeration is impractical (the harness always has an anchor
// session), and parsePaneOutput is unexported (in-package), so we assert the
// contract through the exported ListAllPaneHookKeys via a mock Commander that
// returns empty stdout with no error — keeping the assertion inside the
// package_test boundary without a fabricated live-empty read. (Choice note: mock
// Commander over an in-package test file, so the empty contract is proven
// through the public method surface end to end.)
func TestListAllPaneHookKeys_EmptyOutputReturnsNonNilEmptySlice(t *testing.T) {
	client := tmux.NewClient(&MockCommander{Output: ""})

	keys, err := client.ListAllPaneHookKeys()
	if err != nil {
		t.Fatalf("ListAllPaneHookKeys on empty output: unexpected error %v", err)
	}
	if keys == nil {
		t.Fatal("ListAllPaneHookKeys returned a nil slice on empty output, want non-nil []string{}")
	}
	if len(keys) != 0 {
		t.Errorf("ListAllPaneHookKeys on empty output = %v (len %d), want empty slice", keys, len(keys))
	}
}

// TestListAllPaneHookKeys_ListPanesFailurePropagates pins the discriminating
// contract inherited from ListAllPanes: an underlying `list-panes -a` failure
// returns (nil, err) with the error wrapped and recoverable — NOT an empty
// slice. Treating a tmux failure as "no live panes" would mass-orphan every
// hooks.json entry, the exact bug the discriminating contract prevents.
//
// The reliable read-failure path on this tmux is a read against a server that
// is not running (exits non-zero with "no server running"), so we tear the
// isolated server down first, then enumerate. The socketCommander wraps the exec
// failure via tmux.WrapCommandError, so the returned error stays recoverable via
// errors.As while the returned slice is nil.
func TestListAllPaneHookKeys_ListPanesFailurePropagates(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-hookkeys-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// Tear the server down so the subsequent list-panes -a fails with "no
	// server running" — the reliable read-failure path.
	ts.KillServer()

	keys, err := client.ListAllPaneHookKeys()
	if err == nil {
		t.Fatal("expected a wrapped error from a failed list-panes -a read, got nil")
	}
	if keys != nil {
		t.Errorf("hook keys on read failure = %v, want nil (MUST NOT treat a tmux failure as an empty live set)", keys)
	}

	// The error must be a recoverable *tmux.CommandError, not a bare
	// fmt.Errorf, so callers can discriminate it via errors.As.
	var cmdErr *tmux.CommandError
	if !errors.As(err, &cmdErr) {
		t.Errorf("error %v is not a recoverable *tmux.CommandError (errors.As failed)", err)
	}
}
