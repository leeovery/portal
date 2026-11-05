package migrate

import (
	"slices"
	"strings"

	"github.com/leeovery/portal/internal/nanoid"
)

// hookIndex finds the session a token key moves to, and judges what any other
// key was.
type hookIndex struct {
	// byToken is the position of the first pane carrying each token.
	byToken map[string]int
	// saved holds the name of every tmux session version 1 saved.
	saved map[string]bool
}

// newHookIndex indexes legacy's sessions and panes, the panes migrating in the
// order of the sessions made of them.
func newHookIndex(legacy Legacy, panes []savedPane) hookIndex {
	x := hookIndex{byToken: map[string]int{}, saved: map[string]bool{}}
	// Backward, so the first pane carrying a token is the one left holding it.
	for i, p := range slices.Backward(panes) {
		if p.pane.PortalPaneID != "" {
			x.byToken[p.pane.PortalPaneID] = i
		}
	}
	for _, s := range legacy.index.Sessions {
		x.saved[s.Name] = true
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
		removals = append(removals, HookRemoval{Key: key, Kind: x.kind(key)})
	}
	return moves, removals
}

// kind is what key, carried by no saved pane, was. A pane address whose owner
// is token-shaped is a session token's, unless a saved session has that name.
func (x hookIndex) kind(key string) HookKeyKind {
	owner, isAddress := paneAddressOwner(key)
	switch {
	case nanoid.IsTokenShaped(key):
		return UnsavedTokenKey
	case isPaneID(key):
		return PaneIDKey
	case !isAddress:
		return OtherKey
	case nanoid.IsTokenShaped(owner) && !x.saved[owner]:
		return PortalIDKey
	}
	return PaneAddressKey
}

// paneAddressOwner is the owner of key when it is <owner>:<window>.<pane>.
func paneAddressOwner(key string) (string, bool) {
	i := strings.LastIndexByte(key, ':')
	if i < 1 {
		return "", false
	}
	window, pane, ok := strings.Cut(key[i+1:], ".")
	return key[:i], ok && isNumber(window) && isNumber(pane)
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
