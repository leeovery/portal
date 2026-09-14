//go:build integration

package cmd

import (
	"context"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

// The shared fragment matches two seeded names; the solo term matches one, so
// one seeding drives both counts.
const (
	searchBootSharedTerm = "shared"
	searchBootSoloTerm   = "solo"
)

var searchBootSeeds = []string{"cc-shared-alpha", "cc-shared-bravo", "cc-solo-charlie"}

// searchDecisionProbe records what the boot had already done at the instant the
// search decision ran, read from the decision closure itself.
type searchDecisionProbe struct {
	calls          int
	stepsAtCall    int
	completeAtCall bool
	restoring      bool
	restoringErr   error
	sessions       []string
	listErr        error
}

type searchColdBootRun struct {
	probe    searchDecisionProbe
	model    tui.Model
	steps    int
	complete bool
	fatal    bool
}

// driveSearchColdBoot runs the real ten-step orchestrator through the real
// progress pipe into a real search-form model, pumping the pipe by hand. The
// model's own returned commands are discarded deliberately: it carries the same
// receiver closure, so running both would race two consumers on one channel.
func driveSearchColdBoot(t *testing.T, client *tmux.Client, stateDir, term string) *searchColdBootRun {
	t.Helper()

	orch := buildConcurrentColdBootOrchestrator(t, client, stateDir)
	pipe := newBootstrapProgressPipe()
	pipe.start(context.Background(), orch)

	run := &searchColdBootRun{}

	decide := func() (string, error) {
		run.probe.calls++
		run.probe.stepsAtCall = run.steps
		run.probe.completeAtCall = run.complete
		run.probe.restoring, run.probe.restoringErr = state.IsRestoringSet(client)

		sessions, err := client.ListSessionsProbe()
		run.probe.listErr = err
		for _, s := range sessions {
			run.probe.sessions = append(run.probe.sessions, s.Name)
		}
		if err != nil {
			return "", err
		}
		if matches := searchMatches(term, sessions); len(matches) == 1 {
			return matches[0].Name, nil
		}
		return "", nil
	}

	var model tea.Model = tui.Build(tui.Deps{
		Lister:           client,
		ServerStarted:    true,
		ProgressReceiver: pipe.receiver(),
		Search:           &tui.SearchForm{Term: term, Decide: decide},
	})
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(tui.LoadingMinElapsedMsg{})

	receiver := pipe.receiver()
	deadline := time.After(concurrentBootDrainBudget)
	for run.probe.calls == 0 {
		got := make(chan tea.Msg, 1)
		go func() { got <- receiver() }()

		select {
		case msg := <-got:
			realStep := false
			switch m := msg.(type) {
			case tui.BootstrapProgressMsg:
				// Restore's per-session events also ride Index 6, so only the
				// zero-counter ticks are real steps.
				realStep = m.RestoreM == 0 && m.RestoreN == 0
				if realStep {
					run.steps++
				}
			case tui.BootstrapCompleteMsg:
				run.complete = true
			case tui.BootstrapFatalMsg:
				run.fatal = true
			case bootstrapChannelClosedMsg:
				run.model = model.(tui.Model)
				return run
			}

			model, _ = model.Update(msg)

			if realStep {
				assertStandingOnLoadingPage(t, model.(tui.Model), run)
			}
		case <-deadline:
			t.Fatalf("driveSearchColdBoot(%q): pipe drained for %s without closing — "+
				"the orchestrator goroutine never sent the terminal event\n--- portal.log ---\n%s",
				term, concurrentBootDrainBudget, portaltest.ReadPortalLogSafe(stateDir))
		}
	}

	run.model = model.(tui.Model)
	return run
}

// assertStandingOnLoadingPage pins the half of the contract that only holds
// mid-boot: no step may dismiss the loading page or take the decision early.
func assertStandingOnLoadingPage(t *testing.T, m tui.Model, run *searchColdBootRun) {
	t.Helper()
	if m.ActivePage() != tui.PageLoading {
		t.Errorf("after step event %d the model left PageLoading (page = %d); "+
			"the loading page stands until the count can be taken", run.steps, m.ActivePage())
	}
	if run.probe.calls != 0 {
		t.Errorf("the search decision ran at step event %d — it must wait for the whole "+
			"bootstrap, not the end of restore", run.steps)
	}
}

func assertCleanSearchBoot(t *testing.T, run *searchColdBootRun, stateDir string) {
	t.Helper()
	if run.fatal {
		t.Fatalf("the boot reported a BootstrapFatalMsg; want a clean complete\n--- portal.log ---\n%s",
			portaltest.ReadPortalLogSafe(stateDir))
	}
	if err := run.model.FatalError(); err != nil {
		t.Errorf("FatalError() = %v, want nil", err)
	}
}

func TestConcurrentColdBoot_SearchDecision_SingleMatchAttachesAfterWholeBootstrap(t *testing.T) {
	_, client, stateDir, _ := setupConcurrentColdBootEnv(t)
	restoretest.SeedSessionsJSON(t, stateDir, searchBootSeeds...)

	run := driveSearchColdBoot(t, client, stateDir, searchBootSoloTerm)
	assertCleanSearchBoot(t, run, stateDir)

	t.Run("it takes the search decision only after every bootstrap step", func(t *testing.T) {
		if run.probe.stepsAtCall != 10 {
			t.Errorf("step events delivered when the decision ran = %d, want 10 — "+
				"acting at the end of restore abandons the steps that follow it, "+
				"among them the clearing of @portal-restoring", run.probe.stepsAtCall)
		}
		if !run.probe.completeAtCall {
			t.Error("the decision ran before the terminal complete event")
		}
	})

	t.Run("it takes the search decision with the restoring marker cleared", func(t *testing.T) {
		if run.probe.restoringErr != nil {
			t.Fatalf("IsRestoringSet inside the decision: %v", run.probe.restoringErr)
		}
		if run.probe.restoring {
			t.Error("@portal-restoring was still SET when the decision ran — the marker must " +
				"be cleared before anything the decision drives fires")
		}
	})

	t.Run("it sees the restored sessions in the decision's own read", func(t *testing.T) {
		if run.probe.listErr != nil {
			t.Fatalf("the decision's own ListSessionsProbe: %v", run.probe.listErr)
		}
		for _, want := range searchBootSeeds {
			if !slices.Contains(run.probe.sessions, want) {
				t.Errorf("restored session %q absent from the decision's own read %v — "+
					"the count must be taken over post-restore reality", want, run.probe.sessions)
			}
		}
	})

	t.Run("it invokes the decision exactly once", func(t *testing.T) {
		if run.probe.calls != 1 {
			t.Errorf("decision calls = %d across the whole boot, want 1", run.probe.calls)
		}
	})

	t.Run("it attaches the single match without painting the picker", func(t *testing.T) {
		if got := run.model.Selected(); got != "cc-solo-charlie" {
			t.Errorf("Selected() = %q, want %q", got, "cc-solo-charlie")
		}
		if !run.model.SearchAttached() {
			t.Error("SearchAttached() = false, want true")
		}
		if run.model.ActivePage() != tui.PageLoading {
			t.Errorf("ActivePage() = %d, want PageLoading — no picker frame is composed on an attach",
				run.model.ActivePage())
		}
	})
}

func TestConcurrentColdBoot_SearchDecision_SharedFragmentOpensPicker(t *testing.T) {
	_, client, stateDir, _ := setupConcurrentColdBootEnv(t)
	restoretest.SeedSessionsJSON(t, stateDir, searchBootSeeds...)

	run := driveSearchColdBoot(t, client, stateDir, searchBootSharedTerm)
	assertCleanSearchBoot(t, run, stateDir)

	t.Run("it opens the picker when two sessions match", func(t *testing.T) {
		if run.model.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions", run.model.ActivePage())
		}
		if got := run.model.Selected(); got != "" {
			t.Errorf("Selected() = %q, want empty", got)
		}
		if run.model.SearchAttached() {
			t.Error("SearchAttached() = true on a picker decision, want false")
		}
	})

	t.Run("it invokes the decision exactly once", func(t *testing.T) {
		if run.probe.calls != 1 {
			t.Errorf("decision calls = %d across the whole boot, want 1", run.probe.calls)
		}
	})

	t.Run("it takes the search decision only after every bootstrap step", func(t *testing.T) {
		if run.probe.stepsAtCall != 10 || !run.probe.completeAtCall {
			t.Errorf("decision ran after %d step events (complete delivered = %v), want 10 and true",
				run.probe.stepsAtCall, run.probe.completeAtCall)
		}
	})
}
