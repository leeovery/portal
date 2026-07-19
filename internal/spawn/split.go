package spawn

// SplitNetN splits an ordered selection into the net-N external/trigger pair the
// picker's all-attach multi-select burst opens: external is the leading N-1 rows
// (the windows opened in the host terminal) and trigger is the last row (the
// self-attach target the calling window hands off to). It computes the "net N
// windows, never N+1" invariant for the TRAILING-trigger convention.
//
// SOLE consumer: the picker's dispatchBurst (internal/tui). It is deliberately
// DISTINCT from SplitTriggerFirst — the LEADING-trigger split the multi-target open
// burst (cmd/open_burst_run.go) uses — so the trailing-vs-leading trigger choice
// reads as a deliberate per-caller convention, not an accident.
//
// Precondition: callers guarantee len(ordered) >= 1. The picker's decideBurst
// returns early on an empty ordered set, so a zero-length slice never reaches here
// (indexing ordered[len(ordered)-1] would panic). A single-element slice returns an
// empty external set and that element as the trigger. Pure and side-effect free.
func SplitNetN(ordered []string) (external []string, trigger string) {
	return ordered[:len(ordered)-1], ordered[len(ordered)-1]
}

// SplitTriggerFirst splits an ordered surface selection into the FIRST-trigger
// pair the multi-target open burst opens: trigger is the LEADING surface (the one
// the invoking terminal absorbs and self-connects to LAST — via switch-client /
// exec attach for an attach surface, or a local mint for a mint surface) and
// external is the trailing N-1 surfaces (the windows spawned FIRST into host
// terminals). It is the first-trigger convention of spec § "The trigger absorbs
// the first target": the trigger takes the first target in command-line order.
//
// SOLE consumer: the multi-target open burst's runOpenBurstWithDeps
// (cmd/open_burst_run.go). It is deliberately DISTINCT from the trailing-trigger
// SplitNetN — which the picker's all-attach multi-select burst uses — so the two
// conventions cannot drift onto the wrong caller.
//
// Precondition: callers guarantee len(ordered) >= 1 (runOpenBurstWithDeps is only
// reached with len(surfaces) >= 2; dispatchOpenBurst routes a single surface to
// the plain single-target connect). A single-element slice returns that element as
// the trigger and an empty external set. Pure and side-effect free.
func SplitTriggerFirst(ordered []Surface) (trigger Surface, external []Surface) {
	return ordered[0], ordered[1:]
}
