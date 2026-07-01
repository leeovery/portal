package state

import (
	"strings"
	"testing"
)

// portalIDLiteral is the exact "@portal-id" session user-option name that MUST
// be embedded in captureFormat's trailing column. It is spelled out here as a
// literal (rather than imported from session.PortalIDOption) to avoid an import
// cycle: internal/session imports internal/state, so internal/state cannot
// import internal/session. This is a white-box (package state) test because
// captureFormat is unexported. The literal MUST stay byte-identical to
// session.PortalIDOption and to the "@portal-id" embedded in captureFormat.
const portalIDLiteral = "@portal-id"

// TestCaptureFormatContainsPortalIDLiteral is a fast static byte-identity
// tripwire for captureFormat's trailing #{@portal-id} column. The fix's
// correctness rests on three independent embeddings of "@portal-id" staying
// byte-identical (session.PortalIDOption, tmux.HookKeyFormat, and this
// captureFormat); the durability round-trip that exercises captureFormat
// end-to-end is SkipIfNoTmux-gated, so it SKIPS silently where tmux is absent.
// This guard runs under plain `go test` with NO tmux (it is deliberately NOT
// gated by SkipIfNoTmux), so a one-character typo in the trailing column (e.g.
// @portal_id) is caught even where tmux is unavailable.
func TestCaptureFormatContainsPortalIDLiteral(t *testing.T) {
	if portalIDLiteral != "@portal-id" {
		t.Fatalf("portalIDLiteral = %q; want %q (must stay byte-identical to session.PortalIDOption)", portalIDLiteral, "@portal-id")
	}
	if !strings.Contains(captureFormat, portalIDLiteral) {
		t.Errorf("captureFormat = %q does not contain the exact literal %q", captureFormat, portalIDLiteral)
	}
}
