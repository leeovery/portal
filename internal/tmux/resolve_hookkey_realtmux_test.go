package tmux_test

// Real-tmux round-trip guard for (*Client).ResolveHookKey.
//
// ResolveHookKey is the canonical single-read live hook-key resolver: it issues
// exactly one `display-message -p -t <paneID> <HookKeyFormat>` and lets tmux
// pick the id-vs-name branch per session (stamped @portal-id → "<id>:w.p",
// un-stamped → "<name>:w.p"). Whether the tmux conditional in HookKeyFormat
// takes the id branch for a stamped session and the name branch for an
// un-stamped one is a property of the live server, not of the Go string, so a
// pure unit test cannot prove the method resolves correctly end to end.
//
// This test closes that seam by driving ResolveHookKey through a real
// display-message read on an isolated socket and asserting the resolved key for
// a stamped session (<id>:w.p) and an un-stamped session (<name>:w.p), plus the
// read-failure contract: a failing display-message read MUST return
// ("", wrapped error) and NEVER synthesize a name-based key (which would
// silently orphan a stamped session's hook). Like the other real-tmux guards in
// this package it carries NO build tag and is gated only by SkipIfNoTmux(t) so
// tmux-less environments skip cleanly rather than fail.
//
// The harness runs -f /dev/null, so base-index and pane-base-index default to
// 0 — hence the ":0.0" suffixes below.
//
// Spec: .workflows/session-rename-orphans-resume-hook/specification §
// "Hook-Key Derivation → Stage 1 Registration + Failure contract" and
// § "Testing Requirements → cross-site consistency".

import (
	"errors"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// TestResolveHookKey_StampedSession proves ResolveHookKey resolves a stamped
// pane to "<@portal-id>:w.p" — the rename-immune case. It creates a session,
// stamps @portal-id via the production SetSessionOption (the literal
// "@portal-id" is used to avoid an import cycle), then resolves the hook key
// and asserts the id (not the session name) is the prefix. The session name is
// used as the display-message target: with no pane suffix, tmux resolves it
// against the session's active pane, so the single pane of the single window
// resolves to window 0, pane 0.
func TestResolveHookKey_StampedSession(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-hookkey-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	const sessionName = "rhk-stamped"
	if err := client.NewSession(sessionName, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession(%q): %v", sessionName, err)
	}
	ts.WaitForSession(t, sessionName, 2*time.Second)

	// Use the literal "@portal-id" (session.PortalIDOption) to avoid an import
	// cycle; it must stay byte-identical to the literal embedded in
	// tmux.HookKeyFormat.
	if err := client.SetSessionOption(sessionName, "@portal-id", "tok123"); err != nil {
		t.Fatalf("SetSessionOption(%q, @portal-id, tok123): %v", sessionName, err)
	}

	got, err := client.ResolveHookKey(sessionName)
	if err != nil {
		t.Fatalf("ResolveHookKey(%q): %v", sessionName, err)
	}
	if got != "tok123:0.0" {
		t.Errorf("stamped hook key = %q, want %q (conditional must take the @portal-id branch)", got, "tok123:0.0")
	}
}

// TestResolveHookKey_UnstampedSession proves ResolveHookKey resolves an
// un-stamped pane to "<session_name>:w.p" — the legacy/no-migration fallback.
// It creates a session WITHOUT stamping @portal-id and asserts the session name
// (not an empty prefix) resolves, i.e. tmux treats the unset option as the
// conditional's false branch.
func TestResolveHookKey_UnstampedSession(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-hookkey-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	const sessionName = "rhk-unstamped"
	if err := client.NewSession(sessionName, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession(%q): %v", sessionName, err)
	}
	ts.WaitForSession(t, sessionName, 2*time.Second)

	// Deliberately do NOT stamp @portal-id — the conditional must fall back to
	// #{session_name}.
	want := sessionName + ":0.0"
	got, err := client.ResolveHookKey(sessionName)
	if err != nil {
		t.Fatalf("ResolveHookKey(%q): %v", sessionName, err)
	}
	if got != want {
		t.Errorf("un-stamped hook key = %q, want %q (unset @portal-id must take the #{session_name} branch)", got, want)
	}
}

// TestResolveHookKey_ReadFailureWrapsError proves the Stage 1 Failure contract:
// when the display-message read fails, ResolveHookKey returns ("", wrapped
// error) and never synthesizes a name-based key.
//
// tmux 3.7's display-message is tolerant of a non-existent pane/session target
// (it returns ":." with exit 0 rather than failing), so a bogus "-t" target
// does NOT reliably drive the read-failure path on this tmux. The reliably
// failing condition through this harness is a read against a server that is not
// running, which exits non-zero with "no server running". We therefore kill the
// isolated server first, then resolve — the socketCommander wraps the exec
// failure via tmux.WrapCommandError, so the returned error stays recoverable
// via errors.As/errors.Is while the returned key is the empty string.
func TestResolveHookKey_ReadFailureWrapsError(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-hookkey-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// Tear the server down so the subsequent read fails with "no server
	// running" — the reliable read-failure path on tmux 3.7.
	ts.KillServer()

	got, err := client.ResolveHookKey("%nonexistent")
	if err == nil {
		t.Fatal("expected a wrapped error from a failed display-message read, got nil")
	}
	if got != "" {
		t.Errorf("hook key on read failure = %q, want \"\" (MUST NOT synthesize a name-based key)", got)
	}

	// The error must be a recoverable *tmux.CommandError, not a bare
	// fmt.Errorf, so callers can discriminate it via errors.As.
	var cmdErr *tmux.CommandError
	if !errors.As(err, &cmdErr) {
		t.Errorf("error %v is not a recoverable *tmux.CommandError (errors.As failed)", err)
	}
}
