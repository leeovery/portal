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

	// Call counters — zero-valued by default so existing tests are unaffected.
	// Used by TestCaptureStructurePreLoopFailFatal to assert which pre-loop
	// commands run (and which do not) after each fail-fatal path.
	listSessionsCalls int
	listPanesCalls    int
	showEnvCalls      int
}

func (m *captureMock) Run(args ...string) (string, error) {
	if len(args) == 0 {
		m.t.Fatalf("captureMock invoked with no args")
	}
	switch args[0] {
	case "list-sessions":
		m.listSessionsCalls++
		return m.listSessions, m.listSessionsE
	case "list-panes":
		m.listPanesCalls++
		return m.listPanes, m.listPanesE
	case "show-environment":
		m.showEnvCalls++
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
// numeric fields and the trailing @portal-dir field are placeholders;
// CaptureStructure only consumes the names. The trailing empty field matches
// the 4-field "name|windows|attached|@portal-dir" format ListSessions emits.
func listSessionsFor(names ...string) string {
	lines := make([]string, 0, len(names))
	for _, n := range names {
		lines = append(lines, n+"|1|0|")
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

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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
		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

		_, err := state.CaptureStructure(client, nil, nil, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// noSuchSessionErr returns a *tmux.CommandError whose stderr carries tmux's
// canonical lowercase "no such session" phrasing. The tmux client's
// ShowEnvironment wraps such errors via wrapNoSuchSession so callers see an
// errors.Is(err, tmux.ErrNoSuchSession) match. Anomalous failures (any other
// shape) are not wrapped and must fall into the non-natural-churn branch of
// the per-session loop.
func noSuchSessionErr(session string) error {
	return &tmux.CommandError{
		Stderr: "no such session: " + session,
		Err:    errors.New("exit status 1"),
	}
}

// TestCaptureStructurePerSessionLogAndContinue exercises the per-session
// log-and-continue behaviour introduced by spec § Component E: a single
// failing session must not abort the whole capture. The post-loop
// discriminator returns the partial index unchanged when at least one session
// succeeded, the empty index with nil err when every failure was natural
// churn, and a wrapped error only when zero sessions succeeded and at least
// one failure was anomalous.
func TestCaptureStructurePerSessionLogAndContinue(t *testing.T) {
	t.Run("it skips a failing session and captures the survivors", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("alpha", "bravo", "charlie"),
			listPanes: strings.Join([]string{
				paneLine("alpha", 0, "m", "L", false, true, 0, "/a", true, "zsh"),
				paneLine("bravo", 0, "m", "L", false, true, 0, "/b", true, "zsh"),
				paneLine("charlie", 0, "m", "L", false, true, 0, "/c", true, "zsh"),
			}, "\n"),
			envErrs: map[string]error{
				"alpha": errors.New("boom"),
			},
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(idx.Sessions) != 2 {
			t.Fatalf("got %d sessions, want 2 (alpha skipped)", len(idx.Sessions))
		}
		if idx.Sessions[0].Name != "bravo" || idx.Sessions[1].Name != "charlie" {
			t.Errorf("Sessions = [%s, %s], want [bravo, charlie]",
				idx.Sessions[0].Name, idx.Sessions[1].Name)
		}
	})

	t.Run("it proceeds with empty index when every session is natural churn", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("alpha", "bravo"),
			listPanes: strings.Join([]string{
				paneLine("alpha", 0, "m", "L", false, true, 0, "/a", true, "zsh"),
				paneLine("bravo", 0, "m", "L", false, true, 0, "/b", true, "zsh"),
			}, "\n"),
			envErrs: map[string]error{
				"alpha": noSuchSessionErr("alpha"),
				"bravo": noSuchSessionErr("bravo"),
			},
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil, nil)
		if err != nil {
			t.Fatalf("expected nil error for natural-churn-only, got %v", err)
		}
		if idx.Sessions == nil {
			t.Fatal("Sessions is nil; want non-nil empty slice")
		}
		if len(idx.Sessions) != 0 {
			t.Errorf("got %d sessions, want 0 (all natural churn)", len(idx.Sessions))
		}
	})

	t.Run("it returns an error when every session fails with anomalous errors", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("alpha", "bravo"),
			listPanes: strings.Join([]string{
				paneLine("alpha", 0, "m", "L", false, true, 0, "/a", true, "zsh"),
				paneLine("bravo", 0, "m", "L", false, true, 0, "/b", true, "zsh"),
			}, "\n"),
			envErrs: map[string]error{
				"alpha": errors.New("alpha boom"),
				"bravo": errors.New("bravo boom"),
			},
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil, nil)
		if err == nil {
			t.Fatal("expected non-nil error for all-anomalous, got nil")
		}
		if len(idx.Sessions) != 0 {
			t.Errorf("expected empty Sessions on total-failure error, got %d", len(idx.Sessions))
		}
	})

	t.Run("it returns an error when failure set is mixed natural+anomalous and no session succeeded", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("alpha", "bravo"),
			listPanes: strings.Join([]string{
				paneLine("alpha", 0, "m", "L", false, true, 0, "/a", true, "zsh"),
				paneLine("bravo", 0, "m", "L", false, true, 0, "/b", true, "zsh"),
			}, "\n"),
			envErrs: map[string]error{
				"alpha": noSuchSessionErr("alpha"),
				"bravo": errors.New("anomalous"),
			},
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil, nil)
		if err == nil {
			t.Fatal("expected non-nil error for mixed natural+anomalous with 0 successes, got nil")
		}
		if len(idx.Sessions) != 0 {
			t.Errorf("expected empty Sessions, got %d", len(idx.Sessions))
		}
	})

	t.Run("it returns nil error and partial index when some sessions succeed despite mixed failures", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("alpha", "bravo", "charlie"),
			listPanes: strings.Join([]string{
				paneLine("alpha", 0, "m", "L", false, true, 0, "/a", true, "zsh"),
				paneLine("bravo", 0, "m", "L", false, true, 0, "/b", true, "zsh"),
				paneLine("charlie", 0, "m", "L", false, true, 0, "/c", true, "zsh"),
			}, "\n"),
			envErrs: map[string]error{
				"alpha": noSuchSessionErr("alpha"),
				"bravo": errors.New("anomalous"),
				// charlie succeeds
			},
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil, nil)
		if err != nil {
			t.Fatalf("expected nil error when ≥1 session succeeded, got %v", err)
		}
		if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "charlie" {
			t.Fatalf("Sessions = %+v, want only [charlie]", idx.Sessions)
		}
	})

	t.Run("it emits a WARN log entry per failing session naming the session and error", func(t *testing.T) {
		dir := t.TempDir()
		logger, sink := openTestLogger(t, dir)

		mock := &captureMock{
			listSessions: listSessionsFor("alpha", "bravo", "charlie"),
			listPanes: strings.Join([]string{
				paneLine("alpha", 0, "m", "L", false, true, 0, "/a", true, "zsh"),
				paneLine("bravo", 0, "m", "L", false, true, 0, "/b", true, "zsh"),
				paneLine("charlie", 0, "m", "L", false, true, 0, "/c", true, "zsh"),
			}, "\n"),
			envErrs: map[string]error{
				"alpha": noSuchSessionErr("alpha"),
				"bravo": errors.New("bravo-boom-sentinel"),
			},
			t: t,
		}
		client := tmux.NewClient(mock)

		_, err := state.CaptureStructure(client, nil, nil, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		log := sink.Body()
		// Exactly two WARN entries: one per failing session.
		warnCount := strings.Count(log, "WARN ")
		if warnCount != 2 {
			t.Errorf("WARN entries = %d, want 2; log:\n%s", warnCount, log)
		}
		// Each warn line names its session (via the session attr) and includes
		// the underlying error.
		if !strings.Contains(log, "session=alpha") {
			t.Errorf("expected WARN for session alpha; log:\n%s", log)
		}
		if !strings.Contains(log, "session=bravo") {
			t.Errorf("expected WARN for session bravo; log:\n%s", log)
		}
		if !strings.Contains(log, "bravo-boom-sentinel") {
			t.Errorf("expected anomalous error text in WARN body; log:\n%s", log)
		}
	})

	t.Run("it does not invoke the per-session loop when keep is empty", func(t *testing.T) {
		// keep is empty because the only session is internal-prefixed. The
		// per-session loop must not run (so any envErr for the internal session
		// would never fire); the result is the existing empty-Sessions slice.
		mock := &captureMock{
			listSessions: listSessionsFor("_portal-saver"),
			envErrs: map[string]error{
				"_portal-saver": errors.New("would fail if iterated"),
			},
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil, nil)
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

	t.Run("it preserves canonical ordering of surviving sessions", func(t *testing.T) {
		// Skipping "bravo" must not perturb the alpha→charlie ordering. Mock
		// returns listSessions out of order to confirm the survivor set itself
		// is canonicalised.
		mock := &captureMock{
			listSessions: listSessionsFor("zeta", "alpha", "mike"),
			listPanes: strings.Join([]string{
				paneLine("zeta", 0, "m", "L", false, true, 0, "/z", true, "zsh"),
				paneLine("alpha", 0, "m", "L", false, true, 0, "/a", true, "zsh"),
				paneLine("mike", 0, "m", "L", false, true, 0, "/m", true, "zsh"),
			}, "\n"),
			envErrs: map[string]error{
				"mike": noSuchSessionErr("mike"),
			},
			t: t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(idx.Sessions) != 2 {
			t.Fatalf("got %d sessions, want 2", len(idx.Sessions))
		}
		if idx.Sessions[0].Name != "alpha" || idx.Sessions[1].Name != "zeta" {
			t.Errorf("Sessions = [%s, %s], want [alpha, zeta] (canonical ascending)",
				idx.Sessions[0].Name, idx.Sessions[1].Name)
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

		idx, err := state.CaptureStructure(client, skip, &prev, nil)
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

	t.Run("merges hydrate-in-progress pane from prev at matching coords", func(t *testing.T) {
		// Phase A of restore creates the session in tmux BEFORE setting the
		// @portal-skeleton-<paneKey> marker, so a marker-protected pane in the
		// legitimate hydrate-in-progress flow has its session, window, and pane
		// all present in the fresh enumeration. The structural live-set filter
		// must NOT regress this case: prev's authoritative pane state
		// (CWD/CurrentCommand captured pre-boot) must still win at matching
		// coords, and a session present in BOTH fresh and prev must not be
		// duplicated by the merge. See specification → Fix Component A →
		// Preserved Behavior; Acceptance Criteria #6.
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

		idx, err := state.CaptureStructure(client, skip, &prev, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Prev wins at matching coords: CWD/CurrentCommand are prev's, not
		// fresh's.
		p := findPane(idx, "work", 0, 0)
		if p == nil {
			t.Fatalf("missing pane work:0.0 in result")
		}
		if p.CWD != "/old" || p.CurrentCommand != "vim" {
			t.Errorf("pane = %+v, want prev's /old + vim (skip-set wins at matching coords)", p)
		}

		// No session duplication: the same session present in BOTH fresh and
		// prev appears exactly once.
		if len(idx.Sessions) != 1 {
			t.Errorf("len(idx.Sessions) = %d, want 1 (no duplication when session is in both fresh and prev)", len(idx.Sessions))
		}
		if idx.Sessions[0].Name != "work" {
			t.Errorf("Sessions[0].Name = %q, want %q", idx.Sessions[0].Name, "work")
		}

		// Canonical ordering survives the merge: one window at index 0, one
		// pane at index 0.
		work := idx.Sessions[0]
		if len(work.Windows) != 1 || work.Windows[0].Index != 0 {
			t.Errorf("work.Windows = %+v, want one window at index 0 (canonical ordering)", work.Windows)
		}
		w0 := work.Windows[0]
		if len(w0.Panes) != 1 || w0.Panes[0].Index != 0 {
			t.Errorf("work:0.Panes = %+v, want one pane at index 0 (canonical ordering)", w0.Panes)
		}
	})

	t.Run("does not merge a skipped pane whose session is absent from fresh", func(t *testing.T) {
		// prev has session "old"; fresh capture lists only "new". Skip set
		// marks the prev pane. Even though the marker is present, "old" must
		// NOT be reintroduced into the result because tmux no longer
		// acknowledges the session — a stale skeleton marker cannot resurrect
		// a killed session. See specification → Fix Component A → Filtering
		// Levels.
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

		idx, err := state.CaptureStructure(client, skip, &prev, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// "new" still present.
		if findPane(idx, "new", 0, 0) == nil {
			t.Errorf("fresh pane new:0.0 missing")
		}
		// "old" must NOT be merged — its session is absent from fresh.
		if p := findPane(idx, "old", 1, 2); p != nil {
			t.Errorf("dead session pane old:1.2 was reintroduced via merge: %+v", p)
		}
		if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "new" {
			t.Errorf("Sessions = %+v, want only fresh 'new'", idx.Sessions)
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

		idx, err := state.CaptureStructure(client, map[string]struct{}{}, &prev, nil)
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

		idx, err := state.CaptureStructure(client, skip, nil, nil)
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

	t.Run("does not merge a skipped pane whose window is absent from a live fresh session", func(t *testing.T) {
		// prev session "work" has windows 0 and 5, each with one pane. Fresh
		// has session "work" with only window 0. The skipSet marks window 5's
		// pane. Even though the session is live, the window has been killed
		// (e.g. via tmux kill-window) so the prev pane must NOT be merged
		// — see specification → Fix Component A → Filtering Levels (window
		// level).
		prev := state.Index{
			Version: state.SchemaVersion,
			Sessions: []state.Session{{
				Name:        "work",
				Environment: map[string]string{},
				Windows: []state.Window{
					{
						Index: 0, Name: "main", Layout: "L0", Active: true,
						Panes: []state.Pane{{
							Index:          0,
							CWD:            "/old0",
							Active:         true,
							CurrentCommand: "vim",
							ScrollbackFile: "scrollback/work__0.0.bin",
						}},
					},
					{
						Index: 5, Name: "stale", Layout: "L5", Active: false,
						Panes: []state.Pane{{
							Index:          0,
							CWD:            "/old5",
							Active:         true,
							CurrentCommand: "tmux",
							ScrollbackFile: "scrollback/work__5.0.bin",
						}},
					},
				},
			}},
		}
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLine("work", 0, "main", "L0", false, true, 0, "/new", true, "zsh"),
			t:            t,
		}
		client := tmux.NewClient(mock)
		skip := map[string]struct{}{
			state.SanitizePaneKey("work", 5, 0): {},
		}

		idx, err := state.CaptureStructure(client, skip, &prev, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Stale window 5 must not appear in the merged result.
		if p := findPane(idx, "work", 5, 0); p != nil {
			t.Errorf("dead window pane work:5.0 was reintroduced via merge: %+v", p)
		}
		if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "work" {
			t.Fatalf("Sessions = %+v, want only fresh 'work'", idx.Sessions)
		}
		work := idx.Sessions[0]
		if len(work.Windows) != 1 || work.Windows[0].Index != 0 {
			t.Errorf("work windows = %+v, want only [0]", work.Windows)
		}
	})

	t.Run("drops only stale windows from a mixed prev session", func(t *testing.T) {
		// prev session "work" has windows 0 (live in fresh) and 7 (absent).
		// Both panes are in skipSet. Window 0's pane must merge with prev's
		// authoritative CWD/CurrentCommand; window 7 must be dropped entirely.
		prev := state.Index{
			Version: state.SchemaVersion,
			Sessions: []state.Session{{
				Name:        "work",
				Environment: map[string]string{},
				Windows: []state.Window{
					{
						Index: 0, Name: "main", Layout: "L0", Active: true,
						Panes: []state.Pane{{
							Index:          0,
							CWD:            "/prev0",
							Active:         true,
							CurrentCommand: "vim",
							ScrollbackFile: "scrollback/work__0.0.bin",
						}},
					},
					{
						Index: 7, Name: "stale", Layout: "L7", Active: false,
						Panes: []state.Pane{{
							Index:          0,
							CWD:            "/prev7",
							Active:         true,
							CurrentCommand: "tmux",
							ScrollbackFile: "scrollback/work__7.0.bin",
						}},
					},
				},
			}},
		}
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLine("work", 0, "main", "L0", false, true, 0, "/fresh0", true, "zsh"),
			t:            t,
		}
		client := tmux.NewClient(mock)
		skip := map[string]struct{}{
			state.SanitizePaneKey("work", 0, 0): {},
			state.SanitizePaneKey("work", 7, 0): {},
		}

		idx, err := state.CaptureStructure(client, skip, &prev, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Live-window pane retains prev's authoritative data.
		p0 := findPane(idx, "work", 0, 0)
		if p0 == nil {
			t.Fatalf("live-window pane work:0.0 missing from merge")
		}
		if p0.CWD != "/prev0" || p0.CurrentCommand != "vim" {
			t.Errorf("work:0.0 = %+v, want prev's /prev0 + vim", p0)
		}
		// Stale-window pane must not be reintroduced.
		if p7 := findPane(idx, "work", 7, 0); p7 != nil {
			t.Errorf("dead window pane work:7.0 was reintroduced via merge: %+v", p7)
		}
		if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "work" {
			t.Fatalf("Sessions = %+v, want only 'work'", idx.Sessions)
		}
		work := idx.Sessions[0]
		if len(work.Windows) != 1 || work.Windows[0].Index != 0 {
			t.Errorf("work windows = %+v, want only [0]", work.Windows)
		}
	})

	t.Run("canonical ordering preserved after window-level drop", func(t *testing.T) {
		// prev contributes panes for two live windows out-of-order plus a
		// stale window that must be dropped. After merge, surviving windows
		// must be sorted ascending by index.
		prev := state.Index{
			Version: state.SchemaVersion,
			Sessions: []state.Session{{
				Name:        "work",
				Environment: map[string]string{},
				Windows: []state.Window{
					{
						Index: 9, Name: "stale", Layout: "L9", Active: false,
						Panes: []state.Pane{{
							Index: 0, CWD: "/prev9", Active: true, CurrentCommand: "tmux",
							ScrollbackFile: "scrollback/work__9.0.bin",
						}},
					},
					{
						Index: 2, Name: "two", Layout: "L2", Active: false,
						Panes: []state.Pane{{
							Index: 0, CWD: "/prev2", Active: true, CurrentCommand: "vim",
							ScrollbackFile: "scrollback/work__2.0.bin",
						}},
					},
					{
						Index: 0, Name: "main", Layout: "L0", Active: true,
						Panes: []state.Pane{{
							Index: 0, CWD: "/prev0", Active: true, CurrentCommand: "zsh",
							ScrollbackFile: "scrollback/work__0.0.bin",
						}},
					},
				},
			}},
		}
		// Fresh has live windows 0 and 2 only; window 9 is dead.
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes: strings.Join([]string{
				paneLine("work", 2, "two", "L2", false, false, 0, "/fresh2", true, "zsh"),
				paneLine("work", 0, "main", "L0", false, true, 0, "/fresh0", true, "zsh"),
			}, "\n"),
			t: t,
		}
		client := tmux.NewClient(mock)
		skip := map[string]struct{}{
			state.SanitizePaneKey("work", 0, 0): {},
			state.SanitizePaneKey("work", 2, 0): {},
			state.SanitizePaneKey("work", 9, 0): {},
		}

		idx, err := state.CaptureStructure(client, skip, &prev, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "work" {
			t.Fatalf("Sessions = %+v, want only 'work'", idx.Sessions)
		}
		work := idx.Sessions[0]
		if len(work.Windows) != 2 {
			t.Fatalf("work windows = %+v, want 2 (stale window 9 dropped)", work.Windows)
		}
		if work.Windows[0].Index != 0 || work.Windows[1].Index != 2 {
			t.Errorf("window order = [%d, %d], want [0, 2]",
				work.Windows[0].Index, work.Windows[1].Index)
		}
	})

	t.Run("does not merge a skipped pane whose pane index is absent from a live fresh window", func(t *testing.T) {
		// prev session "work" window 0 has panes 0 and 1. Fresh has session
		// "work" window 0 with only pane 0. The skipSet marks pane 1. Even
		// though session and window are live, pane 1 has been killed (e.g.
		// via tmux kill-pane) so prev's pane 1 must NOT be merged — see
		// specification → Fix Component A → Filtering Levels (pane level).
		prev := state.Index{
			Version: state.SchemaVersion,
			Sessions: []state.Session{{
				Name:        "work",
				Environment: map[string]string{},
				Windows: []state.Window{{
					Index: 0, Name: "main", Layout: "L0", Active: true,
					Panes: []state.Pane{
						{
							Index:          0,
							CWD:            "/old0",
							Active:         true,
							CurrentCommand: "vim",
							ScrollbackFile: "scrollback/work__0.0.bin",
						},
						{
							Index:          1,
							CWD:            "/old1",
							Active:         false,
							CurrentCommand: "tmux",
							ScrollbackFile: "scrollback/work__0.1.bin",
						},
					},
				}},
			}},
		}
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    paneLine("work", 0, "main", "L0", false, true, 0, "/new", true, "zsh"),
			t:            t,
		}
		client := tmux.NewClient(mock)
		skip := map[string]struct{}{
			state.SanitizePaneKey("work", 0, 1): {},
		}

		idx, err := state.CaptureStructure(client, skip, &prev, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Stale pane 1 must not appear in the merged result.
		if p := findPane(idx, "work", 0, 1); p != nil {
			t.Errorf("dead pane work:0.1 was reintroduced via merge: %+v", p)
		}
		if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "work" {
			t.Fatalf("Sessions = %+v, want only 'work'", idx.Sessions)
		}
		work := idx.Sessions[0]
		if len(work.Windows) != 1 || work.Windows[0].Index != 0 {
			t.Fatalf("work windows = %+v, want only [0]", work.Windows)
		}
		w0 := work.Windows[0]
		if len(w0.Panes) != 1 || w0.Panes[0].Index != 0 {
			t.Errorf("work:0 panes = %+v, want only [0]", w0.Panes)
		}
	})

	t.Run("canonical ordering preserved after pane-level drop", func(t *testing.T) {
		// prev contributes panes for one live window: pane 0 (live in fresh),
		// pane 2 (live in fresh) out-of-order, plus pane 9 (absent from
		// fresh). After merge, surviving panes must be sorted ascending by
		// index and the dead pane must be dropped.
		prev := state.Index{
			Version: state.SchemaVersion,
			Sessions: []state.Session{{
				Name:        "work",
				Environment: map[string]string{},
				Windows: []state.Window{{
					Index: 0, Name: "main", Layout: "L0", Active: true,
					Panes: []state.Pane{
						{
							Index: 9, CWD: "/prev9", Active: false, CurrentCommand: "tmux",
							ScrollbackFile: "scrollback/work__0.9.bin",
						},
						{
							Index: 2, CWD: "/prev2", Active: false, CurrentCommand: "vim",
							ScrollbackFile: "scrollback/work__0.2.bin",
						},
						{
							Index: 0, CWD: "/prev0", Active: true, CurrentCommand: "zsh",
							ScrollbackFile: "scrollback/work__0.0.bin",
						},
					},
				}},
			}},
		}
		// Fresh has live panes 0 and 2 only; pane 9 is dead.
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes: strings.Join([]string{
				paneLine("work", 0, "main", "L0", false, true, 2, "/fresh2", false, "zsh"),
				paneLine("work", 0, "main", "L0", false, true, 0, "/fresh0", true, "zsh"),
			}, "\n"),
			t: t,
		}
		client := tmux.NewClient(mock)
		skip := map[string]struct{}{
			state.SanitizePaneKey("work", 0, 0): {},
			state.SanitizePaneKey("work", 0, 2): {},
			state.SanitizePaneKey("work", 0, 9): {},
		}

		idx, err := state.CaptureStructure(client, skip, &prev, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "work" {
			t.Fatalf("Sessions = %+v, want only 'work'", idx.Sessions)
		}
		work := idx.Sessions[0]
		if len(work.Windows) != 1 || work.Windows[0].Index != 0 {
			t.Fatalf("work windows = %+v, want only [0]", work.Windows)
		}
		w0 := work.Windows[0]
		if len(w0.Panes) != 2 {
			t.Fatalf("work:0 panes = %+v, want 2 (stale pane 9 dropped)", w0.Panes)
		}
		if w0.Panes[0].Index != 0 || w0.Panes[1].Index != 2 {
			t.Errorf("pane order = [%d, %d], want [0, 2]",
				w0.Panes[0].Index, w0.Panes[1].Index)
		}
	})

	t.Run("self-heals after a killed mid-flight session leaks into prev", func(t *testing.T) {
		// Empirical-scenario regression test mirroring the live-in-the-wild
		// case (e.g. agentic-workflows-XXrJ3J): a session is captured into
		// prev, has a stale @portal-skeleton-* marker (its paneKey is in
		// skipSet), and is then killed in tmux. The next daemon tick sees
		// the marker but the fresh enumeration omits the session. The merge
		// must NOT reintroduce the killed session — see specification →
		// Empirical Confirmation; Acceptance Criteria #1.
		//
		// Prev-population is LOAD-BEARING: mergeSkippedPanes is gated on
		// `prev != nil` and only resurrects sessions present in
		// prev.Sessions. Without seeding prev with the killed session this
		// test would pass on the buggy code (false-green). Seeding it forces
		// the merge layer to make the structural live-set decision.
		//
		// Tick 2 then threads the just-returned (clean) idx back in as prev
		// with the same skipSet, mirroring the daemon's
		// `deps.PrevIndex = &idx` line at cmd/state_daemon.go:156. This
		// asserts the self-healing behaviour: even if the marker persists,
		// once the dead session is gone from prev it stays gone — see
		// specification → Self-Healing Behavior; Acceptance Criteria #3.
		const killed = "agentic-workflows-XXrJ3J"
		const survivor = "survivor"

		prev := state.Index{
			Version: state.SchemaVersion,
			Sessions: []state.Session{{
				Name:        killed,
				Environment: map[string]string{},
				Windows: []state.Window{{
					Index: 1, Name: "main", Layout: "L", Active: true,
					Panes: []state.Pane{{
						Index:          1,
						CWD:            "/old",
						Active:         true,
						CurrentCommand: "vim",
						ScrollbackFile: "scrollback/" + state.SanitizePaneKey(killed, 1, 1) + ".bin",
					}},
				}},
			}},
		}
		mock := &captureMock{
			listSessions: listSessionsFor(survivor),
			listPanes:    paneLine(survivor, 0, "main", "L", false, true, 0, "/new", true, "zsh"),
			t:            t,
		}
		client := tmux.NewClient(mock)
		skip := map[string]struct{}{
			state.SanitizePaneKey(killed, 1, 1): {},
		}

		// Tick 1: marker is set, session has been killed in tmux, prev still
		// contains it. Fresh enumeration omits the killed session.
		idx, err := state.CaptureStructure(client, skip, &prev, nil)
		if err != nil {
			t.Fatalf("tick 1: unexpected error: %v", err)
		}
		if p := findPane(idx, killed, 1, 1); p != nil {
			t.Errorf("tick 1: killed session %q reintroduced via merge: %+v", killed, p)
		}
		if findPane(idx, survivor, 0, 0) == nil {
			t.Errorf("tick 1: survivor session %q missing from result", survivor)
		}
		if len(idx.Sessions) != 1 || idx.Sessions[0].Name != survivor {
			t.Fatalf("tick 1: Sessions = %+v, want only %q", idx.Sessions, survivor)
		}

		// Tick 2: re-use the just-returned clean idx as prev (same skipSet).
		// The mock is reused unchanged, so tmux's reported state is
		// identical between ticks — only prev differs. Even with the marker
		// still present, the killed session must remain absent —
		// sessions.json self-heals on the next tick because the polluted
		// prev was discarded.
		idx2, err := state.CaptureStructure(client, skip, &idx, nil)
		if err != nil {
			t.Fatalf("tick 2: unexpected error: %v", err)
		}
		if p := findPane(idx2, killed, 1, 1); p != nil {
			t.Errorf("tick 2 (self-heal): killed session %q reintroduced: %+v", killed, p)
		}
		if findPane(idx2, survivor, 0, 0) == nil {
			t.Errorf("tick 2: survivor session %q missing from result", survivor)
		}
		if len(idx2.Sessions) != 1 || idx2.Sessions[0].Name != survivor {
			t.Errorf("tick 2: Sessions = %+v, want only %q", idx2.Sessions, survivor)
		}
	})

	t.Run("re-sorts sessions, windows, and panes after merge", func(t *testing.T) {
		// Fresh emits "zeta" before "alpha" and out-of-order windows/panes
		// within each session. Prev contributes prev-authoritative pane data
		// at one set of coords (zeta:0.0) so the merge mutates fresh; the
		// canonical post-merge ordering must be sessions ascending by name,
		// windows ascending by index, panes ascending by index. Both sessions
		// are live in fresh so the session-level filter does not drop the
		// merge — the test exercises sort-order, not the filter.
		prev := state.Index{
			Version: state.SchemaVersion,
			Sessions: []state.Session{{
				Name:        "zeta",
				Environment: map[string]string{},
				Windows: []state.Window{{
					Index: 0, Name: "z", Layout: "L", Active: true,
					Panes: []state.Pane{{
						Index: 0, CWD: "/prev", Active: true, CurrentCommand: "vim",
						ScrollbackFile: "scrollback/zeta__0.0.bin",
					}},
				}},
			}},
		}
		mock := &captureMock{
			listSessions: listSessionsFor("zeta", "alpha"),
			listPanes: strings.Join([]string{
				// Out-of-order window indices and pane indices to verify sort.
				paneLine("zeta", 1, "z1", "L", false, false, 1, "/z1.1", false, "zsh"),
				paneLine("zeta", 1, "z1", "L", false, false, 0, "/z1.0", true, "zsh"),
				paneLine("zeta", 0, "z0", "L", false, true, 0, "/z0.0", true, "zsh"),
				paneLine("alpha", 0, "a", "L", false, true, 0, "/a", true, "zsh"),
			}, "\n"),
			t: t,
		}
		client := tmux.NewClient(mock)
		skip := map[string]struct{}{
			state.SanitizePaneKey("zeta", 0, 0): {},
		}

		idx, err := state.CaptureStructure(client, skip, &prev, nil)
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
		zeta := idx.Sessions[1]
		if len(zeta.Windows) != 2 || zeta.Windows[0].Index != 0 || zeta.Windows[1].Index != 1 {
			t.Errorf("zeta window order = %+v, want indices [0, 1]", zeta.Windows)
		}
		w1 := zeta.Windows[1]
		if len(w1.Panes) != 2 || w1.Panes[0].Index != 0 || w1.Panes[1].Index != 1 {
			t.Errorf("zeta window 1 pane order = %+v, want indices [0, 1]", w1.Panes)
		}
		// Skip-set authority is preserved at matching coords: prev's CWD wins
		// for zeta:0.0.
		p := findPane(idx, "zeta", 0, 0)
		if p == nil || p.CWD != "/prev" || p.CurrentCommand != "vim" {
			t.Errorf("zeta:0.0 = %+v, want prev's /prev + vim", p)
		}
	})
}

// failFastCaptureClient is a CaptureClient implementation that lets a test
// drive an error from ListSessionNames without going through *tmux.Client
// (which swallows list-sessions exec errors). ListAllPanesWithFormat and
// ShowEnvironment t.Fatalf if invoked — they MUST NOT run after a pre-loop
// fail-fatal on ListSessionNames.
type failFastCaptureClient struct {
	t                   *testing.T
	listSessionNames    []string
	listSessionNamesErr error
}

func (f *failFastCaptureClient) ListSessionNames() ([]string, error) {
	return f.listSessionNames, f.listSessionNamesErr
}

func (f *failFastCaptureClient) ListAllPanesWithFormat(format string) (string, error) {
	f.t.Fatalf("ListAllPanesWithFormat unexpectedly called with format %q", format)
	return "", nil
}

func (f *failFastCaptureClient) ShowEnvironment(session string) (string, error) {
	f.t.Fatalf("ShowEnvironment unexpectedly called for session %q", session)
	return "", nil
}

// TestCaptureStructurePreLoopFailFatal pins the invariant from spec § Component
// E (lines 315, 339): the pre-loop tmux calls — ListSessionNames,
// ListAllPanesWithFormat, and parsePaneRows — MUST remain fail-fatal. A
// regression that relaxed any of these to log-and-continue would silently
// produce partial or empty indexes that the daemon would then Commit, wiping
// scrollback. These tests are orthogonal to the per-session log-and-continue
// behaviour exercised by TestCaptureStructurePerSessionLogAndContinue: they
// assert the pre-loop discipline that protects the per-session loop from ever
// running on a broken capture.
func TestCaptureStructurePreLoopFailFatal(t *testing.T) {
	t.Run("it returns an error when ListSessionNames fails and does not call show-environment", func(t *testing.T) {
		// Bypass tmux.NewClient — *tmux.Client.ListSessions swallows the
		// underlying exec error (returns []Session{}, nil) so it cannot
		// drive a ListSessionNames failure path. The CaptureStructure /
		// CaptureClient contract is what we're pinning here: if the client
		// returns an error from ListSessionNames, CaptureStructure must
		// fail-fatal and skip downstream commands.
		client := &failFastCaptureClient{
			t:                   t,
			listSessionNamesErr: errors.New("exec: tmux broken"),
		}

		idx, err := state.CaptureStructure(client, nil, nil, nil)
		if err == nil {
			t.Fatal("expected error from ListSessionNames failure, got nil")
		}
		if len(idx.Sessions) != 0 {
			t.Errorf("expected empty Sessions on pre-loop fail-fatal, got %d", len(idx.Sessions))
		}
		// Pre-loop fail-fatal on dispatch: tmux is broken, so no downstream
		// commands run. The failFastCaptureClient t.Fatalfs if either is
		// invoked.
	})

	t.Run("it returns an error when ListAllPanesWithFormat fails with non-empty keep", func(t *testing.T) {
		mock := &captureMock{
			listSessions: listSessionsFor("work", "side"),
			listPanesE:   errors.New("list-panes failed"),
			t:            t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil, nil)
		if err == nil {
			t.Fatal("expected error from ListAllPanesWithFormat failure, got nil")
		}
		if len(idx.Sessions) != 0 {
			t.Errorf("expected empty Sessions on pre-loop fail-fatal, got %d", len(idx.Sessions))
		}
		// list-panes was attempted (and failed); the per-session loop must
		// not have been entered.
		if mock.listPanesCalls != 1 {
			t.Errorf("list-panes calls = %d, want 1", mock.listPanesCalls)
		}
		if mock.showEnvCalls != 0 {
			t.Errorf("show-environment calls = %d, want 0 (must not run after ListAllPanesWithFormat failure)", mock.showEnvCalls)
		}
	})

	t.Run("it returns an error when parsePaneRows hits a malformed row", func(t *testing.T) {
		// Malformed row: too few "|||"-separated fields. captureFormat
		// expects 10; this emits 3. parsePaneRows must return an error and
		// CaptureStructure must propagate it before the per-session loop.
		mock := &captureMock{
			listSessions: listSessionsFor("work"),
			listPanes:    "work|||0|||main",
			t:            t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil, nil)
		if err == nil {
			t.Fatal("expected error from parsePaneRows on malformed row, got nil")
		}
		if len(idx.Sessions) != 0 {
			t.Errorf("expected empty Sessions on pre-loop fail-fatal, got %d", len(idx.Sessions))
		}
		if mock.showEnvCalls != 0 {
			t.Errorf("show-environment calls = %d, want 0 (must not run after parsePaneRows failure)", mock.showEnvCalls)
		}
	})

	t.Run("it returns an empty index with nil error when keep is empty after filtering", func(t *testing.T) {
		// Only internal-prefixed sessions present: keep is empty after the
		// internalSessionPrefix filter. The pre-loop list-panes call is
		// skipped (gated on len(keep) > 0) and the per-session loop never
		// runs. Result is the canonical empty index with nil err.
		mock := &captureMock{
			listSessions: listSessionsFor("_portal-saver"),
			t:            t,
		}
		client := tmux.NewClient(mock)

		idx, err := state.CaptureStructure(client, nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error when keep is empty: %v", err)
		}
		if idx.Sessions == nil {
			t.Fatal("Sessions is nil; want non-nil empty slice")
		}
		if len(idx.Sessions) != 0 {
			t.Errorf("got %d sessions, want 0", len(idx.Sessions))
		}
		if mock.listPanesCalls != 0 {
			t.Errorf("list-panes calls = %d, want 0 (must skip when keep is empty)", mock.listPanesCalls)
		}
		if mock.showEnvCalls != 0 {
			t.Errorf("show-environment calls = %d, want 0 (must skip when keep is empty)", mock.showEnvCalls)
		}
	})
}
