// Package shellquote holds the rule for composing a string a shell will later
// word-split: quoting one value as exactly one word, and rendering a whole argv
// as one command line. It depends on the standard library alone, so packages
// that must not import each other — the restore engine, the session tree and
// the host-terminal spawn service among them — can each reach the rule without
// an edge between them.
package shellquote

import "strings"

// Single wraps s in POSIX single quotes so it survives as one word when a shell
// word-splits the string it is composed into. An embedded single quote is
// escaped with the close-escape-reopen idiom, since single quotes admit no
// backslash escape of their own; an empty s renders as an empty quoted word
// rather than disappearing.
func Single(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Join renders argv as one shell command line: every element quoted through
// Single, joined by the separator a shell reads as a word boundary, so a value
// holding spaces, quotes, an expansion or a newline reaches the command it is
// composed into as the single argument it left as. An empty argv renders as the
// empty string.
func Join(argv []string) string {
	words := make([]string, len(argv))
	for i, arg := range argv {
		words[i] = Single(arg)
	}
	return strings.Join(words, " ")
}
