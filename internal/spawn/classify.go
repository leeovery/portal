package spawn

// classify.go is the single count-semantics chokepoint the spec designates: a
// WindowResult is "opened" exactly when Confirmed() is true, the confirmed/failed
// partition drives leave-what-opened retry, and "first permission wins" drives the
// burst-stop. Both callers — the CLI (cmd/spawn.go) and the TUI picker
// (internal/tui) — derive every opened / failed / all-confirmed / permission
// decision from these three pure functions so the two paths cannot drift; a future
// change to what "confirmed" means is a single edit here. They live alongside the
// other shared spawn renderers (PreflightMissing / QuoteJoin / GoneVerb).

// Confirmed reports whether this window's spawn was confirmed — its token marker
// appeared within the per-window ack budget (Ack == AckConfirmed). It is the sole
// "opened" predicate; every other ack (AckTimeout, AckFailed) is not confirmed.
func (r WindowResult) Confirmed() bool {
	return r.Ack == AckConfirmed
}

// PartitionResults splits results into the session names that Confirmed() (opened)
// and everything else (failed), each preserving input (list) order. "Failed"
// unifies an adapter spawn-failed (AckFailed) and an ack timeout (AckTimeout) into
// one class, exactly as the leave-what-opened retry names them. A batch is
// all-confirmed precisely when the returned failed slice is empty. Pure and
// side-effect free — it returns nil (not an empty slice) for an absent class.
func PartitionResults(results []WindowResult) (confirmed, failed []string) {
	for _, r := range results {
		if r.Confirmed() {
			confirmed = append(confirmed, r.Session)
			continue
		}
		failed = append(failed, r.Session)
	}
	return confirmed, failed
}

// FirstPermission returns the first WindowResult whose adapter Outcome is
// permission-required, plus true — the burst-stop signal both callers surface
// before the generic not-all-confirmed branch (the macOS Automation grant is
// per-(source, target), so once one window hits the wall every later window would
// hit the identical wall). It switches on the generic Outcome alone — never a
// driver detail string — keeping the permission-quarantine boundary intact. It
// returns the zero WindowResult and false when no window hit the wall.
func FirstPermission(results []WindowResult) (WindowResult, bool) {
	for _, r := range results {
		if r.Result.Outcome == OutcomePermissionRequired {
			return r, true
		}
	}
	return WindowResult{}, false
}
