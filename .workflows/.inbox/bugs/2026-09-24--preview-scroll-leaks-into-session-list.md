# Scrolling in the preview leaks into the session list on dismiss

After scrolling inside the session preview (opened with `Space` from the Sessions list), dismissing the preview with `Space` or `Esc` sometimes causes the cursor bar on the session list to race across the rows, as though the user had scrolled the list rapidly. The user did not scroll the list — the movement happens on its own immediately after returning from the preview.

It feels as though the scroll input given while in the preview builds up and is then released all at once into the session list at the moment the preview is dismissed. That is the reporter's description of how it presents, not a confirmed mechanism.

It happens whether or not scrolling in the preview actually worked. Even in cases where the preview had no scrollable history (see the related bug `preview-scrollback-too-short`), scroll attempts made in the preview still appear to carry over and move the session-list cursor after dismissal. It is intermittent — "sometimes" rather than on every dismiss — and it occurs with both dismiss keys, `Space` and `Esc`.

Impact: the user lands back on the session list with the cursor on a different session from the one they previewed, possibly several rows away or on another page. Since the natural next action after a quick look is often to press `Enter` to attach, this risks attaching to the wrong session, and at minimum forces the user to find their place again. It also makes the preview feel unreliable to use with a trackpad or mouse wheel.

Relevant area: the TUI's `pagePreview` → `pageSessions` transition in `internal/tui` (the preview dismiss handler, which also triggers a sessions-list refresh on the way back), and the Sessions list's cursor navigation.

Logged alongside `preview-scrollback-too-short`, which was observed in the same preview interaction.
