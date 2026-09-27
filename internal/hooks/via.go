package hooks

// Via names the calling surface a store operation was reached through, and its
// String is carried into the "via" attr of that operation's breadcrumb. The
// vocabulary is closed, and the type is deliberately not a string kind: a
// caller cannot pass an invented literal that compiles and then produces a log
// attr no search will ever match.
type Via int

const (
	// ViaCLI is a user-typed portal hook command.
	ViaCLI Via = iota + 1
	// ViaInternal is Portal reaching the store on its own behalf — the daemon's
	// stale sweep and the reads that support it.
	ViaInternal
	// ViaHydrate is the hydrate helper's lookup of a restored pane's hook.
	ViaHydrate
	// ViaDoctor is a portal doctor diagnosis.
	ViaDoctor
	// ViaPanel is a waiting pane's resume panel answering the user's discard.
	ViaPanel
)

var viaNames = map[Via]string{
	ViaCLI:      "cli",
	ViaInternal: "internal",
	ViaHydrate:  "hydrate",
	ViaDoctor:   "doctor",
	ViaPanel:    "panel",
}

// String returns the logged value, and the empty string for a Via outside the
// vocabulary — the unset zero value reads as absent rather than impersonating
// whichever surface happens to be first.
func (v Via) String() string {
	return viaNames[v]
}
