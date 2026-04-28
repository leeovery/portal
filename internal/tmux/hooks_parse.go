package tmux

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/leeovery/portal/internal/tmuxout"
)

// HookEntry is a single entry parsed from `tmux show-hooks -g` output.
// Index is the array index tmux assigned to the entry; Command is the
// stored command body with any matched outer quoting stripped.
type HookEntry struct {
	Index   int
	Command string
}

// hookLineRegexp matches a single line of `show-hooks -g` output of the form:
//
//	<event>[<index>] => <command>
//	<event>[<index>] <command>
//
// Event names are alphanumeric with hyphens (e.g. session-created,
// client-session-changed). The separator between the bracketed index and
// the command body is either ` => ` (with optional surrounding whitespace)
// or one or more whitespace characters.
var hookLineRegexp = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9-]*)\[(\d+)\](?:\s*=>\s*|\s+)(.*)$`)

// ParseShowHooks parses raw `tmux show-hooks -g` output into a per-event
// map of HookEntry slices. Each event's slice is sorted by Index ascending.
//
// Behavior:
//   - Empty input yields a non-nil empty map.
//   - Lines that do not match the expected shape are silently skipped.
//   - Entries with non-numeric bracket content are silently skipped.
//   - Outer matched single or double quote pairs are stripped from the
//     command body; mismatched outer characters are preserved verbatim.
//
// The function performs no I/O.
func ParseShowHooks(raw string) map[string][]HookEntry {
	out := make(map[string][]HookEntry)

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		match := hookLineRegexp.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		event := match[1]
		index, err := strconv.Atoi(match[2])
		if err != nil {
			continue
		}
		command := tmuxout.StripMatchedOuterQuotes(match[3])

		out[event] = append(out[event], HookEntry{Index: index, Command: command})
	}

	for event := range out {
		sort.Slice(out[event], func(i, j int) bool {
			return out[event][i].Index < out[event][j].Index
		})
	}

	return out
}
