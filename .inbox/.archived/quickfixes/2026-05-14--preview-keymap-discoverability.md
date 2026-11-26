# Preview keymap discoverability — two small TUI polish gaps

Two related UX polish gaps surfaced while reviewing the session-scrollback-preview feature's keymap after a user could not figure out the bindings from the visible chrome.

**Gap 1: Space is missing from the Sessions-page help bar.** `sessionHelpKeys()` in `internal/tui/model.go:476-485` returns bindings for enter/r/k/p/n/q but does not include the Space-opens-preview binding. The binding itself is live and works (`model.go:1273` — `tea.KeySpace` flips `activePage` to `pagePreview`), it is simply not advertised in the help footer alongside the other Sessions-page keys. Result: a user reading the help bar has no signal that preview exists. Add an entry to the returned slice: `key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "preview"))`.

**Gap 2: The preview chrome bar is cryptic.** `chromeLine()` in `internal/tui/pagepreview.go:163-172` hardcodes the format `"Window %d of %d · Pane %d of %d · %s    ] [ Tab Esc"`. Two issues compound:

(a) `] [ Tab Esc` appear as bare key tokens with no action labels, unlike the bottom help bar's `enter attach · r rename · …` style. Users have no way to discover that `]`/`[` cycle windows, `Tab` cycles panes, and `Esc` dismisses — they look like incidental punctuation.

(b) The `%s` token is the tmux window name. tmux defaults window names to the basename of the running command, and Claude renames its `argv[0]` to its version string (e.g. `2.1.138`), so the chrome ends up displaying what looks like a version number with no context.

Annotate the key tokens (e.g. `] next win · [ prev win · tab next pane · esc back`) and either drop the windowName when it is a basename echo or prefix it (`win: %s`) so it is recognisable as a window label.

Both gaps are pure UX polish — no behavior change, no spec amendment required. Single file each, mechanical edits.
