package session

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/leeovery/portal/internal/nanoid"
)

// IDGenerator mints one opaque nanoid per call; a generated session name ends
// in one.
type IDGenerator = nanoid.Generator

// FallbackProject is the project part of a name whose project sanitises to
// nothing. A name is never the nanoid alone: a bare nanoid is token-shaped,
// and the tmux version's daemon, which shares hooks.json, reaps a token-shaped
// key that names none of its panes.
const FallbackProject = "session"

// leadingDropped are the characters a generated name may not open with: zmx
// reads a leading "-" as a flag, and a leading "_" marks Portal's own
// sessions, which listings hide.
const leadingDropped = "-_"

// SanitiseProjectName reduces a project name to a fragment a generated session
// name can open with: a character no session name can hold (a slash, a
// control character, a byte that is not UTF-8) becomes a hyphen, and the
// characters a name may not open with are dropped from its start rather than
// replaced, so one never escapes to another. The result may be empty.
func SanitiseProjectName(name string) string {
	clean := strings.Map(func(r rune) rune {
		if r == '/' || r == utf8.RuneError || unicode.IsControl(r) {
			return '-'
		}
		return r
	}, name)
	return strings.TrimLeft(clean, leadingDropped)
}

// SessionName composes {project}-{suffix} from the sanitised project, or from
// FallbackProject when that is empty. While validate refuses the name, the
// project part is cut short a character at a time, since what a backend
// refuses in a generated name is its length; once nothing is left to cut,
// validate's refusal is returned.
func SessionName(project, suffix string, validate func(string) error) (string, error) {
	part := SanitiseProjectName(project)
	if part == "" {
		part = FallbackProject
	}
	for {
		name := part + "-" + suffix
		err := validate(name)
		if err == nil {
			return name, nil
		}
		part = shortened(part)
		if part == "" {
			return "", err
		}
	}
}

// shortened is part less its last character and any hyphens that leaves
// trailing, so a cut never ends the part in the separator.
func shortened(part string) string {
	_, size := utf8.DecodeLastRuneInString(part)
	return strings.TrimRight(part[:len(part)-size], "-")
}
