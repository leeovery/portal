// Package hooksweep holds the hook-staleness cycle: the judgement of persisted
// hook keys against the session names that protect them, those the backend
// lists and those sessions.json holds, the conditions that forbid that
// judgement, and the vocabulary a caller reports either in. It owns the whole
// cycle's emission — its counts, its stand-downs and its failures alike ride
// the hooks component, whichever caller drove it — so a caller keeps only the
// rendering its own surface owes.
package hooksweep

// Reason is the closed vocabulary of reasons a cycle declined to run under.
// It is a type rather than a convention over plain strings so no string-typed
// value can be reported as one; an untyped constant still converts implicitly.
type Reason string

// The reasons a cycle declined to run under, which the logged reason attr also
// names. Every surface that renders one works from this set, so their
// vocabularies cannot drift from it. A sweep that ran and failed is not among
// them: it declined nothing. A hooks.json that can't be read or doesn't parse
// is such a failure, a defect to report rather than a condition to wait out.
const (
	ReasonRestoring           Reason = "restoring"
	ReasonRestoringReadFailed Reason = "restoring-read-failed"
	ReasonStateDirUnresolved  Reason = "state-dir-unresolved"
	ReasonListFailed          Reason = "session-list-failed"
	ReasonIndexUnusable       Reason = "index-unusable"
	ReasonNoSessions          Reason = "no-sessions"
	ReasonLockTimeout         Reason = "lock-timeout"
	ReasonTmuxKeys            Reason = "tmux-keys"
	ReasonMassReap            Reason = "mass-reap"
)

// Reasons makes the reasons above enumerable, so anything that must cover
// every one of them ranges over the set rather than restating it. A reason
// declared and left out of it is invisible to everything that works from it.
//
// The set is complete rather than reachable from every surface that renders
// it, so a reader meeting a phrase with no path to it is not hunting for one
// that was never written: lock-timeout's not-evaluable phrase exists for
// vocabulary completeness, since the diagnosis takes no lock.
var Reasons = []Reason{
	ReasonRestoring,
	ReasonRestoringReadFailed,
	ReasonStateDirUnresolved,
	ReasonListFailed,
	ReasonIndexUnusable,
	ReasonNoSessions,
	ReasonLockTimeout,
	ReasonTmuxKeys,
	ReasonMassReap,
}
