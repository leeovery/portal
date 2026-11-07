package spawn

import (
	"fmt"
	"strings"
)

func QuoteJoin(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = "'" + name + "'"
	}
	return strings.Join(quoted, ", ")
}

func GoneVerb(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

// GoneMessage carries no component prefix and no glyph; callers add those.
func GoneMessage(names []string) string {
	return fmt.Sprintf("%s %s gone — nothing opened", QuoteJoin(names), GoneVerb(len(names)))
}

// PartialFailureMessage's othersOpened selects the genuine-partial wording over
// the total-failure one.
func PartialFailureMessage(failed []string, othersOpened bool) string {
	if othersOpened {
		return fmt.Sprintf("%s failed to open — others left open", QuoteJoin(failed))
	}
	return fmt.Sprintf("%s failed to open — nothing opened", QuoteJoin(failed))
}

// UnsureTerminal is why an unsure identity opens nothing, for every surface
// that says so.
const UnsureTerminal = "can't tell which terminal Portal is running in"

// UnsupportedNoopMessage names the bundle id for a non-NULL identity: it is the
// terminals.json key the user needs.
func UnsupportedNoopMessage(id Identity) string {
	switch {
	case id.Unsure:
		return UnsureTerminal + " — nothing opened"
	case id.IsNull():
		return "can't open new windows over a remote connection — nothing opened"
	}
	return fmt.Sprintf("can't open new windows in %s · %s — nothing opened", id.Name, id.BundleID)
}
