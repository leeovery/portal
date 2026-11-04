package bootstrap

import (
	"strconv"

	"github.com/leeovery/portal/internal/warning"
)

// FatalError is the typed sentinel for unrecoverable bootstrap conditions.
// UserMessage is printed to stderr verbatim by the top-level error path. Soft
// failures are never wrapped in it — they degrade locally and continue.
type FatalError struct {
	UserMessage string
	Cause       error
	// Step is the index of the step that failed, as the loading page numbers
	// it, or 0 for a failure before any step ran.
	Step int
}

func (e *FatalError) Error() string { return e.UserMessage }

func (e *FatalError) Unwrap() error { return e.Cause }

func NewFatal(userMsg string, cause error) *FatalError {
	return &FatalError{UserMessage: userMsg, Cause: cause}
}

// Warning is an alias, not a local type: cmd, cmd/bootstrap and tui must share
// one canonical shape across the import boundary.
type Warning = warning.Warning

// CorruptSessionsJSONWarning covers an unreadable sessions.json as well as an
// unparseable one.
func CorruptSessionsJSONWarning() Warning {
	return Warning{Lines: []string{
		"Portal state file unusable — restoration skipped.",
		"Check `portal doctor` or ~/.config/portal/state/portal.log.",
	}}
}

// SessionsUnlistedWarning is a restore that couldn't list zmx's sessions, so
// made none.
func SessionsUnlistedWarning() Warning {
	return Warning{Lines: []string{
		"Portal couldn't list zmx's sessions — restoration skipped.",
		"Run `portal doctor` for details.",
	}}
}

// UnrestoredWarning counts the saved sessions a restore couldn't make again,
// whose entries it keeps for the next to retry.
func UnrestoredWarning(n int) Warning {
	sessions := "sessions"
	if n == 1 {
		sessions = "session"
	}
	return Warning{Lines: []string{
		"Portal couldn't restore " + strconv.Itoa(n) + " saved " + sessions + " — kept for the next restore.",
		"Check `portal doctor` or ~/.config/portal/state/portal.log.",
	}}
}

func SaverDownWarning() Warning {
	return Warning{Lines: []string{
		"Portal save daemon failed to start — sessions won't be captured.",
		"Run `portal doctor` for details.",
	}}
}
