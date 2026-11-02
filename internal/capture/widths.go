package capture

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/mattn/go-runewidth"
)

// narrowWidthsEnv draws East Asian ambiguous-width glyphs, such as ▌, one cell
// wide. go-runewidth and x/ansi read it, and the locale, at init, past the
// reach of anything a running program can set, so it pins them only for a
// process started with it.
const narrowWidthsEnv = "RUNEWIDTH_EASTASIAN=0"

// RunNarrow runs the tests in m with ambiguous-width glyphs one cell wide, as
// every frame is drawn. Where this process's environment draws them wide, it
// runs the test binary again with narrowWidthsEnv and returns that run's exit
// code; the second run reads narrow, so it never runs a third.
func RunNarrow(m interface{ Run() int }) int {
	if !drawsAmbiguousWide() {
		return m.Run()
	}
	again := exec.Command(os.Args[0], os.Args[1:]...)
	again.Env = append(os.Environ(), narrowWidthsEnv)
	again.Stdin, again.Stdout, again.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := again.Run()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return exit.ExitCode()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "capture: run the tests again with narrow widths:", err)
		return 1
	}
	return 0
}

// drawsAmbiguousWide reports whether either width library read this process's
// environment as drawing ambiguous glyphs two cells wide: go-runewidth from
// RUNEWIDTH_EASTASIAN or a CJK locale, x/ansi from RUNEWIDTH_EASTASIAN alone.
func drawsAmbiguousWide() bool {
	wide, err := strconv.ParseBool(os.Getenv("RUNEWIDTH_EASTASIAN"))
	return runewidth.EastAsianWidth || err == nil && wide
}
