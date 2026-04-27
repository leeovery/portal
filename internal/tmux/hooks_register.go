package tmux

import (
	"fmt"
	"strings"
)

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
