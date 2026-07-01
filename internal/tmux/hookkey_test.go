package tmux_test

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// portalIDLiteral is the exact "@portal-id" session user-option name that MUST
// be embedded in tmux.HookKeyFormat. It is spelled out here as a literal
// (rather than imported from session.PortalIDOption) to avoid an import cycle:
// internal/session imports internal/tmux, so internal/tmux cannot import
// internal/session, and this test lives alongside the tmux package. The literal
// MUST stay byte-identical to session.PortalIDOption and to the "@portal-id"
// embedded in HookKeyFormat.
const portalIDLiteral = "@portal-id"

// TestHookKeyFormatContainsPortalIDLiteral is a fast static byte-identity
// tripwire for HookKeyFormat's embedded @portal-id conditional. The fix's
// correctness rests on three independent embeddings of "@portal-id" staying
// byte-identical (session.PortalIDOption, tmux.HookKeyFormat, and the
// unexported captureFormat in internal/state); the end-to-end consistency is
// otherwise exercised only by the SkipIfNoTmux-gated real-tmux guards, which
// SKIP silently where tmux is absent. This guard runs under plain `go test`
// with NO tmux (it is deliberately NOT gated by SkipIfNoTmux), so a
// one-character typo in the conditional (e.g. @portal_id) is caught even where
// tmux is unavailable.
func TestHookKeyFormatContainsPortalIDLiteral(t *testing.T) {
	if portalIDLiteral != "@portal-id" {
		t.Fatalf("portalIDLiteral = %q; want %q (must stay byte-identical to session.PortalIDOption)", portalIDLiteral, "@portal-id")
	}
	if !strings.Contains(tmux.HookKeyFormat, portalIDLiteral) {
		t.Errorf("HookKeyFormat = %q does not contain the exact literal %q", tmux.HookKeyFormat, portalIDLiteral)
	}
}

func TestHookKey(t *testing.T) {
	tests := []struct {
		name     string
		portalID string
		session  string
		window   int
		pane     int
		want     string
	}{
		{
			name:     "it returns id:w.p when portalID is non-empty",
			portalID: "id-abc",
			session:  "my-project",
			window:   0,
			pane:     1,
			want:     "id-abc:0.1",
		},
		{
			name:     "it falls back to name:w.p when portalID is empty",
			portalID: "",
			session:  "my-project",
			window:   2,
			pane:     3,
			want:     "my-project:2.3",
		},
		{
			name:     "it yields :w.p when both portalID and name are empty",
			portalID: "",
			session:  "",
			window:   0,
			pane:     0,
			want:     ":0.0",
		},
		{
			name:     "it passes base-index-1 indices through verbatim",
			portalID: "id-abc",
			session:  "x",
			window:   1,
			pane:     1,
			want:     "id-abc:1.1",
		},
		{
			name:     "it passes zero indices through verbatim",
			portalID: "id-abc",
			session:  "x",
			window:   0,
			pane:     0,
			want:     "id-abc:0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tmux.HookKey(tt.portalID, tt.session, tt.window, tt.pane)
			if got != tt.want {
				t.Errorf("HookKey(%q, %q, %d, %d) = %q, want %q",
					tt.portalID, tt.session, tt.window, tt.pane, got, tt.want)
			}
		})
	}
}

func TestHookKey_DistinctSuffixesUnderOneID(t *testing.T) {
	// A multi-pane session stamped with a single @portal-id must yield a
	// distinct w.p suffix per pane, so hooks address individual panes.
	const id = "id-abc"
	keys := map[string]struct{}{}
	panes := []struct {
		window int
		pane   int
		want   string
	}{
		{0, 0, "id-abc:0.0"},
		{0, 1, "id-abc:0.1"},
		{1, 0, "id-abc:1.0"},
	}

	for _, p := range panes {
		got := tmux.HookKey(id, "my-project", p.window, p.pane)
		if got != p.want {
			t.Errorf("HookKey(%q, _, %d, %d) = %q, want %q",
				id, p.window, p.pane, got, p.want)
		}
		if _, dup := keys[got]; dup {
			t.Errorf("duplicate hook key %q under one id", got)
		}
		keys[got] = struct{}{}
	}
}
