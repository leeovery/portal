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

func needsQuoting(arg string) bool {
	return arg == "" || strings.ContainsAny(arg, " \t\"'\\$`!(){}[]|&;<>?*~#\n")
}
