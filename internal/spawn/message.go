package spawn

import (
	"fmt"
	"strings"
)

// QuoteJoin single-quotes each name and joins them with ", " — rendering 's2'
// for one name and 's2', 's4' for several. It is the shared renderer for the
// spawn one-line messages (the pre-flight gone-session error and the Phase-6
// picker's equivalents), so the CLI (cmd/spawn.go) and the picker (internal/tui)
// name sessions identically. An empty slice renders the empty string.
func QuoteJoin(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = "'" + name + "'"
	}
	return strings.Join(quoted, ", ")
}

// GoneVerb is the count-aware verb for the gone-session message: "is" for a
// single name (n == 1) and "are" for several (any other count, including zero).
// Paired with QuoteJoin so "'s2' is gone" / "'s2', 's4' are gone" agree in
// number.
func GoneVerb(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

// GoneMessage is the single renderer for the pre-flight gone-session outcome
// sentence — "'s2' is gone — nothing opened" for one name, "'s2', 's4' are gone
// — nothing opened" for several — composed from the shared QuoteJoin + GoneVerb
// primitives. Every caller (the CLI abort error and outcome log, the picker
// outcome log, the picker abort banner, and the capture-harness seed banner)
// renders through it so a copy edit lands in exactly one place. The body carries
// no "spawn:" prefix and no ⚠ glyph: the CLI adds the prefix at its call sites
// and the notice band prepends the glyph via statusGlyph.
func GoneMessage(names []string) string {
	return fmt.Sprintf("%s %s gone — nothing opened", QuoteJoin(names), GoneVerb(len(names)))
}

// PartialFailureMessage is the single renderer for the leave-what-opened
// partial-failure outcome sentence — "'s2' failed to open — others left open"
// for one name, "'s2', 's3' failed to open — others left open" for several —
// composed from the shared QuoteJoin primitive. Both callers (the CLI's exit-1
// error and the picker's re-asserted flash) render through it so the spec's
// "same one-line message the picker would show" contract holds and a copy edit
// lands in exactly one place. The body needs no count-aware verb: "failed to
// open" agrees with a single name and with several. The body carries no
// "spawn:" prefix and no ⚠ glyph: the CLI adds the prefix at its call site and
// the notice band prepends the glyph via statusGlyph.
func PartialFailureMessage(failed []string) string {
	return fmt.Sprintf("%s failed to open — others left open", QuoteJoin(failed))
}

// UnsupportedNoopMessage is the single renderer for the N≥2 unsupported-terminal
// atomic no-op outcome sentence. A NULL identity (remote/mosh, or a transient
// detection error folded to Identity{}) gets the honest "no host-local terminal
// — nothing opened" line; a recognised-but-undriven identity names its friendly
// name and bundle id, separated by the U+00B7 middle dot that mirrors the
// --detect echo and the design banner. Both callers (the CLI unsupported message
// and the picker's re-asserted flash) render through it. The body carries no
// "spawn:" prefix and no ⚠ glyph: the CLI adds the prefix and the notice band
// prepends the glyph via statusGlyph.
func UnsupportedNoopMessage(id Identity) string {
	if id.IsNull() {
		return "no host-local terminal — nothing opened"
	}
	return fmt.Sprintf("unsupported terminal — %s · %s — nothing opened", id.Name, id.BundleID)
}
