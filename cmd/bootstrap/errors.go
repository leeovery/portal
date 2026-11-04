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

// SaverLeftMessage is the way out of a _portal-saver Portal can neither use
// nor end: the fatal of a bootstrap that finds one, and uninstall's failure on
// one.
const SaverLeftMessage = "Portal can't use the _portal-saver session left running on another zmx, and couldn't end it. End it with that zmx (zmx kill _portal-saver), or restart your computer."

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
// made none, and is left for the next command.
func SessionsUnlistedWarning() Warning {
	return Warning{Lines: []string{
		"Portal couldn't list zmx's sessions — restoration skipped; it will try again on the next command.",
		"Run `portal doctor` for details.",
	}}
}

// UnjudgedWarning is a saver whose zmx protocol Portal couldn't judge, or no
// saver to judge it by, so restore is left for the next command.
func UnjudgedWarning() Warning {
	return Warning{Lines: []string{
		"Portal couldn't tell which protocol zmx speaks — restoration skipped; it will try again on the next command.",
		"Run `portal doctor` for details.",
	}}
}

// OwedUnmarkedWarning is a bootstrap that found no saver and couldn't record
// the sessions owed a restore, so restore is left for the next command.
func OwedUnmarkedWarning() Warning {
	return Warning{Lines: []string{
		"Portal couldn't record which sessions to restore — restoration skipped; it will try again on the next command.",
		"Check `portal doctor` or ~/.config/portal/state/portal.log.",
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

// SaverDownWarning is a saver whose daemon failed to start.
func SaverDownWarning() Warning {
	return Warning{Lines: []string{
		"Portal save daemon failed to start — sessions won't be captured.",
		"Run `portal doctor` for details.",
	}}
}

// SaverUnreadWarning is a saver that couldn't be read, so was neither kept nor
// replaced.
func SaverUnreadWarning() Warning {
	return Warning{Lines: []string{
		"Portal couldn't read its save daemon — sessions may not be captured.",
		"Run `portal doctor` for details.",
	}}
}

// SaverNotReplacedWarning is a saver of another version that couldn't be
// stopped, its daemon still running.
func SaverNotReplacedWarning() Warning {
	return Warning{Lines: []string{
		"Portal couldn't replace its save daemon — the old one is still saving.",
		"Run `portal doctor` for details.",
	}}
}

// BootIDUnreadWarning is a latch that can't be written for want of this
// boot's id, so every command bootstraps.
func BootIDUnreadWarning() Warning {
	return Warning{Lines: []string{
		"Portal couldn't read this boot's id — it will start in full on every command.",
		"Check `portal doctor` or ~/.config/portal/state/portal.log.",
	}}
}

// LatchUnwrittenWarning is a latch that couldn't be written, so the next
// command bootstraps again.
func LatchUnwrittenWarning() Warning {
	return Warning{Lines: []string{
		"Portal couldn't record that it has started — it will start in full again on the next command.",
		"Check `portal doctor` or ~/.config/portal/state/portal.log.",
	}}
}
