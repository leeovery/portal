package cmd

import (
	"bytes"
	"context"
	"errors"
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
	model, _ = model.Update(tui.LoadingMinElapsedMsg{})
	model, _ = model.Update(tui.BootstrapCompleteMsg{Warnings: warnings})

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

func TestEmitSearchTeardownWarnings(t *testing.T) {
	warnings := soakedWarnings()

	t.Run("it writes the buffered warnings on a decision attach", func(t *testing.T) {
		model := searchTeardownModel(t, func() (string, error) { return "portal-a1b2", nil }, warnings)
		if !model.SearchAttached() {
			t.Fatal("model must record the search attach")
		}

		var buf bytes.Buffer
		emitSearchTeardownWarnings(&buf, model)

		if want := wantWarningOutput(warnings); buf.String() != want {
			t.Errorf("wrote %q, want %q", buf.String(), want)
		}
	})

	t.Run("it writes the buffered warnings on a decision read failure", func(t *testing.T) {
		readErr := errors.New("no server running")
		model := searchTeardownModel(t, func() (string, error) { return "", readErr }, warnings)
		if !errors.Is(model.SearchError(), readErr) {
			t.Fatalf("SearchError() = %v, want %v", model.SearchError(), readErr)
		}

		var buf bytes.Buffer
		emitSearchTeardownWarnings(&buf, model)

		if want := wantWarningOutput(warnings); buf.String() != want {
			t.Errorf("wrote %q, want %q", buf.String(), want)
		}
	})

	t.Run("it writes nothing after teardown when the picker opened", func(t *testing.T) {
		model := searchTeardownModel(t, func() (string, error) { return "", nil }, warnings)
		if model.SearchAttached() || model.SearchError() != nil {
			t.Fatal("a picker decision records neither an attach nor an error")
		}

		var buf bytes.Buffer
		emitSearchTeardownWarnings(&buf, model)

		if buf.Len() != 0 {
			t.Errorf("wrote %q, want nothing — the notice band owns a picker's warnings", buf.String())
		}
	})

	t.Run("it writes nothing when no warnings accumulated", func(t *testing.T) {
		model := searchTeardownModel(t, func() (string, error) { return "portal-a1b2", nil }, nil)

		var buf bytes.Buffer
		emitSearchTeardownWarnings(&buf, model)

		if buf.Len() != 0 {
			t.Errorf("wrote %q, want nothing", buf.String())
		}
	})

	t.Run("it writes nothing when the loading page was cancelled", func(t *testing.T) {
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

		var buf bytes.Buffer
		emitSearchTeardownWarnings(&buf, m)

		if buf.Len() != 0 {
			t.Errorf("wrote %q, want nothing — the user asked for nothing and is owed no report", buf.String())
		}
	})

	t.Run("it writes before connecting", func(t *testing.T) {
		model := searchTeardownModel(t, func() (string, error) { return "portal-a1b2", nil }, warnings)

		var buf bytes.Buffer
		emitSearchTeardownWarnings(&buf, model)

		var writtenAtConnect string
		connector := &observingConnector{onConnect: func() { writtenAtConnect = buf.String() }}
		if err := processTUIResult(model, connector); err != nil {
			t.Fatalf("processTUIResult: %v", err)
		}

		if want := wantWarningOutput(warnings); writtenAtConnect != want {
			t.Errorf("output at connect = %q, want %q — the exec'd attach never returns", writtenAtConnect, want)
		}
	})
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
