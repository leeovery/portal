# General TUI flash / toast infrastructure

Portal's TUI has no general-purpose notification surface today. `bubbles/list`'s built-in status bar is explicitly disabled (`SetShowStatusBar(false)` in `internal/tui/model.go:507, 549`), and the only "warning" mechanism is the post-exit stderr flush used by the bootstrap orchestrator — useless for anything that needs feedback while the TUI is still running.

Add a general flash/toast layer reusable from every page (Loading, Sessions, Projects, FileBrowser, Preview) with:

- Pinned chrome line (header or footer placement, TBD) holding zero or one active message at a time.
- Severity styling (info / warning / error) to differentiate.
- Auto-dismissal — either tick-based (e.g. 3s) or next-keystroke, or both.
- A simple `tea.Cmd` API so any page's `Update` can emit a flash without owning chrome layout.

Use cases that motivate the infra (multiple pre-existing UX gaps the minimal one-liner in `enter-attaches-from-preview` won't close on its own):

- **Esc-refresh in preview** silently drops externally-killed sessions from the post-dismiss list. User has no signal that a session went away mid-preview.
- **Dismiss-from-preview to Sessions list** generally — any list mutation since open is silent.
- **Enter-from-preview attaching to a killed session** — handled with a feature-local one-liner in `enter-attaches-from-preview`, but that flash is bespoke to the Sessions page; promote to general infra so the same surface serves the case below.
- **Future affordances** — "session created", "alias saved", "hook registered", "bootstrap warning surfaced without exit" all want a non-modal feedback surface.

Surfaced during the `enter-attaches-from-preview` discussion when locking the session-killed-externally edge case. The feature-local one-liner was scoped for that decision; this idea captures the broader infrastructure as a separate work unit so it does not contaminate the in-flight feature's scope.
