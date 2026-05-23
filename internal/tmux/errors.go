package tmux

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNoSuchSession is the typed sentinel returned (wrapped) by per-session
// tmux operations when the underlying tmux invocation reports that the
// addressed session does not exist. It fires when the *CommandError stderr
// captured from the failing tmux call contains the case-sensitive substring
// "no such session" — the canonical lowercase phrasing emitted by tmux.
//
// Callers consume this sentinel exclusively via errors.Is(err, ErrNoSuchSession).
// The wrap is multi-%w so the original *CommandError remains recoverable on
// the same chain via errors.As(err, &cmdErr); discrimination and recovery are
// independent.
//
// Daemon-layer and other downstream callers MUST NOT perform substring
// matching against tmux stderr to detect this condition. Substring matching
// at the boundary above internal/tmux couples those layers to tmux's exact
// error-string surface — a surface that is not a stable contract. All such
// classification belongs here, behind a single sentinel, so a future tmux
// rephrasing requires a one-line change to the boundary discriminator and
// nothing else.
var ErrNoSuchSession = errors.New("no such session")

// ErrEmptyPaneList is the typed sentinel returned (wrapped) by SaverPanePID
// when the underlying `tmux list-panes -t =<session> -F '#{pane_pid}'`
// invocation succeeds (no exec error, no "no such session" stderr) but
// produces stdout with no non-empty lines. The shape is observably distinct
// from ErrNoSuchSession: the session exists, but tmux reported zero panes —
// an unusual but possible transient (e.g., a pane mid-respawn).
//
// Callers consume this sentinel via errors.Is(err, ErrEmptyPaneList).
// Component D's saverMembershipProbe collapses this — like every other
// SaverPanePID failure mode — to "absent" so the daemon's self-supervision
// counter increments without coupling to the underlying classification.
var ErrEmptyPaneList = errors.New("empty pane list")

// ErrPanePIDParse is the typed sentinel returned (wrapped) by SaverPanePID
// when the underlying tmux invocation succeeds with a non-empty first line
// that cannot be parsed as a base-10 integer via strconv.Atoi. Observed in
// practice when tmux's format expansion emits an unexpected token (e.g., a
// future format-string regression upstream) rather than a numeric pane_pid.
//
// Callers consume this sentinel via errors.Is(err, ErrPanePIDParse).
// Like ErrEmptyPaneList, Component D's saverMembershipProbe collapses this
// to "absent" — the daemon cannot prove membership without a valid pid, and
// any classification it cannot interpret is, by the spec's "treat any error
// as absent" rule, equivalent to the saver being gone.
var ErrPanePIDParse = errors.New("pane pid parse")

// noSuchSessionStderrSubstr is the case-sensitive substring used to detect
// tmux's "no such session" stderr phrasing. tmux emits the lowercase form;
// matching is intentionally case-sensitive so we never absorb unrelated
// phrasings (e.g. a future tool layered on top that capitalises differently)
// into the natural-churn classification.
const noSuchSessionStderrSubstr = "no such session"

// wrapNoSuchSession inspects err for the tmux "no such session" signature and,
// when present, wraps it so callers can discriminate via errors.Is against
// ErrNoSuchSession while still recovering the original *CommandError via
// errors.As. Returns:
//
//   - nil if err is nil.
//   - A multi-%w wrap "ErrNoSuchSession: <err>" if err unwraps to
//     *CommandError AND that CommandError.Stderr contains the lowercase
//     substring "no such session".
//   - The original err otherwise (non-*CommandError chains, empty stderr,
//     mixed-case stderr, unrelated stderr).
//
// The Go 1.20+ multi-%w form is required so both the sentinel and the
// underlying chain remain reachable on the same error value.
func wrapNoSuchSession(err error) error {
	if err == nil {
		return nil
	}
	var cmdErr *CommandError
	if errors.As(err, &cmdErr) && strings.Contains(cmdErr.Stderr, noSuchSessionStderrSubstr) {
		return fmt.Errorf("%w: %w", ErrNoSuchSession, err)
	}
	return err
}
