package migrate

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/session"
)

// ErrNameTaken is why a session's name was generated when the one migrate
// would give it is taken.
var ErrNameTaken = errors.New("the name is taken")

// labelSeparator divides a tmux session's name from the words its user added
// after it.
const labelSeparator = " · "

// maxAttempts bounds the nanoids a generated name draws while the names it
// composes are taken, as a mint's does.
const maxAttempts = 10

// ownName is a tmux session's own name: its name up to " · ", trimmed of
// surrounding whitespace.
func ownName(tmuxName string) string {
	given, _, _ := strings.Cut(tmuxName, labelSeparator)
	return strings.TrimSpace(given)
}

// withNumber is s for the first pane, and s with the pane's number for another.
func withNumber(s string, number int) string {
	if number == 1 {
		return s
	}
	return fmt.Sprintf("%s-%d", s, number)
}

// name is a pane's planned name; why it was generated, nil when it wasn't; and
// its number: 1 for a session's first pane, and for another the number its
// name carries, or would have carried had it fit.
type name struct {
	name      string
	generated error
	number    int
}

// namer hands out names, each one the backend takes and none folding to one
// taken already or handed out before.
type namer struct {
	validate func(string) error
	mint     nanoid.Generator
	taken    []string
}

// reserve takes candidate when it can, and says why it can't otherwise.
func (n *namer) reserve(candidate string) error {
	if err := n.validate(candidate); err != nil {
		return err
	}
	if holder, ok := n.holder(candidate); ok {
		return fmt.Errorf("%w by %q", ErrNameTaken, holder)
	}
	n.taken = append(n.taken, candidate)
	return nil
}

// names names each pane of s: the first by s's own name, reserved already
// unless that was refused, then by a generated one; each other by the first's
// with the lowest free number above the last pane's, or a generated one once
// no number fits.
func (n *namer) names(s migratingSession) ([]name, error) {
	first := name{name: s.own, number: 1}
	if s.refused != nil {
		generated, err := n.generate(s.panes[0].dir)
		if err != nil {
			return nil, err
		}
		first = name{name: generated, generated: s.refused, number: 1}
	}
	names := []name{first}
	for _, p := range s.panes[1:] {
		extra, err := n.extra(first, names[len(names)-1].number+1, p.dir)
		if err != nil {
			return nil, err
		}
		names = append(names, extra)
	}
	return names, nil
}

// extra is first's name with the lowest free number from number on, or, once
// the backend refuses it, a longer number fitting no better, a generated name
// for a session in dir.
func (n *namer) extra(first name, number int, dir string) (name, error) {
	for ; ; number++ {
		candidate := withNumber(first.name, number)
		if refused := n.validate(candidate); refused != nil {
			generated, err := n.generate(dir)
			if err != nil {
				return name{}, err
			}
			return name{name: generated, generated: refused, number: number}, nil
		}
		if _, held := n.holder(candidate); !held {
			n.taken = append(n.taken, candidate)
			return name{name: candidate, generated: first.generated, number: number}, nil
		}
	}
}

// holder is the taken name candidate folds to, as a filesystem that folds
// case takes them for one zmx socket.
func (n *namer) holder(candidate string) (string, bool) {
	for _, taken := range n.taken {
		if strings.EqualFold(taken, candidate) {
			return taken, true
		}
	}
	return "", false
}

// generate takes a free {project}-{nanoid} name by the minting rule, the
// project dir's last element.
func (n *namer) generate(dir string) (string, error) {
	project := ""
	if dir != "" {
		project = filepath.Base(dir)
	}
	for range maxAttempts {
		suffix, err := n.mint()
		if err != nil {
			return "", fmt.Errorf("generate a session name: %w", err)
		}
		generated, err := session.SessionName(project, suffix, n.validate)
		if err != nil {
			return "", fmt.Errorf("generate a session name for %q: %w", project, err)
		}
		if _, held := n.holder(generated); !held {
			n.taken = append(n.taken, generated)
			return generated, nil
		}
	}
	return "", fmt.Errorf("generate a session name for %q: every one of %d was taken", project, maxAttempts)
}
