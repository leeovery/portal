package tmux

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/leeovery/portal/internal/log"
)

// bootstrapLogger is the component=bootstrap sink for the show-hooks failure
// WARN emitted by RegisterHookIfAbsent, which (unlike its migrate siblings)
// takes no injected logger. Named distinctly from saverLogger (portal_saver.go,
// same package) to avoid a collision. log.For never returns nil, so the WARN is
// always routable; production resolves it to log.For("bootstrap"), matching the
// injected log the two migrate helpers receive at bootstrap step 2.
var bootstrapLogger = log.For("bootstrap")

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
// killed-session-resurrects-within-tick-window fix it migrates onto
// commitNowCommand (synchronous sessions.json write) instead of the shared
// dirty-flag touch. See migrateSessionClosedHook for the migration algorithm
// and the spec's "Hook Registration Migration" section for the rationale.
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
// signalHydrateSubstring so the two categories cannot cross-contaminate.
const notifySubstring = "portal state notify"

// signalHydrateSubstring is the per-event content fingerprint used to detect
// a previously-registered Portal hydration-trigger hook. The trailing `--`
// is intentional: it distinguishes the new fixed entry from any pre-existing
// un-separated entry registered by an older portal install, so the dedupe
// check in RegisterHookIfAbsent does not mistake a stale entry for the
// current one.
const signalHydrateSubstring = "portal state signal-hydrate --"

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

// RegisterHookIfAbsent appends fullCommand to the global hook array for event
// only when no existing entry on that event already contains expectedSubstring.
//
// It is the content-based idempotency primitive used by Portal's hook
// registration: the (event, expectedSubstring) pair scopes the dedupe check so
// the same command body cannot be registered twice on the same event, while
// matching substrings on other events do not suppress registration.
//
// On a show-hooks failure the error is wrapped with "show-hooks failed: %w"
// and no append is attempted. On an append failure the wrapping owned by
// AppendGlobalHook is propagated to the caller verbatim.
//
// The fullCommand argument is opaque — it is passed verbatim to the
// underlying tmux set-hook -ga invocation.
func RegisterHookIfAbsent(c *Client, event, expectedSubstring, fullCommand string) error {
	raw, err := c.ShowGlobalHooks()
	if err != nil {
		// Failure drops this event's hook registration (a unit of work) →
		// WARN error_class=unexpected per the level table. The wrapped err
		// (a *CommandError carrying tmux argv + stderr) is passed directly so
		// the rendered line carries the diagnostic context.
		bootstrapLogger.Warn("show-hooks failed", "error", err, "error_class", "unexpected")
		return fmt.Errorf("show-hooks failed: %w", err)
	}

	for _, entry := range ParseShowHooks(raw)[event] {
		if strings.Contains(entry.Command, expectedSubstring) {
			return nil
		}
	}

	return c.AppendGlobalHook(event, fullCommand)
}

// hookCategory bundles a set of tmux events with the (substring, command)
// pair RegisterPortalHooks should append on each. Adding a new category is a
// single table entry — no new loop, no new branch.
//
// `session-closed` is intentionally absent from every category here — it has
// its own scan-and-remove + append-if-absent migration path (see
// migrateSessionClosedHook). The six non-session-closed save-trigger events
// retain the original substring-based append-if-absent discipline.
type hookCategory struct {
	events    []string
	substring string
	command   string
}

// portalHookCategories is the registration table consumed by
// RegisterPortalHooks. Order is significant: categories are processed in
// declaration order, and within each category events are processed in the
// order declared on the category's events slice. The current ordering
// (save-trigger first, then hydration-trigger) matches the spec.
//
// The save-trigger category's events slice is saveTriggerEvents with
// `session-closed` filtered out by RegisterPortalHooks. The category table
// itself stays uniform — the filter lives at the loop site so the table
// continues to read as a single declarative truth.
var portalHookCategories = []hookCategory{
	{events: saveTriggerEvents, substring: notifySubstring, command: notifyCommand},
	{events: HydrationTriggerEvents, substring: signalHydrateSubstring, command: signalHydrateCommand},
}

// sessionClosedEvent is the single save-trigger event whose registration has
// been migrated off the shared notifyCommand and onto commitNowCommand. The
// constant is named (not inlined) so the (a) skip predicate in
// RegisterPortalHooks and (b) target string inside migrateSessionClosedHook
// reference a single source of truth.
const sessionClosedEvent = "session-closed"

// staleSignalHydratePrefix is one of the two substrings the eviction
// predicate requires: every Portal-authored signal-hydrate hook body
// begins with `command -v portal >/dev/null 2>&1 &&` regardless of vintage.
// Requiring this guard prefix prevents the migration from removing
// hand-authored user hooks that reference `portal state signal-hydrate` in
// a different shape.
const staleSignalHydratePrefix = "command -v portal >/dev/null 2>&1 &&"

// staleSignalHydrateMarker is the second substring the eviction predicate
// requires: any Portal-authored signal-hydrate body contains the literal
// `portal state signal-hydrate`. Combined with the absence of
// signalHydrateSubstring (`portal state signal-hydrate --`), this isolates
// the legacy un-separated body from the new fixed body.
const staleSignalHydrateMarker = "portal state signal-hydrate"

// isStaleSignalHydrateEntry reports whether a hook body is the legacy
// Portal-authored signal-hydrate command lacking the `--` end-of-flags
// separator. Eligible-for-eviction iff it contains both the
// `command -v portal` guard prefix and `portal state signal-hydrate`, AND
// does NOT contain `portal state signal-hydrate --`.
func isStaleSignalHydrateEntry(cmd string) bool {
	return strings.Contains(cmd, staleSignalHydratePrefix) &&
		strings.Contains(cmd, staleSignalHydrateMarker) &&
		!strings.Contains(cmd, signalHydrateSubstring)
}

// migrateHydrationHooks scans every event in HydrationTriggerEvents and
// evicts any pre-existing hook entry whose body matches the legacy
// un-separated `portal state signal-hydrate` shape. Indices are processed
// in descending order so successful removals do not shift the indices of
// later targets.
//
// Returns (evicted, nil) on the happy path and on partial-failure paths —
// per-index UnsetGlobalHookAt failures are best-effort and surface only as
// WARN log lines via the supplied MigrationLogger. The only path that
// returns a non-nil err is a ShowGlobalHooks failure, which is wrapped with
// "show-hooks failed: %w" and aborts the migration before any unset call.
//
// When at least one entry was evicted across all events, the function
// emits a single INFO line "evicted stale signal-hydrate hooks" carrying the
// eviction count under the "reaped" cycle-summary attr. Bootstraps with no
// evictions are silent.
//
// Sealed inside RegisterPortalHooks: it is unexported to ensure exactly one
// canonical entry point for "hook installation". A second invocation against
// the same hook table is a no-op (idempotent).
//
// A nil logger is tolerated and falls through to the shared internal/log
// discard sink via log.OrDiscard.
func migrateHydrationHooks(c *Client, logger *slog.Logger) (int, error) {
	logger = log.OrDiscard(logger)

	raw, err := c.ShowGlobalHooks()
	if err != nil {
		// Failure aborts the hydration-hook migration (a unit of work) →
		// WARN error_class=unexpected per the level table. Wrapped err passed
		// directly so the *CommandError stderr surfaces.
		logger.Warn("show-hooks failed", "error", err, "error_class", "unexpected")
		return 0, fmt.Errorf("show-hooks failed: %w", err)
	}

	parsed := ParseShowHooks(raw)

	var evicted int
	for _, event := range HydrationTriggerEvents {
		// Collect indices of stale entries on this event in descending order.
		var staleIndices []int
		for _, entry := range parsed[event] {
			if isStaleSignalHydrateEntry(entry.Command) {
				staleIndices = append(staleIndices, entry.Index)
			}
		}
		// Process highest-first so removing one does not shift earlier indices.
		sort.Sort(sort.Reverse(sort.IntSlice(staleIndices)))

		for _, idx := range staleIndices {
			if err := c.UnsetGlobalHookAt(event, idx); err != nil {
				// event name and hook index have no closed attr keys; "error"
				// carries the signal.
				logger.Warn("failed to evict stale signal-hydrate hook", "error", err)
				continue
			}
			evicted++
		}
	}

	if evicted > 0 {
		logger.Info("evicted stale signal-hydrate hooks lacking '--' separator", "reaped", evicted)
	}

	return evicted, nil
}

// migrateSessionClosedHook is the dedicated registration path for the
// `session-closed` event. Unlike the substring-based append-if-absent
// discipline used for every other Portal hook, the session-closed migration
// must both (a) evict any stale pre-fix notifyCommand entries left behind by
// an older Portal install and (b) idempotently install commitNowCommand.
//
// Algorithm (per spec § Hook Registration Migration):
//
//  1. ShowGlobalHooks → filter to session-closed entries.
//  2. Build the slice of indices whose body exact-string matches the
//     historical notifyCommand literal. Sort descending so unset calls do
//     not shift later indices.
//  3. For each index: UnsetGlobalHookAt(sessionClosedEvent, idx). Per-index
//     failure is best-effort — log WARN under ComponentBootstrap and
//     continue to the next index.
//  4. After eviction, if no remaining entry exact-matches commitNowCommand,
//     AppendGlobalHook(sessionClosedEvent, commitNowCommand). The exact-
//     match check is computed during step 2 so the post-removal scan does
//     not require a second ShowGlobalHooks round-trip — none of the unset
//     calls can have introduced a commitNowCommand entry, so the
//     pre-eviction observation is authoritative.
//
// Exact-string match (not substring/regex) is load-bearing: an exact match
// against the historical Portal-emitted literal cannot accidentally remove
// user-customised hooks (e.g. `portal state notify --debug` or a script
// wrapper invoking portal). Tolerating quoting drift would create false
// positives.
//
// Returns a non-nil error only when ShowGlobalHooks fails (wrapped with
// "show-hooks failed: %w") or when the AppendGlobalHook call fails — both
// surface to RegisterPortalHooks as folded entries in the errors.Join
// aggregate, consistent with other step-2 register failures.
func migrateSessionClosedHook(c *Client, logger *slog.Logger) error {
	logger = log.OrDiscard(logger)

	raw, err := c.ShowGlobalHooks()
	if err != nil {
		// Failure skips the session-closed migration (a unit of work) →
		// WARN error_class=unexpected, normalized to the uniform shape shared
		// with the two sibling show-hooks branches. Wrapped err passed
		// directly so the *CommandError stderr surfaces.
		logger.Warn("show-hooks failed", "error", err, "error_class", "unexpected")
		return fmt.Errorf("show-hooks failed: %w", err)
	}

	entries := ParseShowHooks(raw)[sessionClosedEvent]

	var staleIndices []int
	var commitNowPresent bool
	for _, entry := range entries {
		switch entry.Command {
		case notifyCommand:
			staleIndices = append(staleIndices, entry.Index)
		case commitNowCommand:
			commitNowPresent = true
		}
	}

	// Descending so each unset targets the entry it identified during the
	// pre-removal scan — ascending would shift later indices.
	sort.Sort(sort.Reverse(sort.IntSlice(staleIndices)))
	for _, idx := range staleIndices {
		if err := c.UnsetGlobalHookAt(sessionClosedEvent, idx); err != nil {
			// event name and hook index have no closed attr keys; "error"
			// carries the signal.
			logger.Warn("failed to evict stale notify hook", "error", err)
			continue
		}
	}

	if commitNowPresent {
		return nil
	}
	if err := c.AppendGlobalHook(sessionClosedEvent, commitNowCommand); err != nil {
		return fmt.Errorf("append commit-now hook: %w", err)
	}
	return nil
}

// RegisterPortalHooks idempotently registers Portal's full hook table,
// threading a *slog.Logger through to migrateHydrationHooks and
// migrateSessionClosedHook so the bootstrap-step logger can capture
// eviction diagnostics. A nil logger is tolerated and falls through to the
// shared internal/log discard sink via log.OrDiscard.
//
// Categories are processed in the order declared in portalHookCategories;
// within each category, events are processed in the order declared in the
// spec. The save-trigger category's `session-closed` event is filtered out
// of the substring-based append-if-absent loop and routed instead through
// migrateSessionClosedHook, which performs an exact-string scan-and-remove
// for stale pre-fix notifyCommand entries before appending commitNowCommand.
//
// Before the per-category register loop reaches the hydration-trigger
// category, migrateHydrationHooks runs once to evict any pre-existing
// un-separated `portal state signal-hydrate` entry left behind by an older
// portal install. Migration failures are best-effort and never abort
// bootstrap — only a ShowGlobalHooks failure inside the migration would
// produce an error, and that error is folded into the same errors.Join
// aggregate as register failures.
//
// Each non-session-closed registration is delegated to RegisterHookIfAbsent,
// which performs the content-based dedupe check. A failure on one event
// does not short-circuit the remaining events — every event is attempted.
// On any failures the returned error is an errors.Join aggregate; each leaf
// error names the failing event and wraps the underlying tmux error so
// callers can use errors.Is on a sentinel.
func RegisterPortalHooks(c *Client, logger *slog.Logger) error {
	logger = log.OrDiscard(logger)

	var errs []error

	if _, err := migrateHydrationHooks(c, logger); err != nil {
		errs = append(errs, fmt.Errorf("migrate hydration hooks: %w", err))
	}

	for _, cat := range portalHookCategories {
		for _, event := range cat.events {
			if event == sessionClosedEvent {
				if err := migrateSessionClosedHook(c, logger); err != nil {
					errs = append(errs, fmt.Errorf("register hook on %s: %w", event, err))
				}
				continue
			}
			if err := RegisterHookIfAbsent(c, event, cat.substring, cat.command); err != nil {
				errs = append(errs, fmt.Errorf("register hook on %s: %w", event, err))
			}
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
