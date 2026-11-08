package zmx

import (
	"strings"

	"github.com/leeovery/portal/internal/shellquote"
)

const ellipsis = "..."

// DisplayCommand renders command as zmx reports it, which is what
// Session.Command holds: each argument single-quoted when the shell would
// read it specially, joined by spaces, and cut at the first argument that
// would pass zmx's 256 bytes, with "..." in its place.
func DisplayCommand(command []string) string {
	var b strings.Builder
	for i, arg := range command {
		if needsQuoting(arg) {
			arg = shellquote.Single(arg)
		}
		sep := min(i, 1)
		if b.Len()+sep+len(arg) > maxCmd {
			if b.Len()+len(ellipsis) <= maxCmd {
				b.WriteString(ellipsis)
			}
			break
		}
		if sep > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(arg)
	}
	return b.String()
}

// CommandBegins reports whether shown is how zmx reports a command that begins
// with start, followed by further arguments or none. zmx cuts the command it
// reports before the first argument that would take it past its 256 bytes,
// marking the cut with "..." where that fits: a cut within start shows as start
// alone does, whatever follows it, and one after it shows start whole, then
// each further argument that fits, or the mark alone when even the first
// doesn't. zmx never shows an argument in part, so a start whose last argument
// is a prefix of shown's next one doesn't begin it. A report cut before the
// argument that tells two commands apart shows both alike, and so begins with
// either.
func CommandBegins(shown string, start []string) bool {
	alone := DisplayCommand(start)
	switch {
	case shown == alone:
		return true
	case alone != uncut(start):
		return false
	}
	rest, ok := strings.CutPrefix(shown, alone)
	return ok && len(shown) <= maxCmd && (strings.HasPrefix(rest, " ") || rest == ellipsis)
}

// uncut is command as zmx shows each of its arguments, joined by spaces,
// however long that is.
func uncut(command []string) string {
	words := make([]string, len(command))
	for i, arg := range command {
		words[i] = DisplayCommand([]string{arg})
	}
	return strings.Join(words, " ")
}

func needsQuoting(arg string) bool {
	return arg == "" || strings.ContainsAny(arg, " \t\"'\\$`!(){}[]|&;<>?*~#\n")
}
