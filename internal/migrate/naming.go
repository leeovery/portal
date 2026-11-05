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

// name is a pane's planned name, and why it was generated, nil when it wasn't.
type name struct {
	name      string
	generated error
}

// namer hands out names, each one the backend takes and none folding to one
// taken already or handed out before.
type namer struct {
	validate func(string) error
	mint     nanoid.Generator
	taken    []string
}

// names names each of a tmux session's panes, in order: the first by the
// session's name up to " · ", each other by the first's with its number.
func (n *namer) names(tmuxName string, panes []savedPane) ([]name, error) {
	given, _, _ := strings.Cut(tmuxName, labelSeparator)
	var names []name
	for i, p := range panes {
		candidate, inherited := given, error(nil)
		if i > 0 {
			candidate, inherited = numbered(names[0].name, i+1), names[0].generated
		}
		named, err := n.claim(candidate, inherited, p.pane.CWD)
		if err != nil {
			return nil, err
		}
		names = append(names, named)
	}
	return names, nil
}

// claim takes candidate when it can, generated for the reason inherited;
// otherwise it takes a generated name for a session in dir, generated for the
// reason candidate couldn't be taken.
func (n *namer) claim(candidate string, inherited error, dir string) (name, error) {
	refused := n.refusal(candidate)
	if refused == nil {
		n.taken = append(n.taken, candidate)
		return name{name: candidate, generated: inherited}, nil
	}
	generated, err := n.generate(dir)
	if err != nil {
		return name{}, err
	}
	n.taken = append(n.taken, generated)
	return name{name: generated, generated: refused}, nil
}

// refusal says why candidate can't be taken: the backend refuses it, or it
// folds to a name taken already. It is nil when it can.
func (n *namer) refusal(candidate string) error {
	if err := n.validate(candidate); err != nil {
		return err
	}
	if holder, ok := n.holder(candidate); ok {
		return fmt.Errorf("%w by %q", ErrNameTaken, holder)
	}
	return nil
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

// generate is a free {project}-{nanoid} name by the minting rule, the project
// dir's last element.
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
			return generated, nil
		}
	}
	return "", fmt.Errorf("generate a session name for %q: every one of %d was taken", project, maxAttempts)
}
