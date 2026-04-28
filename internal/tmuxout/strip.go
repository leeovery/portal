// Package tmuxout provides shared, dependency-free helpers for parsing
// `tmux show-*` command output. It is a leaf package — it must not import
// any other internal/* package — so both internal/tmux and internal/state
// can depend on it without risking an import cycle.
package tmuxout

// StripMatchedOuterQuotes returns s with its first and last bytes removed
// if and only if they are an identical matched pair of single or double
// quotes (i.e. `"…"` or `'…'`). Inner content is preserved verbatim and
// only the outermost pair is stripped — nested quotes are left intact.
//
// Inputs shorter than two bytes, or with mismatched outer characters, are
// returned unchanged. The function performs no I/O and never allocates a
// new string for inputs it returns unchanged.
//
// It is the canonical helper for un-quoting values produced by
// `tmux show-options -sv` and `tmux show-hooks -g`, both of which wrap
// values in matched outer quotes by default.
func StripMatchedOuterQuotes(s string) string {
	if len(s) < 2 {
		return s
	}
	first := s[0]
	last := s[len(s)-1]
	if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}
