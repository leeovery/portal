package tmux

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/leeovery/portal/internal/log"
)

// managedEvent pairs a single Portal-managed tmux event with the eviction
// fingerprint(s) that identify a Portal-authored entry on that event and the
// desired command body the event must converge to. The convergence engine
// (convergeEvent) reads the event's entries via ShowGlobalHooksForEvent,
// treats an entry as Portal-authored iff its command contains any of the
// fingerprints, and ensures exactly one entry carrying desiredBody survives.
//
// session-closed carries a two-element fingerprint set (notify + commit-now)
// so it collapses both a stale pre-fix notifyCommand left by an older binary
// and any duplicate commit-now entries onto the single commitNowCommand.
// Every other event carries a single fingerprint.
type managedEvent struct {
	event        string
	fingerprints []string
	desiredBody  string
}

// managedEvents is the per-event convergence table consumed by
// RegisterPortalHooks. Declaration order is retained for stable log/test
// output but is not load-bearing: each event converges independently and is
// self-contained, so no event depends on another being processed first.
//
// The fingerprints are substrings inside the run-shell wrapper (e.g.
// `portal state notify` lives inside notifyCommand). The desired bodies are
// the full wrapped, guard-prefixed run-shell strings — the same constants the
// six notify events, session-closed, and the two hydration events have always
// registered.
//
// `portal state migrate-rename` is intentionally absent from every
// fingerprint set: registration's job is "ensure exactly one of this event's
// desired body", and reaping a stale cross-category migrate-rename entry is
// the teardown/clean path's responsibility. See the spec's "What is
// intentionally not consolidated" section.
var managedEvents = []managedEvent{
	{event: "session-created", fingerprints: []string{notifySubstring}, desiredBody: notifyCommand},
	{event: sessionClosedEvent, fingerprints: []string{notifySubstring, commitNowSubstring}, desiredBody: commitNowCommand},
	{event: "session-renamed", fingerprints: []string{notifySubstring}, desiredBody: notifyCommand},
	{event: "window-linked", fingerprints: []string{notifySubstring}, desiredBody: notifyCommand},
	{event: "window-unlinked", fingerprints: []string{notifySubstring}, desiredBody: notifyCommand},
	{event: "window-layout-changed", fingerprints: []string{notifySubstring}, desiredBody: notifyCommand},
	{event: "pane-focus-out", fingerprints: []string{notifySubstring}, desiredBody: notifyCommand},
	{event: "client-attached", fingerprints: []string{signalHydrateMarker}, desiredBody: signalHydrateCommand},
	{event: "client-session-changed", fingerprints: []string{signalHydrateMarker}, desiredBody: signalHydrateCommand},
}

// saveTriggerEvents lists every tmux event on which Portal registers a
// `portal state notify` hook. Order is significant — RegisterPortalHooks
// processes save-trigger events before hydration-trigger events.
var saveTriggerEvents = []string{
	"session-created",
	"session-closed",
	"session-renamed",
	"window-linked",
	"window-unlinked",
	"window-layout-changed",
	"pane-focus-out",
}

// HydrationTriggerEvents lists every tmux event on which Portal registers a
// `portal state signal-hydrate #{session_name}` hook. The literal
// `#{session_name}` is preserved verbatim — tmux expands it at hook-fire time.
//
// Exported so external test packages (in-package external tests and the
// cross-package bootstrap round-trip) can iterate the canonical list rather
// than maintaining hand-rolled mirrors that would silently under-cover
// extension. Adding a new event here automatically widens coverage in every
// consuming test. Treat the slice as read-only at runtime.
var HydrationTriggerEvents = []string{
	"client-attached",
	"client-session-changed",
}

// notifyCommand is the exact command Portal appends to each of the six
// non-session-closed save-trigger events. The defensive `command -v portal`
// guard short-circuits the invocation when the binary is absent so tmux does
// not log "command not found" spam during a binary swap or after uninstall.
//
// `session-closed` is excluded: following the
// killed-session-resurrects-within-tick-window fix it converges onto
// commitNowCommand (synchronous sessions.json write) instead of the shared
// dirty-flag touch. The session-closed convergence is intrinsic to the
// per-event ensure-exactly-one engine (its two-element fingerprint set evicts
// a stale pre-fix notifyCommand and converges to commitNowCommand) — see the
// spec's "Migration-Helper Consolidation" section for the rationale.
const notifyCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`

// commitNowCommand is the exact command Portal appends to `session-closed`.
// Unlike notifyCommand, this invokes `portal state commit-now` — a
// synchronous sessions.json write that closes the resurrection window
// between a kill and the daemon's next tick. `session-closed` is the single
// tmux-side seam that fires uniformly across every kill path (TUI confirm,
// `portal kill`, user keybindings, external `tmux kill-session`), so this
// one registration covers them all without per-call-site changes.
//
// Spec reference: `.workflows/killed-session-resurrects-within-tick-window/
// specification/.../specification.md` § Hook Registration Migration.
const commitNowCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state commit-now"`

// signalHydrateCommand is the exact command Portal appends to every
// hydration-trigger event. Same defensive guard as notifyCommand. The
// `#{session_name}` token is a tmux format variable expanded at fire time.
//
// The ` -- ` end-of-flags separator before #{session_name} is load-bearing:
// session names that begin with `-` (e.g. `-dotfiles-HM9Zhw`, which arises
// when SanitiseProjectName substitutes `.` -> `-` for projects whose basename
// starts with `.`) would otherwise be parsed by cobra/pflag as short-flag
// clusters, producing `unknown shorthand flag: 'd'` and exiting non-zero
// before runSignalHydrate runs. With `--`, every following token is treated
// as a positional argument regardless of leading dashes.
const signalHydrateCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate -- #{session_name}"`

// notifySubstring is the per-event content fingerprint used to detect a
// previously-registered Portal save-trigger hook. Distinct from
// signalHydrateMarker so the two categories cannot cross-contaminate.
const notifySubstring = "portal state notify"

// commitNowSubstring is the per-event content fingerprint identifying a
// Portal-authored `session-closed` commit-now hook. session-closed carries
// this alongside notifySubstring as a two-element eviction set so the
// convergence engine collapses both a stale pre-fix notifyCommand and any
// duplicate commit-now entries onto the single commitNowCommand.
const commitNowSubstring = "portal state commit-now"

// signalHydrateMarker is the per-event content fingerprint identifying a
// Portal-authored hydration hook on the two hydration-trigger events. It
// matches the bare `portal state signal-hydrate`, so it catches BOTH the
// legacy un-separated body and the current `--` form — letting the
// convergence engine migrate the legacy body to the current one as an
// ordinary ensure-exactly-one side effect.
const signalHydrateMarker = "portal state signal-hydrate"

// Note on the v1 deferral of the rename-key migration hook:
//
// An earlier iteration registered a third category (`portal state migrate-rename`)
// on `session-renamed`. tmux's `session-renamed` event does not reliably expose
// the prior session name, so the previous name had to come from a daemon-side
// last-seen-names map — work that exceeds v1 scope. The hook was registered
// with both arguments expanding to the same (new) name, making the body a
// silent no-op. Rather than ship inert scaffolding, the registration is
// dropped here. `cmd/state_migrate_rename.go` remains as the future endpoint;
// see the spec's "Resume Hook Firing → Session Rename: Hook Key Migration"
// section for the v2 plan.

// sessionClosedEvent is the single save-trigger event whose registration is
// converged onto commitNowCommand rather than the shared notifyCommand. The
// constant is named (not inlined) so the managedEvents table references a
// single source of truth for the session-closed literal.
const sessionClosedEvent = "session-closed"

// convergeEvent ensures the global hook array for event holds exactly one
// Portal entry carrying desiredBody, reading per-event via
// ShowGlobalHooksForEvent so the tmux 3.6b blind-event omission (the no-arg
// `show-hooks -g` does not enumerate pane-* / geometry-window-* events) cannot
// cause runaway appends.
//
// Algorithm (spec § Registration Redesign — "Ensure Exactly One"):
//
//  1. Read the event's entries. On a read failure, emit the canonical
//     show-hooks-failed WARN (error_class=unexpected) and return the wrapped
//     error so the caller folds it into the errors.Join aggregate; no other
//     event is affected.
//  2. An entry is Portal-authored iff its Command contains any of the event's
//     fingerprints (union across the set). User / other-plugin entries match
//     none and are never touched.
//  3. Idempotent fast path: if exactly one Portal-authored entry exists AND
//     its Command equals desiredBody byte-for-byte, return (0, nil) — no
//     unset, no append, no churn.
//  4. Otherwise converge: unset every Portal-authored entry in DESCENDING
//     index order (so a removal never shifts a not-yet-processed index), then
//     AppendGlobalHook(event, desiredBody) exactly once.
//
// A per-index UnsetGlobalHookAt failure is best-effort: it emits a WARN
// carrying the underlying error, the loop continues, and the failed index is
// NOT counted as evicted. An AppendGlobalHook failure is returned wrapped so
// it folds into the aggregate. The returned int is the count of
// successfully-unset entries.
//
// A nil logger is tolerated and falls through to the shared internal/log
// discard sink via log.OrDiscard.
func convergeEvent(c *Client, logger *slog.Logger, event string, fingerprints []string, desiredBody string) (int, error) {
	logger = log.OrDiscard(logger)

	raw, err := c.ShowGlobalHooksForEvent(event)
	if err != nil {
		// Canonical show-hooks-failed WARN+wrap: message "show-hooks failed",
		// error_class=unexpected, and the underlying err wrapped verbatim.
		logger.Warn("show-hooks failed", "error", err, "error_class", "unexpected")
		return 0, fmt.Errorf("show-hooks failed: %w", err)
	}

	var portalIndices []int
	var alreadyConverged bool
	for _, entry := range ParseShowHooks(raw)[event] {
		if !containsAny(entry.Command, fingerprints) {
			continue
		}
		portalIndices = append(portalIndices, entry.Index)
		if entry.Command == desiredBody {
			alreadyConverged = true
		}
	}

	// Idempotent fast path: exactly one Portal-authored entry and it already
	// carries the desired body — nothing to do.
	if len(portalIndices) == 1 && alreadyConverged {
		return 0, nil
	}

	// Descending so each unset targets the entry it identified during the
	// pre-removal scan — ascending would shift later indices.
	sort.Sort(sort.Reverse(sort.IntSlice(portalIndices)))
	var evicted int
	for _, idx := range portalIndices {
		if err := c.UnsetGlobalHookAt(event, idx); err != nil {
			// event name and hook index have no closed attr keys; "error"
			// carries the signal.
			logger.Warn("failed to evict portal hook", "error", err)
			continue
		}
		evicted++
	}

	if err := c.AppendGlobalHook(event, desiredBody); err != nil {
		return evicted, fmt.Errorf("append hook: %w", err)
	}
	return evicted, nil
}

// RegisterPortalHooks converges Portal's full hook table to exactly one entry
// per managed event, reading each event independently via
// ShowGlobalHooksForEvent. A nil logger is tolerated and falls through to the
// shared internal/log discard sink via log.OrDiscard.
//
// Each event in managedEvents is converged via convergeEvent. The loop never
// short-circuits: a per-event read or append failure is folded into an
// errors.Join aggregate (each leaf naming the failing event and wrapping the
// underlying tmux error so callers can errors.Is a sentinel), and every other
// event is still converged. This collapses any depth-N stack to one entry and
// migrates stale legacy bodies in place, as an ordinary side effect of
// bootstrap step 2 — no separate cleanup pass.
//
// When the total number of evicted entries across all events is > 0, a single
// INFO line is emitted under the bootstrap component carrying the total via
// the existing `reaped` cycle-summary attr. A registration that evicts
// nothing — including the all-fast-path no-op case — emits NO eviction line;
// that absence is the asserted churn-free signal.
func RegisterPortalHooks(c *Client, logger *slog.Logger) error {
	logger = log.OrDiscard(logger)

	var errs []error
	var totalEvicted int

	for _, me := range managedEvents {
		evicted, err := convergeEvent(c, logger, me.event, me.fingerprints, me.desiredBody)
		if err != nil {
			errs = append(errs, fmt.Errorf("register hook on %s: %w", me.event, err))
		}
		totalEvicted += evicted
	}

	if totalEvicted > 0 {
		logger.Info("collapsed stacked portal hooks", "reaped", totalEvicted)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
