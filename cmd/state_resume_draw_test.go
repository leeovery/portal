package cmd

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
	"github.com/leeovery/portal/internal/xdg"
)

// resumeDrawProbe records everything the draw hands to its seams, so a test can
// assert on the hand-off without a process ever being replaced.
type resumeDrawProbe struct {
	stdout     bytes.Buffer
	execProg   string
	execArgs   []string
	execCalls  int
	colourless []bool

	// Bytes already on stdout when the theme resolved, so a resolve that moved
	// below the paint is visible as a non-zero reading.
	paintedAtResolve int
}

func newResumeDrawConfig(t *testing.T, p *resumeDrawProbe, payload resumeChainPayload, size func() (int, int, error)) resumeDrawConfig {
	t.Helper()
	th := themetest.DefaultDark(t)
	return resumeDrawConfig{
		resumeChainPayload: payload,
		Stdout:             &p.stdout,
		Logger:             drawTestLogger(t),
		Size:               size,
		ResolveTheme: func(colourless bool) theme.Theme {
			p.colourless = append(p.colourless, colourless)
			p.paintedAtResolve = p.stdout.Len()
			return th
		},
		ExecSelf: func(prog string, args []string) {
			p.execProg = prog
			p.execArgs = args
			p.execCalls++
		},
	}
}

func drawTestLogger(t *testing.T) *slog.Logger {
	t.Helper()
	logger, _ := logtest.NewCaptureLogger(t)
	return logger
}

func fixedSize(w, h int) func() (int, int, error) {
	return func() (int, int, error) { return w, h, nil }
}

func samplePayload() resumeChainPayload {
	return resumeChainPayload{
		Command: "make deploy",
		HookKey: "tok123",
		Pane:    "%7",
		PaneKey: "proj-a1b2:0.1",
	}
}

func TestRunResumeDraw_Painting(t *testing.T) {
	t.Run("it writes the alternate-screen entry before the panel", func(t *testing.T) {
		var probe resumeDrawProbe
		cfg := newResumeDrawConfig(t, &probe, samplePayload(), fixedSize(100, 30))

		if err := runResumeDraw(cfg); err != nil {
			t.Fatalf("runResumeDraw() error = %v", err)
		}

		out := probe.stdout.String()
		prefix := hydrateAltScreenEnter + resumeCursorHome
		if !strings.HasPrefix(out, prefix) {
			t.Fatalf("draw opened with %q, want the alternate-screen entry then a cursor home", out[:min(len(out), 24)])
		}
		if strings.Contains(out, hydrateResetPreamble) {
			t.Error("the draw wrote the leave sequence; the leave belongs to whatever answers the panel")
		}
	})

	t.Run("it resolves the theme before it paints", func(t *testing.T) {
		var probe resumeDrawProbe
		cfg := newResumeDrawConfig(t, &probe, samplePayload(), fixedSize(100, 30))

		if err := runResumeDraw(cfg); err != nil {
			t.Fatalf("runResumeDraw() error = %v", err)
		}

		if probe.paintedAtResolve != 0 {
			t.Errorf("%d bytes were on stdout when the theme resolved, want 0: the appearance question must settle before a screen is painted",
				probe.paintedAtResolve)
		}
	})

	t.Run("it paints the production renderer byte-identically", func(t *testing.T) {
		cases := []struct {
			name   string
			report string
		}{
			{"with nothing to report", ""},
			{"with a report", "could not clear the pending marker"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var probe resumeDrawProbe
				payload := samplePayload()
				payload.Report = tc.report
				cfg := newResumeDrawConfig(t, &probe, payload, fixedSize(100, 30))

				if err := runResumeDraw(cfg); err != nil {
					t.Fatalf("runResumeDraw() error = %v", err)
				}

				want := tui.RenderResumePanel(tui.ResumeScreen{
					Command: payload.Command,
					Report:  tc.report,
					Width:   100,
					Height:  30,
					Theme:   themetest.DefaultDark(t),
				})
				if got := strings.TrimPrefix(probe.stdout.String(), hydrateAltScreenEnter+resumeCursorHome); got != want {
					t.Errorf("painted bytes differ from RenderResumePanel's\n got: %q\nwant: %q", got, want)
				}
			})
		}
	})

	t.Run("it paints at the renderer's fallback when the size read fails", func(t *testing.T) {
		cases := []struct {
			name string
			size func() (int, int, error)
			w, h int
		}{
			{"read error", func() (int, int, error) { return 0, 0, errors.New("not a terminal") }, 0, 0},
			{"zero by zero", fixedSize(0, 0), 0, 0},
			{"negative", fixedSize(-4, -2), -4, -2},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var probe resumeDrawProbe
				cfg := newResumeDrawConfig(t, &probe, samplePayload(), tc.size)

				if err := runResumeDraw(cfg); err != nil {
					t.Fatalf("runResumeDraw() error = %v", err)
				}

				want := tui.RenderResumePanel(tui.ResumeScreen{
					Command: samplePayload().Command,
					Width:   tc.w,
					Height:  tc.h,
					Theme:   themetest.DefaultDark(t),
				})
				got := strings.TrimPrefix(probe.stdout.String(), hydrateAltScreenEnter+resumeCursorHome)
				if got != want {
					t.Errorf("painted bytes differ from the renderer's bounded fallback\n got: %q\nwant: %q", got, want)
				}
				if strings.TrimSpace(got) == "" {
					t.Error("the draw painted an empty screen")
				}
			})
		}
	})

	t.Run("it writes no background and runs no detection under NO_COLOR", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")

		var probe resumeDrawProbe
		cfg := newResumeDrawConfig(t, &probe, samplePayload(), fixedSize(100, 30))
		cfg.Colourless = noColorEnabled()

		if err := runResumeDraw(cfg); err != nil {
			t.Fatalf("runResumeDraw() error = %v", err)
		}

		if !slices.Equal(probe.colourless, []bool{true}) {
			t.Errorf("theme resolver called with colourless %v, want exactly one call with true", probe.colourless)
		}
		want := tui.RenderResumePanel(tui.ResumeScreen{
			Command:    samplePayload().Command,
			Width:      100,
			Height:     30,
			Theme:      themetest.DefaultDark(t),
			Colourless: true,
		})
		got := strings.TrimPrefix(probe.stdout.String(), hydrateAltScreenEnter+resumeCursorHome)
		if got != want {
			t.Errorf("painted bytes are not the colourless render\n got: %q\nwant: %q", got, want)
		}
		if strings.Contains(got, "48;2;") {
			t.Error("the painted bytes carry an SGR background parameter under NO_COLOR")
		}
	})
}

func TestRunResumeDraw_HandOff(t *testing.T) {
	t.Run("it execs the waiter carrying the payload and the size it drew at", func(t *testing.T) {
		var probe resumeDrawProbe
		payload := samplePayload()
		payload.Report = "could not clear the pending marker"
		cfg := newResumeDrawConfig(t, &probe, payload, fixedSize(100, 30))

		if err := runResumeDraw(cfg); err != nil {
			t.Fatalf("runResumeDraw() error = %v", err)
		}

		exe, err := resumeChainExe()
		if err != nil {
			t.Fatalf("resumeChainExe() error = %v", err)
		}
		drawn := payload
		drawn.Width, drawn.Height = 100, 30
		want := resumeChainArgv(exe, resumeWaitSubcommand, drawn)

		if probe.execCalls != 1 {
			t.Fatalf("ExecSelf called %d times, want exactly 1", probe.execCalls)
		}
		if probe.execProg != exe {
			t.Errorf("exec target = %q, want %q", probe.execProg, exe)
		}
		if !slices.Equal(probe.execArgs, want) {
			t.Errorf("exec argv = %q, want %q", probe.execArgs, want)
		}
	})

	t.Run("it omits the report flag when there is nothing to report", func(t *testing.T) {
		var probe resumeDrawProbe
		cfg := newResumeDrawConfig(t, &probe, samplePayload(), fixedSize(80, 24))

		if err := runResumeDraw(cfg); err != nil {
			t.Fatalf("runResumeDraw() error = %v", err)
		}

		if slices.Contains(probe.execArgs, "--"+resumeFlagReport) {
			t.Errorf("exec argv carries --%s for an empty report: %q", resumeFlagReport, probe.execArgs)
		}
	})

	t.Run("it exits non-zero when the hand-off exec fails", func(t *testing.T) {
		t.Setenv("PORTAL_LOG_LEVEL", "info")
		sink := logtest.Install(t)

		var exitCode atomic.Int32
		exitCode.Store(-1)
		var exitCalls atomic.Int32
		withOsExitFake(t, func(code int) {
			exitCalls.Add(1)
			exitCode.Store(int32(code))
		})

		var probe resumeDrawProbe
		cfg := newResumeDrawConfig(t, &probe, samplePayload(), fixedSize(100, 30))
		cfg.Logger = nil
		cfg.ExecSelf = func(_ string, args []string) {
			defaultExecShell("/nonexistent/portal-resume-draw-probe", args)
		}

		if err := runResumeDraw(cfg); err != nil {
			t.Fatalf("runResumeDraw() error = %v", err)
		}

		if got := exitCalls.Load(); got != 1 {
			t.Fatalf("osExit invoked %d times, want exactly 1", got)
		}
		if got := exitCode.Load(); got != 1 {
			t.Errorf("osExit code = %d, want 1", got)
		}
		if got := len(sink.Records().WithMessage("exec handoff failed")); got != 1 {
			t.Errorf("exec-failure WARN recorded %d times, want exactly 1", got)
		}
		if !strings.HasPrefix(probe.stdout.String(), hydrateAltScreenEnter) {
			t.Error("the alternate screen was not entered before the failing hand-off")
		}
	})
}

func TestPaneDrawNomination(t *testing.T) {
	shippedPair := func(t *testing.T) (theme.Theme, theme.Theme) {
		t.Helper()
		return themetest.DefaultLight(t), themetest.DefaultDark(t)
	}

	t.Run("it paints from the shipped pair when the prefs route fails", func(t *testing.T) {
		brokenLoader := theme.Loader{BuiltinSource: func(string) ([]byte, bool) { return []byte("not a theme"), true }}

		cases := []struct {
			name   string
			open   func() (*prefs.Store, error)
			loader theme.Loader
		}{
			{"path error", func() (*prefs.Store, error) { return nil, errors.New("no home") }, newThemeLoader()},
			{"read error", func() (*prefs.Store, error) { return prefs.NewStore(unreadablePrefsFile(t)), nil }, newThemeLoader()},
			{"resolution error", func() (*prefs.Store, error) { return prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json")), nil }, brokenLoader},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := paneDrawNomination(tc.open, tc.loader)
				light, dark := shippedPair(t)
				if got.Select(theme.MemberLight) != light || got.Select(theme.MemberDark) != dark {
					t.Errorf("nomination did not degrade to the shipped light/dark pair")
				}
			})
		}
	})

	t.Run("it never takes the migrating prefs route", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "prefs.json")
		body := []byte(`{"appearance":"dark"}`)
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatalf("seeding prefs.json: %v", err)
		}
		t.Setenv(xdg.PrefsFile.EnvVar, path)

		var translations atomic.Int32
		withFuncSeam(t, &persistTranslation, func(*prefs.Store, string) { translations.Add(1) })

		paneDrawNomination(loadPrefsStoreNoMigrate, newThemeLoader())

		if got := translations.Load(); got != 0 {
			t.Errorf("the theme read dispatched %d appearance translations, want 0", got)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("re-reading prefs.json: %v", err)
		}
		if !bytes.Equal(after, body) {
			t.Errorf("prefs.json = %q after the theme read, want the seeded %q unchanged", after, body)
		}
	})
}

// The command is driven end to end because the flag names live in two places —
// what resumeChainArgv emits and what the command registers — and a
// transposition between them compiles.
func TestStateResumeDrawCommand(t *testing.T) {
	t.Run("it parses the chain argv into the payload it was composed from", func(t *testing.T) {
		payload := resumeChainPayload{
			Command: "make deploy",
			Report:  "could not clear the pending marker",
			HookKey: "tok123",
			Pane:    "%7",
			PaneKey: "proj-a1b2:0.1",
			Width:   120,
			Height:  40,
		}

		var got resumeDrawConfig
		withFuncSeam(t, &resumeDrawRunFunc, func(cfg resumeDrawConfig) error {
			got = cfg
			return nil
		})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		errBuf := new(bytes.Buffer)
		rootCmd.SetErr(errBuf)
		rootCmd.SetArgs(resumeChainArgv("portal", resumeDrawSubcommand, payload)[1:])
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("executing the composed argv: %v\nstderr: %s", err, errBuf)
		}

		// Width and Height are left out: the draw measures the pane itself, and
		// parsing the argv at all is what proves the two flags are registered.
		want := payload
		want.Width, want.Height = 0, 0
		if got.resumeChainPayload != want {
			t.Errorf("payload = %+v, want %+v", got.resumeChainPayload, want)
		}
	})

	t.Run("it refuses an invocation naming no command", func(t *testing.T) {
		withFuncSeam(t, &resumeDrawRunFunc, func(resumeDrawConfig) error { return nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"state", resumeDrawSubcommand, "--pane", "%7"})
		if err := rootCmd.Execute(); err == nil {
			t.Error("executing resume-draw with no --command succeeded; the flag is required")
		}
	})
}

func unreadablePrefsFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prefs.json")
	if err := os.WriteFile(path, []byte(`{"theme":"nord"}`), 0o600); err != nil {
		t.Fatalf("seeding prefs.json: %v", err)
	}
	if err := themetest.DenyRead(t, path); err == nil {
		t.Fatal("themetest.DenyRead staged a readable file")
	}
	return path
}
