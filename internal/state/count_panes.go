package state

// CountPanes returns the total number of panes across every window in every
// session of idx. The zero-pane case (empty session or window) contributes 0,
// so the count is exact regardless of canonicalisation state. It is the single
// pane-counting implementation shared by `portal doctor`'s sessions.json check.
func CountPanes(idx Index) int {
	total := 0
	for _, s := range idx.Sessions {
		for _, w := range s.Windows {
			total += len(w.Panes)
		}
	}
	return total
}
