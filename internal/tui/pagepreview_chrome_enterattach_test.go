package tui

import (
	"errors"
	"strings"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// TestPreviewChromeLine_EnterAttachTokenByteIdenticalAcrossViewportStates pins
// spec § Discoverability > Token wording is unconditional: chromeLine() must
// produce a byte-identical string regardless of whether the viewport rendered
// real bytes, the (nil, nil) "(no saved content)" placeholder, or the (nil,
// err) error string. Chrome is a pure function of cached groups + windowIdx +
// paneIdx and must not branch on viewport content state.
func TestPreviewChromeLine_EnterAttachTokenByteIdenticalAcrossViewportStates(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "logs", PaneIndices: []int{0}},
	}

	cases := []struct {
		name   string
		reader ScrollbackReader
	}{
		{name: "real bytes", reader: &recordingReader{bytes: []byte("hello world\n")}},
		{name: "(nil, nil) placeholder", reader: &nilNilReader{}},
		{name: "OS read error", reader: &nilErrReader{err: syscall.EACCES}},
		{name: "OS read error EIO", reader: &nilErrReader{err: errors.New("EIO synthetic")}},
	}

	var first string
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enum := &stubEnumerator{groups: groups}
			m, ok := NewPreviewModel("work", enum, tc.reader, nil, 80, 24)
			if !ok {
				t.Fatalf("expected ok=true on construction, got false")
			}
			got := stripANSI(m.chromeLine())

			if !strings.Contains(got, "· tab next pane · enter attach · esc back") {
				t.Errorf("chromeLine() = %q; missing canonical enter-attach segment", got)
			}

			if i == 0 {
				first = got
				return
			}
			if got != first {
				t.Errorf("chromeLine() under %q = %q; want byte-identical to first case %q", tc.name, got, first)
			}
		})
	}
}

// TestSessionsPageView_DoesNotContainPreviewChromeEnterAttachToken pins spec
// § Discoverability > Sessions-page help bar: the preview chrome's new
// 'enter attach' token must not propagate to or duplicate on the Sessions
// page. The preview chrome renders the token surrounded by middle-dot
// separators ("· enter attach ·") — that exact chrome phrasing is
// preview-specific and must never appear on the Sessions page.
//
// Note: the Sessions-page help bar independently renders "enter" and "attach"
// as a bubbles/list key/help pair (see model.go binding at line ~506), which
// is the pre-existing Sessions-page help advertised by bubbles/list. That
// bar is "unaffected" by this change per spec — the preview chrome's
// dot-separated chrome token does not appear there.
func TestSessionsPageView_DoesNotContainPreviewChromeEnterAttachToken(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha"},
		{Name: "beta"},
	}
	m := NewModelWithSessions(sessions)

	got := stripANSI(m.View())

	// The preview chrome's specific phrasing — token between middle-dot
	// separators — is preview-specific. The Sessions page must not contain
	// this exact phrasing.
	if strings.Contains(got, "· enter attach") {
		t.Errorf("Sessions-page View() contains forbidden preview-chrome token '· enter attach'; got %q", got)
	}
	if strings.Contains(got, "enter attach ·") {
		t.Errorf("Sessions-page View() contains forbidden preview-chrome token 'enter attach ·'; got %q", got)
	}
}
