package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/runnerreg"
	"github.com/leeovery/portal/internal/tui"
)

func TestOpenSession_InsideASessionSwitchesThroughTheRegistry(t *testing.T) {
	t.Run("the runner the command was typed in switches, and Portal exits 0 with nothing exec'd", func(t *testing.T) {
		fake, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
		fake.SetCurrent("blog-c3d4")
		rec.runners.switchErr = nil

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession = %v, want nil", err)
		}
		rec.assertNone(t)
		if want := []switchCall{{"blog-c3d4", "api-x7Kd9a"}}; !slices.Equal(rec.runners.switches, want) {
			t.Errorf("switches = %+v, want %+v", rec.runners.switches, want)
		}
	})

	for name, why := range map[string]error{
		"with no runner":    runnerreg.ErrNoRunner,
		"on a refusal":      fmt.Errorf("ask runner 4242 to switch: %w: the runner is not attached", runnerreg.ErrRefused),
		"when it can't ask": errors.New("reach runner 4242: connection refused"),
	} {
		t.Run(name+" it execs the attach command, which zmx routes to the client that typed it", func(t *testing.T) {
			fake, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
			fake.SetCurrent("blog-c3d4")
			rec.runners.switchErr = why

			if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
				t.Fatalf("openSession = %v, want nil", err)
			}
			if len(rec.runners.switches) != 1 {
				t.Errorf("switches = %+v, want the registry asked once first", rec.runners.switches)
			}
			assertExeced(t, rec, "api-x7Kd9a")
		})
	}

	t.Run("outside a session the registry is never asked to switch", func(t *testing.T) {
		_, rec := stageAttach(t, "api-x7Kd9a")
		rec.runners.switchErr = nil

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		assertAttached(t, rec, "api-x7Kd9a")
		if len(rec.runners.switches) != 0 {
			t.Errorf("switches = %+v, want none", rec.runners.switches)
		}
	})

	t.Run("a mint inside a session creates the session, then switches to it", func(t *testing.T) {
		fake, rec := stageAttach(t, "blog-c3d4")
		stageMint(t, fake, time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC))
		fake.SetCurrent("blog-c3d4")
		rec.runners.switchErr = nil
		dir := t.TempDir()

		if err := openPath(contextCommand(), dir, nil); err != nil {
			t.Fatalf("openPath: %v", err)
		}
		creates := fake.CallsTo(backendtest.OpCreate)
		if len(creates) != 1 {
			t.Fatalf("Create calls = %+v, want one", creates)
		}
		if want := []switchCall{{"blog-c3d4", creates[0].Name}}; !slices.Equal(rec.runners.switches, want) {
			t.Errorf("switches = %+v, want %+v: the session made, then switched to", rec.runners.switches, want)
		}
		rec.assertNone(t)
	})

	t.Run("a pick in a picker inside a session switches through the registry", func(t *testing.T) {
		fake, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
		fake.SetCurrent("api-x7Kd9a")
		rec.runners.switchErr = nil
		t.Setenv("PORTAL_PROJECTS_FILE", filepath.Join(t.TempDir(), "projects.json"))
		t.Setenv("PORTAL_ALIASES_FILE", filepath.Join(t.TempDir(), "aliases"))
		setPrefsFile(t, `{"theme":"nord"}`)
		withFuncSeam(t, &runProgramFunc, func(m tui.Model) (tea.Model, error) {
			var model tea.Model = m
			model, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			model, _ = model.Update(tui.SessionsMsg{Sessions: []tui.Session{{Name: "blog-c3d4"}}})
			model, _ = model.Update(tui.ProjectsLoadedMsg{})
			model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			return model, nil
		})

		if err := openTUI(contextCommand(), pickerLanding{}, nil, false); err != nil {
			t.Fatalf("openTUI: %v", err)
		}
		if want := []switchCall{{"api-x7Kd9a", "blog-c3d4"}}; !slices.Equal(rec.runners.switches, want) {
			t.Errorf("switches = %+v, want %+v", rec.runners.switches, want)
		}
		rec.assertNone(t)
	})
}

func TestAttach_ASwitchAttachesTheSessionItNames(t *testing.T) {
	t.Run("the session switched to attaches next, with no picker, and the runner stays listed across", func(t *testing.T) {
		_, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
		rec.answer(runResult{out: runner.Outcome{Switch: "blog-c3d4"}}, outcome(attachkeys.DetachToShell))
		picker := stagePicker(t)

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if want := []string{"api-x7Kd9a", "blog-c3d4"}; !slices.Equal(ranSessions(rec), want) {
			t.Errorf("attached %q, want %q", ranSessions(rec), want)
		}
		if len(picker.flashes) != 0 {
			t.Errorf("the picker opened %d times, want never", len(picker.flashes))
		}
		if want := []string{"listed api-x7Kd9a", "ended", "moved blog-c3d4", "ended", "unlisted"}; !slices.Equal(rec.runners.events, want) {
			t.Errorf("listing = %q, want %q", rec.runners.events, want)
		}
	})

	t.Run("a switch to a session gone meanwhile ends Portal with the attach's error", func(t *testing.T) {
		_, rec := stageAttach(t, "api-x7Kd9a")
		rec.answer(runResult{out: runner.Outcome{Switch: "gone-a1b2c3"}})
		stagePicker(t)

		if err := openSession(contextCommand(), "api-x7Kd9a"); err == nil {
			t.Fatal("openSession = nil, want the failed attach of the session switched to")
		}
		if want := []string{"listed api-x7Kd9a", "ended", "unlisted"}; !slices.Equal(rec.runners.events, want) {
			t.Errorf("listing = %q, want %q: unlisted on the way out", rec.runners.events, want)
		}
	})

	t.Run("the runner is unlisted while the picker is open, and listed again for its pick", func(t *testing.T) {
		_, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
		rec.answer(outcome(attachkeys.Detach))
		picker := stagePicker(t, "blog-c3d4")
		listedAtPicker := true
		picker.onOpen = func() { listedAtPicker = rec.runners.listed }

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if listedAtPicker {
			t.Error("the runner was still listed while the picker was open")
		}
		if want := []string{"listed api-x7Kd9a", "ended", "unlisted", "listed blog-c3d4", "ended", "unlisted"}; !slices.Equal(rec.runners.events, want) {
			t.Errorf("listing = %q, want %q", rec.runners.events, want)
		}
	})

	t.Run("an attach that never begins is never listed", func(t *testing.T) {
		_, rec := stageAttach(t, "api-x7Kd9a")
		rec.answer(runResult{err: runner.ErrNoTerminal})

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if want := []string{"ended"}; !slices.Equal(rec.runners.events, want) {
			t.Errorf("listing = %q, want %q", rec.runners.events, want)
		}
	})
}

// The production registry lives in the state directory, and a cmd test never
// reaches it: the package's state dir is poisoned, and stageAttach stages a
// fake.
func TestStateRunners_LivesInTheStateDirectory(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ps-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("PORTAL_STATE_DIR", dir)

	listing := stateRunners{}.Listing()
	cfg := runner.Config{Session: "api-x7Kd9a"}
	listing.Attach(&cfg)
	cfg.Began()
	cfg.Forwarded()
	t.Cleanup(listing.Close)

	runners, err := runnerreg.New(dir).On(t.Context(), "api-x7Kd9a")
	if err != nil || len(runners) != 1 || runners[0].PID != os.Getpid() {
		t.Fatalf("runners on api-x7Kd9a under %s = %+v, %v; want this one", dir, runners, err)
	}
	listing.Ended()
	if err := (stateRunners{}).SwitchFrom(t.Context(), "api-x7Kd9a", "blog-c3d4"); !errors.Is(err, runnerreg.ErrRefused) {
		t.Errorf("SwitchFrom = %v, want the refusal of the runner listed there, its attach ended", err)
	}
}
