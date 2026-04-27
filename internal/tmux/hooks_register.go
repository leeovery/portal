package tmux

import (
	"errors"
	"fmt"
	"strings"
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

// hydrationTriggerEvents lists every tmux event on which Portal registers a
// `portal state signal-hydrate #{session_name}` hook. The literal
// `#{session_name}` is preserved verbatim — tmux expands it at hook-fire time.
var hydrationTriggerEvents = []string{
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
const signalHydrateCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"`

// notifySubstring is the per-event content fingerprint used to detect a
// previously-registered Portal save-trigger hook. Distinct from
// signalHydrateSubstring so the two categories cannot cross-contaminate.
const notifySubstring = "portal state notify"

// signalHydrateSubstring is the per-event content fingerprint used to detect
// a previously-registered Portal hydration-trigger hook.
const signalHydrateSubstring = "portal state signal-hydrate"

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

// RegisterPortalHooks idempotently registers Portal's full Phase 1 hook
// table on the supplied tmux Client. Save-trigger events are processed
// first, then hydration-trigger events; within each category, events are
// processed in the order declared in the spec.
//
// Each registration is delegated to RegisterHookIfAbsent, which performs the
// content-based dedupe check. A failure on one event does not short-circuit
// the remaining events — every event is attempted. On any failures the
// returned error is an errors.Join aggregate; each leaf error names the
// failing event and wraps the underlying tmux error so callers can use
// errors.Is on a sentinel.
func RegisterPortalHooks(c *Client) error {
	var errs []error

	for _, event := range saveTriggerEvents {
		if err := RegisterHookIfAbsent(c, event, notifySubstring, notifyCommand); err != nil {
			errs = append(errs, fmt.Errorf("register hook on %s: %w", event, err))
		}
	}

	for _, event := range hydrationTriggerEvents {
		if err := RegisterHookIfAbsent(c, event, signalHydrateSubstring, signalHydrateCommand); err != nil {
			errs = append(errs, fmt.Errorf("register hook on %s: %w", event, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
