# Esc after preview-from-filtered-list hides the session list

In the session-list TUI (`portal open` / `x`), there is a sequence of keystrokes that leaves the UI in a visibly empty state and silently discards the committed filter.

Reproduction:

1. Launch the TUI (`portal open` or `x`) so the sessions page is showing.
2. Press `/` to enter filter mode.
3. Type characters until the list narrows to an available session.
4. Press `Enter` to commit the filter. The filter input exits and the matching session row is re-highlighted in the list — this is the expected behaviour.
5. Press `Space`. The scrollback preview page opens for the highlighted session — also expected.
6. Press `Esc`. **Bug:** the session list disappears entirely and the previously committed filter is also gone. The screen does not return to the filtered session list as one would expect from dismissing a preview.
7. Press `Esc` a second time. The session list reappears.

So the first `Esc` after returning from the preview puts the TUI into a hidden / empty-looking state, and a second `Esc` is required to bring the list back. Either way, the filter text that was committed in step 4 is lost — the reappearing list is unfiltered.

The bug only manifests on this specific path: filter → commit with Enter → preview with Space → Esc. Without the filter step (i.e. previewing an unfiltered list and pressing Esc) the preview dismisses straight back to the session list as expected, so the issue is tied to the interaction between a committed filter and the preview-dismiss return path on the sessions page.

Impact is mainly UX friction: the user briefly sees what looks like the whole UI vanishing, has to press Esc again to recover, and then has to re-type their filter because the committed filter state was discarded. Nothing is destroyed and no tmux state is affected — it is purely a TUI page-state and filter-state interaction issue. Worth investigating in `internal/tui/` around the `pagePreview → pageSessions` dismiss handler and the sessions-page Esc handling.
