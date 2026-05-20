# Space dismisses scrollback preview

In the TUI session list, pressing `Space` opens the scrollback preview page for the highlighted session. Today the only way back to the session list is `Esc`. Make `Space` act as a second toggle: pressing it while the preview is active should behave identically to `Esc` and return to the session list.

The change is mechanical and localised. `internal/tui/pagepreview.go:467` is the `tea.KeyEsc` case inside `previewModel.Update`, which returns `previewDismissedMsg{}`. A sibling case for the spacebar (either `tea.KeySpace` or the `tea.KeyRunes` form where `msg.Runes == []rune{' '}`) returning the same `previewDismissedMsg{}` is all that's needed. The existing dismiss pathway already handles the `pagePreview → pageSessions` transition, including the sessions-list refresh on dismissal, so no other wiring needs touching.

The motivation is ergonomic symmetry: `Space` is the open gesture, so it reads naturally as the close gesture too. Users who reach for `Space` to toggle the preview shouldn't have to switch hand position to `Esc` to back out.
