package migrate

import (
	"slices"
	"strings"

	"github.com/leeovery/portal/internal/nanoid"
)

// hookIndex finds the session a token key moves to: byToken is the position
// of the first pane carrying each token.
type hookIndex struct {
	byToken map[string]int
}

// newHookIndex indexes panes, in the order of the sessions made of them.
func newHookIndex(panes []savedPane) hookIndex {
	x := hookIndex{byToken: map[string]int{}}
	// Backward, so the first pane carrying a token is the one left holding it.
	for i, p := range slices.Backward(panes) {
		if p.pane.PortalPaneID != "" {
			x.byToken[p.pane.PortalPaneID] = i
		}
	}
	return x
}

// hooks moves each token key of keys to the session the pane carrying it
// became, sessions holding the session of each pane, and removes every other
// key, saying what it was.
func (x hookIndex) hooks(keys []string, sessions []Session) ([]HookMove, []HookRemoval) {
	var moves []HookMove
	var removals []HookRemoval
	for _, key := range slices.Compact(slices.Sorted(slices.Values(keys))) {
		if i, ok := x.byToken[key]; ok {
			moves = append(moves, HookMove{From: key, To: sessions[i].Name})
			continue
		}
		removals = append(removals, HookRemoval{Key: key, Kind: kindOf(key)})
	}
	return moves, removals
}

// kindOf is what key, carried by no saved pane, was.
func kindOf(key string) HookKeyKind {
	switch {
	case nanoid.IsTokenShaped(key):
		return UnsavedTokenKey
	case isPaneID(key):
		return PaneIDKey
	case isPaneAddress(key):
		return PaneAddressKey
	}
	return OtherKey
}

// isPaneAddress reports whether key is <owner>:<window>.<pane>.
func isPaneAddress(key string) bool {
	i := strings.LastIndexByte(key, ':')
	if i < 1 {
		return false
	}
	window, pane, ok := strings.Cut(key[i+1:], ".")
	return ok && isNumber(window) && isNumber(pane)
}

// isPaneID reports whether key is tmux's %N.
func isPaneID(key string) bool {
	n, ok := strings.CutPrefix(key, "%")
	return ok && isNumber(n)
}

// isNumber reports whether s is a decimal number, digits alone.
func isNumber(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}
