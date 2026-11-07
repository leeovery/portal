package log

import (
	"regexp"
	"strings"
	"time"
)

// LogLine holds one parsed portal.log line. Message is the human message only —
// contextual attrs and the pid/version/process_role baselines are excluded.
type LogLine struct {
	Time      time.Time
	Level     string
	Component string
	Message   string
}

// Anchored so only a token that genuinely opens a key=value attr pair matches,
// never a key=value-shaped fragment inside a quoted multi-word value.
var attrKeyToken = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*=`)

// ParseLogLine is the inverse of textHandler's line format; ok is false for any
// line that does not match it.
func ParseLogLine(line string) (parsed LogLine, ok bool) {
	tokens := strings.Fields(line)
	if len(tokens) < 2 {
		return LogLine{}, false
	}

	t, err := time.Parse(time.RFC3339Nano, tokens[0])
	if err != nil {
		return LogLine{}, false
	}
	parsed.Time = t
	parsed.Level = tokens[1]

	// The colon scan starts after the level token so the timestamp's own colons
	// cannot end the component.
	levelEnd := levelTokenEnd(line, tokens[0], tokens[1])
	rel := strings.IndexByte(line[levelEnd:], ':')
	if rel < 0 {
		return LogLine{}, false
	}
	colon := levelEnd + rel
	parsed.Component = strings.TrimSpace(line[levelEnd:colon])

	rest := strings.TrimPrefix(line[colon+1:], " ")
	parsed.Message = messageBeforeAttrs(rest)
	return parsed, true
}

func levelTokenEnd(line, timeToken, levelToken string) int {
	tsIdx := strings.Index(line, timeToken)
	afterTS := tsIdx + len(timeToken)
	levelIdx := strings.Index(line[afterTS:], levelToken)
	return afterTS + levelIdx + len(levelToken)
}

func messageBeforeAttrs(rest string) string {
	end := len(rest)
	for i := 0; i < len(rest); {
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
