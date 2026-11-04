package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// hydrateSteps records the helper's steps in the order it takes them, each
// seam checking, as it is reached, what the steps before it must have left.
type hydrateSteps struct {
	t     *testing.T
	w     *hydrateWorld
	taken []string
}

func (s *hydrateSteps) take(step string) { s.taken = append(s.taken, step) }

// stepBackend is the world's backend, reporting each read of the session to
// its steps.
type stepBackend struct {
	*backendtest.Fake
	read func()
}

func (b stepBackend) Session(ctx context.Context, name string) (backend.Session, error) {
	b.read()
	return b.Fake.Session(ctx, name)
}

// stalledBackend's Session answers only once its context ends.
type stalledBackend struct{ *backendtest.Fake }

func (stalledBackend) Session(ctx context.Context, _ string) (backend.Session, error) {
	<-ctx.Done()
	return backend.Session{}, context.Cause(ctx)
}

// instrument wires every step of cfg to report to s.
func (s *hydrateSteps) instrument(cfg *hydrateConfig, body string) {
	t, w := s.t, s.w
	cfg.Stdout = writerFunc(func(p []byte) {
		if bytes.Contains(p, []byte(body)) {
			s.take("replay")
		}
	})
	cfg.Settle = func() { s.take("settle") }
	cfg.SetPending = func(dir, name string) error {
		s.take("raise resume-pending")
		if !w.hydrating(t) {
			t.Error("resume-pending was raised after the hydrating flag came down")
		}
		return state.ResumePending.Set(dir, name)
	}
	withBackendDeps(t, BackendDeps{Backend: stepBackend{Fake: w.backend, read: func() {
		s.take("read creation")
		if !w.hydrating(t) {
			t.Error("the session's creation was read for its claim after the hydrating flag came down")
		}
	}}})
	cfg.ClearHydrating = func(dir, name string) (bool, error) {
		s.take("lower hydrating")
		if !w.recordsLiveCreation(t) {
			t.Error("the hydrating flag came down before the session's creation was recorded")
		}
		return state.Hydrating.Clear(dir, name)
	}
	exec := cfg.ExecShell
	cfg.ExecShell = func(prog string, args []string) {
		s.take("hand off")
		if w.hydrating(t) {
			t.Error("the session was handed on with its hydrating flag still raised")
		}
		exec(prog, args)
	}
}

func TestHydrate_TakesItsStepsInOrder(t *testing.T) {
	cases := []struct {
		name string
		mode resumemode.Mode
		want []string
	}{
		{
			name: "a lazy session raises resume-pending before its creation is claimed and its restore ends",
			mode: resumemode.Lazy,
			want: []string{"replay", "settle", "raise resume-pending", "read creation", "lower hydrating", "hand off"},
		},
		{
			name: "an eager session claims its creation before its restore ends",
			mode: resumemode.Eager,
			want: []string{"replay", "settle", "read creation", "lower hydrating", "hand off"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newHydrateWorld(t)
			const body = "BEFORE REBOOT"
			exec := &stubExecShell{}
			cfg := w.config(exec)
			cfg.File = transcript(t, body)
			cfg.HookStore = hydrateStoreWithMode(t, hydrateSession, "echo hi", tc.mode)
			cfg.ResolveExe = func() (string, error) { return lazyExe, nil }
			steps := &hydrateSteps{t: t, w: w}
			steps.instrument(&cfg, body)

			runHydrate(t.Context(), cfg)

			if !slices.Equal(steps.taken, tc.want) {
				t.Errorf("steps = %q, want %q", steps.taken, tc.want)
			}
			if tc.mode == resumemode.Lazy && !flagRaised(t, w.stateDir, hydrateSession) {
				t.Error("the waiting session's resume-pending flag is down at its hand-off")
			}
		})
	}
}

func TestHydrate_RecordsTheSessionsCreation(t *testing.T) {
	saved := sessionindex.Entry{
		Name:           hydrateSession,
		Label:          "api work",
		Dir:            "/work/api",
		CreatedAt:      testBoot.Add(-24 * time.Hour),
		PID:            900,
		ScrollbackFile: "scrollback/proj-a1b2.bin",
	}

	t.Run("it records the creation restore made over the entry it saved, keeping the rest", func(t *testing.T) {
		w := newHydrateWorld(t)
		w.seedIndex(t, saved)

		runHydrate(t.Context(), w.config(&stubExecShell{}))

		got, _ := w.entry(t)
		want := saved
		want.CreatedAt, want.PID = w.live.Created, w.live.PID
		if got != want {
			t.Errorf("entry = %+v, want %+v", got, want)
		}
	})

	// Under the rule a claim judges by, an entry from this boot recording
	// another creation is replaced, unless its session is restoring. Only a
	// claim made while the hydrating flag is up keeps the label.
	t.Run("it claims while the session is flagged hydrating, so an entry of this boot is adopted, never replaced", func(t *testing.T) {
		w := newHydrateWorld(t)
		withFuncSeam(t, &claimRule, func(dir string) sessionindex.Rule {
			return sessionindex.NewRule(testBoot, func(name string) bool { return state.Flagged(dir, name) })
		})
		thisBoot := saved
		thisBoot.CreatedAt = testBoot.Add(30 * time.Minute)
		w.seedIndex(t, thisBoot)

		runHydrate(t.Context(), w.config(&stubExecShell{}))

		got, _ := w.entry(t)
		if got.Label != saved.Label || got.ScrollbackFile != saved.ScrollbackFile {
			t.Errorf("entry = %+v, want it adopted with its label and scrollback kept", got)
		}
		if !w.recordsLiveCreation(t) {
			t.Errorf("entry = %+v, want the live creation recorded", got)
		}
	})

	t.Run("an entry already recording the creation is left as it is", func(t *testing.T) {
		w := newHydrateWorld(t)
		recorded := saved
		recorded.CreatedAt, recorded.PID = w.live.Created, w.live.PID
		w.seedIndex(t, recorded)
		before, err := os.ReadFile(sessionindex.Path(w.stateDir))
		if err != nil {
			t.Fatal(err)
		}

		runHydrate(t.Context(), w.config(&stubExecShell{}))

		after, err := os.ReadFile(sessionindex.Path(w.stateDir))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Errorf("sessions.json rewritten:\n%s\nwas:\n%s", after, before)
		}
	})

	t.Run("a session the index holds no entry for gets one recording its creation", func(t *testing.T) {
		w := newHydrateWorld(t)

		runHydrate(t.Context(), w.config(&stubExecShell{}))

		if !w.recordsLiveCreation(t) {
			got, _ := w.entry(t)
			t.Errorf("entry = %+v, want one recording the live creation", got)
		}
	})
}

func TestHydrate_ARecordThatFailsNeverHoldsTheHandOff(t *testing.T) {
	cases := []struct {
		name  string
		stage func(t *testing.T, w *hydrateWorld, cfg *hydrateConfig)
		cause string
	}{
		{name: "the backend can't read the session", cause: "session is gone", stage: func(_ *testing.T, w *hydrateWorld, _ *hydrateConfig) {
			w.backend.Fail(backendtest.OpSession, errors.New("session is gone"))
		}},
		{name: "the session reports no creation", cause: "did not report when it was created", stage: func(t *testing.T, w *hydrateWorld, _ *hydrateConfig) {
			w.backend.Add(backend.Session{Name: hydrateSession, PID: 4242})
		}},
		{name: "the read outlasts its bound", cause: "deadline exceeded", stage: func(t *testing.T, w *hydrateWorld, cfg *hydrateConfig) {
			withBackendDeps(t, BackendDeps{Backend: stalledBackend{w.backend}})
			cfg.SessionBound = 20 * time.Millisecond
		}},
		{name: "the index is the tmux version's", cause: "portal migrate", stage: func(t *testing.T, w *hydrateWorld, _ *hydrateConfig) {
			if err := os.WriteFile(sessionindex.Path(w.stateDir), []byte(legacyIndexBody), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run("when "+tc.name+", it records one WARN and hands the session on", func(t *testing.T) {
			w := newHydrateWorld(t)
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			t.Setenv("SHELL", "/bin/zsh")
			exec := &stubExecShell{}
			cfg := w.config(exec)
			cfg.Logger = logger
			tc.stage(t, w, &cfg)
			start := time.Now()

			runHydrate(t.Context(), cfg)

			if took := time.Since(start); took > time.Second {
				t.Errorf("the helper took %v to hand the session on", took)
			}
			if !reflect.DeepEqual(exec.args, []string{"-zsh"}) {
				t.Errorf("exec args = %q, want the login shell", exec.args)
			}
			warn := execLogLine(t, sink.Body(), "WARN", "record session creation failed")
			if !strings.Contains(warn, "session="+hydrateSession) || !strings.Contains(warn, tc.cause) {
				t.Errorf("WARN %q, want it to name the session and %q", warn, tc.cause)
			}
			if n := strings.Count(sink.Body(), "WARN"); n != 1 {
				t.Errorf("got %d WARNs, want the record's alone: %q", n, sink.Body())
			}
			if w.hydrating(t) {
				t.Error("the session's restore never ended")
			}
		})
	}
}

func TestHydrate_EndsTheSessionsRestore(t *testing.T) {
	t.Run("it lowers the session's hydrating flag", func(t *testing.T) {
		w := newHydrateWorld(t)

		runHydrate(t.Context(), w.config(&stubExecShell{}))

		if w.hydrating(t) {
			t.Error("the hydrating flag is still raised")
		}
	})

	t.Run("a flag it can't lower is one WARN, and the session is handed on", func(t *testing.T) {
		w := newHydrateWorld(t)
		flags := filepath.Join(w.stateDir, string(state.Hydrating))
		if err := os.Chmod(flags, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(flags, 0o700) })
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		exec := &stubExecShell{}
		cfg := w.config(exec)
		cfg.Logger = logger

		runHydrate(t.Context(), cfg)

		warn := execLogLine(t, sink.Body(), "WARN", "clear hydrating flag failed")
		if !strings.Contains(warn, "session="+hydrateSession) || !strings.Contains(warn, "error=") {
			t.Errorf("WARN %q, want it to name the session and the error", warn)
		}
		if n := strings.Count(sink.Body(), "WARN"); n != 1 {
			t.Errorf("got %d WARNs, want the lowering's alone: %q", n, sink.Body())
		}
		if !exec.called {
			t.Error("the session was never handed on")
		}
	})
}

func TestHydrate_ActsOnlyInTheSessionItRestores(t *testing.T) {
	cases := []struct {
		name    string
		current string
		cause   string
	}{
		{name: "zmx names another session", current: "proj-other", cause: `ZMX_SESSION is "proj-other"`},
		{name: "zmx names no session", current: "", cause: "ZMX_SESSION is unset"},
	}
	for _, tc := range cases {
		t.Run("when "+tc.name+", it runs the plain login shell and touches nothing the name keys", func(t *testing.T) {
			w := newHydrateWorld(t)
			w.seedIndex(t, sessionindex.Entry{Name: hydrateSession, CreatedAt: testBoot.Add(-time.Hour)})
			index, err := os.ReadFile(sessionindex.Path(w.stateDir))
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("SHELL", "/bin/zsh")
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			stdout := new(bytes.Buffer)
			exec := &stubExecShell{}
			cfg := w.config(exec)
			cfg.Current, cfg.Logger, cfg.Stdout = tc.current, logger, stdout
			cfg.File = transcript(t, "OLD")
			cfg.HookStore = hydrateStoreWithMode(t, hydrateSession, "echo hi", resumemode.Lazy)
			cfg.ResolveExe = func() (string, error) { return lazyExe, nil }

			runHydrate(t.Context(), cfg)

			if exec.target != "/bin/zsh" || !reflect.DeepEqual(exec.args, []string{"-zsh"}) {
				t.Errorf("exec = %q %q, want the plain login shell", exec.target, exec.args)
			}
			warn := execLogLine(t, sink.Body(), "WARN", "not run in the session it restores, so no hook and no panel")
			if !strings.Contains(warn, "session="+hydrateSession) || !strings.Contains(warn, tc.cause) {
				t.Errorf("WARN %q, want it to name the session and %s", warn, tc.cause)
			}
			if n := strings.Count(sink.Body(), "WARN"); n != 1 {
				t.Errorf("got %d WARNs, want the refusal alone: %q", n, sink.Body())
			}
			if strings.Contains(sink.Body(), "hook lookup") {
				t.Errorf("the helper looked a hook up: %q", sink.Body())
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing replayed", stdout.String())
			}
			if !w.hydrating(t) || flagRaised(t, w.stateDir, hydrateSession) {
				t.Errorf("hydrating raised = %v, resume-pending raised = %v; want both left as restore left them", w.hydrating(t), flagRaised(t, w.stateDir, hydrateSession))
			}
			if after, _ := os.ReadFile(sessionindex.Path(w.stateDir)); !bytes.Equal(after, index) {
				t.Errorf("sessions.json changed to %s", after)
			}
			if calls := w.backend.Calls(); len(calls) != 0 {
				t.Errorf("backend calls = %+v, want none", calls)
			}
		})
	}

	t.Run("an empty session name reaches no hook, not even one registered under it", func(t *testing.T) {
		w := newHydrateWorld(t)
		t.Setenv("SHELL", "/bin/zsh")
		exec := &stubExecShell{}
		cfg := w.config(exec)
		cfg.Session, cfg.Current = "", ""
		cfg.HookStore = hydrateStoreWithMode(t, "", "rm -rf /", resumemode.Eager)

		runHydrate(t.Context(), cfg)

		if !reflect.DeepEqual(exec.args, []string{"-zsh"}) {
			t.Errorf("exec args = %q, want the plain login shell", exec.args)
		}
	})
}

func TestStateHydrateCommand(t *testing.T) {
	t.Run("it runs for the session it names, checked against the one zmx names", func(t *testing.T) {
		w := newHydrateWorld(t)
		w.backend.SetCurrent("proj-current")
		var got hydrateConfig
		withFuncSeam(t, &hydrateRunFunc, func(_ context.Context, cfg hydrateConfig) { got = cfg })

		if _, _, err := runRootCmd(t, "state", "hydrate", "--session", hydrateSession, "--file", "/state/scrollback/x.bin"); err != nil {
			t.Fatalf("state hydrate: %v", err)
		}

		if got.Session != hydrateSession || got.Current != "proj-current" || got.File != "/state/scrollback/x.bin" {
			t.Errorf("session, current, file = %q, %q, %q; want %q, %q, %q",
				got.Session, got.Current, got.File, hydrateSession, "proj-current", "/state/scrollback/x.bin")
		}
	})

	t.Run("it runs on a version 1 sessions.json, as a helper a restored session runs must", func(t *testing.T) {
		seedSessionsJSON(t, legacyIndexBody)
		ran := false
		withFuncSeam(t, &hydrateRunFunc, func(context.Context, hydrateConfig) { ran = true })

		if _, _, err := runRootCmd(t, "state", "hydrate", "--session", hydrateSession); err != nil {
			t.Fatalf("state hydrate on a version 1 sessions.json: %v", err)
		}
		if !ran {
			t.Error("the helper's body did not run")
		}
	})
}
