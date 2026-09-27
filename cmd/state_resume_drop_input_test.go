package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
	"github.com/leeovery/portal/internal/xdg"
)

// opened is the payload the hand-off that puts the confirmation up carries.
func opened(p resumeChainPayload) resumeChainPayload {
	p = onScreen(p, resumeScreenDiscard)
	p.DropInput = true
	return p
}

func assertArgvCarriesDropInput(t *testing.T, argv []string, want bool) {
	t.Helper()
	if got := slices.Contains(argv, flagArg(resumeFlagDropInput)); got != want {
		t.Errorf("hand-off argv = %q, carries %s = %v, want %v", argv, flagArg(resumeFlagDropInput), got, want)
	}
}

// dropDraw runs one draw whose theme resolution honours the drop the way
// tui.ResolvePaneTheme does — the appearance query first, then the drop — with
// every step landing in one shared sequence.
type dropDraw struct {
	calls     []string
	dropErr   error
	handed    []bool
	execArgs  []string
	execCalls int
	stdout    bytes.Buffer
}

type sequencedWriter struct{ d *dropDraw }

func (w sequencedWriter) Write(p []byte) (int, error) {
	w.d.calls = append(w.d.calls, "write")
	return w.d.stdout.Write(p)
}

func runDropDraw(t *testing.T, payload resumeChainPayload, dropErr error) *dropDraw {
	t.Helper()
	d := &dropDraw{dropErr: dropErr}
	th := themetest.DefaultDark(t)
	cfg := resumeDrawConfig{
		resumeChainPayload: payload,
		Stdout:             sequencedWriter{d: d},
		Logger:             drawTestLogger(t),
		Size:               fixedSize(100, 30),
		ResolveTheme: func(_ bool, dropInput func() error) (theme.Theme, error) {
			d.calls = append(d.calls, "appearance-query")
			d.handed = append(d.handed, dropInput != nil)
			if dropInput == nil {
				return th, nil
			}
			return th, dropInput()
		},
		DropInputQueue: func() error {
			d.calls = append(d.calls, "drop")
			return d.dropErr
		},
		ExecSelf: func(_ string, args []string) {
			d.calls = append(d.calls, "exec")
			d.execArgs = args
			d.execCalls++
		},
	}
	if err := runResumeDraw(cfg); err != nil {
		t.Fatalf("runResumeDraw() error = %v", err)
	}
	return d
}

func (d *dropDraw) drops() int {
	n := 0
	for _, c := range d.calls {
		if c == "drop" {
			n++
		}
	}
	return n
}

func (d *dropDraw) painted() string {
	return strings.TrimPrefix(d.stdout.String(), hydrateAltScreenEnter+resumeCursorHome)
}

func openingPayload() resumeChainPayload {
	return opened(samplePayload())
}

func TestResumeDropInput_HandOffs(t *testing.T) {
	t.Run("it opens the confirmation with the drop flag set", func(t *testing.T) {
		payload := reportedPayload(resumeScreenPanel)

		probe := answered(t, payload, "d")

		assertHandOff(t, probe, opened(payload))
		i := slices.Index(probe.execArgs, flagArg(resumeFlagScreen))
		if i < 0 || i+1 >= len(probe.execArgs) || probe.execArgs[i+1] != resumeScreenDiscard {
			t.Errorf("hand-off argv = %q, want --%s %s", probe.execArgs, resumeFlagScreen, resumeScreenDiscard)
		}
		assertArgvCarriesDropInput(t, probe.execArgs, true)
	})

	t.Run("no other hand-off in the waiter carries the drop flag", func(t *testing.T) {
		cases := []struct {
			name string
			run  func(t *testing.T) *resumeWaitProbe
		}{
			{"y on the confirmation", func(t *testing.T) *resumeWaitProbe {
				return answered(t, reportedPayload(resumeScreenDiscard), "y")
			}},
			{"Escape on the confirmation", func(t *testing.T) *resumeWaitProbe {
				return backOut(t, reportedPayload(resumeScreenDiscard))
			}},
			{"Enter whose marker will not clear", func(t *testing.T) *resumeWaitProbe {
				var probe resumeWaitProbe
				probe.clearErr = errors.New("marker refused")
				answerEnter(t, &probe, reportedPayload(resumeScreenPanel))
				return &probe
			}},
			{"a settle redraw of the confirmation", func(t *testing.T) *resumeWaitProbe {
				payload := drawnPayload()
				payload.Screen = resumeScreenDiscard
				h := startResumeResize(t, payload, resizedSize)
				h.elapse(h.resize())
				if err := h.wait(); err != nil {
					t.Fatalf("runResumeWait() error = %v", err)
				}
				return h.probe
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				probe := tc.run(t)
				if probe.execCalls != 1 {
					t.Fatalf("ExecSelf called %d times, want exactly 1", probe.execCalls)
				}
				assertArgvCarriesDropInput(t, probe.execArgs, false)
			})
		}
	})

	t.Run("only the confirmation's opening sets the drop flag", func(t *testing.T) {
		var setters []string
		for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
			ast.Inspect(source.File, func(n ast.Node) bool {
				fn, ok := n.(*ast.FuncDecl)
				if !ok {
					return true
				}
				if setsDropInputTrue(fn) {
					setters = append(setters, fn.Name.Name)
				}
				return false
			})
		}
		if want := []string{"resumeOpenDiscardConfirm"}; !slices.Equal(setters, want) {
			t.Errorf("functions setting DropInput true = %q, want %q", setters, want)
		}
	})
}

func setsDropInputTrue(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "DropInput" && i < len(node.Rhs) && isTrueIdent(node.Rhs[i]) {
					found = true
				}
			}
		case *ast.KeyValueExpr:
			if key, ok := node.Key.(*ast.Ident); ok && key.Name == "DropInput" && isTrueIdent(node.Value) {
				found = true
			}
		}
		return true
	})
	return found
}

func isTrueIdent(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "true"
}

func TestResumeDropInput_Draw(t *testing.T) {
	t.Run("it drops the input queue after the theme resolves and before it paints", func(t *testing.T) {
		d := runDropDraw(t, openingPayload(), nil)

		want := []string{"appearance-query", "drop", "write", "write", "write", "exec"}
		if !slices.Equal(d.calls, want) {
			t.Errorf("draw sequence = %q, want %q", d.calls, want)
		}
		if d.drops() != 1 {
			t.Errorf("the drop ran %d times, want exactly 1", d.drops())
		}
	})

	t.Run("it drops nothing when the flag is unset", func(t *testing.T) {
		cases := []struct {
			name    string
			payload func() resumeChainPayload
		}{
			{"panel draw", samplePayload},
			{"resize redraw of the confirmation", func() resumeChainPayload {
				p := drawnPayload()
				p.Screen = resumeScreenDiscard
				return p
			}},
			{"report redraw", func() resumeChainPayload { return reportedPayload(resumeScreenPanel) }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				d := runDropDraw(t, tc.payload(), errors.New("must not be reached"))

				if d.drops() != 0 {
					t.Errorf("the drop ran %d times, want 0", d.drops())
				}
				if !slices.Equal(d.handed, []bool{false}) {
					t.Errorf("theme resolution handed a drop = %v, want one resolution handed nil", d.handed)
				}
			})
		}
	})

	t.Run("it clears the drop flag from the hand-off", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			err  error
		}{{"drop succeeded", nil}, {"drop failed", errors.New("ioctl refused")}} {
			t.Run(tc.name, func(t *testing.T) {
				d := runDropDraw(t, openingPayload(), tc.err)

				if d.execCalls != 1 {
					t.Fatalf("ExecSelf called %d times, want exactly 1", d.execCalls)
				}
				if len(d.execArgs) < 3 || d.execArgs[2] != resumeWaitSubcommand {
					t.Fatalf("exec argv = %q, want a %s invocation", d.execArgs, resumeWaitSubcommand)
				}
				assertArgvCarriesDropInput(t, d.execArgs, false)
			})
		}
	})

	t.Run("it paints the waiting panel with the reason when the drop fails", func(t *testing.T) {
		const reason = "flush input queue: inappropriate ioctl for device"
		d := runDropDraw(t, openingPayload(), errors.New(reason))

		want := tui.RenderResumePanel(tui.ResumeScreen{
			Command: samplePayload().Command,
			Report:  reason,
			Width:   100,
			Height:  30,
			Theme:   themetest.DefaultDark(t),
		})
		if got := d.painted(); got != want {
			t.Errorf("painted bytes differ from the reported waiting panel\n got: %q\nwant: %q", got, want)
		}

		exe, err := resumeChainExe()
		if err != nil {
			t.Fatalf("resumeChainExe() error = %v", err)
		}
		next := samplePayload()
		next.Report = reason
		next.Width, next.Height = 100, 30
		if wantArgv := resumeChainArgv(exe, resumeWaitSubcommand, next); !slices.Equal(d.execArgs, wantArgv) {
			t.Errorf("exec argv = %q, want a waiter on the panel carrying the report %q", d.execArgs, wantArgv)
		}
	})

	t.Run("it paints no confirmation when the drop fails", func(t *testing.T) {
		d := runDropDraw(t, openingPayload(), errors.New("ioctl refused"))

		confirmation := tui.RenderResumeDiscardConfirm(tui.ResumeScreen{
			Command: samplePayload().Command,
			Width:   100,
			Height:  30,
			Theme:   themetest.DefaultDark(t),
		})
		if d.painted() == confirmation {
			t.Fatal("the confirmation was painted over input the drop could not discard")
		}
		if strings.Contains(ansi.Strip(d.painted()), "Discard resume?") {
			t.Errorf("the painted screen carries the confirmation's title:\n%s", ansi.Strip(d.painted()))
		}
		assertArgvLacks(t, d.execArgs, resumeFlagScreen)
	})

	t.Run("it paints the confirmation unchanged when the drop succeeds", func(t *testing.T) {
		d := runDropDraw(t, openingPayload(), nil)

		want := tui.RenderResumeDiscardConfirm(tui.ResumeScreen{
			Command: samplePayload().Command,
			Width:   100,
			Height:  30,
			Theme:   themetest.DefaultDark(t),
		})
		if got := d.painted(); got != want {
			t.Errorf("painted bytes differ from the confirmation's\n got: %q\nwant: %q", got, want)
		}
	})

	t.Run("it consumes no byte on the drop path", func(t *testing.T) {
		d := runDropDraw(t, openingPayload(), errors.New("ioctl refused"))

		if i := slices.Index(d.calls, "drop"); i != 1 || d.calls[0] != "appearance-query" {
			t.Errorf("draw sequence = %q, want the appearance probe as the only stdin read and ahead of the drop", d.calls)
		}
	})
}

func TestPaneDrawTheme_HandsTheDropThrough(t *testing.T) {
	t.Setenv(xdg.PrefsFile.EnvVar, filepath.Join(t.TempDir(), "prefs.json"))

	t.Run("it runs the drop once and answers its error", func(t *testing.T) {
		refused := errors.New("ioctl refused")
		calls := 0
		_, err := paneDrawTheme(true, func() error {
			calls++
			return refused
		})
		if calls != 1 {
			t.Errorf("the drop ran %d times, want exactly 1", calls)
		}
		if !errors.Is(err, refused) {
			t.Errorf("paneDrawTheme() error = %v, want the drop's %v", err, refused)
		}
	})

	t.Run("it answers no error for a nil drop", func(t *testing.T) {
		if _, err := paneDrawTheme(true, nil); err != nil {
			t.Errorf("paneDrawTheme() error = %v, want nil", err)
		}
	})
}

func TestStateResumeDrawCommand_DropInput(t *testing.T) {
	t.Run("it parses the drop flag on the draw", func(t *testing.T) {
		payload := openingPayload()
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
		if got.resumeChainPayload != payload {
			t.Errorf("payload = %+v, want %+v", got.resumeChainPayload, payload)
		}
		if got.DropInputQueue == nil {
			t.Error("the draw was wired with no drop seam")
		}
	})

	t.Run("it refuses the drop flag on the waiter", func(t *testing.T) {
		withFuncSeam(t, &resumeWaitRunFunc, func(resumeWaitConfig) error { return nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs(resumeChainArgv("portal", resumeWaitSubcommand, openingPayload())[1:])
		if err := rootCmd.Execute(); err == nil {
			t.Error("resume-wait accepted --drop-input; a flag the waiter does not honour must fail its parse")
		}
	})
}

// Two flushes would eat a keystroke the user meant after the confirmation went
// up, so the queue is touched from one place in the chain.
func TestFlushTTYInput_TouchesTheInputQueueInExactlyOnePlace(t *testing.T) {
	t.Run("it touches the input queue in exactly one place", func(t *testing.T) {
		_, sources := sourceguardtest.RepoSources(t, sourceguardtest.NonTestSources)

		var sites []string
		var flushIoctls []string
		for _, source := range sources {
			declared := map[*ast.Ident]bool{}
			for _, decl := range source.File.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "flushTTYInput" {
					declared[fn.Name] = true
				}
			}
			ast.Inspect(source.File, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.Ident:
					if node.Name == "flushTTYInput" && !declared[node] {
						sites = append(sites, positionOf(source, node.Pos()))
					}
				case *ast.SelectorExpr:
					if node.Sel.Name == "TIOCFLUSH" || node.Sel.Name == "TCFLSH" {
						flushIoctls = append(flushIoctls, source.Path)
					}
				}
				return true
			})
		}

		if len(sites) != 1 {
			t.Errorf("flushTTYInput is referenced at %q, want exactly one site", sites)
		}
		for _, path := range flushIoctls {
			if !strings.HasPrefix(path, "cmd/tty_flush_") {
				t.Errorf("%s issues a tty input flush outside flushTTYInput", path)
			}
		}
		if len(flushIoctls) != 2 {
			t.Errorf("found the flush ioctl in %q, want one declaration per platform", flushIoctls)
		}
	})
}

func positionOf(source sourceguardtest.ParsedSource, pos token.Pos) string {
	p := source.Position(pos)
	return fmt.Sprintf("%s:%d", p.Filename, p.Line)
}
