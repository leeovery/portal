package transienttest

import (
	"fmt"
	"sync/atomic"

	"github.com/leeovery/portal/internal/tmux"
)

// FailureMode selects the per-test policy applied by Commander when it
// observes a `list-panes -a` invocation. PassThrough disables the
// interception entirely so a single Commander instance can be constructed
// and rotated between modes during a test without rebuilding the wrapping
// chain.
type FailureMode int

const (
	// PassThrough disables interception — every call (including
	// `list-panes -a`) delegates to the inner Commander verbatim.
	PassThrough FailureMode = iota
	// FailExitNonZero makes intercepted calls return
	// ("", error) — modelling tmux exit != 0 on the wire.
	FailExitNonZero
	// FailEmptyStdout makes intercepted calls return ("", nil) —
	// modelling tmux exit 0 with empty stdout (the mode (b) trigger
	// for the bootstrap hazard guard).
	FailEmptyStdout
)

// Commander wraps an inner tmux.Commander and intercepts only invocations
// matching `list-panes` with the `-a` flag. All other invocations
// (including `list-panes` without `-a`, `list-windows -a`, capture-pane,
// etc.) are delegated to the inner Commander verbatim — preserving
// production fidelity for every non-target tmux call.
//
// Policy semantics:
//   - Mode == PassThrough: no interception, inner Commander handles
//     every call.
//   - Mode == FailExitNonZero: intercepted calls return
//     ("", fmt.Errorf("tmux list-panes -a: exit 1 (simulated transient)")).
//   - Mode == FailEmptyStdout: intercepted calls return ("", nil).
//
// The OneShot toggle is the lever used by tests that need an earlier
// bootstrap step (e.g. step 4 orphan sweep) to succeed before a later
// step (e.g. step 11 CleanStale) observes the transient. When OneShot is
// true, the FIRST intercepted call applies the policy; every subsequent
// intercepted call falls through to the inner Commander. When OneShot is
// false (the default), every intercepted call applies the policy —
// "sticky failure" matching the prevailing semantics across consumers.
//
// Concurrent-safety: the interception counter uses atomic.Int64 so the
// OneShot toggle is safe under the parallel `tmux ...` calls that
// bootstrap step 4 (orphan sweep) and step 11 (CleanStale) may issue.
// The Mode and Inner fields are NOT protected because tests are expected
// to flip them only between phases, not during concurrent tmux activity.
type Commander struct {
	// Inner is the downstream Commander. Defaults at construction
	// time to &tmux.RealCommander{} in production-fidelity tests;
	// integration tests targeting an isolated tmux server should
	// wire a socket-anchored Commander here instead.
	Inner tmux.Commander
	// Mode selects the interception policy. Default zero value is
	// PassThrough — explicitly require the test to opt in to a
	// failure policy.
	Mode FailureMode
	// OneShot, when true, causes only the first intercepted call to
	// apply the policy. Subsequent intercepted calls delegate to
	// Inner verbatim. Default false (sticky failure).
	OneShot bool

	// intercepted is the atomic counter backing the OneShot toggle.
	// Zero-value means "no intercepted calls observed yet".
	intercepted atomic.Int64
}

// shouldIntercept reports whether the supplied tmux argv targets
// `list-panes -a`. The check is positional on argv[0] and a substring
// scan for "-a" in the remaining args — matching the production
// callsites (tmux.ListAllPanesWithFormat and bootstrap step 4's
// orphan-sweep pgrep precondition).
func (c *Commander) shouldIntercept(args []string) bool {
	if len(args) == 0 || args[0] != "list-panes" {
		return false
	}
	for _, a := range args[1:] {
		if a == "-a" {
			return true
		}
	}
	return false
}

// applyPolicy applies the per-mode policy AFTER the OneShot gate has
// decided this invocation is the one to act on. Returns the
// (output, error) pair every Run / RunRaw caller expects.
func (c *Commander) applyPolicy() (string, error) {
	switch c.Mode {
	case FailExitNonZero:
		return "", fmt.Errorf("tmux list-panes -a: exit 1 (simulated transient)")
	case FailEmptyStdout:
		return "", nil
	case PassThrough:
		// Should not be reached — PassThrough is filtered before
		// applyPolicy via the caller's policy check. Defensive
		// fall-through to a clear error so a future regression
		// surfaces immediately rather than silently degrading.
		return "", fmt.Errorf("transienttest.Commander: applyPolicy called with PassThrough mode")
	default:
		return "", fmt.Errorf("transienttest.Commander: unknown failure mode %d", c.Mode)
	}
}

// intercept centralises the OneShot / Mode dispatch shared by Run and
// RunRaw. Returns (output, error, true) when the policy applied, or
// ("", nil, false) when the caller should delegate to Inner.
func (c *Commander) intercept(args []string) (string, error, bool) {
	if c.Mode == PassThrough {
		return "", nil, false
	}
	if !c.shouldIntercept(args) {
		return "", nil, false
	}
	n := c.intercepted.Add(1)
	if c.OneShot && n > 1 {
		return "", nil, false
	}
	out, err := c.applyPolicy()
	return out, err, true
}

// Run implements tmux.Commander. Intercepts `list-panes -a` per the
// configured policy and delegates every other call to Inner.
func (c *Commander) Run(args ...string) (string, error) {
	if out, err, handled := c.intercept(args); handled {
		return out, err
	}
	return c.Inner.Run(args...)
}

// RunRaw implements tmux.Commander. Intercepts `list-panes -a` per the
// configured policy and delegates every other call to Inner —
// scrollback-capturing paths in bootstrap depend on RunRaw fidelity.
func (c *Commander) RunRaw(args ...string) (string, error) {
	if out, err, handled := c.intercept(args); handled {
		return out, err
	}
	return c.Inner.RunRaw(args...)
}

// Compile-time guard: Commander must satisfy tmux.Commander so consumers
// can build a *tmux.Client from it.
var _ tmux.Commander = (*Commander)(nil)
