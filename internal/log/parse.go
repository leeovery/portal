package log

import (
	"regexp"
	"strings"
	"time"
)

// LogLine holds the fields parsed from one rendered portal.log text line.
type LogLine struct {
	Time      time.Time // parsed from the RFC3339Nano timestamp token
	Level     string    // "DEBUG" | "INFO" | "WARN" | "ERROR"
	Component string    // subsystem prefix (trailing ':' removed); "" if absent
	Message   string    // human message only — contextual attrs and the
	// pid/version/process_role baselines excluded
}

// attrKeyToken matches a whitespace-delimited token that opens a key=value attr
// pair, anchored so only a genuine attr key (e.g. "pid=", "took=", "version=")
// matches — never a key=value-shaped fragment buried inside a quoted multi-word
// attr value. It is the message/attrs boundary: the first matching token ends
// the human message and begins the trailing attrs + baselines region.
var attrKeyToken = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*=`)

// ParseLogLine parses one portal.log text line and is the single inverse of the
// writer's line format (textHandler.Handle):
//
//	<RFC3339Nano> <LEVEL> <component>: <msg> <attrs k=v…> pid=… version=… process_role=…
//
// It lives in the writer's package so any future writer-format change forces
// this parser to change with it.
//
// ok == false for any line that does not match the layout — specifically when
// any of: the line contains no ':' (no component delimiter); the line has fewer
// than two whitespace-delimited tokens; or the first token does not parse as an
// RFC3339Nano timestamp. The empty string falls under these (no tokens / no
// colon) and so yields ok == false.
func ParseLogLine(line string) (parsed LogLine, ok bool) {
	// Level: second whitespace-delimited token. Splitting the leading portion
	// into at most three fields isolates the timestamp and level tokens while
	// leaving the component/message/attrs remainder intact in the third field.
	tokens := strings.Fields(line)
	if len(tokens) < 2 {
		return LogLine{}, false
	}

	// Time: first whitespace-delimited token, parsed with the writer's layout.
	t, err := time.Parse(time.RFC3339Nano, tokens[0])
	if err != nil {
		return LogLine{}, false
	}
	parsed.Time = t
	parsed.Level = tokens[1]

	// Component: text between the level token and the first ':' that follows the
	// level token, surrounding whitespace trimmed. The search begins after the
	// level token so colons inside the RFC3339 timestamp are ignored. Component
	// names carry no ':' so the first such ':' reliably ends the component; any
	// later ':' belongs to the message.
	levelEnd := levelTokenEnd(line, tokens[0], tokens[1])
	rel := strings.IndexByte(line[levelEnd:], ':')
	if rel < 0 {
		return LogLine{}, false
	}
	colon := levelEnd + rel
	parsed.Component = strings.TrimSpace(line[levelEnd:colon])

	// Message: text after the component's "colon-space", up to (but excluding)
	// the first whitespace-delimited token matching attrKeyToken. That single
	// boundary drops both contextual attrs and the trailing baselines in one
	// pass; trailing whitespace from the split is trimmed.
	rest := strings.TrimPrefix(line[colon+1:], " ")
	parsed.Message = messageBeforeAttrs(rest)
	return parsed, true
}

// levelTokenEnd returns the byte offset in line immediately after the level
// token. It locates the timestamp token, then the level token that follows it,
// so the component scan starts at the correct position regardless of the
// (single-space) inter-token spacing the writer emits.
func levelTokenEnd(line, timeToken, levelToken string) int {
	tsIdx := strings.Index(line, timeToken)
	afterTS := tsIdx + len(timeToken)
	levelIdx := strings.Index(line[afterTS:], levelToken)
	return afterTS + levelIdx + len(levelToken)
}

// messageBeforeAttrs returns the human message portion of the post-colon
// remainder: everything up to but excluding the first whitespace-delimited
// token that opens a key=value attr pair, with trailing whitespace trimmed. An
// empty remainder (or one that begins immediately with an attr token) yields "".
func messageBeforeAttrs(rest string) string {
	end := len(rest)
	for i := 0; i < len(rest); {
		// Skip leading whitespace, recording the start of this token.
		if rest[i] == ' ' {
			i++
			continue
		}
		tokenStart := i
		for i < len(rest) && rest[i] != ' ' {
			i++
		}
		if attrKeyToken.MatchString(rest[tokenStart:i]) {
			end = tokenStart
			break
		}
	}
	return strings.TrimRight(rest[:end], " ")
}
