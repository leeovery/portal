package cmd_test

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/tmux"
)

// TestPortalIDOptionBindsHookKeyFormat is the fast, tmux-less binding tripwire
// that ties the source-of-truth constant session.PortalIDOption to the
// "@portal-id" literal embedded in tmux.HookKeyFormat.
//
// Why this guard lives in cmd: internal/session imports internal/tmux (and
// internal/state), so neither internal/tmux nor internal/state can import
// internal/session — an import cycle. The cmd package is the only package that
// already imports BOTH internal/session and internal/tmux cycle-free, so it is
// the sole place a static test can compare session.PortalIDOption directly
// against the tmux format string without dragging in tmux at runtime.
//
// Drift class caught: a change to session.PortalIDOption (e.g. to "@portal-uid")
// silently orphans every stamped session's resume hook — stamping would write
// the new value while tmux.HookKeyFormat (and state's captureFormat) still read
// "@portal-id", so no key-producing site would ever match the stamped identity.
// Only the //go:build integration + SkipIfNoTmux-gated end-to-end guards catch
// such a change today, and those do not run under the default tmux-less
// `go test ./...`. This guard deliberately runs WITHOUT tmux (no SkipIfNoTmux,
// no integration build tag) to close that gap and complement — not replace — the
// end-to-end guards.
//
// Three-way picture: the fix's invariant requires three independent embeddings
// of "@portal-id" to stay byte-identical — session.PortalIDOption,
// tmux.HookKeyFormat, and the unexported state.captureFormat. Two cycle-1
// sibling guards pin each format string to a LOCAL copy of the literal:
//   - internal/tmux/hookkey_test.go       (TestHookKeyFormatContainsPortalIDLiteral)
//   - internal/state/portal_id_literal_guard_test.go (TestCaptureFormatContainsPortalIDLiteral)
//
// This guard pins the source-of-truth constant to that same literal and ties it
// to HookKeyFormat, transitively closing the loop:
//
//	session.PortalIDOption == "@portal-id" == HookKeyFormat literal == captureFormat literal
//
// state.captureFormat is unexported and thus unreachable from cmd; its cycle-1
// guard already pins it to the shared literal, so the transitive chain holds via
// the shared literal value without cmd needing to reach captureFormat.
func TestPortalIDOptionBindsHookKeyFormat(t *testing.T) {
	if session.PortalIDOption != "@portal-id" {
		t.Fatalf("session.PortalIDOption = %q; want %q (a change silently orphans every stamped session's resume hook)", session.PortalIDOption, "@portal-id")
	}
	if !strings.Contains(tmux.HookKeyFormat, session.PortalIDOption) {
		t.Errorf("tmux.HookKeyFormat = %q does not contain session.PortalIDOption %q", tmux.HookKeyFormat, session.PortalIDOption)
	}
}
