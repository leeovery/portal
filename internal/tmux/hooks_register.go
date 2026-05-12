package tmux

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/leeovery/portal/internal/state"
)

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

// notifyCommand is the exact command Portal appends to every save-trigger
// event. The defensive `command -v portal` guard short-circuits the
// invocation when the binary is absent so tmux does not log "command not
// found" spam during a binary swap or after uninstall.
const notifyCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`

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
var portalHookCategories = []hookCategory{
	{events: saveTriggerEvents, substring: notifySubstring, command: notifyCommand},
	{events: HydrationTriggerEvents, substring: signalHydrateSubstring, command: signalHydrateCommand},
}

// MigrationLogger is the minimal logging seam migrateHydrationHooks needs.
// Two methods (Info, Warn) is the smallest surface that conveys the spec's
// observable shape: a single INFO line summarising eviction count, plus a
// per-failure WARN line on UnsetGlobalHookAt errors.
//
// *state.Logger satisfies this interface structurally; defining a local
// interface avoids a dependency cycle (internal/state imports internal/tmux
// transitively via its callers) while keeping the seam mockable from tests.
type MigrationLogger interface {
	Info(component, format string, args ...any)
	Warn(component, format string, args ...any)
}

// noopMigrationLogger satisfies MigrationLogger with no-op methods. Used as
// the internal fallback when callers pass a nil MigrationLogger to
// RegisterPortalHooks so the migration code path always has a safe sink.
type noopMigrationLogger struct{}

func (noopMigrationLogger) Info(component, format string, args ...any) {}
func (noopMigrationLogger) Warn(component, format string, args ...any) {}

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
// emits a single INFO line of the form "evicted N stale signal-hydrate
// hook(s) lacking '--' separator". Bootstraps with no evictions are silent.
//
// Sealed inside RegisterPortalHooks: it is unexported to ensure exactly one
// canonical entry point for "hook installation". A second invocation against
// the same hook table is a no-op (idempotent).
func migrateHydrationHooks(c *Client, log MigrationLogger) (int, error) {
	if log == nil {
		log = noopMigrationLogger{}
	}

	raw, err := c.ShowGlobalHooks()
	if err != nil {
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
				log.Warn(state.ComponentBootstrap, "failed to evict stale signal-hydrate hook on %s at index %d: %v", event, idx, err)
				continue
			}
			evicted++
		}
	}

	if evicted > 0 {
		log.Info(state.ComponentBootstrap, "evicted %d stale signal-hydrate hook(s) lacking '--' separator", evicted)
	}

	return evicted, nil
}

// RegisterPortalHooks idempotently registers Portal's full hook table,
// threading a MigrationLogger through to migrateHydrationHooks so the
// bootstrap-step *state.Logger can capture eviction diagnostics. A nil
// log is tolerated and falls through to a no-op sink.
//
// Categories are processed in the order declared in portalHookCategories;
// within each category, events are processed in the order declared in the
// spec.
//
// Before the per-category register loop reaches the hydration-trigger
// category, migrateHydrationHooks runs once to evict any pre-existing
// un-separated `portal state signal-hydrate` entry left behind by an older
// portal install. Migration failures are best-effort and never abort
// bootstrap — only a ShowGlobalHooks failure inside the migration would
// produce an error, and that error is folded into the same errors.Join
// aggregate as register failures.
//
// Each registration is delegated to RegisterHookIfAbsent, which performs
// the content-based dedupe check. A failure on one event does not
// short-circuit the remaining events — every event is attempted. On any
// failures the returned error is an errors.Join aggregate; each leaf error
// names the failing event and wraps the underlying tmux error so callers
// can use errors.Is on a sentinel.
func RegisterPortalHooks(c *Client, log MigrationLogger) error {
	if log == nil {
		log = noopMigrationLogger{}
	}

	var errs []error

	if _, err := migrateHydrationHooks(c, log); err != nil {
		errs = append(errs, fmt.Errorf("migrate hydration hooks: %w", err))
	}

	for _, cat := range portalHookCategories {
		for _, event := range cat.events {
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
