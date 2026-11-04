package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/runnertoken"
	"github.com/leeovery/portal/internal/tui"
)

// leading is a leader's tracked environment carrying the runner token.
func leading(token string) backend.ClientEnv {
	return backend.ClientEnv{{Name: "DISPLAY", Unset: true}, {Name: runnertoken.Var, Value: token}}
}

func TestOpenSession_InsideASessionSwitchesThroughTheRegistry(t *testing.T) {
	t.Run("the runner whose terminal leads the session switches, and Portal exits 0 with nothing exec'd", func(t *testing.T) {
		fake, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
		fake.SetCurrent("blog-c3d4")
		fake.SetLeaderEnv("blog-c3d4", leading("lead01"))
		rec.runners.switchErr = nil

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession = %v, want nil", err)
		}
		rec.assertNone(t)
		if want := []switchCall{{"lead01", "blog-c3d4", "api-x7Kd9a"}}; !slices.Equal(rec.runners.switches, want) {
			t.Errorf("switches = %+v, want %+v", rec.runners.switches, want)
		}
	})

	for name, leader := range map[string]backend.ClientEnv{
		"a raw zmx client leading":      {{Name: "DISPLAY", Value: ":0"}},
		"a leader lacking the token":    {{Name: runnertoken.Var, Unset: true}},
		"no client leading the session": nil,
	} {
		t.Run(name+" asks no runner by token, and execs the attach command, which zmx routes to the leader", func(t *testing.T) {
			fake, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
			fake.SetCurrent("blog-c3d4")
			fake.SetLeaderEnv("blog-c3d4", leader)

			if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
				t.Fatalf("openSession = %v, want nil", err)
			}
			if want := []switchCall{{"", "blog-c3d4", "api-x7Kd9a"}}; !slices.Equal(rec.runners.switches, want) {
				t.Errorf("switches = %+v, want %+v: no token to ask by", rec.runners.switches, want)
			}
			assertExeced(t, rec, "api-x7Kd9a")
		})
	}

	t.Run("a leader the backend can't tell is no runner, with a WARN, and the attach command is exec'd", func(t *testing.T) {
		sink := logtest.Install(t)
		fake, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
		fake.SetCurrent("blog-c3d4")
		fake.SetLeaderEnv("blog-c3d4", leading("lead01"))
		fake.Fail(backendtest.OpLeaderEnv, errors.New("the daemon did not answer"))

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession = %v, want nil", err)
		}
		if want := []switchCall{{"", "blog-c3d4", "api-x7Kd9a"}}; !slices.Equal(rec.runners.switches, want) {
			t.Errorf("switches = %+v, want %+v", rec.runners.switches, want)
		}
		assertExeced(t, rec, "api-x7Kd9a")
		warn := sink.Records().Matching("attach", "leader unknown").AtExactLevel(slog.LevelWarn).Only(t, "leader unknown record")
		if got := warn.AttrString(t, "session"); got != "blog-c3d4" || !strings.Contains(warn.AttrString(t, "error"), "did not answer") {
			t.Errorf("leader unknown = %+v, want the session and why", warn)
		}
	})

	for name, why := range map[string]error{
		"refuses":          fmt.Errorf("ask runner 4242 to switch: %w: the runner is not attached", runnertoken.ErrRefused),
		"can't be reached": errors.New("reach runner 4242: connection refused"),
		"is gone":          fmt.Errorf("%w: runner 4242 has exited", runnertoken.ErrNoRunner),
	} {
		t.Run("a leading runner that "+name+" leaves the switch to the attach command, exec'd", func(t *testing.T) {
			fake, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
			fake.SetCurrent("blog-c3d4")
			fake.SetLeaderEnv("blog-c3d4", leading("lead01"))
			rec.runners.switchErr = why

			if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
				t.Fatalf("openSession = %v, want nil", err)
			}
			if want := []switchCall{{"lead01", "blog-c3d4", "api-x7Kd9a"}}; !slices.Equal(rec.runners.switches, want) {
				t.Errorf("switches = %+v, want the leading runner asked first, %+v", rec.runners.switches, want)
			}
			assertExeced(t, rec, "api-x7Kd9a")
		})
	}

	t.Run("opening the session it was typed in moves nothing, and Portal exits 0", func(t *testing.T) {
		fake, rec := stageAttach(t, "blog-c3d4")
		fake.SetCurrent("blog-c3d4")
		fake.SetLeaderEnv("blog-c3d4", leading("lead01"))
		rec.runners.switchErr = nil

		if err := openSession(contextCommand(), "blog-c3d4"); err != nil {
			t.Fatalf("openSession = %v, want nil", err)
		}
		rec.assertNone(t)
		if len(rec.runners.switches) != 0 || len(fake.CallsTo(backendtest.OpLeaderEnv)) != 0 {
			t.Errorf("switches = %+v, leader reads = %+v; want neither", rec.runners.switches, fake.CallsTo(backendtest.OpLeaderEnv))
		}
	})

	t.Run("outside a session neither the leader nor the registry is asked", func(t *testing.T) {
		fake, rec := stageAttach(t, "api-x7Kd9a")
		rec.runners.switchErr = nil

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		assertAttached(t, rec, "api-x7Kd9a")
		if len(rec.runners.switches) != 0 || len(fake.CallsTo(backendtest.OpLeaderEnv)) != 0 {
			t.Errorf("switches = %+v, leader reads = %+v; want neither", rec.runners.switches, fake.CallsTo(backendtest.OpLeaderEnv))
		}
	})

	t.Run("a mint inside a session creates the session, then switches to it", func(t *testing.T) {
		fake, rec := stageAttach(t, "blog-c3d4")
		stageMint(t, fake, time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC))
		fake.SetCurrent("blog-c3d4")
		fake.SetLeaderEnv("blog-c3d4", leading("lead01"))
		rec.runners.switchErr = nil
		dir := t.TempDir()

		if err := openPath(contextCommand(), dir, nil); err != nil {
			t.Fatalf("openPath: %v", err)
		}
		creates := fake.CallsTo(backendtest.OpCreate)
		if len(creates) != 1 {
			t.Fatalf("Create calls = %+v, want one", creates)
		}
		if want := []switchCall{{"lead01", "blog-c3d4", creates[0].Name}}; !slices.Equal(rec.runners.switches, want) {
			t.Errorf("switches = %+v, want %+v: the session made, then switched to", rec.runners.switches, want)
		}
		rec.assertNone(t)
	})

	t.Run("a pick in a picker inside a session switches through the registry", func(t *testing.T) {
		fake, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
		fake.SetCurrent("api-x7Kd9a")
		fake.SetLeaderEnv("api-x7Kd9a", leading("lead01"))
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

		if err := openTUI(contextCommand(), pickerLanding{}, nil); err != nil {
			t.Fatalf("openTUI: %v", err)
		}
		if want := []switchCall{{"lead01", "api-x7Kd9a", "blog-c3d4"}}; !slices.Equal(rec.runners.switches, want) {
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

	t.Run("a switch to a session that can't be attached for a fault of its own brings the picker, worded as kill words it, and its pick attaches", func(t *testing.T) {
		for _, tc := range []struct {
			name, target string
			stage        func(*backendtest.Fake)
			flash        string
		}{
			{"gone", "gone-a1b2c3", func(*backendtest.Fake) {}, "No session found: gone-a1b2c3"},
			{"held under another spelling", "BLOG-c3d4", func(*backendtest.Fake) {}, "No session found: BLOG-c3d4; did you mean blog-c3d4?"},
			{"refusing connections", "blog-c3d4", func(fake *backendtest.Fake) {
				fake.Fail(backendtest.OpAttachCommand, fmt.Errorf("attach session %q: %w", "blog-c3d4", zmx.ErrRefusing))
			}, "session blog-c3d4 refuses connections: " + crashedDaemon},
			{"not answering", "blog-c3d4", func(fake *backendtest.Fake) { fake.StopAnswering("blog-c3d4") }, "session blog-c3d4 is not answering; nothing was opened"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				fake, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4", "docs-e5f6")
				rec.answer(runResult{out: runner.Outcome{Switch: tc.target}, ended: func() { tc.stage(fake) }})
				picker := stagePicker(t, "docs-e5f6")
				// A staged failure fails every attach command, the pick's too.
				picker.onOpen = func() { fake.Fail(backendtest.OpAttachCommand, nil) }

				if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
					t.Fatalf("openSession = %v, want the picker's pick attached", err)
				}
				if want := []string{tc.flash}; !slices.Equal(picker.flashes, want) {
					t.Errorf("the picker's flashes = %q, want %q", picker.flashes, want)
				}
				if want := []string{"api-x7Kd9a", "docs-e5f6"}; !slices.Equal(ranSessions(rec), want) {
					t.Errorf("attached %q, want %q", ranSessions(rec), want)
				}
				if want := []string{"listed api-x7Kd9a", "ended", "unlisted", "listed docs-e5f6", "ended", "unlisted"}; !slices.Equal(rec.runners.events, want) {
					t.Errorf("listing = %q, want %q: unlisted while the picker shows", rec.runners.events, want)
				}
			})
		}
	})

	t.Run("a switched attach that fails other than for a fault of its session's returns the error, as a first attach does", func(t *testing.T) {
		for name, failure := range map[string]error{
			"the runner fails":              errors.New("the attach broke"),
			"the zmx binary fails to start": &exec.Error{Name: "zmx", Err: exec.ErrNotFound},
		} {
			t.Run(name, func(t *testing.T) {
				_, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
				rec.answer(runResult{out: runner.Outcome{Switch: "blog-c3d4"}}, runResult{err: failure})
				picker := stagePicker(t)

				if err := openSession(contextCommand(), "api-x7Kd9a"); !errors.Is(err, failure) {
					t.Errorf("openSession = %v, want the switched attach's own %v", err, failure)
				}
				if len(picker.flashes) != 0 {
					t.Errorf("the picker opened with %q, want it never opened", picker.flashes)
				}
				if events := rec.runners.events; len(events) == 0 || events[len(events)-1] != "unlisted" {
					t.Errorf("listing = %q, want the runner unlisted as Portal exits", events)
				}
			})
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

	listing := stateRunners{}.Listing(backendtest.New())
	cfg := runner.Config{Session: "api-x7Kd9a"}
	listing.Attach(&cfg)
	cfg.Began()
	t.Cleanup(listing.Close)

	var token string
	for _, kv := range cfg.Command.Env {
		token, _ = strings.CutPrefix(kv, runnertoken.Var+"=")
	}
	entry := filepath.Join(dir, "runners", token+".json")
	if data, err := os.ReadFile(entry); token == "" || err != nil || !strings.Contains(string(data), `"session":"api-x7Kd9a"`) {
		t.Fatalf("the entry of the token %q the attach carries, %s = %s, %v; want it on api-x7Kd9a", token, entry, data, err)
	}
	listing.Ended()
	if err := (stateRunners{}).Switch(t.Context(), token, "api-x7Kd9a", "blog-c3d4"); !errors.Is(err, runnertoken.ErrRefused) {
		t.Errorf("Switch = %v, want the refusal of the runner listed there, its attach ended", err)
	}
}
