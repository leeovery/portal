package bootstrap

// FatalError is the typed sentinel for unrecoverable bootstrap conditions
// (tmux missing, version too old, EnsureServer failure, hook registration
// failure, @portal-restoring marker failure). The orchestrator's job is
// only to surface a single user-facing line on stderr and exit non-zero;
// FatalError carries that line in UserMessage and the underlying cause for
// errors.Is / errors.As traversal and portal.log diagnostics.
//
// Soft failures (EnsureSaver, CleanStale, Restore content errors) are NOT
// wrapped in FatalError — they degrade locally and continue per spec.
type FatalError struct {
	// UserMessage is the single line emitted to stderr at the top-level
	// Execute path. It is the only text the user sees; the spec mandates
	// a single line, no banners, no colors.
	UserMessage string

	// Cause is the underlying error this FatalError wraps. Exposed via
	// Unwrap for errors.Is / errors.As traversal so callers can match on
	// sentinel values further down the stack.
	Cause error
}

// Error returns the UserMessage so the default Cobra/main.go error path
// (fmt.Fprintln(os.Stderr, err)) emits exactly the user-facing line.
func (e *FatalError) Error() string { return e.UserMessage }

// Unwrap exposes the underlying cause to errors.Is and errors.As.
func (e *FatalError) Unwrap() error { return e.Cause }

// NewFatal constructs a FatalError pairing the user-facing message with
// the underlying cause. Both arguments are stored verbatim; callers are
// responsible for formatting userMsg per the spec ("Portal failed to ...:
// <err>").
func NewFatal(userMsg string, cause error) *FatalError {
	return &FatalError{UserMessage: userMsg, Cause: cause}
}
