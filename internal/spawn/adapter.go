package spawn

// Adapter is the single-capability contract for a host-terminal window driver:
// open one new host window running a given command. It is the seam that
// quarantines every OS/terminal-specific concern (AppleScript, osascript,
// AppleEvent codes, TCC) behind a driver, so Portal's general spawn/report/UI
// code never handles a terminal-specific string or error number — it switches
// on the generic Result taxonomy alone.
type Adapter interface {
	// OpenWindow opens one new host-terminal window running command verbatim
	// as a real argv, and reports the outcome as a generic typed Result.
	//
	// command is the composed env-self-sufficient attach argv
	// (/usr/bin/env … <exe> attach <session>); the adapter runs it as-is and
	// is NOT session-aware — it never bakes in `portal attach` and never
	// parses the session out of the argv (the spec rejects a session-aware
	// OpenAttached(session)).
	OpenWindow(command []string) Result
}

// Outcome is the generic, terminal-agnostic classification of an OpenWindow
// attempt. General code switches on Outcome and never inspects the OS-specific
// Detail/Guidance text to decide what happened. The three real members (Success,
// SpawnFailed, PermissionRequired) are the whole closed taxonomy fixed by the
// spec's Permissions & Error Quarantine boundary; OutcomeUnknown is the excluded
// zero-value sentinel (see the const block below), not a fourth outcome a driver
// may return.
//
// "Unsupported" is deliberately NOT an Outcome: whether a host terminal has a
// driver is a resolution-tier decision (Resolver.Resolve returns
// ResolutionUnsupported when no adapter matches — see resolver.go), taken before
// any OpenWindow call. OpenWindow only ever runs on a resolved, supported
// adapter, so it must never report "unsupported"; the resolution tier owns that
// outcome and its atomic no-op handling. A future driver author must return one
// of the three members below.
type Outcome int

const (
	// OutcomeUnknown is the invalid/unset zero-value sentinel: a bare
	// Result{} carries it, so a zero-value Result fails OK() and is never
	// silently mistaken for a success (which would wrongly gate a
	// self-attach). It mirrors RecipeKind's zero-invalid treatment in
	// recipe.go. OpenWindow must NEVER return it — a driver reports exactly
	// one of the three real members below.
	OutcomeUnknown Outcome = iota
	// OutcomeSuccess — the host window opened cleanly.
	OutcomeSuccess
	// OutcomeSpawnFailed — a driver was available but the window failed to
	// open (e.g. a non-zero osascript exit).
	OutcomeSpawnFailed
	// OutcomePermissionRequired — the OS refused for a permission reason
	// (e.g. TCC Automation); Guidance carries the driver-composed hint. The
	// AppleEvent-code → this-member mapping is layered in a later phase; the
	// member and its Guidance field exist now so the taxonomy is complete.
	OutcomePermissionRequired
)

// Result is the generic typed outcome of an OpenWindow attempt. Detail and
// Guidance are opaque, driver-owned payloads: Detail is the OS-specific text
// (e.g. an osascript error body) that rides up only as a log `detail` attr,
// and Guidance is the permission-guidance text (target terminal + Automation
// hint) populated only on the permission path. General code passes both
// through verbatim and classifies solely on Outcome.
type Result struct {
	Outcome  Outcome
	Detail   string
	Guidance string
}

// Success builds an OutcomeSuccess result carrying the opaque detail.
func Success(detail string) Result {
	return Result{Outcome: OutcomeSuccess, Detail: detail}
}

// SpawnFailed builds an OutcomeSpawnFailed result carrying the opaque detail.
func SpawnFailed(detail string) Result {
	return Result{Outcome: OutcomeSpawnFailed, Detail: detail}
}

// PermissionRequired builds an OutcomePermissionRequired result carrying the
// opaque detail plus the driver-composed permission guidance text.
func PermissionRequired(detail, guidance string) Result {
	return Result{Outcome: OutcomePermissionRequired, Detail: detail, Guidance: guidance}
}

// OK reports whether the window opened successfully. It is the single
// predicate general code uses to gate the self-attach; it never reads Detail
// or Guidance.
func (r Result) OK() bool {
	return r.Outcome == OutcomeSuccess
}
