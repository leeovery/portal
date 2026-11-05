// Command capturetool renders a named, deterministic fixture of Portal's real
// TUI offline, through the production tui.Build constructor with every backend
// seam faked. It runs no bootstrap — no zmx session, no daemon, no config read. It
// runs the fixture live, full screen; or prints its frame once, as text or in
// colour; or writes the golden frames the capture tests hold the code to.
//
// Usage:
//
//	capturetool --fixture sessions-flat                            live, at the terminal's size
//	capturetool --fixture sessions-flat --theme nord
//	capturetool --fixture contrast-validation --theme ~/themes/mytheme.theme
//	capturetool --fixture sessions-flat --print                    its frame as text, at its own size
//	capturetool --fixture sessions-flat --print --ansi --size 100x30
//	capturetool --frames testdata/vhs/frames                       every frame, written afresh
//	capturetool --frames testdata/vhs/frames sessions-flat projects
//
// --theme names either a built-in slug or a path to a theme file. Both are an
// input rather than config discovery: nothing here reads prefs, resolves the
// themes directory or looks at XDG.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
)

// Read off the shipped default rather than restated, so moving that default
// moves every unflagged capture with it.
const defaultThemeSlug = theme.DefaultDarkSlug

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "capturetool:", err)
		os.Exit(1)
	}
}

// options are what the command line asks for.
type options struct {
	fixture     string
	theme       string
	print, ansi bool
	size        string
	// frames is the directory to write frames into, and named the fixtures whose
	// frames to write, or every fixture's where none is named.
	frames string
	named  []string
}

// run does what args ask: runs the fixture live, prints its frame once to
// stdout, or writes the frames. NO_COLOR, read through getenv, wins over
// --theme for the first two; the frames are written in colour whatever it says.
func run(args []string, stdout, stderr io.Writer, getenv func(string) string) error {
	o, err := parse(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	colourless := getenv("NO_COLOR") != ""
	switch {
	case o.frames != "":
		return writeFrames(o, stdout)
	case o.print:
		return printFrame(o, stdout, stderr, colourless)
	}
	final, err := live(o, stdout, stderr, colourless)
	if err != nil {
		return err
	}
	// Terminals that ignore the OSC 111 reset keep the painted canvas, so set
	// the captured original background back. The swatch paints one too, and is
	// the surface a human runs in their own terminal at a visual gate. Here
	// rather than in live: internal/tui's restore guard follows the writer one
	// call out, to main's os.Stdout.
	switch fm := final.(type) {
	case tui.Model:
		tui.RestoreTerminalBackground(stdout, fm)
	case interface {
		OriginalBackground() string
		PaintedCanvasHex() string
	}:
		tui.RestoreBackground(stdout, fm.OriginalBackground(), fm.PaintedCanvasHex())
	}
	return nil
}

// parse reads the command line, refusing what only --print takes without it,
// and anything beside --frames but the names of the frames it writes.
func parse(args []string, stderr io.Writer) (options, error) {
	var o options
	flags := flag.NewFlagSet("capturetool", flag.ContinueOnError)
	flags.StringVar(&o.fixture, "fixture", "", "named fixture to render: "+strings.Join(capture.FixtureNames(), ", "))
	flags.StringVar(&o.theme, "theme", defaultThemeSlug, "theme to render: a built-in slug (e.g. "+defaultThemeSlug+") or a path to a .theme file")
	flags.BoolVar(&o.print, "print", false, "print its frame once, as text, rather than run it live")
	flags.StringVar(&o.size, "size", "", "with --print, the terminal's size as WxH, such as 120x40: the fixture's frame size unless given")
	flags.BoolVar(&o.ansi, "ansi", false, "with --print, print its colours too")
	flags.StringVar(&o.frames, "frames", "", "write into the directory given the frames of the fixtures named after it, or of every fixture, in "+capture.FramesThemeSlug)
	// Silent while parsing, so a bad flag is said once, by main, as every other
	// refusal is; the usage is printed only when asked for.
	flags.SetOutput(io.Discard)
	err := flags.Parse(args)
	switch {
	case errors.Is(err, flag.ErrHelp):
		flags.SetOutput(stderr)
		flags.Usage()
		return options{}, err
	case err != nil:
		return options{}, fmt.Errorf("%w (--help lists the flags)", err)
	case isSet(flags, "frames"):
		return framesOnly(o, flags)
	}
	switch {
	case flags.NArg() > 0:
		return options{}, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	case !o.print && o.size != "":
		return options{}, errors.New("--size takes --print: live, the terminal gives the size")
	case !o.print && o.ansi:
		return options{}, errors.New("--ansi takes --print: live, the frame is drawn in colour")
	}
	return o, nil
}

// framesOnly is o with the names after --frames, which takes no other flag: the
// frames are written as the tests hold them, in their own theme, at their own
// sizes.
func framesOnly(o options, flags *flag.FlagSet) (options, error) {
	if o.frames == "" {
		return options{}, errors.New("--frames takes the directory to write the frames into")
	}
	var others []string
	flags.Visit(func(f *flag.Flag) {
		if f.Name != "frames" {
			others = append(others, "--"+f.Name)
		}
	})
	// Parsing stops at the first name, so a flag after one arrives as a name.
	for _, arg := range flags.Args() {
		if strings.HasPrefix(arg, "-") {
			others = append(others, arg)
		}
	}
	if len(others) > 0 {
		return options{}, fmt.Errorf("--frames takes the fixtures' names alone, not %s", strings.Join(others, ", "))
	}
	o.named = flags.Args()
	return o, nil
}

// isSet reports whether the command line gave the flag, even as empty.
func isSet(flags *flag.FlagSet, name string) bool {
	set := false
	flags.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

// writeFrames writes the frames of the fixtures o names, or of every fixture,
// into its directory, saying each stem it writes on out. A name with no fixture
// is an error before any frame is written.
func writeFrames(o options, out io.Writer) error {
	drawings, err := framesNamed(o.named)
	if err != nil {
		return err
	}
	for _, d := range drawings {
		if err := d.WriteFrame(o.frames); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, "wrote", d.Stem()); err != nil {
			return fmt.Errorf("say what's written: %w", err)
		}
	}
	return nil
}

// Every fixture keeps a frame, so naming none means all of them rather than
// those a directory happens to hold.
func framesNamed(names []string) ([]capture.Drawing, error) {
	if len(names) == 0 {
		return capture.Drawings(), nil
	}
	drawings := make([]capture.Drawing, len(names))
	for i, name := range names {
		d, err := capture.DrawingByName(name)
		if err != nil {
			return nil, fmt.Errorf("--frames: %w", err)
		}
		drawings[i] = d
	}
	return drawings, nil
}

// printFrame writes the fixture's frame to out, as a terminal of the size asked
// shows it: as text alone, or with --ansi, in colour.
func printFrame(o options, out, warnings io.Writer, colourless bool) error {
	if o.fixture == "" {
		return fixtureRequired()
	}
	d, err := capture.DrawingByName(o.fixture)
	if err != nil {
		return err
	}
	pinned, err := resolveTheme(theme.NewSilentLoader(), o.theme, warnings)
	if err != nil {
		return err
	}
	size := d.Size()
	if o.size != "" {
		if size, err = parseSize(o.size); err != nil {
			return err
		}
	}
	frame := d.Frame(pinned, colourless, size)
	if !o.ansi {
		frame = capture.Plain(frame)
	}
	_, err = io.WriteString(out, frame)
	return err
}

// parseSize reads a terminal's size given as WxH, such as 120x40.
func parseSize(s string) (capture.Size, error) {
	w, h, cut := strings.Cut(s, "x")
	width, wErr := strconv.Atoi(w)
	height, hErr := strconv.Atoi(h)
	if !cut || wErr != nil || hErr != nil || width < 1 || height < 1 {
		return capture.Size{}, fmt.Errorf("--size %q: give it as WxH, such as 120x40", s)
	}
	return capture.Size{Width: width, Height: height}, nil
}

// live runs the fixture full screen in the terminal out is, until it quits, and
// returns the model it ended on. Warnings are written before the program
// starts, so they land on the primary screen and stay readable once the alt
// screen is left.
func live(o options, out, warnings io.Writer, colourless bool) (tea.Model, error) {
	m, err := resolveProgram(o.fixture, o.theme, warnings, colourless)
	if err != nil {
		return nil, err
	}
	if file, ok := out.(term.File); !ok || !term.IsTerminal(file.Fd()) {
		return nil, errors.New("the live view needs a terminal: --print draws without one")
	}

	// The alt screen is declared through tea.View.AltScreen in tui.Model.View,
	// not a program option — Bubble Tea v2 removed tea.WithAltScreen.
	p := tea.NewProgram(m, tea.WithOutput(out), tea.WithFilter(renderSizeFilter(o.fixture)))
	return p.Run()
}

// A live window is the terminal's size rather than the screen's choice, so a
// fixture or surface that declares a render size has it substituted into the
// resize on its way to the model. A surface pins both dimensions — which of the
// card and the degraded stack is captured is the surface's decision, never the
// window's. The swatch declares none, as does an unresolvable name — which the
// program never reaches.
func renderSizeFilter(fixture string) func(tea.Model, tea.Msg) tea.Msg {
	if sf, ok := capture.SurfaceByName(fixture); ok {
		return func(_ tea.Model, msg tea.Msg) tea.Msg { return sf.PinRenderSize(msg) }
	}
	fx, err := capture.FixtureByName(fixture)
	if err != nil {
		return func(_ tea.Model, msg tea.Msg) tea.Msg { return msg }
	}
	return func(_ tea.Model, msg tea.Msg) tea.Msg { return fx.PinRenderSize(msg) }
}

// The theme resolves before either branch, so a screen capture and the contrast
// swatch cannot be judged against different palettes. The loader is the silent
// one: capturetool renders a theme rather than using or diagnosing one, so it
// emits no theme events.
func resolveProgram(fixture, themeArg string, warnings io.Writer, colourless bool) (tea.Model, error) {
	pinned, err := resolveTheme(theme.NewSilentLoader(), themeArg, warnings)
	if err != nil {
		return nil, err
	}
	if fixture == capture.ContrastValidationFixture {
		return capture.NewContrastValidationModel(pinned), nil
	}
	// Before the fixture lookup: a surface is no *Fixture, and FixtureByName
	// rejects its name as unknown.
	if sf, ok := capture.SurfaceByName(fixture); ok {
		return sf.WithTheme(pinned, colourless), nil
	}
	m, err := resolveModel(fixture, pinned, colourless)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// An interface only so the built-in rejection branch is drivable: validation of
// the embedded set makes it otherwise unreachable without breaking a shipped
// .theme file.
type themeLoader interface {
	LoadBuiltin(slug string) (theme.Result, *theme.Rejection, bool)
}

// Failure is hard, never a fallback: silently rendering the wrong theme is the
// one failure a visual gate cannot catch.
func resolveTheme(loader themeLoader, arg string, warnings io.Writer) (theme.Theme, error) {
	if isThemePath(arg) {
		return resolvePathTheme(arg, warnings)
	}
	return resolveBuiltinTheme(loader, arg)
}

// The separator half is load-bearing: without it a real file with an
// unexpected extension is rejected as an unknown built-in, an error naming the
// wrong problem. The extension half lets a file in the working directory be
// named without a `./`. No valid slug can satisfy either half.
func isThemePath(arg string) bool {
	return strings.ContainsRune(arg, filepath.Separator) || strings.HasSuffix(arg, theme.FileExtension)
}

// No filename warning here, deliberately: a slug argument names a built-in by
// design, so a reserved-name check would fire on the documented invocation.
func resolveBuiltinTheme(loader themeLoader, slug string) (theme.Theme, error) {
	result, rejection, found := loader.LoadBuiltin(slug)
	switch {
	case !found:
		return theme.Theme{}, fmt.Errorf("--theme %q names no built-in theme (available: %v)", slug, theme.BuiltinSlugs())
	case rejection != nil:
		return theme.Theme{}, fmt.Errorf("--theme %q is not usable: %w", slug, rejection)
	}
	return result.Theme, nil
}

// theme.LoadPath runs the content rungs and neither filename one, so the
// filename warning is emitted only after a successful load — a broken file
// reports its contents rather than trailing a remark about its name.
func resolvePathTheme(path string, warnings io.Writer) (theme.Theme, error) {
	result, rejection := theme.LoadPath(path)
	if rejection != nil {
		return theme.Theme{}, fmt.Errorf("--theme %q is not usable: %w", path, rejection)
	}

	warnAboutFilename(warnings, filepath.Base(path))
	return result.Theme, nil
}

// Warning rather than blocking: this is a drop-in author's visual-verification
// route, and the export workflow produces a reserved slug right up until the user
// renames the file. The derived candidate slug is never identity — it decides
// which of the two reasons to name.
func warnAboutFilename(w io.Writer, base string) {
	candidate, rejection := theme.SlugFromFilename(base)
	switch {
	case rejection != nil:
		_, _ = fmt.Fprintf(w, "capturetool: warning: %s: %s — a themes-directory file must be named <slug>.theme, lowercase (rendering it anyway)\n", base, theme.ReasonBadName)
	case slices.Contains(theme.BuiltinSlugs(), candidate):
		_, _ = fmt.Fprintf(w, "capturetool: warning: %s: %s — %q is a built-in slug, so a file with this name is ignored in the themes directory (rendering it anyway)\n", base, theme.ReasonReservedName, candidate)
	}
}

// The palette is passed as the constant nomination shape: a constant needs no
// light/dark detection, so there is no OSC 11 race and no frame captured
// mid-gate. Colourless wins over the palette: there is no canvas to select, so
// a capture shows no painted canvas whatever palette was named.
func resolveModel(fixture string, pinned theme.Theme, colourless bool) (tui.Model, error) {
	if fixture == "" {
		return tui.Model{}, fixtureRequired()
	}
	fx, err := capture.FixtureByName(fixture)
	if err != nil {
		return tui.Model{}, err
	}
	// Handed to Deps rather than assigned afterwards: the palette drives both
	// the nomination and the faked ThemeSource, which must agree.
	deps := fx.Deps(pinned)
	deps.NoColor = deps.NoColor || colourless
	return tui.Build(deps), nil
}

func fixtureRequired() error {
	return fmt.Errorf("--fixture is required (available: %v)", capture.FixtureNames())
}
