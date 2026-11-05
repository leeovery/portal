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

// String is the pane's address, <session>:<window>.<pane>.
func (p TmuxPane) String() string { return fmt.Sprintf("%s:%d.%d", p.Session, p.Window, p.Pane) }

// Session is the zmx session migrate makes of one tmux pane.
type Session struct {
	From TmuxPane
	Name string
	// Changed says, in plain words and in order, how Name differs from the
	// tmux session's own name and why: none when it doesn't, a pane's
	// number aside. A pane numbered after a changed first name carries the
	// first's.
	Changed []string
	// Label is the label the entry records: empty when it is the name, or
	// when the index refuses it, LabelRefused then saying why.
	Label        string
	LabelRefused error
	// Dir is the git root of the pane's working directory, the dir the zmx
	// version groups a session with no recorded one under, or that working
	// directory when it has none. Cwd is the working directory.
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
	// source is Named as a path below the tmux version's state dir, for one
	// copied.
	source string
}

// SkippedSession is a tmux session version 1 saved that becomes no zmx
// session, and why.
type SkippedSession struct {
	Name string
	Why  error
}

// HookMove is a token key of hooks.json moved to the name of the session its
// pane became.
type HookMove struct {
	From, To string
}

// HookKeyKind is what a hooks.json key RewriteHooks removes was.
type HookKeyKind int

const (
	// PaneAddressKey is <session>:<window>.<pane>, or for a while
	// <@portal-id>:<window>.<pane>, its session's token in place of its name:
	// the keys by pane position from before pane tokens.
	PaneAddressKey HookKeyKind = iota + 1
	// PaneIDKey is %N, tmux's own pane id, the earliest key.
	PaneIDKey
	// UnsavedTokenKey is a pane token no saved pane carries.
	UnsavedTokenKey
	// OtherKey is a key of no shape Portal wrote.
	OtherKey
)

func (k HookKeyKind) String() string {
	switch k {
	case PaneAddressKey:
		return "pane address"
	case PaneIDKey:
		return "tmux pane id"
	case UnsavedTokenKey:
		return "token of no saved pane"
	case OtherKey:
		return "other"
	}
	return fmt.Sprintf("HookKeyKind(%d)", int(k))
}

// HookRemoval is a hooks.json key RewriteHooks removes, and what it was. Only
// a token key is carried: on the tmux version, restore fired a pane's hook by
// its saved token alone, so a key of any older shape has fired nothing since
// pane tokens, and its coordinates, from when it was set, name whatever pane
// sits there now. hooks.json's backup keeps every key.
type HookRemoval struct {
	Key  string
	Kind HookKeyKind
}

// Plan is everything migrate writes, worked out before it writes anything:
// one session for each tmux pane, in the order version 1 saved the sessions
// and each session's panes in window order then pane order; the tmux sessions
// with no pane to migrate; and each hooks.json key moved or removed, by key.
type Plan struct {
	Sessions []Session
	Skipped  []SkippedSession
	Moves    []HookMove
	Removals []HookRemoval
	// legacyDir is the tmux version's state dir, Build's copies' source.
	legacyDir string
}

// Naming is what the plan's names keep clear of and obey, and where its
// sessions' dirs come from.
type Naming struct {
	// Taken are the names already in use: the backend's sessions and the new
	// state dir's entries.
	Taken []string
	// Validate is the backend's ValidateName: a name it refuses is never
	// planned. A name it refuses as backend.ErrNameTooLong is cut to fit.
	Validate func(name string) error
	// Refuses is the backend's rule on single characters (zmx.RefusesRune):
	// each one a name holds that it refuses becomes '-'.
	Refuses func(r rune) bool
	// Mint draws the nanoid a generated name, or a fresh id, ends in.
	Mint nanoid.Generator
	// GroupingDir is session.GroupingDir bound to a git runner: the dir a
	// session working in cwd groups under, false when there is none. A nil
	// one finds none.
	GroupingDir func(cwd string) (string, bool)
}

// dirOf is the dir the entry of a pane working in cwd records.
func (n Naming) dirOf(cwd string) string {
	if n.GroupingDir != nil {
		if dir, ok := n.GroupingDir(cwd); ok {
			return dir
		}
	}
	return cwd
}

// migratingSession is a tmux session with panes to migrate: what its own
// name claimed, and its panes.
type migratingSession struct {
	tmuxName string
	own      claimed
	panes    []migratingPane
}

// migratingPane is a saved pane and the dir its entry records.
type migratingPane struct {
	savedPane
	dir string
}

// NewPlan works out what migrating legacy writes, given hooks.json's keys.
//   - Every tmux session's own name, its name up to " · " trimmed, is made
//     a name Validate takes and reserved first, unless it is taken, folding
//     case as a filesystem may: by Naming.Taken or an earlier session's. A
//     name Validate refuses is made compatible: each character Refuses
//     refuses becomes '-', a run of '-' that joins collapsing to one, a
//     leading '-' or space is dropped, and a name too long is cut to fit.
//     With nothing of it left, or a name Validate still refuses, such as a
//     reserved word, the session takes a generated {project}-{nanoid}, by
//     the minting rule, the project its first pane's dir's last element. A
//     name taken keeps its stem, less any -<nanoid> it ends in, and takes a
//     fresh nanoid, cut to fit and drawn again while taken. Each of a
//     session's other panes then takes the first's name with the lowest free
//     number above the last pane's, -2, -3 and so on, or a fresh nanoid on
//     the first's stem once the number makes it too long.
//   - Each label is the tmux session's name, another pane's with its number,
//     omitted when it is the name or one the index refuses.
//   - Each entry's dir is the pane's grouping dir, else its cwd.
//   - A pane's scrollback file, when ReadLegacy found it, is copied to the
//     file the zmx version names for the session's creation, taken to be
//     version 1's saved_at, since the entry records none.
//   - A token key moves to the session of the first pane carrying the token.
//     Every other key is removed.
//
// A tmux session with no panes is skipped. It errors when Mint fails, or no
// free name can be generated or given a fresh id.
func NewPlan(legacy Legacy, hookKeys []string, naming Naming) (Plan, error) {
	n := &namer{validate: naming.Validate, refuses: naming.Refuses, mint: naming.Mint, taken: slices.Clone(naming.Taken)}
	plan := Plan{legacyDir: legacy.dir}
	var migrating []migratingSession
	for _, s := range legacy.index.Sessions {
		saved := s.panes()
		if len(saved) == 0 {
			plan.Skipped = append(plan.Skipped, SkippedSession{Name: s.Name, Why: s.empty()})
			continue
		}
		m := migratingSession{tmuxName: s.Name, own: n.claim(ownName(s.Name))}
		for _, p := range saved {
			m.panes = append(m.panes, migratingPane{savedPane: p, dir: naming.dirOf(p.pane.CWD)})
		}
		migrating = append(migrating, m)
	}
	var panes []savedPane
	for _, m := range migrating {
		names, err := n.names(m)
		if err != nil {
			return Plan{}, err
		}
		for i, p := range m.panes {
			made, err := legacy.session(m.tmuxName, p, names[i])
			if err != nil {
				return Plan{}, err
			}
			plan.Sessions = append(plan.Sessions, made)
			panes = append(panes, p.savedPane)
		}
	}
	plan.Moves, plan.Removals = newHookIndex(panes).hooks(hookKeys, plan.Sessions)
	return plan, nil
}

// session is the session made of p, a pane of the tmux session tmuxName,
// named n.
func (l Legacy) session(tmuxName string, p migratingPane, n name) (Session, error) {
	transcript, err := l.transcript(p.pane, n.name)
	if err != nil {
		return Session{}, err
	}
	label, refused := sessionindex.LabelFor(n.name, withNumber(tmuxName, n.number))
	return Session{
		From:         p.at,
		Name:         n.name,
		Changed:      n.changed,
		Label:        label,
		LabelRefused: refused,
		Dir:          p.dir,
		Cwd:          p.pane.CWD,
		Transcript:   transcript,
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
