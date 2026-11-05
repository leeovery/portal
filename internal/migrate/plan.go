package migrate

import (
	"fmt"
	"slices"

	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// TmuxPane is a pane of the tmux version by its address.
type TmuxPane struct {
	Session string
	Window  int
	Pane    int
}

// String is the pane's address, <session>:<window>.<pane>, which keyed its
// hook in hooks.json before panes carried tokens.
func (p TmuxPane) String() string { return fmt.Sprintf("%s:%d.%d", p.Session, p.Window, p.Pane) }

// Session is the zmx session migrate makes of one tmux pane.
type Session struct {
	From TmuxPane
	Name string
	// Generated says why Name was generated rather than taken from the tmux
	// session's name; nil when it wasn't. A pane numbered after a generated
	// first name carries the first's reason.
	Generated error
	// Label is the label the entry records: empty when it is the name.
	Label string
	// Dir and Cwd are both the pane's working directory: version 1 recorded
	// no other.
	Dir, Cwd   string
	Transcript Transcript
}

// Transcript is a pane's saved scrollback and where migrate copies it.
type Transcript struct {
	// Named is the file version 1 named, relative to its state dir; empty
	// when it named none.
	Named string
	// File is the file the entry names, relative to the new state dir: the
	// one the scrollback is copied to. It is empty when nothing is copied.
	File string
	// Missing says why a file version 1 named isn't copied; nil when it is,
	// and when it named none.
	Missing error
	// source is Named under the tmux version's state dir, for one copied.
	source string
}

// HookMove is a hooks.json key moved to the name of the session its pane
// became.
type HookMove struct {
	From, To string
}

// HookRemoval is a hooks.json key Apply removes: it maps to no migrated
// session, and left behind, a key of the tmux version's shape would hold
// zmx's hook sweep down for good. The tmux version's state dir keeps a copy of
// hooks.json.
type HookRemoval struct {
	Key string
	// Superseded is the name of the session the key's pane became when that
	// pane's token key, which wins, moves there; empty when the key maps to
	// no pane.
	Superseded string
}

// Plan is everything migrate writes, worked out before it writes anything:
// one session for each tmux pane, in the order version 1 saved the sessions
// and each session's panes in window order then pane order; and each
// hooks.json key moved or removed, by key.
type Plan struct {
	Sessions []Session
	Moves    []HookMove
	Removals []HookRemoval
}

// Naming is what the plan's names keep clear of and obey.
type Naming struct {
	// Taken are the names already in use: the backend's sessions and the new
	// state dir's entries.
	Taken []string
	// Validate is the backend's ValidateName: a name it refuses is never
	// planned.
	Validate func(name string) error
	// Mint draws the nanoid a generated name ends in.
	Mint nanoid.Generator
}

// NewPlan works out what migrating legacy writes, given hooks.json's keys.
//   - A tmux session's first pane takes the session's name up to " · ",
//     unless Validate refuses it or it is taken, folding case as a filesystem
//     may: by Naming.Taken or an earlier pane's name. Then it takes a
//     generated {project}-{nanoid}, by the minting rule, the project its
//     working directory's last element. Each other pane takes the first's
//     name with its number, -2, -3 and so on, or a generated one when that is
//     refused or taken.
//   - Each label is the tmux session's name, each other pane's with its
//     number too, omitted when it is the name or one the index would refuse.
//   - A pane's scrollback file, when ReadLegacy found it, is copied to the
//     file the zmx version names for the session's creation, taken to be
//     version 1's saved_at, since the entry records none.
//   - A token key moves to the session of the pane carrying the token, the
//     first such pane when several do; any other key to the pane at its
//     address, unless that pane's token key moves too, which wins. Every key
//     that moves nowhere is removed.
//
// It errors when Mint fails, or no free name can be generated.
func NewPlan(legacy Legacy, hookKeys []string, naming Naming) (Plan, error) {
	n := &namer{validate: naming.Validate, mint: naming.Mint, taken: slices.Clone(naming.Taken)}
	var panes []savedPane
	var sessions []Session
	for _, s := range legacy.index.Sessions {
		saved := s.panes()
		names, err := n.names(s.Name, saved)
		if err != nil {
			return Plan{}, err
		}
		for i, p := range saved {
			made, err := legacy.session(s.Name, i+1, p, names[i])
			if err != nil {
				return Plan{}, err
			}
			sessions = append(sessions, made)
		}
		panes = append(panes, saved...)
	}
	moves, removals := newPaneIndex(panes).hooks(hookKeys, sessions)
	return Plan{Sessions: sessions, Moves: moves, Removals: removals}, nil
}

// session is the session made of p, the pane numbered number in its tmux
// session tmuxName, named n.
func (l Legacy) session(tmuxName string, number int, p savedPane, n name) (Session, error) {
	transcript, err := l.transcript(p.pane, n.name)
	if err != nil {
		return Session{}, err
	}
	return Session{
		From:       p.at,
		Name:       n.name,
		Generated:  n.generated,
		Label:      label(tmuxName, number, n.name),
		Dir:        p.pane.CWD,
		Cwd:        p.pane.CWD,
		Transcript: transcript,
	}, nil
}

// transcript is what becomes of the scrollback file p names, for the session
// named name.
func (l Legacy) transcript(p legacyPane, name string) (Transcript, error) {
	if p.ScrollbackFile == "" {
		return Transcript{}, nil
	}
	found := l.transcripts[p.ScrollbackFile]
	if found.Missing != nil {
		return found, nil
	}
	file, err := state.SessionScrollbackFile(name, l.index.SavedAt, 0)
	if err != nil {
		return Transcript{}, fmt.Errorf("name the scrollback file of %s: %w", name, err)
	}
	found.File = file
	return found, nil
}

// label is the label the pane numbered number of the tmux session tmuxName
// records as the session named name: none when the index would refuse it.
func label(tmuxName string, number int, name string) string {
	recorded, err := sessionindex.LabelFor(name, numbered(tmuxName, number))
	if err != nil {
		return ""
	}
	return recorded
}

// numbered is s for the first pane, and s with the pane's number for another.
func numbered(s string, number int) string {
	if number == 1 {
		return s
	}
	return fmt.Sprintf("%s-%d", s, number)
}

// paneIndex finds a pane's position by its token and by its address, each the
// first pane carrying it.
type paneIndex struct {
	byToken   map[string]int
	byAddress map[string]int
}

func newPaneIndex(panes []savedPane) paneIndex {
	x := paneIndex{byToken: map[string]int{}, byAddress: map[string]int{}}
	// Backward, so the first pane carrying a token or an address is the one
	// left holding it.
	for i, p := range slices.Backward(panes) {
		if p.pane.PortalPaneID != "" {
			x.byToken[p.pane.PortalPaneID] = i
		}
		x.byAddress[p.at.String()] = i
	}
	return x
}

// hooks moves each of keys to the session the pane it belongs to became,
// sessions holding the session of each pane, and removes the rest.
func (x paneIndex) hooks(keys []string, sessions []Session) ([]HookMove, []HookRemoval) {
	tokenKeyed := map[int]bool{}
	for _, key := range keys {
		if i, ok := x.byToken[key]; ok {
			tokenKeyed[i] = true
		}
	}
	var moves []HookMove
	var removals []HookRemoval
	for _, key := range slices.Compact(slices.Sorted(slices.Values(keys))) {
		i, byToken := x.byToken[key]
		j, byAddress := x.byAddress[key]
		switch {
		case byToken:
			moves = append(moves, HookMove{From: key, To: sessions[i].Name})
		case byAddress && tokenKeyed[j]:
			removals = append(removals, HookRemoval{Key: key, Superseded: sessions[j].Name})
		case byAddress:
			moves = append(moves, HookMove{From: key, To: sessions[j].Name})
		default:
			removals = append(removals, HookRemoval{Key: key})
		}
	}
	return moves, removals
}

// moves is the plan's hook moves, by the key each moves.
func (p Plan) moves() map[string]string {
	moves := make(map[string]string, len(p.Moves))
	for _, m := range p.Moves {
		moves[m.From] = m.To
	}
	return moves
}

// removedKeys is the keys the plan removes.
func (p Plan) removedKeys() []string {
	keys := make([]string, 0, len(p.Removals))
	for _, r := range p.Removals {
		keys = append(keys, r.Key)
	}
	return keys
}
