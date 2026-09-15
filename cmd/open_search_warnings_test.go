package cmd

import (
	"bytes"
	"context"
	"errors"
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/leeovery/portal/internal/warning"
	"github.com/spf13/cobra"
)

// soakedWarnings is the accumulated soft bootstrap output a search form is owed
// on the routes that end without a picker.
func soakedWarnings() []warning.Warning {
	return []warning.Warning{
		{Lines: []string{"Portal's session saver is not running.", "Run portal doctor for details."}},
		{Lines: []string{"Saved state could not be restored."}},
	}
}

// wantWarningOutput is the CLI path's own rendering of the same warnings, which
// every other route must match byte for byte.
func wantWarningOutput(warnings []warning.Warning) string {
	var buf bytes.Buffer
	warning.WriteLines(&buf, warnings)
	return buf.String()
}

func accumulateWarnings(t *testing.T, warnings []warning.Warning) {
	t.Helper()
	resetBootstrapWarnings(t)
	for _, w := range warnings {
		bootstrapWarnings.Add(w)
	}
}

// warmSearchCommand is the command a warm search form runs under: no deferred
// bootstrap in its context, and a buffer standing in for stderr.
func warmSearchCommand(t *testing.T) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	c := searchCommand(context.Background())
	var buf bytes.Buffer
	c.SetErr(&buf)
	return c, &buf
}

// searchTeardownModel drives a picker-routed search to the gate where its
// decision resolves, carrying the warnings the concurrent route buffers on the
// model rather than in the sink.
func searchTeardownModel(t *testing.T, decide func() (string, error), warnings []warning.Warning) tui.Model {
	t.Helper()

	receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })
	var model tea.Model = tui.Build(tui.Deps{
		Lister:           &mockSessionLister{},
		ServerStarted:    true,
		ProgressReceiver: receiver,
		Search:           &tui.SearchForm{Term: "port", Decide: decide},
	})
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = driveLoadingGates(t, model, warnings)

	m, ok := model.(tui.Model)
	if !ok {
		t.Fatalf("model type = %T, want tui.Model", model)
	}
	return m
}

func TestSearchForm_WarmRoute_WritesAccumulatedWarnings(t *testing.T) {
	t.Run("it writes the accumulated warnings before a warm single-match attach", func(t *testing.T) {
		warnings := soakedWarnings()
		sc := installSearchFormSeams(t, nil)
		sc.source.sessions = []tmux.Session{{Name: "portal-a1b2"}, {Name: "blog-c3d4"}}
		accumulateWarnings(t, warnings)
		cmd, stderr := warmSearchCommand(t)

		var stderrAtAttach string
		withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, name string) error {
			stderrAtAttach = stderr.String()
			sc.attached = name
			return nil
		})

		if err := runSearchForm(cmd, "port"); err != nil {
			t.Fatalf("runSearchForm: %v", err)
		}

		if sc.attached != "portal-a1b2" {
			t.Fatalf("attached session = %q, want %q", sc.attached, "portal-a1b2")
		}
		if want := wantWarningOutput(warnings); stderrAtAttach != want {
			t.Errorf("stderr at attach = %q, want %q", stderrAtAttach, want)
		}
	})

	t.Run("it writes the same lines as the CLI path", func(t *testing.T) {
		warnings := soakedWarnings()
		sc := installSearchFormSeams(t, nil)
		sc.source.sessions = []tmux.Session{{Name: "portal-a1b2"}}
		accumulateWarnings(t, warnings)
		cmd, stderr := warmSearchCommand(t)

		if err := runSearchForm(cmd, "port"); err != nil {
			t.Fatalf("runSearchForm: %v", err)
		}

		if want := wantWarningOutput(warnings); stderr.String() != want {
			t.Errorf("stderr = %q, want the CLI path's %q", stderr.String(), want)
		}
	})

	t.Run("it drains the sink so a later emit writes nothing", func(t *testing.T) {
		sc := installSearchFormSeams(t, nil)
		sc.source.sessions = []tmux.Session{{Name: "portal-a1b2"}}
		accumulateWarnings(t, soakedWarnings())
		cmd, _ := warmSearchCommand(t)

		if err := runSearchForm(cmd, "port"); err != nil {
			t.Fatalf("runSearchForm: %v", err)
		}

		var second bytes.Buffer
		bootstrapWarnings.EmitTo(&second)
		if second.Len() != 0 {
			t.Errorf("a later emit wrote %q, want nothing — the sink must be drained", second.String())
		}
	})

	t.Run("it writes nothing when no warnings accumulated", func(t *testing.T) {
		sc := installSearchFormSeams(t, nil)
		sc.source.sessions = []tmux.Session{{Name: "portal-a1b2"}}
		accumulateWarnings(t, nil)
		cmd, stderr := warmSearchCommand(t)

		if err := runSearchForm(cmd, "port"); err != nil {
			t.Fatalf("runSearchForm: %v", err)
		}

		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want nothing", stderr.String())
		}
	})

	t.Run("it writes the accumulated warnings before a warm failed read", func(t *testing.T) {
		readErr := errors.New("no server running")
		warnings := soakedWarnings()
		sc := installSearchFormSeams(t, nil)
		sc.source.listErr = readErr
		accumulateWarnings(t, warnings)
		cmd, stderr := warmSearchCommand(t)

		err := runSearchForm(cmd, "port")

		if !errors.Is(err, readErr) {
			t.Fatalf("runSearchForm error = %v, want %v", err, readErr)
		}
		if want := wantWarningOutput(warnings); stderr.String() != want {
			t.Errorf("stderr = %q, want %q written before the error surfaces", stderr.String(), want)
		}
	})

	t.Run("it leaves the picker branch's warnings for the model", func(t *testing.T) {
		warnings := soakedWarnings()
		sc := installSearchFormSeams(t, nil)
		sc.source.sessions = []tmux.Session{{Name: "portal-a1b2"}, {Name: "port-agent"}}
		accumulateWarnings(t, warnings)
		cmd, stderr := warmSearchCommand(t)

		if err := runSearchForm(cmd, "port"); err != nil {
			t.Fatalf("runSearchForm: %v", err)
		}

		if !sc.tuiCalled {
			t.Fatal("two matches must open the picker")
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want nothing — a picker invocation surfaces them in the notice band", stderr.String())
		}
		var drained bytes.Buffer
		bootstrapWarnings.EmitTo(&drained)
		if want := wantWarningOutput(warnings); drained.String() != want {
			t.Errorf("sink after a picker branch = %q, want the warnings still staged for the model (%q)", drained.String(), want)
		}
	})
}

func TestWarningsOwedAtTeardown(t *testing.T) {
	warnings := soakedWarnings()

	assertOwed := func(t *testing.T, owed, want []warning.Warning, reason string) {
		t.Helper()
		got, wantRendered := wantWarningOutput(owed), wantWarningOutput(want)
		if got != wantRendered {
			t.Errorf("owed = %q, want %q — %s", got, wantRendered, reason)
		}
	}

	t.Run("it owes the buffered set when the loading gate quit on a named session", func(t *testing.T) {
		model := searchTeardownModel(t, func() (string, error) { return "portal-a1b2", nil }, warnings)
		if !model.SearchAttached() {
			t.Fatal("model must record the search attach")
		}

		assertOwed(t, model.WarningsOwedAtTeardown(), warnings, "the attach paints no picker frame")
	})

	t.Run("it still owes a staged warning at teardown when the gate quits on a search attach", func(t *testing.T) {
		staged := []warning.Warning{{Lines: []string{"Portal's session saver is not running."}}}
		orchestrator := []warning.Warning{{Lines: []string{"Saved state could not be restored."}}}

		receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })
		m := tui.Build(tui.Deps{
			Lister:           &mockSessionLister{},
			ServerStarted:    true,
			ProgressReceiver: receiver,
			Search:           &tui.SearchForm{Term: "port", Decide: func() (string, error) { return "portal-a1b2", nil }},
		})
		m.SetPendingBootstrapWarnings(staged)

		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model = driveLoadingGates(t, model, orchestrator)

		updated := model.(tui.Model)
		if !updated.SearchAttached() {
			t.Fatal("model must record the search attach")
		}

		want := append(append([]warning.Warning{}, staged...), orchestrator...)
		assertOwed(t, updated.WarningsOwedAtTeardown(), want, "a warning staged before the concurrent launch is owed too")
	})

	t.Run("it owes the buffered set when the loading gate quit on a failed session-list read", func(t *testing.T) {
		readErr := errors.New("no server running")
		model := searchTeardownModel(t, func() (string, error) { return "", readErr }, warnings)
		if !errors.Is(model.SearchError(), readErr) {
			t.Fatalf("SearchError() = %v, want %v", model.SearchError(), readErr)
		}

		assertOwed(t, model.WarningsOwedAtTeardown(), warnings, "the failed read paints no picker frame")
	})

	t.Run("it owes the pending set when a picker frame was painted", func(t *testing.T) {
		model := warmPickerModel(t, tui.Deps{}, warnings)

		assertOwed(t, model.WarningsOwedAtTeardown(), warnings, "a warm picker has no loading gate to consume them")
	})

	t.Run("it owes nothing once a loading gate has surfaced the buffer", func(t *testing.T) {
		model := searchTeardownModel(t, func() (string, error) { return "", nil }, warnings)
		if model.SearchAttached() || model.SearchError() != nil {
			t.Fatal("a picker decision records neither an attach nor an error")
		}

		assertOwed(t, model.WarningsOwedAtTeardown(), nil, "the notice band owns a picker's warnings")
	})

	t.Run("it owes nothing when no warnings accumulated", func(t *testing.T) {
		model := searchTeardownModel(t, func() (string, error) { return "portal-a1b2", nil }, nil)

		assertOwed(t, model.WarningsOwedAtTeardown(), nil, "nothing was accumulated to owe")
	})

	t.Run("it owes the buffered set when the loading page was cancelled", func(t *testing.T) {
		receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })
		var model tea.Model = tui.Build(tui.Deps{
			Lister:           &mockSessionLister{},
			ServerStarted:    true,
			ProgressReceiver: receiver,
			Search:           &tui.SearchForm{Term: "port", Decide: func() (string, error) { return "portal-a1b2", nil }},
		})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.BootstrapCompleteMsg{Warnings: warnings})
		model, _ = model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

		m := model.(tui.Model)
		if len(m.BufferedWarnings()) == 0 {
			t.Fatal("the cancelled loading page must still be holding its buffered warnings")
		}
		if m.SearchAttached() || m.SearchError() != nil {
			t.Fatal("a cancelled loading page records neither an attach nor an error")
		}

		assertOwed(t, m.WarningsOwedAtTeardown(), warnings, "no surface ever showed what the gate took")
	})

	t.Run("it owes the buffered set when the loading page was cancelled with the decision still in flight", func(t *testing.T) {
		receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })
		var model tea.Model = tui.Build(tui.Deps{
			Lister:           &mockSessionLister{},
			ServerStarted:    true,
			ProgressReceiver: receiver,
			Search:           &tui.SearchForm{Term: "port", Decide: func() (string, error) { return "portal-a1b2", nil }},
		})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.LoadingMinElapsedMsg{})
		// The decision command this returns is deliberately left unrun: the escape
		// the user takes from a tmux server too slow to answer it.
		model, decision := model.Update(tui.BootstrapCompleteMsg{Warnings: warnings})
		if decision == nil || model.(tui.Model).ActivePage() != tui.PageLoading {
			t.Fatal("both gates satisfied must dispatch the decision and hold the loading page")
		}

		model, _ = model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

		m := model.(tui.Model)
		if m.SearchAttached() || m.SearchError() != nil {
			t.Fatal("an in-flight decision records neither an attach nor an error")
		}

		assertOwed(t, m.WarningsOwedAtTeardown(), warnings, "no surface ever showed what the gate took")
	})
}

func TestFinishTUI(t *testing.T) {
	t.Run("it writes the owed warnings once, before the connect", func(t *testing.T) {
		warnings := soakedWarnings()
		model := searchTeardownModel(t, func() (string, error) { return "portal-a1b2", nil }, warnings)
		model = withCapturedBackground(t, model)

		var canvas, stderr bytes.Buffer
		var canvasAtConnect, warningsAtConnect string
		connector := &observingConnector{onConnect: func() {
			canvasAtConnect = canvas.String()
			warningsAtConnect = stderr.String()
		}}

		if err := finishTUI(model, connector, &canvas, &stderr); err != nil {
			t.Fatalf("finishTUI: %v", err)
		}

		want := wantWarningOutput(warnings)
		if warningsAtConnect != want {
			t.Errorf("warnings at connect = %q, want %q — the exec'd attach never returns", warningsAtConnect, want)
		}
		if stderr.String() != want {
			t.Errorf("stderr after teardown = %q, want %q — the owed set is written exactly once", stderr.String(), want)
		}
		if canvasAtConnect == "" {
			t.Error("nothing written to the canvas writer at connect, want the set-back — the attach leaves Portal's colour stuck otherwise")
		}
	})

	t.Run("it writes the same lines as the CLI path", func(t *testing.T) {
		warnings := soakedWarnings()
		accumulateWarnings(t, warnings)
		var cli bytes.Buffer
		bootstrapWarnings.EmitTo(&cli)

		model := searchTeardownModel(t, func() (string, error) { return "portal-a1b2", nil }, warnings)

		var canvas, stderr bytes.Buffer
		if err := finishTUI(model, &observingConnector{onConnect: func() {}}, &canvas, &stderr); err != nil {
			t.Fatalf("finishTUI: %v", err)
		}

		if stderr.String() != cli.String() {
			t.Errorf("teardown stderr = %q, want the CLI path's %q", stderr.String(), cli.String())
		}
	})
}

// withCapturedBackground gives the model an original background differing from
// its canvas, which is what the restore's echo guard needs before it writes.
func withCapturedBackground(t *testing.T, m tui.Model) tui.Model {
	t.Helper()

	updated, _ := m.Update(tea.BackgroundColorMsg{Color: color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}})
	next, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("model type = %T, want tui.Model", updated)
	}
	if next.OriginalBackground() == "" {
		t.Fatal("model must have captured an original background")
	}
	return next
}

// observingConnector reports what had already been written when the connect
// ran, standing in for the attach handoff that never returns.
type observingConnector struct {
	onConnect func()
}

func (c *observingConnector) Connect(string) error {
	c.onConnect()
	return nil
}

// warmPickerModel is the model a warm picker route builds: no progress
// receiver and an unstarted server, which lands on the Sessions page from the
// first frame with no loading gate to consume its staged warnings.
func warmPickerModel(t *testing.T, deps tui.Deps, warnings []warning.Warning) tui.Model {
	t.Helper()

	deps.Lister = &mockSessionLister{sessions: []tmux.Session{{Name: "portal-a1b2"}}}
	m := tui.Build(deps)
	m.SetPendingBootstrapWarnings(warnings)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(tui.SessionsMsg{Sessions: []tmux.Session{{Name: "portal-a1b2"}}})

	updated, ok := model.(tui.Model)
	if !ok {
		t.Fatalf("model type = %T, want tui.Model", model)
	}
	if updated.ActivePage() != tui.PageSessions {
		t.Fatalf("ActivePage() = %d, want PageSessions — a warm picker has no loading gate", updated.ActivePage())
	}
	return updated
}

func TestFinishTUI_StagedBootstrapWarnings(t *testing.T) {
	t.Run("it writes the staged bootstrap warnings at teardown when no loading gate consumed them", func(t *testing.T) {
		warnings := soakedWarnings()
		model := warmPickerModel(t, tui.Deps{}, warnings)

		var canvas, stderr bytes.Buffer
		if err := finishTUI(model, &observingConnector{onConnect: func() {}}, &canvas, &stderr); err != nil {
			t.Fatalf("finishTUI: %v", err)
		}

		if want := wantWarningOutput(warnings); stderr.String() != want {
			t.Errorf("stderr = %q, want %q", stderr.String(), want)
		}
	})

	t.Run("it writes the same lines as the CLI path", func(t *testing.T) {
		warnings := soakedWarnings()
		accumulateWarnings(t, warnings)
		var cli bytes.Buffer
		bootstrapWarnings.EmitTo(&cli)

		model := warmPickerModel(t, tui.Deps{}, warnings)

		var canvas, stderr bytes.Buffer
		if err := finishTUI(model, &observingConnector{onConnect: func() {}}, &canvas, &stderr); err != nil {
			t.Fatalf("finishTUI: %v", err)
		}

		if stderr.String() != cli.String() {
			t.Errorf("teardown stderr = %q, want the CLI path's %q", stderr.String(), cli.String())
		}
	})

	t.Run("it writes nothing at teardown when the loading gate already surfaced them", func(t *testing.T) {
		warnings := soakedWarnings()
		m := tui.Build(tui.Deps{Lister: &mockSessionLister{}, ServerStarted: true})
		m.SetPendingBootstrapWarnings(warnings)

		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.LoadingMinElapsedMsg{})
		// What Init synthesizes on the warm loading-page route.
		model, _ = model.Update(tui.BootstrapCompleteMsg{})

		updated := model.(tui.Model)
		if updated.ActivePage() == tui.PageLoading {
			t.Fatal("both gates are satisfied; the loading page must have dismissed")
		}

		var canvas, stderr bytes.Buffer
		if err := finishTUI(updated, &observingConnector{onConnect: func() {}}, &canvas, &stderr); err != nil {
			t.Fatalf("finishTUI: %v", err)
		}

		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want nothing — the gate surfaced them already", stderr.String())
		}
	})

	t.Run("it writes the warnings the gate took when the loading page was cancelled", func(t *testing.T) {
		warnings := soakedWarnings()
		m := tui.Build(tui.Deps{Lister: &mockSessionLister{}, ServerStarted: true})
		m.SetPendingBootstrapWarnings(warnings)

		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.BootstrapCompleteMsg{})
		model, _ = model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageLoading {
			t.Fatalf("ActivePage() = %d, want PageLoading — the pad has not elapsed", updated.ActivePage())
		}

		var canvas, stderr bytes.Buffer
		if err := finishTUI(updated, &observingConnector{onConnect: func() {}}, &canvas, &stderr); err != nil {
			t.Fatalf("finishTUI: %v", err)
		}

		if want := wantWarningOutput(warnings); stderr.String() != want {
			t.Errorf("stderr = %q, want %q — the cancel left them on no surface", stderr.String(), want)
		}
	})

	t.Run("it writes nothing at teardown when no warnings accumulated", func(t *testing.T) {
		model := warmPickerModel(t, tui.Deps{}, nil)

		var canvas, stderr bytes.Buffer
		if err := finishTUI(model, &observingConnector{onConnect: func() {}}, &canvas, &stderr); err != nil {
			t.Fatalf("finishTUI: %v", err)
		}

		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want nothing", stderr.String())
		}
	})

	t.Run("it writes the staged warnings before the connector runs", func(t *testing.T) {
		warnings := soakedWarnings()
		model := warmPickerModel(t, tui.Deps{}, warnings)
		model = withCapturedBackground(t, model)
		model = selectFirstSession(t, model)

		var canvas, stderr bytes.Buffer
		var warningsAtConnect, canvasAtConnect string
		connector := &observingConnector{onConnect: func() {
			warningsAtConnect = stderr.String()
			canvasAtConnect = canvas.String()
		}}

		if err := finishTUI(model, connector, &canvas, &stderr); err != nil {
			t.Fatalf("finishTUI: %v", err)
		}

		if want := wantWarningOutput(warnings); warningsAtConnect != want {
			t.Errorf("warnings at connect = %q, want %q — the exec'd attach never returns", warningsAtConnect, want)
		}
		if canvasAtConnect == "" {
			t.Error("nothing written to the canvas writer at connect, want the set-back")
		}
	})

	t.Run("it writes the staged warnings for a warm picker opened by -f and by a bare open", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			deps tui.Deps
		}{
			{name: "-f term", deps: tui.Deps{InitialFilter: "term"}},
			{name: "bare open", deps: tui.Deps{}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				warnings := soakedWarnings()
				model := warmPickerModel(t, tc.deps, warnings)

				var canvas, stderr bytes.Buffer
				if err := finishTUI(model, &observingConnector{onConnect: func() {}}, &canvas, &stderr); err != nil {
					t.Fatalf("finishTUI: %v", err)
				}

				if want := wantWarningOutput(warnings); stderr.String() != want {
					t.Errorf("stderr = %q, want %q", stderr.String(), want)
				}
			})
		}
	})
}

// selectFirstSession drives the picker to the selection a teardown connects on.
func selectFirstSession(t *testing.T, m tui.Model) tui.Model {
	t.Helper()

	var model tea.Model = m
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated, ok := model.(tui.Model)
	if !ok {
		t.Fatalf("model type = %T, want tui.Model", model)
	}
	if updated.Selected() == "" {
		t.Fatal("Enter on the sessions list must record a selection")
	}
	return updated
}
