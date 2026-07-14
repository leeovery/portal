# Decide whether burst outcome flashes should auto-clear on a timer

The unsupported / partial-failure burst outcome flashes return `(m, nil)` after `setFlash` and schedule no `flashTickCmd`, so — unlike the async externally-killed gone flash (which schedules `flashTickCmd` at `model.go:2413`) — they do not auto-clear on a timer; they clear only on the next actionable keystroke. On the deferred (async `terminalDetectedMsg`) path no keystroke is guaranteed, so an idle user keeps the flash indefinitely.

Decide whether burst-family outcome flashes should auto-clear for consistency with the gone-flash. This is a family-wide design choice shared with the already-shipped partial-failure path, not specific to one task.

Location: `internal/tui/burst_progress.go:436` (and siblings `internal/tui/burst_partial_failure.go:58,77`). (Report 6-9.)

Source: review of restore-host-terminal-windows/restore-host-terminal-windows
