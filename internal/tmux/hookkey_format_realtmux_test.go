package tmux_test

// Real-tmux round-trip guard for the tmux.HookKeyFormat conditional.
//
// HookKeyFormat carries a per-session tmux conditional
// (#{?@portal-id,#{@portal-id},#{session_name}}) whose correctness cannot be
// proven by a pure Go unit test: whether tmux treats an unset/empty @portal-id
// as the false branch, and whether the #{@portal-id}/#{session_name} fields
// resolve as expected, is a property of the live tmux server, not of the Go
// string. A drift in the conditional syntax or field names would compile and
// pass every in-Go test while silently orphaning every stamped session's hook
// against a real server.
//
// This test closes that seam by driving the exact production format string
// through a real display-message read on an isolated socket and asserting the
// resolved key for a stamped session (<id>:w.p), an un-stamped session
// (<name>:w.p), and a multi-window/multi-pane stamped session (distinct :w.p
// suffixes sharing one id). Like the other real-tmux guards in this package it
// carries NO build tag and is gated only by SkipIfNoTmux(t) so tmux-less
// environments skip cleanly rather than fail.
//
// The harness runs -f /dev/null, so base-index and pane-base-index default to
// 0 — hence the ":0.0" suffixes below.
//
// Spec: .workflows/session-rename-orphans-resume-hook/specification §
// "Hook-Key Derivation" (new derivation primitives) and § "Testing
// Requirements → Derivation primitives (unit)".

import (
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// portalIDOption is the literal session user-option name embedded in
// tmux.HookKeyFormat. It is repeated here as a literal (rather than imported
// from session.PortalIDOption, which does not yet exist) to avoid an import
// for a single string and must stay byte-identical to the literal in
// HookKeyFormat.
const portalIDOption = "@portal-id"

// readHookKey drives the production tmux.HookKeyFormat through a real
// display-message read against target on the isolated socket, returning the
// resolved key with the trailing newline trimmed. Reading via the exact
// production format string is the point of the guard — the assertion runs
// against HookKeyFormat itself, not a copy.
func readHookKey(t *testing.T, ts *tmuxtest.Socket, target string) string {
	t.Helper()
	out := ts.Run(t, "display-message", "-p", "-t", target, tmux.HookKeyFormat)
	return strings.TrimRight(out, "\n")
}

// TestHookKeyFormat_StampedSession proves HookKeyFormat resolves to
// "<@portal-id>:w.p" for a session carrying @portal-id — the rename-immune
// case. It creates a session, stamps @portal-id via the production
// SetSessionOption, then reads the pane's hook key through HookKeyFormat and
// asserts the id (not the session name) is the prefix.
func TestHookKeyFormat_StampedSession(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "hookkey-")
	client := ts.Client()

	const sessionName = "hk-stamped"
	if err := client.NewSession(sessionName, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession(%q): %v", sessionName, err)
	}
	ts.WaitForSession(t, sessionName, 2*time.Second)

	if err := client.SetSessionOption(sessionName, portalIDOption, "tok123"); err != nil {
		t.Fatalf("SetSessionOption(%q, %q, %q): %v", sessionName, portalIDOption, "tok123", err)
	}

	// Base-index/pane-base-index default to 0 under -f /dev/null, so the
	// single pane of the single window is window 0, pane 0.
	if got := readHookKey(t, ts, sessionName); got != "tok123:0.0" {
		t.Errorf("stamped session hook key = %q, want %q (conditional must take the @portal-id branch)", got, "tok123:0.0")
	}
}

// TestHookKeyFormat_UnstampedSession proves HookKeyFormat resolves to
// "<session_name>:w.p" for a session with no @portal-id — the
// legacy/no-migration fallback. It creates a session WITHOUT stamping
// @portal-id and asserts the session name (not an empty prefix) resolves, i.e.
// tmux treats the unset option as the conditional's false branch.
func TestHookKeyFormat_UnstampedSession(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "hookkey-")
	client := ts.Client()

	const sessionName = "hk-unstamped"
	if err := client.NewSession(sessionName, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession(%q): %v", sessionName, err)
	}
	ts.WaitForSession(t, sessionName, 2*time.Second)

	// Deliberately do NOT stamp @portal-id — the conditional must fall back
	// to #{session_name}.
	want := sessionName + ":0.0"
	if got := readHookKey(t, ts, sessionName); got != want {
		t.Errorf("un-stamped session hook key = %q, want %q (unset @portal-id must take the #{session_name} branch)", got, want)
	}
}

// TestHookKeyFormat_MultiWindowMultiPane proves HookKeyFormat resolves distinct
// ":w.p" suffixes across multiple windows and panes of one stamped session,
// all sharing the single @portal-id prefix. This confirms the id is
// session-scoped (shared) while window/pane indices vary per pane — the
// property that makes each pane's hook key unique yet rename-immune.
func TestHookKeyFormat_MultiWindowMultiPane(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "hookkey-")
	client := ts.Client()

	const sessionName = "hk-multi"
	if err := client.NewSession(sessionName, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession(%q): %v", sessionName, err)
	}
	ts.WaitForSession(t, sessionName, 2*time.Second)

	if err := client.SetSessionOption(sessionName, portalIDOption, "tokMulti"); err != nil {
		t.Fatalf("SetSessionOption(%q, %q, %q): %v", sessionName, portalIDOption, "tokMulti", err)
	}

	// Split the initial pane (window 0 now has panes 0 and 1) and add a
	// second window (window 1, pane 0). Targets address panes directly via
	// session:window.pane.
	ts.Run(t, "split-window", "-t", sessionName+":0")
	ts.Run(t, "new-window", "-t", sessionName)

	cases := []struct {
		name   string
		target string
		want   string
	}{
		{name: "window 0 pane 0", target: sessionName + ":0.0", want: "tokMulti:0.0"},
		{name: "window 0 pane 1", target: sessionName + ":0.1", want: "tokMulti:0.1"},
		{name: "window 1 pane 0", target: sessionName + ":1.0", want: "tokMulti:1.0"},
	}

	seen := make(map[string]struct{}, len(cases))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := readHookKey(t, ts, tc.target)
			if got != tc.want {
				t.Errorf("hook key for %q = %q, want %q", tc.target, got, tc.want)
			}
			if !strings.HasPrefix(got, "tokMulti:") {
				t.Errorf("hook key %q does not share the single @portal-id prefix %q", got, "tokMulti:")
			}
			if _, dup := seen[got]; dup {
				t.Errorf("hook key %q is not distinct across panes (duplicate suffix)", got)
			}
			seen[got] = struct{}{}
		})
	}
}
