package cmd

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
)

func drawScreenPayload(screen string) resumeChainPayload {
	p := samplePayload()
	p.Screen = screen
	return p
}

func drawScreen(t *testing.T, payload resumeChainPayload, w, h int) (string, *resumeDrawProbe) {
	t.Helper()
	probe := &resumeDrawProbe{}
	cfg := newResumeDrawConfig(t, probe, payload, fixedSize(w, h))
	if err := runResumeDraw(cfg); err != nil {
		t.Fatalf("runResumeDraw() error = %v", err)
	}
	prefix := hydrateAltScreenEnter + resumeCursorHome
	out := probe.stdout.String()
	if !strings.HasPrefix(out, prefix) {
		t.Fatalf("draw opened with %q, want the alternate-screen entry then a cursor home", out[:min(len(out), 24)])
	}
	return strings.TrimPrefix(out, prefix), probe
}

func screenOf(t *testing.T, payload resumeChainPayload, w, h int) tui.ResumeScreen {
	t.Helper()
	return tui.ResumeScreen{
		Command: payload.Command,
		Report:  payload.Report,
		Width:   w,
		Height:  h,
		Theme:   themetest.DefaultDark(t),
	}
}

func TestRunResumeDraw_Screen(t *testing.T) {
	t.Run("it paints the confirmation through the production renderer", func(t *testing.T) {
		payload := drawScreenPayload(resumeScreenDiscard)
		got, _ := drawScreen(t, payload, 100, 30)

		want := tui.RenderResumeDiscardConfirm(screenOf(t, payload, 100, 30))
		if got != want {
			t.Errorf("painted bytes differ from RenderResumeDiscardConfirm's\n got: %q\nwant: %q", got, want)
		}
		if got == tui.RenderResumePanel(screenOf(t, payload, 100, 30)) {
			t.Error("the confirmation painted the waiting panel's bytes")
		}
	})

	t.Run("it paints the waiting panel for an absent, empty or unrecognised screen", func(t *testing.T) {
		cases := []struct {
			name   string
			screen string
		}{
			{"absent", resumeScreenPanel},
			{"empty", ""},
			{"panel", "panel"},
			{"upper-case discard", "DISCARD"},
			{"x", "x"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				payload := drawScreenPayload(tc.screen)
				got, _ := drawScreen(t, payload, 100, 30)

				want := tui.RenderResumePanel(screenOf(t, payload, 100, 30))
				if got != want {
					t.Errorf("painted bytes differ from RenderResumePanel's\n got: %q\nwant: %q", got, want)
				}
			})
		}
	})

	t.Run("it takes one path for both screens", func(t *testing.T) {
		sequenceFor := func(t *testing.T, screen string) []string {
			t.Helper()
			var calls []string
			rec := &labellingWriter{calls: &calls}
			th := themetest.DefaultDark(t)
			cfg := resumeDrawConfig{
				resumeChainPayload: drawScreenPayload(screen),
				Stdout:             rec,
				Logger:             drawTestLogger(t),
				Size: func() (int, int, error) {
					calls = append(calls, "size")
					return 100, 30, nil
				},
				ResolveTheme: func(bool, func() error) (theme.Theme, error) {
					calls = append(calls, "theme")
					return th, nil
				},
				ExecSelf: func(string, []string) {
					calls = append(calls, "exec")
				},
			}
			if err := runResumeDraw(cfg); err != nil {
				t.Fatalf("runResumeDraw() error = %v", err)
			}
			return calls
		}

		want := []string{"size", "theme", "write:alt-screen", "write:cursor-home", "write:paint", "exec"}
		panel := sequenceFor(t, resumeScreenPanel)
		discard := sequenceFor(t, resumeScreenDiscard)
		if !slices.Equal(panel, want) {
			t.Errorf("panel seam calls = %q, want %q", panel, want)
		}
		if !slices.Equal(discard, panel) {
			t.Errorf("confirmation seam calls = %q, want the panel's %q", discard, panel)
		}
	})

	t.Run("it carries the screen onto the waiter", func(t *testing.T) {
		cases := []struct {
			name       string
			screen     string
			wantScreen bool
		}{
			{"panel", resumeScreenPanel, false},
			{"discard", resumeScreenDiscard, true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				payload := drawScreenPayload(tc.screen)
				_, probe := drawScreen(t, payload, 100, 30)

				if probe.execCalls != 1 {
					t.Fatalf("ExecSelf called %d times, want exactly 1", probe.execCalls)
				}
				if len(probe.execArgs) < 3 || probe.execArgs[2] != resumeWaitSubcommand {
					t.Fatalf("exec argv = %q, want a %s invocation", probe.execArgs, resumeWaitSubcommand)
				}
				i := slices.Index(probe.execArgs, flagArg(resumeFlagScreen))
				if !tc.wantScreen {
					if i >= 0 {
						t.Errorf("exec argv carries --%s for the panel: %q", resumeFlagScreen, probe.execArgs)
					}
					return
				}
				if i < 0 || i+1 >= len(probe.execArgs) || probe.execArgs[i+1] != resumeScreenDiscard {
					t.Errorf("exec argv = %q, want --%s %s", probe.execArgs, resumeFlagScreen, resumeScreenDiscard)
				}
			})
		}
	})

	t.Run("it renders the report on whichever screen is drawn", func(t *testing.T) {
		const report = "could not remove the registration"
		cases := []struct {
			name   string
			screen string
			render func(tui.ResumeScreen) string
		}{
			{"panel", resumeScreenPanel, tui.RenderResumePanel},
			{"discard", resumeScreenDiscard, tui.RenderResumeDiscardConfirm},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				payload := drawScreenPayload(tc.screen)
				payload.Report = report
				got, _ := drawScreen(t, payload, 100, 30)

				want := tc.render(screenOf(t, payload, 100, 30))
				if got != want {
					t.Errorf("painted bytes differ from the screen's renderer\n got: %q\nwant: %q", got, want)
				}
				if !strings.Contains(ansi.Strip(got), report) {
					t.Errorf("the %s screen does not carry the report:\n%s", tc.name, ansi.Strip(got))
				}
			})
		}
	})

	t.Run("it degrades the confirmation with the pane", func(t *testing.T) {
		const frameCorner = "╭"
		cases := []struct {
			name      string
			w, h      int
			wantFrame bool
		}{
			{"roomy", 100, 30, true},
			{"just the default terminal", 80, 24, true},
			{"below the card's size", 30, 12, false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				payload := drawScreenPayload(resumeScreenDiscard)
				got, _ := drawScreen(t, payload, tc.w, tc.h)

				want := tui.RenderResumeDiscardConfirm(screenOf(t, payload, tc.w, tc.h))
				if got != want {
					t.Errorf("painted bytes differ from RenderResumeDiscardConfirm's\n got: %q\nwant: %q", got, want)
				}
				if framed := strings.Contains(ansi.Strip(got), frameCorner); framed != tc.wantFrame {
					t.Errorf("frame drawn = %v, want %v:\n%s", framed, tc.wantFrame, ansi.Strip(got))
				}
			})
		}
	})
}

func TestResumeChainArgv_Screen(t *testing.T) {
	t.Run("it composes the waiting panel's argv unchanged", func(t *testing.T) {
		payload := resumeChainPayload{
			Command: "claude --resume abc",
			Report:  "could not clear the pending marker",
			HookKey: "tok123",
			Pane:    "%7",
			PaneKey: "proj-a1b2:0.1",
			Width:   120,
			Height:  40,
			Screen:  resumeScreenPanel,
		}
		for _, sub := range []string{resumeDrawSubcommand, resumeWaitSubcommand} {
			got := resumeChainArgv("/usr/local/bin/portal", sub, payload)
			want := []string{
				"/usr/local/bin/portal", "state", sub,
				"--command", "claude --resume abc",
				"--report", "could not clear the pending marker",
				"--hook-key", "tok123",
				"--pane", "%7",
				"--pane-key", "proj-a1b2:0.1",
				"--width", "120",
				"--height", "40",
			}
			if !slices.Equal(got, want) {
				t.Errorf("%s argv = %q, want %q", sub, got, want)
			}
		}
	})

	t.Run("it adds only the screen flag to the confirmation's argv", func(t *testing.T) {
		panel := resumeChainPayload{Command: "c", HookKey: "k", Pane: "%1", PaneKey: "s:0.0", Width: 80, Height: 24}
		discard := panel
		discard.Screen = resumeScreenDiscard

		for _, sub := range []string{resumeDrawSubcommand, resumeWaitSubcommand} {
			base := resumeChainArgv("/p", sub, panel)
			got := resumeChainArgv("/p", sub, discard)
			want := append(slices.Clone(base), "--screen", "discard")
			if !slices.Equal(got, want) {
				t.Errorf("%s argv = %q, want the panel's argv plus --screen discard: %q", sub, got, want)
			}
		}
	})

	t.Run("it carries no screen to the chain's tail", func(t *testing.T) {
		got := resumeChainArgv("/p", resumeRecoverSubcommand, resumeChainPayload{Pane: "%7", PaneKey: "s:0.0", Screen: resumeScreenDiscard})
		want := []string{"/p", "state", "resume-recover", "--pane", "%7", "--pane-key", "s:0.0"}
		if !slices.Equal(got, want) {
			t.Errorf("argv = %q, want %q", got, want)
		}
	})
}

// Both chain commands are driven end to end because the flag lives in two
// places — what resumeChainArgv emits and what each command registers — and a
// command that forgot to read it would compile.
func TestStateResumeCommands_Screen(t *testing.T) {
	payload := resumeChainPayload{
		Command: "make deploy",
		HookKey: "tok123",
		Pane:    "%7",
		PaneKey: "proj-a1b2:0.1",
		Screen:  resumeScreenDiscard,
	}

	execute := func(t *testing.T, sub string) {
		t.Helper()
		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		errBuf := new(bytes.Buffer)
		rootCmd.SetErr(errBuf)
		rootCmd.SetArgs(resumeChainArgv("portal", sub, payload)[1:])
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("executing the composed %s argv: %v\nstderr: %s", sub, err, errBuf)
		}
	}

	t.Run("the draw carries the screen onto its payload", func(t *testing.T) {
		var got resumeDrawConfig
		withFuncSeam(t, &resumeDrawRunFunc, func(cfg resumeDrawConfig) error {
			got = cfg
			return nil
		})
		execute(t, resumeDrawSubcommand)
		if got.Screen != resumeScreenDiscard {
			t.Errorf("draw payload screen = %q, want %q", got.Screen, resumeScreenDiscard)
		}
	})

	t.Run("the waiter carries the screen onto its payload", func(t *testing.T) {
		var got resumeWaitConfig
		withFuncSeam(t, &resumeWaitRunFunc, func(cfg resumeWaitConfig) error {
			got = cfg
			return nil
		})
		execute(t, resumeWaitSubcommand)
		if got.Screen != resumeScreenDiscard {
			t.Errorf("wait payload screen = %q, want %q", got.Screen, resumeScreenDiscard)
		}
	})
}

type labellingWriter struct {
	calls *[]string
}

func (w *labellingWriter) Write(p []byte) (int, error) {
	switch string(p) {
	case hydrateAltScreenEnter:
		*w.calls = append(*w.calls, "write:alt-screen")
	case resumeCursorHome:
		*w.calls = append(*w.calls, "write:cursor-home")
	default:
		*w.calls = append(*w.calls, "write:paint")
	}
	return len(p), nil
}
