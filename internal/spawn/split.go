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
