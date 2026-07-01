package tmux_test

import (
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

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
