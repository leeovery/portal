package spawn

// PreflightMissing probes every session (in list order) through the injected
// exists predicate and returns those for which exists reports false, preserving
// input order. It is the shared pre-flight has-session gate for the spawn burst:
// the multi-target open burst (cmd/open_burst_run.go) aborts atomically when the
// result is non-empty, and the picker (internal/tui) reuses it to prune the gone
// sessions from its selection.
//
// It is pure and side-effect free — it performs no I/O of its own beyond calling
// exists — and returns nil (not an empty slice) when every session is present.
// Production wires exists to *tmux.Client.HasSession, which folds any probe
// error to false, so an unprobeable session is conservatively reported gone.
func PreflightMissing(sessions []string, exists func(name string) bool) []string {
	var gone []string
	for _, s := range sessions {
		if !exists(s) {
			gone = append(gone, s)
		}
	}
	return gone
}
