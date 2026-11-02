package capture

import (
	"fmt"
	"os"
	"strconv"

	"github.com/mattn/go-runewidth"
)

// RunNarrow runs the tests in m only where ambiguous-width glyphs, such as ▌,
// draw one cell wide, as every frame is drawn. go-runewidth and x/ansi read
// RUNEWIDTH_EASTASIAN, and go-runewidth a CJK locale, at init, past the reach
// of anything a running test can set, so a wide environment is refused with
// the fix rather than failing every frame.
func RunNarrow(m interface{ Run() int }) int {
	if drawsAmbiguousWide() {
		fmt.Fprintln(os.Stderr, "capture: frames are drawn with ambiguous-width glyphs one cell wide; run with RUNEWIDTH_EASTASIAN=0")
		return 1
	}
	return m.Run()
}

func drawsAmbiguousWide() bool {
	wide, err := strconv.ParseBool(os.Getenv("RUNEWIDTH_EASTASIAN"))
	return runewidth.EastAsianWidth || err == nil && wide
}
