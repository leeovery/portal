package tmux

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// minTmuxMajor is the minimum supported tmux major version.
// Portal requires tmux >= 3.0 for array-indexed global hooks
// (`set-hook -ga` semantics) and the `show-hooks -g` output format.
const minTmuxMajor = 3

// ParseTmuxVersion extracts the major and minor numbers and a preserved
// version label from a raw `tmux -V` output string.
//
// It tolerates several real-world shapes:
//
//	"tmux 3.3a"
//	"tmux 3.0"
//	"tmux-next 4.0"
//	"tmux 3.3-rc"
//	"  tmux 3.0 (OpenBSD)  "
//
// The returned label preserves the original version token (e.g. "3.3a",
// "3.0-rc1", "3.0") for use in user-facing messages. A missing minor
// component is treated as `.0` (so "tmux 3" parses as major=3, minor=0,
// label="3"). Returns a descriptive error when no digit-prefixed token is
// present.
func ParseTmuxVersion(raw string) (major, minor int, label string, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, 0, "", errors.New("tmux version string is empty")
	}

	token := findVersionToken(trimmed)
	if token == "" {
		return 0, 0, "", fmt.Errorf("could not parse tmux version from %q", raw)
	}

	major, minor, err = splitMajorMinor(token)
	if err != nil {
		return 0, 0, "", fmt.Errorf("could not parse tmux version from %q: %w", raw, err)
	}
	return major, minor, token, nil
}

// findVersionToken returns the first whitespace-delimited token in s that
// begins with a decimal digit, or "" if no such token exists. Tokens
// wrapped in parentheses (e.g. "(OpenBSD)") are skipped.
func findVersionToken(s string) string {
	for field := range strings.FieldsSeq(s) {
		if field == "" || field[0] < '0' || field[0] > '9' {
			continue
		}
		return field
	}
	return ""
}

// splitMajorMinor parses a version token (e.g. "3.3a", "3.0-rc1", "3") into
// integer major/minor components. Any non-digit suffix after the minor
// component (letter qualifiers like "a", "b" or pre-release markers like
// "-rc") is ignored. A missing minor is treated as 0.
func splitMajorMinor(token string) (int, int, error) {
	majorStr, rest := takeDigits(token)
	if majorStr == "" {
		return 0, 0, fmt.Errorf("no major version digit in %q", token)
	}
	major, err := strconv.Atoi(majorStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid major %q: %w", majorStr, err)
	}

	if !strings.HasPrefix(rest, ".") {
		return major, 0, nil
	}

	minorStr, _ := takeDigits(rest[1:])
	if minorStr == "" {
		return major, 0, nil
	}
	minor, err := strconv.Atoi(minorStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid minor %q: %w", minorStr, err)
	}
	return major, minor, nil
}

// takeDigits returns the leading run of decimal digits from s and the
// remainder of the string after them.
func takeDigits(s string) (digits, rest string) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i], s[i:]
}

// CheckTmuxVersion runs `tmux -V` via cmd and verifies the installed tmux
// version satisfies Portal's minimum requirement of tmux >= 3.0.
//
// Returns nil when the version is supported. Returns the spec-defined
// user-facing error message when the major version is below 3. Wraps any
// Commander failure with a "failed to detect tmux version" prefix and
// returns a descriptive error when the output is empty or unparseable.
func CheckTmuxVersion(cmd Commander) error {
	output, err := cmd.Run("-V")
	if err != nil {
		return fmt.Errorf("failed to detect tmux version: %w", err)
	}
	if strings.TrimSpace(output) == "" {
		return errors.New("tmux -V returned no output")
	}

	major, _, label, err := ParseTmuxVersion(output)
	if err != nil {
		return err
	}
	if major < minTmuxMajor {
		return fmt.Errorf("Portal requires tmux \u2265 3.0 (found %s). Please upgrade.", label) //nolint:staticcheck // user-facing message requires capitalization per spec
	}
	return nil
}
