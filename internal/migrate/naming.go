package migrate

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/session"
)

// labelSeparator divides a tmux session's name from the words its user added
// after it.
const labelSeparator = " · "

// maxAttempts bounds the nanoids a name draws while the names it composes
// are taken, as a mint's does.
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

// name is a pane's planned name; how it differs from its tmux session's own
// name, and why, in order, none when it doesn't; and its number: 1 for a
// session's first pane, and for another the number its name carries, or
// would have carried had it fit.
type name struct {
	name    string
	changed []string
	number  int
}

// claimed is what a tmux session's own name became before its panes are
// named: the name the backend takes made of it, empty when nothing of it
// makes one; how it was changed and why, or why nothing makes one; and
// whether it was free, and so reserved.
type claimed struct {
	name     string
	changed  []string
	reserved bool
}

// namer hands out names, each one the backend takes and none folding to one
// taken already or handed out before.
type namer struct {
	validate func(string) error
	refuses  func(rune) bool
	mint     nanoid.Generator
	taken    []string
}

// claim makes own a name the backend takes, and reserves it when it is free.
func (n *namer) claim(own string) claimed {
	made, changed, ok := n.compatible(own)
	if !ok {
		return claimed{changed: changed}
	}
	_, held := n.holder(made)
	if !held {
		n.taken = append(n.taken, made)
	}
	return claimed{name: made, changed: changed, reserved: !held}
}

// compatible is own as a name the backend takes, and how it was changed and
// why: as it is when the backend takes it; otherwise each character it
// refuses becomes '-', a run of '-' that joins collapsing to one, then a '-'
// or space it would start with is dropped, and a name too long is cut to
// fit. With nothing left, or a name the backend still refuses, such as a
// reserved word, ok is false and changed says why.
func (n *namer) compatible(own string) (made string, changed []string, ok bool) {
	if n.validate(own) == nil {
		return own, nil, true
	}
	made, replaced := replaceRefused(own, n.refuses)
	if len(replaced) > 0 {
		changed = append(changed, replacedWhy(replaced))
	}
	if trimmed := strings.TrimLeftFunc(made, isEdgeTrimmed); trimmed != made {
		changed = append(changed, leadDroppedCopy)
		made = trimmed
	}
	if made == "" {
		return "", []string{noPartCopy}, false
	}
	fitted, err := n.fit(made, "")
	if err != nil {
		return "", []string{fmt.Sprintf(stillRefusedCopy, err)}, false
	}
	if fitted != made {
		changed = append(changed, fmt.Sprintf(cutCopy, utf8.RuneCountInString(fitted)))
	}
	return fitted, changed, true
}

// replaceRefused is s with each character refuses refuses replaced by '-',
// any '-' beside one joining it in a single '-', and the characters it
// replaced, each once, in order.
func replaceRefused(s string, refuses func(rune) bool) (string, []rune) {
	var out, replaced []rune
	joining := false
	for _, r := range s {
		switch {
		case refuses(r):
			if !slices.Contains(replaced, r) {
				replaced = append(replaced, r)
			}
			for len(out) > 0 && out[len(out)-1] == '-' {
				out = out[:len(out)-1]
			}
			out = append(out, '-')
			joining = true
		case r == '-' && joining:
			// joins the '-' a replaced character became
		default:
			out = append(out, r)
			joining = false
		}
	}
	return string(out), replaced
}

// isEdgeTrimmed reports whether r is dropped from the start of a name made
// compatible, and from the end of one cut: zmx reads a leading '-' as a
// flag, and a '-' or space left at either edge is a separator with nothing
// beyond it.
func isEdgeTrimmed(r rune) bool { return r == '-' || unicode.IsSpace(r) }

// replacedWhy says why the characters replaced became '-'.
func replacedWhy(replaced []rune) string {
	quoted := make([]string, 0, len(replaced))
	for _, r := range replaced {
		quoted = append(quoted, fmt.Sprintf("'%c'", r))
	}
	if len(quoted) == 1 {
		return fmt.Sprintf(replacedOneCopy, quoted[0])
	}
	return fmt.Sprintf(replacedManyCopy, strings.Join(quoted[:len(quoted)-1], ", "), quoted[len(quoted)-1])
}

// fit is part+suffix, part cut a character at a time while the backend
// refuses it as too long; once nothing of part is left, or the backend
// refuses it for anything else, the backend's refusal.
func (n *namer) fit(part, suffix string) (string, error) {
	for {
		candidate := part + suffix
		err := n.validate(candidate)
		if !errors.Is(err, backend.ErrNameTooLong) {
			return candidate, err
		}
		if part = shortened(part); part == "" {
			return "", err
		}
	}
}

// shortened is part less its last character and any '-' or space that
// leaves trailing, so a cut never ends a name in a separator.
func shortened(part string) string {
	_, size := utf8.DecodeLastRuneInString(part)
	return strings.TrimRightFunc(part[:len(part)-size], isEdgeTrimmed)
}

// names names each pane of s: the first by the name its own name claimed,
// a fresh id on its stem when another holds it, or a generated name when
// nothing of it makes one; each other by the first's with the lowest free
// number above the last pane's, or a fresh id on the first's stem once the
// number makes it too long.
func (n *namer) names(s migratingSession) ([]name, error) {
	first, err := n.first(s)
	if err != nil {
		return nil, err
	}
	names := []name{first}
	for range s.panes[1:] {
		extra, err := n.extra(first, names[len(names)-1].number+1)
		if err != nil {
			return nil, err
		}
		names = append(names, extra)
	}
	return names, nil
}

// first is the name of s's first pane.
func (n *namer) first(s migratingSession) (name, error) {
	own := s.own
	switch {
	case own.name == "":
		generated, err := n.generate(s.panes[0].dir)
		return name{name: generated, changed: own.changed, number: 1}, err
	case own.reserved:
		return name{name: own.name, changed: own.changed, number: 1}, nil
	}
	fresh, err := n.fresh(own.name)
	return name{name: fresh, changed: slices.Concat(own.changed, []string{fmt.Sprintf(takenCopy, own.name)}), number: 1}, err
}

// extra is first's name with the lowest free number from number on, or,
// once the number makes it too long, a fresh id on first's stem. first's
// name is one the backend takes, so only its length refuses it numbered.
func (n *namer) extra(first name, number int) (name, error) {
	for ; ; number++ {
		candidate := withNumber(first.name, number)
		if n.validate(candidate) != nil {
			fresh, err := n.fresh(first.name)
			return name{name: fresh, changed: slices.Concat(first.changed, []string{fmt.Sprintf(numberTooLongCopy, candidate)}), number: number}, err
		}
		if _, held := n.holder(candidate); !held {
			n.taken = append(n.taken, candidate)
			return name{name: candidate, changed: first.changed, number: number}, nil
		}
	}
}

// fresh takes a free <stem>-<nanoid>, the stem base less any id it ends in
// and any '-' or space that leaves trailing, cut to fit, drawing again while
// the name is taken.
func (n *namer) fresh(base string) (string, error) {
	stem := strings.TrimRightFunc(idStem(base), isEdgeTrimmed)
	for range maxAttempts {
		id, err := n.mint()
		if err != nil {
			return "", fmt.Errorf("give %s a new id: %w", base, err)
		}
		candidate, err := n.fit(stem, "-"+id)
		if err != nil {
			return "", fmt.Errorf("give %s a new id: %w", base, err)
		}
		if _, held := n.holder(candidate); !held {
			n.taken = append(n.taken, candidate)
			return candidate, nil
		}
	}
	return "", fmt.Errorf("give %s a new id: every one of %d was taken", base, maxAttempts)
}

// idStem is name less a trailing -<nanoid>, a pane token's width, when it
// ends in one.
func idStem(name string) string {
	if i := strings.LastIndexByte(name, '-'); i > 0 && nanoid.IsTokenShaped(name[i+1:]) {
		return name[:i]
	}
	return name
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
