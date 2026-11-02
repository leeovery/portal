package zmx

import "strings"

// maxCommand is the most of a command zmx v0.8.1 reports, in bytes.
const maxCommand = 256

const ellipsis = "..."

// DisplayCommand renders command as zmx v0.8.1 reports it, which is what
// Session.Command holds: each argument single-quoted when the shell would
// read it specially, joined by spaces, and cut at the first argument that
// would pass 256 bytes, with "..." in its place.
func DisplayCommand(command []string) string {
	var b strings.Builder
	for i, arg := range command {
		if needsQuoting(arg) {
			arg = shellQuote(arg)
		}
		sep := min(i, 1)
		if b.Len()+sep+len(arg) > maxCommand {
			if b.Len()+len(ellipsis) <= maxCommand {
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

func shellQuote(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}
