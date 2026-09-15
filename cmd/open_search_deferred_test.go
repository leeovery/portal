package cmd

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

// searchCommand stands in for the command open's RunE hands runSearchForm,
// carrying nothing but the context that decides which route the form takes.
func searchCommand(ctx context.Context) *cobra.Command {
	c := &cobra.Command{}
	c.SetContext(ctx)
	return c
}

// deferredSearchContext is the context PersistentPreRunE leaves behind on the
// concurrent cold-boot route, where the bootstrap has not run yet.
func deferredSearchContext() context.Context {
	return context.WithValue(context.Background(), deferredBootstrapKey, &deferredBootstrap{})
}

func runDeferredSearch(t *testing.T, sc *searchFormCapture, term string) func() (string, error) {
	t.Helper()
	if err := runSearchForm(searchCommand(deferredSearchContext()), term); err != nil {
		t.Fatalf("runSearchForm(%q) = %v, want nil", term, err)
	}
	if !sc.tuiCalled {
		t.Fatal("a deferred search form must open the picker")
	}
	if sc.landing.search == nil || sc.landing.search.Decide == nil {
		t.Fatal("landing.search.Decide = nil, want the deferred decision closure")
	}
	return sc.landing.search.Decide
}

func TestSearchForm_DeferredBootstrap_DefersTheCount(t *testing.T) {
	t.Run("it takes no count when a bootstrap is in flight", func(t *testing.T) {
		sc := installSearchFormSeams(t)
		sc.source.sessions = []tmux.Session{{Name: "portal-a1b2"}}

		runDeferredSearch(t, sc, "port")

		if sc.readsAtTUI != 0 {
			t.Errorf("session reads before the picker seam ran = %d, want 0", sc.readsAtTUI)
		}
		if sc.sessionCalled {
			t.Errorf("attached %q up front; a deferred search attaches nothing before the picker runs", sc.attached)
		}
	})

	t.Run("it hands the picker a decision closure on the deferred route", func(t *testing.T) {
		sc := installSearchFormSeams(t)

		runDeferredSearch(t, sc, "port")

		got, want := shapeOfLanding(sc.landing), (landingShape{term: "port", search: true, decided: true})
		if got != want {
			t.Errorf("landing = %+v, want %+v", got, want)
		}
	})

	t.Run("it returns the single matching session from the deferred closure", func(t *testing.T) {
		sc := installSearchFormSeams(t)
		sc.source.sessions = []tmux.Session{{Name: "portal-a1b2"}, {Name: "blog-c3d4"}}

		decide := runDeferredSearch(t, sc, "port")

		name, err := decide()
		if err != nil {
			t.Fatalf("decide() error = %v, want nil", err)
		}
		if name != "portal-a1b2" {
			t.Errorf("decide() = %q, want %q", name, "portal-a1b2")
		}
	})

	t.Run("it returns no session for zero matches and for two or more", func(t *testing.T) {
		tests := []struct {
			name     string
			sessions []tmux.Session
		}{
			{name: "zero matches", sessions: []tmux.Session{{Name: "blog-c3d4"}}},
			{name: "two matches", sessions: []tmux.Session{{Name: "portal-a1b2"}, {Name: "port-agent"}}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sc := installSearchFormSeams(t)
				sc.source.sessions = tt.sessions

				decide := runDeferredSearch(t, sc, "port")

				name, err := decide()
				if err != nil {
					t.Fatalf("decide() error = %v, want nil", err)
				}
				if name != "" {
					t.Errorf("decide() = %q, want empty — the picker opens on any count but one", name)
				}
			})
		}
	})

	t.Run("it returns the enumeration error from the deferred closure", func(t *testing.T) {
		readErr := errors.New("no server running")
		sc := installSearchFormSeams(t)
		sc.source.listErr = readErr

		decide := runDeferredSearch(t, sc, "port")

		name, err := decide()
		if !errors.Is(err, readErr) {
			t.Errorf("decide() error = %v, want %v", err, readErr)
		}
		if name != "" {
			t.Errorf("decide() = %q, want empty on a failed read", name)
		}
	})

	t.Run("it excludes the current session from the deferred closure's candidates", func(t *testing.T) {
		sc := installSearchFormSeams(t)
		sc.source.sessions = []tmux.Session{{Name: "portal-a1b2"}, {Name: "blog-c3d4"}}
		sc.source.current = "portal-a1b2"

		decide := runDeferredSearch(t, sc, "port")

		name, err := decide()
		if err != nil {
			t.Fatalf("decide() error = %v, want nil", err)
		}
		if name != "" {
			t.Errorf("decide() = %q, want empty — the session the user is in is not a candidate", name)
		}
	})

	t.Run("it supplies no closure for the term-less form on either route", func(t *testing.T) {
		routes := map[string]context.Context{
			"deferred": deferredSearchContext(),
			"warm":     context.Background(),
		}

		for name, ctx := range routes {
			t.Run(name, func(t *testing.T) {
				sc := installSearchFormSeams(t)
				sc.source.sessions = []tmux.Session{{Name: "portal-a1b2"}}

				if err := runSearchForm(searchCommand(ctx), ""); err != nil {
					t.Fatalf("runSearchForm(\"\") = %v, want nil", err)
				}

				got, want := shapeOfLanding(sc.landing), (landingShape{search: true})
				if got != want {
					t.Errorf("landing = %+v, want %+v", got, want)
				}
				if sc.source.listCalls != 0 || sc.source.currentCalls != 0 {
					t.Errorf("term-less form read the session set: ListSessionsProbe=%d CurrentSessionName=%d, want 0 and 0",
						sc.source.listCalls, sc.source.currentCalls)
				}
			})
		}
	})
}

func TestSearchForm_WarmInvocation_CountsUpFront(t *testing.T) {
	t.Run("it counts up front with no closure on a warm invocation", func(t *testing.T) {
		sc := installSearchFormSeams(t)
		sc.source.sessions = []tmux.Session{{Name: "portal-a1b2"}, {Name: "port-agent"}}

		if err := runSearchForm(searchCommand(context.Background()), "port"); err != nil {
			t.Fatalf("runSearchForm: %v", err)
		}

		if sc.readsAtTUI == 0 {
			t.Error("a warm invocation must take its count before the picker opens")
		}
		got, want := shapeOfLanding(sc.landing), (landingShape{term: "port", search: true})
		if got != want {
			t.Errorf("landing = %+v, want %+v", got, want)
		}
	})

	t.Run("it attaches the single match up front on a warm invocation", func(t *testing.T) {
		sc := installSearchFormSeams(t)
		sc.source.sessions = []tmux.Session{{Name: "portal-a1b2"}, {Name: "blog-c3d4"}}

		if err := runSearchForm(searchCommand(context.Background()), "port"); err != nil {
			t.Fatalf("runSearchForm: %v", err)
		}

		if sc.attached != "portal-a1b2" {
			t.Errorf("attached session = %q, want %q", sc.attached, "portal-a1b2")
		}
		if sc.tuiCalled {
			t.Error("a single warm match must not open the picker")
		}
	})

	t.Run("it returns the enumeration error up front on a warm invocation", func(t *testing.T) {
		readErr := errors.New("no server running")
		sc := installSearchFormSeams(t)
		sc.source.listErr = readErr

		err := runSearchForm(searchCommand(context.Background()), "port")

		if !errors.Is(err, readErr) {
			t.Errorf("runSearchForm error = %v, want %v", err, readErr)
		}
		if sc.tuiCalled {
			t.Error("a failed read has searched nothing: the picker must not open")
		}
	})
}

// searchDecisionTUI runs a model built the way openTUI builds one — through
// tui.Build, so the picker's own wiring of the decision closure is exercised —
// to the loading gate where the decision resolves.
func searchDecisionTUI(t *testing.T, decide func() (string, error)) tui.Model {
	t.Helper()

	receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })
	var model tea.Model = tui.Build(tui.Deps{
		Lister:           &mockSessionLister{},
		ServerStarted:    true,
		ProgressReceiver: receiver,
		Search:           &tui.SearchForm{Term: "port", Decide: decide},
	})
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = driveLoadingGates(t, model, nil)

	m, ok := model.(tui.Model)
	if !ok {
		t.Fatalf("model type = %T, want tui.Model", model)
	}
	return m
}

func TestProcessTUIResult_SearchDecision(t *testing.T) {
	t.Run("it returns the model's search error without connecting", func(t *testing.T) {
		readErr := errors.New("no server running")
		m := searchDecisionTUI(t, func() (string, error) { return "", readErr })

		connector := &mockSessionConnector{}
		err := processTUIResult(m, connector)

		if !errors.Is(err, readErr) {
			t.Fatalf("processTUIResult error = %v, want %v", err, readErr)
		}
		if usage, ok := errors.AsType[*UsageError](err); ok {
			t.Errorf("a failed session read was reported as a usage error: %v", usage)
		}
		if fatal, ok := errors.AsType[*bootstrap.FatalError](err); ok {
			t.Errorf("a failed session read was reported as a bootstrap fatal: %v", fatal)
		}
		if connector.connectedTo != "" {
			t.Errorf("connector was called on a failed read (%q); must skip connect", connector.connectedTo)
		}
	})

	t.Run("it connects the single match the decision named", func(t *testing.T) {
		m := searchDecisionTUI(t, func() (string, error) { return "portal-a1b2", nil })

		connector := &mockSessionConnector{}
		if err := processTUIResult(m, connector); err != nil {
			t.Fatalf("processTUIResult error = %v, want nil", err)
		}
		if connector.connectedTo != "portal-a1b2" {
			t.Errorf("connector called with %q, want %q", connector.connectedTo, "portal-a1b2")
		}
	})

	t.Run("it prefers the bootstrap fatal over the search error", func(t *testing.T) {
		fatal := bootstrap.NewFatal("Portal failed to clear @portal-restoring", errors.New("permission denied"))
		readErr := errors.New("no server running")

		var model tea.Model = searchDecisionTUI(t, func() (string, error) { return "", readErr })
		model, _ = model.Update(tui.BootstrapFatalMsg{FailedStep: 8, Message: fatal.UserMessage, Err: fatal})

		connector := &mockSessionConnector{}
		err := processTUIResult(model.(tui.Model), connector)

		if !errors.Is(err, fatal) {
			t.Fatalf("processTUIResult error = %v, want the bootstrap fatal %v", err, fatal)
		}
		if connector.connectedTo != "" {
			t.Errorf("connector was called on a fatal (%q); must skip connect", connector.connectedTo)
		}
	})
}
