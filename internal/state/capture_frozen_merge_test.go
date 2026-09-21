package state_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

type prevPane struct {
	session string
	window  int
	pane    state.Pane
}

func prevIndexOf(entries ...prevPane) state.Index {
	idx := state.Index{Version: state.SchemaVersion}
	for _, e := range entries {
		si := -1
		for i := range idx.Sessions {
			if idx.Sessions[i].Name == e.session {
				si = i
				break
			}
		}
		if si < 0 {
			idx.Sessions = append(idx.Sessions, state.Session{
				Name:        e.session,
				Environment: map[string]string{},
			})
			si = len(idx.Sessions) - 1
		}
		s := &idx.Sessions[si]
		wi := -1
		for i := range s.Windows {
			if s.Windows[i].Index == e.window {
				wi = i
				break
			}
		}
		if wi < 0 {
			s.Windows = append(s.Windows, state.Window{
				Index: e.window, Name: "main", Layout: "L", Active: true,
			})
			wi = len(s.Windows) - 1
		}
		w := &s.Windows[wi]
		w.Panes = append(w.Panes, e.pane)
	}
	idx.Canonicalize()
	return idx
}

func prevRecord(paneIdx int, cwd, command, scrollback, token string) state.Pane {
	return state.Pane{
		Index:          paneIdx,
		CWD:            cwd,
		Active:         false,
		CurrentCommand: command,
		ScrollbackFile: scrollback,
		PortalPaneID:   token,
	}
}

func captureAgainst(t *testing.T, prev state.Index, skip map[string]struct{}, sessions []string, paneLines ...string) state.Index {
	t.Helper()
	mock := &captureMock{
		listSessions: listSessionsFor(sessions...),
		listPanes:    strings.Join(paneLines, "\n"),
		t:            t,
	}
	idx, _, err := state.CaptureStructure(tmux.NewClient(mock.commander()), skip, &prev, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return idx
}

func TestCaptureStructureFrozenMerge(t *testing.T) {
	t.Run("it carries a frozen pane's previous record onto its new address", func(t *testing.T) {
		cases := map[string]struct {
			prevSession string
			prevWindow  int
			prevPane    int
			session     string
			window      int
			pane        int
		}{
			"moved window":     {"work", 0, 0, "work", 4, 0},
			"moved pane index": {"work", 0, 0, "work", 0, 3},
			"renamed session":  {"old-name", 0, 0, "work", 0, 0},
		}
		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				prevFile := "scrollback/" + state.SanitizePaneKey(tc.prevSession, tc.prevWindow, tc.prevPane) + ".bin"
				prev := prevIndexOf(prevPane{tc.prevSession, tc.prevWindow,
					prevRecord(tc.prevPane, "/frozen", "claude", prevFile, "tok-a")})

				idx := captureAgainst(t, prev, nil, []string{tc.session},
					paneLineWithPending(tc.session, tc.window, "main", "L", false, true, tc.pane, "/live", true, "zsh", "tok-a", "1"))

				p := findPane(idx, tc.session, tc.window, tc.pane)
				if p == nil {
					t.Fatalf("pane %s:%d.%d missing from index", tc.session, tc.window, tc.pane)
				}
				if p.ScrollbackFile != prevFile {
					t.Errorf("ScrollbackFile = %q, want %q", p.ScrollbackFile, prevFile)
				}
				if p.CWD != "/frozen" {
					t.Errorf("CWD = %q, want %q", p.CWD, "/frozen")
				}
				if p.CurrentCommand != "claude" {
					t.Errorf("CurrentCommand = %q, want %q", p.CurrentCommand, "claude")
				}
			})
		}
	})

	t.Run("it ends a pane carrying both markers on the token-matched record", func(t *testing.T) {
		frozenFile := "scrollback/" + state.SanitizePaneKey("work", 1, 0) + ".bin"
		prev := prevIndexOf(
			prevPane{"work", 0, prevRecord(0, "/other", "vim", "scrollback/work__0.0.bin", "tok-y")},
			prevPane{"work", 1, prevRecord(0, "/frozen", "claude", frozenFile, "tok-x")},
		)
		skip := map[string]struct{}{state.SanitizePaneKey("work", 0, 0): {}}

		idx := captureAgainst(t, prev, skip, []string{"work"},
			paneLineWithPending("work", 0, "main", "L", false, true, 0, "/live", true, "zsh", "tok-x", "1"))

		p := findPane(idx, "work", 0, 0)
		if p == nil {
			t.Fatalf("pane work:0.0 missing from index")
		}
		want := state.Pane{
			Index:          0,
			CWD:            "/frozen",
			Active:         true,
			CurrentCommand: "claude",
			ScrollbackFile: frozenFile,
			PortalPaneID:   "tok-x",
		}
		if *p != want {
			t.Errorf("pane = %+v, want %+v", *p, want)
		}
		if _, found := state.ComputeReferencedSet(idx)[frozenFile]; !found {
			t.Errorf("referenced set does not hold the frozen pane's file %q", frozenFile)
		}
	})

	t.Run("it keeps the scrollback file the frozen pane's bytes are in after the pane moves", func(t *testing.T) {
		prevFile := "scrollback/" + state.SanitizePaneKey("work", 0, 0) + ".bin"
		prev := prevIndexOf(prevPane{"work", 0, prevRecord(0, "/frozen", "claude", prevFile, "tok-a")})

		idx := captureAgainst(t, prev, nil, []string{"work"},
			paneLineWithPending("work", 1, "main", "L", false, true, 2, "/live", true, "zsh", "tok-a", "1"))

		refs := state.ComputeReferencedSet(idx)
		if _, found := refs[prevFile]; !found {
			t.Errorf("referenced set %v does not hold %q", refs, prevFile)
		}
		freshFile := "scrollback/" + state.SanitizePaneKey("work", 1, 2) + ".bin"
		if _, found := refs[freshFile]; found {
			t.Errorf("referenced set holds the unwritten fresh path %q", freshFile)
		}
	})

	t.Run("it takes the live address and the live active flag with the previous content", func(t *testing.T) {
		prevFile := "scrollback/" + state.SanitizePaneKey("work", 0, 0) + ".bin"
		prev := prevIndexOf(prevPane{"work", 0, prevRecord(0, "/frozen", "claude", prevFile, "tok-a")})

		idx := captureAgainst(t, prev, nil, []string{"work"},
			paneLineWithPending("work", 0, "main", "L", false, true, 5, "/live", true, "zsh", "tok-a", "1"))

		p := findPane(idx, "work", 0, 5)
		if p == nil {
			t.Fatalf("pane work:0.5 missing from index")
		}
		want := state.Pane{
			Index:          5,
			CWD:            "/frozen",
			Active:         true,
			CurrentCommand: "claude",
			ScrollbackFile: prevFile,
			PortalPaneID:   "tok-a",
		}
		if *p != want {
			t.Errorf("pane = %+v, want %+v", *p, want)
		}
	})

	t.Run("it falls back to the positional match for a pending pane carrying no token", func(t *testing.T) {
		prevFile := "scrollback/" + state.SanitizePaneKey("work", 0, 0) + ".bin"
		prev := prevIndexOf(prevPane{"work", 0, prevRecord(0, "/frozen", "claude", prevFile, "")})

		idx := captureAgainst(t, prev, nil, []string{"work"},
			paneLineWithPending("work", 0, "main", "L", false, true, 0, "/live", true, "zsh", "", "1"))

		p := findPane(idx, "work", 0, 0)
		if p == nil {
			t.Fatalf("pane work:0.0 missing from index")
		}
		if p.CWD != "/frozen" || p.CurrentCommand != "claude" || p.ScrollbackFile != prevFile {
			t.Errorf("pane = %+v, want the previous record's content", *p)
		}
	})

	t.Run("it leaves a pending pane with no previous record on its fresh record", func(t *testing.T) {
		prev := prevIndexOf(prevPane{"other", 0,
			prevRecord(0, "/frozen", "claude", "scrollback/other__0.0.bin", "tok-b")})

		idx := captureAgainst(t, prev, nil, []string{"work"},
			paneLineWithPending("work", 0, "main", "L", false, true, 0, "/live", true, "zsh", "tok-a", "1"))

		p := findPane(idx, "work", 0, 0)
		if p == nil {
			t.Fatalf("pane work:0.0 missing from index")
		}
		freshFile := "scrollback/" + state.SanitizePaneKey("work", 0, 0) + ".bin"
		if p.CWD != "/live" || p.CurrentCommand != "zsh" || p.ScrollbackFile != freshFile {
			t.Errorf("pane = %+v, want the fresh record", *p)
		}
	})

	t.Run("it never resurrects a previous record whose pane is absent from the live enumeration", func(t *testing.T) {
		cases := map[string]prevPane{
			"gone session": {"gone", 0, prevRecord(0, "/frozen", "claude", "scrollback/gone__0.0.bin", "tok-gone")},
			"gone window":  {"work", 7, prevRecord(0, "/frozen", "claude", "scrollback/work__7.0.bin", "tok-gone")},
			"gone pane":    {"work", 0, prevRecord(9, "/frozen", "claude", "scrollback/work__0.9.bin", "tok-gone")},
		}
		for name, entry := range cases {
			t.Run(name, func(t *testing.T) {
				prev := prevIndexOf(entry)

				idx := captureAgainst(t, prev, nil, []string{"work"},
					paneLineWithPending("work", 0, "main", "L", false, true, 0, "/live", true, "zsh", "tok-a", "1"))

				if p := findPane(idx, entry.session, entry.window, entry.pane.Index); p != nil {
					t.Errorf("absent pane reintroduced: %+v", *p)
				}
				if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "work" {
					t.Fatalf("Sessions = %+v, want only work", idx.Sessions)
				}
			})
		}
	})

	t.Run("it consumes a duplicated token once and resolves it deterministically", func(t *testing.T) {
		prev := prevIndexOf(
			prevPane{"work", 1, prevRecord(0, "/second", "vim", "scrollback/work__1.0.bin", "dup")},
			prevPane{"work", 0, prevRecord(0, "/first", "claude", "scrollback/work__0.0.bin", "dup")},
		)

		idx := captureAgainst(t, prev, nil, []string{"work"},
			paneLineWithPending("work", 2, "main", "L", false, true, 0, "/live", true, "zsh", "dup", "1"),
			paneLineWithPending("work", 2, "main", "L", false, true, 1, "/live", false, "zsh", "dup", "1"))

		first := findPane(idx, "work", 2, 0)
		second := findPane(idx, "work", 2, 1)
		if first == nil || second == nil {
			t.Fatalf("live panes missing: %+v", idx.Sessions)
		}
		if first.ScrollbackFile != "scrollback/work__0.0.bin" || first.CurrentCommand != "claude" {
			t.Errorf("first pane = %+v, want the canonically first previous record", *first)
		}
		freshSecond := "scrollback/" + state.SanitizePaneKey("work", 2, 1) + ".bin"
		if second.ScrollbackFile != freshSecond || second.CurrentCommand != "zsh" {
			t.Errorf("second pane = %+v, want its fresh record", *second)
		}
	})

	t.Run("it merges for a caller that passed no skip set", func(t *testing.T) {
		prevFile := "scrollback/" + state.SanitizePaneKey("work", 0, 0) + ".bin"
		prev := prevIndexOf(prevPane{"work", 0, prevRecord(0, "/frozen", "claude", prevFile, "tok-a")})
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLineWithPending("work", 3, "main", "L", false, true, 0, "/live", true, "zsh", "tok-a", "1"),
			t:            t,
		}

		idx, _, err := state.CaptureStructure(tmux.NewClient(mock.commander()), nil, &prev, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		p := findPane(idx, "work", 3, 0)
		if p == nil {
			t.Fatalf("pane work:3.0 missing from index")
		}
		if p.ScrollbackFile != prevFile {
			t.Errorf("ScrollbackFile = %q, want %q", p.ScrollbackFile, prevFile)
		}
	})

	t.Run("it produces an identical index on a second capture of an unchanged server", func(t *testing.T) {
		lines := []string{
			paneLineWithPending("work", 1, "main", "L", false, true, 2, "/live", true, "zsh", "tok-a", "1"),
			paneLine("work", 1, "main", "L", false, true, 3, "/other", false, "vim"),
		}
		prevFile := "scrollback/" + state.SanitizePaneKey("work", 0, 0) + ".bin"
		prev := prevIndexOf(prevPane{"work", 0, prevRecord(0, "/frozen", "claude", prevFile, "tok-a")})

		first := captureAgainst(t, prev, nil, []string{"work"}, lines...)
		second := captureAgainst(t, first, nil, []string{"work"}, lines...)

		a, b := first, second
		a.SavedAt, b.SavedAt = time.Time{}, time.Time{}
		if !reflect.DeepEqual(a, b) {
			t.Errorf("second capture differs:\nfirst  = %+v\nsecond = %+v", a, b)
		}
	})
}
