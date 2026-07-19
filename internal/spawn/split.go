package spawn

// SplitNetN splits an ordered selection into the net-N external/trigger pair the
// spawn burst opens: external is the leading N-1 rows (the windows opened in the
// host terminal) and trigger is the last row (the self-attach target the calling
// window hands off to). It is the single computation of the "net N windows, never
// N+1" invariant — the CLI (runSpawn in cmd/spawn.go) and the picker (dispatchBurst
// in internal/tui) both derive their split through it so the two paths cannot drift.
//
// Precondition: callers guarantee len(ordered) >= 1. The CLI's N≥2 spawn path only
// reaches the split after the empty-args usage gate, and the picker's decideBurst
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
// It is deliberately DISTINCT from the trailing-trigger SplitNetN — which serves
// the legacy `portal spawn` CLI and the picker's all-attach multi-select burst —
// so the two conventions cannot drift onto the wrong caller.
//
// Precondition: callers guarantee len(ordered) >= 1 (runOpenBurstWithDeps is only
// reached with len(surfaces) >= 2; dispatchOpenBurst routes a single surface to
// the plain single-target connect). A single-element slice returns that element as
// the trigger and an empty external set. Pure and side-effect free.
func SplitTriggerFirst(ordered []Surface) (trigger Surface, external []Surface) {
	return ordered[0], ordered[1:]
}
