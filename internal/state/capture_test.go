package state_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// captureMock is a Commander that dispatches to a per-command handler. Using a
// dispatch table keeps each test's intent local — callers configure only the
// commands their scenario exercises and the mock fails fast on anything else.
type captureMock struct {
	listSessions  string
	listSessionsE error
	listPanes     string
	listPanesE    error
	envBySession  map[string]string
	envErrs       map[string]error
	t             *testing.T
}

func (m *captureMock) Run(args ...string) (string, error) {
	if len(args) == 0 {
		m.t.Fatalf("captureMock invoked with no args")
	}
	switch args[0] {
	case "list-sessions":
		return m.listSessions, m.listSessionsE
	case "list-panes":
		return m.listPanes, m.listPanesE
	case "show-environment":
		// args == [show-environment, -t, <session>]
		if len(args) < 3 {
			m.t.Fatalf("show-environment called with insufficient args: %v", args)
		}
		session := args[2]
		if err, ok := m.envErrs[session]; ok {
			return "", err
		}
		if out, ok := m.envBySession[session]; ok {
			return out, nil
		}
		// Default to empty environment for sessions not configured explicitly.
		return "", nil
	default:
		m.t.Fatalf("captureMock: unexpected command %v", args)
		return "", nil
	}
}

// RunRaw satisfies tmux.Commander; CaptureStructure never invokes it, so any
// call here indicates a test-setup mismatch.
func (m *captureMock) RunRaw(args ...string) (string, error) {
	m.t.Fatalf("captureMock.RunRaw unexpectedly called with %v", args)
	return "", nil
}

// listSessionsFor returns a list-sessions output line for the given names. The
// numeric fields are placeholders; CaptureStructure only consumes the names.
func listSessionsFor(names ...string) string {
	lines := make([]string, 0, len(names))
	for _, n := range names {
		lines = append(lines, n+"|1|0")
	}
	return strings.Join(lines, "\n")
}

// paneLine renders one pane row in the structural list-panes output format
// CaptureStructure expects.
func paneLine(session string, windowIdx int, windowName, layout string, zoomed, windowActive bool, paneIdx int, cwd string, paneActive bool, currentCommand string) string {
	bool01 := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}
	return fmt.Sprintf(
		"%s|||%d|||%s|||%s|||%s|||%s|||%d|||%s|||%s|||%s",
		session, windowIdx, windowName, layout, bool01(zoomed), bool01(windowActive), paneIdx, cwd, bool01(paneActive), currentCommand,
	)
}

func TestCaptureStructure(t *testing.T) {
	t.Run("captures a single session with one window and one pane", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes: paneLine(
				"work", 0, "main", "b25f,200x50,0,0",
				false, true,
				0, "/Users/leeovery/Code/portal", true, "zsh",
			),
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(idx.Sessions) != 1 {
			t.Fatalf("got %d sessions, want 1", len(idx.Sessions))
		}
		s := idx.Sessions[0]
		if s.Name != "work" {
			t.Errorf("session name = %q, want %q", s.Name, "work")
		}
		if len(s.Windows) != 1 {
			t.Fatalf("got %d windows, want 1", len(s.Windows))
		}
		w := s.Windows[0]
		if w.Index != 0 || w.Name != "main" || w.Layout != "b25f,200x50,0,0" {
			t.Errorf("window header = %+v", w)
		}
		if !w.Active || w.Zoomed {
			t.Errorf("window flags = active:%v zoomed:%v, want true/false", w.Active, w.Zoomed)
		}
		if len(w.Panes) != 1 {
			t.Fatalf("got %d panes, want 1", len(w.Panes))
		}
		p := w.Panes[0]
		if p.Index != 0 || p.CWD != "/Users/leeovery/Code/portal" || !p.Active || p.CurrentCommand != "zsh" {
			t.Errorf("pane = %+v", p)
		}
		if p.ScrollbackFile != "scrollback/work__0.0.bin" {
			t.Errorf("scrollback_file = %q, want %q", p.ScrollbackFile, "scrollback/work__0.0.bin")
		}
	})

	t.Run("filters sessions whose names begin with underscore", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("_portal-saver", "work"),
			listPanes: strings.Join([]string{
				paneLine("_portal-saver", 0, "main", "L1", false, true, 0, "/", true, "portal"),
				paneLine("work", 0, "main", "L2", false, true, 0, "/tmp", true, "zsh"),
			}, "\n"),
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(idx.Sessions) != 1 {
			t.Fatalf("got %d sessions, want 1", len(idx.Sessions))
		}
		if idx.Sessions[0].Name != "work" {
			t.Errorf("session name = %q, want %q", idx.Sessions[0].Name, "work")
		}
	})

	t.Run("returns an empty Sessions slice when zero non-internal sessions exist", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("_portal-saver"),
			t:            t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if idx.Sessions == nil {
			t.Fatal("Sessions is nil; want non-nil empty slice")
		}
		if len(idx.Sessions) != 0 {
			t.Errorf("got %d sessions, want 0", len(idx.Sessions))
		}
	})

	t.Run("captures per-session environment from show-environment", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLine("work", 0, "m", "L", false, true, 0, "/", true, "zsh"),
			envBySession: map[string]string{
				"work": "LANG=en_US.UTF-8\nTERM=xterm-256color",
			},
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		env := idx.Sessions[0].Environment
		if env["LANG"] != "en_US.UTF-8" {
			t.Errorf("env[LANG] = %q, want %q", env["LANG"], "en_US.UTF-8")
		}
		if env["TERM"] != "xterm-256color" {
			t.Errorf("env[TERM] = %q, want %q", env["TERM"], "xterm-256color")
		}
	})

	t.Run("ignores removed-form environment entries starting with a dash", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLine("work", 0, "m", "L", false, true, 0, "/", true, "zsh"),
			envBySession: map[string]string{
				"work": "-OLD_VAR\nLANG=en_US.UTF-8",
			},
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		env := idx.Sessions[0].Environment
		if _, present := env["OLD_VAR"]; present {
			t.Errorf("env contains OLD_VAR; removed-form entries must be skipped")
		}
		if _, present := env["-OLD_VAR"]; present {
			t.Errorf("env contains -OLD_VAR; removed-form entries must be skipped")
		}
		if env["LANG"] != "en_US.UTF-8" {
			t.Errorf("env[LANG] = %q, want %q", env["LANG"], "en_US.UTF-8")
		}
	})

	t.Run("returns an empty Environment map when show-environment output is empty", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLine("work", 0, "m", "L", false, true, 0, "/", true, "zsh"),
			envBySession: map[string]string{
				"work": "",
			},
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		env := idx.Sessions[0].Environment
		if env == nil {
			t.Fatal("Environment is nil; want non-nil empty map")
		}
		if len(env) != 0 {
			t.Errorf("got %d env entries, want 0", len(env))
		}
	})

	t.Run("preserves multi-byte UTF-8 characters in session names", func(t *testing.T) {
		const name = "café-проект"
		mock := &captureMock{
			listSessions: listSessionsFor(name),
			listPanes:    paneLine(name, 0, "m", "L", false, true, 0, "/", true, "zsh"),
			t:            t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(idx.Sessions) != 1 || idx.Sessions[0].Name != name {
			t.Fatalf("Sessions[0].Name = %q, want %q", idx.Sessions[0].Name, name)
		}
	})

	t.Run("splits environment lines on the first equals sign only", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLine("work", 0, "m", "L", false, true, 0, "/", true, "zsh"),
			envBySession: map[string]string{
				"work": "X=A=B",
			},
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		env := idx.Sessions[0].Environment
		if env["X"] != "A=B" {
			t.Errorf("env[X] = %q, want %q", env["X"], "A=B")
		}
	})

	t.Run("captures zoomed and active flags per window", func(t *testing.T) {
		// Two windows: w0 active+not-zoomed, w1 not-active+zoomed.
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes: strings.Join([]string{
				paneLine("work", 0, "a", "L0", false, true, 0, "/", true, "zsh"),
				paneLine("work", 1, "b", "L1", true, false, 0, "/", true, "zsh"),
			}, "\n"),
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		windows := idx.Sessions[0].Windows
		if len(windows) != 2 {
			t.Fatalf("got %d windows, want 2", len(windows))
		}
		if !windows[0].Active || windows[0].Zoomed {
			t.Errorf("window[0] = %+v, want active:true zoomed:false", windows[0])
		}
		if windows[1].Active || !windows[1].Zoomed {
			t.Errorf("window[1] = %+v, want active:false zoomed:true", windows[1])
		}
	})

	t.Run("captures cwd, active, and current_command per pane", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes: strings.Join([]string{
				paneLine("work", 0, "m", "L", false, true, 0, "/a", true, "zsh"),
				paneLine("work", 0, "m", "L", false, true, 1, "/b", false, "vim"),
			}, "\n"),
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		panes := idx.Sessions[0].Windows[0].Panes
		if len(panes) != 2 {
			t.Fatalf("got %d panes, want 2", len(panes))
		}
		if panes[0].CWD != "/a" || !panes[0].Active || panes[0].CurrentCommand != "zsh" {
			t.Errorf("pane[0] = %+v", panes[0])
		}
		if panes[1].CWD != "/b" || panes[1].Active || panes[1].CurrentCommand != "vim" {
			t.Errorf("pane[1] = %+v", panes[1])
		}
	})

	t.Run("sets scrollback_file via the canonical sanitizer", func(t *testing.T) {
		// Session name with a forward slash exercises sanitization + collision suffix.
		const session = "foo/bar"
		mock := &captureMock{
			listSessions: listSessionsFor(session),
			listPanes:    paneLine(session, 2, "m", "L", false, true, 3, "/", true, "zsh"),
			t:            t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := idx.Sessions[0].Windows[0].Panes[0].ScrollbackFile
		want := "scrollback/" + state.SanitizePaneKey(session, 2, 3) + ".bin"
		if got != want {
			t.Errorf("ScrollbackFile = %q, want %q", got, want)
		}
	})

	t.Run("sorts sessions alphabetically by name for stable output", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("zeta", "alpha", "mike"),
			listPanes: strings.Join([]string{
				paneLine("zeta", 0, "m", "L", false, true, 0, "/", true, "zsh"),
				paneLine("alpha", 0, "m", "L", false, true, 0, "/", true, "zsh"),
				paneLine("mike", 0, "m", "L", false, true, 0, "/", true, "zsh"),
			}, "\n"),
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var got []string
		for _, s := range idx.Sessions {
			got = append(got, s.Name)
		}
		want := []string{"alpha", "mike", "zeta"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("Sessions[%d].Name = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("sorts windows by index and panes by index ascending", func(t *testing.T) {
		// Emit out-of-order to verify the sort, not the input order.
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes: strings.Join([]string{
				paneLine("work", 1, "b", "L1", false, false, 1, "/b1", false, "zsh"),
				paneLine("work", 0, "a", "L0", false, true, 1, "/a1", false, "zsh"),
				paneLine("work", 1, "b", "L1", false, false, 0, "/b0", true, "zsh"),
				paneLine("work", 0, "a", "L0", false, true, 0, "/a0", true, "zsh"),
			}, "\n"),
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		windows := idx.Sessions[0].Windows
		if len(windows) != 2 {
			t.Fatalf("got %d windows, want 2", len(windows))
		}
		if windows[0].Index != 0 || windows[1].Index != 1 {
			t.Errorf("window indices = [%d, %d], want [0, 1]", windows[0].Index, windows[1].Index)
		}
		w0 := windows[0]
		if len(w0.Panes) != 2 || w0.Panes[0].Index != 0 || w0.Panes[1].Index != 1 {
			t.Errorf("window[0] panes = %+v, want indices 0,1 ascending", w0.Panes)
		}
	})

	t.Run("sets Index.Version to the schema constant", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLine("work", 0, "m", "L", false, true, 0, "/", true, "zsh"),
			t:            t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if idx.Version != state.SchemaVersion {
			t.Errorf("Version = %d, want %d", idx.Version, state.SchemaVersion)
		}
	})

	t.Run("sets SavedAt to UTC within the call window", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLine("work", 0, "m", "L", false, true, 0, "/", true, "zsh"),
			t:            t,
		}
		client := tmux.NewClient(mock)

		before := time.Now().UTC()
		idx, err := state.CaptureStructure(client, nil, nil)
		after := time.Now().UTC()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if idx.SavedAt.Location() != time.UTC {
			t.Errorf("SavedAt.Location() = %v, want UTC", idx.SavedAt.Location())
		}
		if idx.SavedAt.Before(before) || idx.SavedAt.After(after) {
			t.Errorf("SavedAt = %v, want in [%v, %v]", idx.SavedAt, before, after)
		}
	})

	t.Run("returns an error and no partial index when list-panes fails", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanesE:   errors.New("tmux exploded"),
			t:            t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if len(idx.Sessions) != 0 {
			t.Errorf("expected no sessions on error, got %d", len(idx.Sessions))
		}
	})

	t.Run("returns an error when show-environment fails for a session", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLine("work", 0, "m", "L", false, true, 0, "/", true, "zsh"),
			envErrs: map[string]error{
				"work": errors.New("can't find session"),
			},
			t: t,
		}
		client := tmux.NewClient(mock)

		_, err := state.CaptureStructure(client, nil, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// findPane locates the pane in idx by session name, window index, and pane
// index. Returns nil when the path is missing — tests use that to assert
// presence/absence after the merge.
func findPane(idx state.Index, session string, window, pane int) *state.Pane {
	for si := range idx.Sessions {
		s := &idx.Sessions[si]
		if s.Name != session {
			continue
		}
		for wi := range s.Windows {
			w := &s.Windows[wi]
			if w.Index != window {
				continue
			}
			for pi := range w.Panes {
				if w.Panes[pi].Index == pane {
					return &w.Panes[pi]
				}
			}
		}
	}
	return nil
}

func TestCaptureStructureMergeSkippedPanes(t *testing.T) {
	t.Run("preserves prior pane data when its key is in the skip set", func(t *testing.T) {
		// prev has a pane with CWD /old; fresh has a pane at the same key with /new.
		prev := state.Index{
			Version: state.SchemaVersion,
			Sessions: []state.Session{{
				Name:        "work",
				Environment: map[string]string{},
				Windows: []state.Window{{
					Index: 0, Name: "main", Layout: "L", Active: true,
					Panes: []state.Pane{{
						Index:          0,
						CWD:            "/old",
						Active:         true,
						CurrentCommand: "vim",
						ScrollbackFile: "scrollback/work__0.0.bin",
					}},
				}},
			}},
		}
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLine("work", 0, "main", "L", false, true, 0, "/new", true, "zsh"),
			t:            t,
		}
		client := tmux.NewClient(mock)
		skip := map[string]struct{}{
			state.SanitizePaneKey("work", 0, 0): {},
		}

		idx, err := state.CaptureStructure(client, skip, &prev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		p := findPane(idx, "work", 0, 0)
		if p == nil {
			t.Fatalf("missing pane work:0.0 in result")
		}
		if p.CWD != "/old" || p.CurrentCommand != "vim" {
			t.Errorf("pane = %+v, want prev's /old + vim (skip-set wins)", p)
		}
	})

	t.Run("merges a skipped pane's session and window from prev when missing from fresh", func(t *testing.T) {
		// prev has session "old"; fresh capture lists only "new". Skip set marks
		// the prev pane — the merge must reintroduce its session/window/pane
		// into the result.
		prev := state.Index{
			Version: state.SchemaVersion,
			Sessions: []state.Session{{
				Name:        "old",
				Environment: map[string]string{},
				Windows: []state.Window{{
					Index: 1, Name: "win", Layout: "L", Active: true,
					Panes: []state.Pane{{
						Index:          2,
						CWD:            "/prev",
						Active:         true,
						CurrentCommand: "tmux",
						ScrollbackFile: "scrollback/old__1.2.bin",
					}},
				}},
			}},
		}
		mock := &captureMock{
			listSessions: listSessionsFor("new"),
			listPanes:    paneLine("new", 0, "n", "L", false, true, 0, "/new", true, "zsh"),
			t:            t,
		}
		client := tmux.NewClient(mock)
		skip := map[string]struct{}{
			state.SanitizePaneKey("old", 1, 2): {},
		}

		idx, err := state.CaptureStructure(client, skip, &prev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// "new" still present.
		if findPane(idx, "new", 0, 0) == nil {
			t.Errorf("fresh pane new:0.0 missing")
		}
		// "old" reintroduced via merge.
		p := findPane(idx, "old", 1, 2)
		if p == nil {
			t.Fatalf("merged pane old:1.2 missing")
		}
		if p.CWD != "/prev" || p.CurrentCommand != "tmux" {
			t.Errorf("merged pane = %+v, want prev's /prev + tmux", p)
		}
	})

	t.Run("leaves the fresh capture unchanged when skip set is empty", func(t *testing.T) {
		prev := state.Index{
			Version: state.SchemaVersion,
			Sessions: []state.Session{{
				Name:        "old",
				Environment: map[string]string{},
				Windows: []state.Window{{
					Index: 0, Name: "m", Layout: "L", Active: true,
					Panes: []state.Pane{{
						Index: 0, CWD: "/prev", Active: true, CurrentCommand: "vim",
						ScrollbackFile: "scrollback/old__0.0.bin",
					}},
				}},
			}},
		}
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLine("work", 0, "m", "L", false, true, 0, "/new", true, "zsh"),
			t:            t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, map[string]struct{}{}, &prev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "work" {
			t.Errorf("Sessions = %+v, want only fresh 'work'", idx.Sessions)
		}
	})

	t.Run("leaves the fresh capture unchanged when prev is nil", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLine("work", 0, "m", "L", false, true, 0, "/new", true, "zsh"),
			t:            t,
		}
		client := tmux.NewClient(mock)
		// Skip set non-empty but prev is nil — merge has no source data.
		skip := map[string]struct{}{"work__0.0": {}}

		idx, err := state.CaptureStructure(client, skip, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "work" {
			t.Errorf("Sessions = %+v, want only fresh 'work'", idx.Sessions)
		}
		p := findPane(idx, "work", 0, 0)
		if p == nil || p.CWD != "/new" {
			t.Errorf("pane CWD = %v, want /new (no merge applied)", p)
		}
	})

	t.Run("re-sorts sessions, windows, and panes after merge", func(t *testing.T) {
		// prev has a session "alpha" alphabetically before fresh's "zeta",
		// plus a pane at index 5 with a window at index 3. Verify the
		// post-merge ordering is canonical.
		prev := state.Index{
			Version: state.SchemaVersion,
			Sessions: []state.Session{{
				Name:        "alpha",
				Environment: map[string]string{},
				Windows: []state.Window{{
					Index: 3, Name: "w3", Layout: "L", Active: true,
					Panes: []state.Pane{{
						Index: 5, CWD: "/prev", Active: true, CurrentCommand: "vim",
						ScrollbackFile: "scrollback/alpha__3.5.bin",
					}},
				}},
			}},
		}
		mock := &captureMock{
			listSessions: listSessionsFor("zeta"),
			listPanes:    paneLine("zeta", 0, "z", "L", false, true, 0, "/new", true, "zsh"),
			t:            t,
		}
		client := tmux.NewClient(mock)
		skip := map[string]struct{}{
			state.SanitizePaneKey("alpha", 3, 5): {},
		}

		idx, err := state.CaptureStructure(client, skip, &prev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(idx.Sessions) != 2 {
			t.Fatalf("got %d sessions, want 2", len(idx.Sessions))
		}
		if idx.Sessions[0].Name != "alpha" || idx.Sessions[1].Name != "zeta" {
			t.Errorf("session order = [%s, %s], want [alpha, zeta]",
				idx.Sessions[0].Name, idx.Sessions[1].Name)
		}
	})
}
