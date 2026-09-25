# Session preview shows too little scrollback to scroll

When previewing a session from the Sessions list (pressing `Space` to open the read-only scrollback preview), the amount of history available is often far smaller than expected. In some cases there is no scrollable history at all — the preview fills the view and there is nothing further to scroll back through. In other cases scrolling does work, but the available history runs out after roughly two pages.

A concrete example is the session `dex-4R8PIq`, which is a long-running Claude session. A session like that accumulates a large amount of terminal output over its lifetime, so the expectation when previewing it is to be able to scroll back a substantial distance through that history. Instead the preview either offers no scrolling or stops after about two screens' worth.

The behaviour is inconsistent across sessions and across attempts — "sometimes" there is nothing to scroll, "sometimes" there are ~2 pages. It has not been pinned down which sessions or conditions produce which outcome.

The user's recollection is that the preview was built to show more history than this, so the current depth feels like a regression from, or a shortfall against, what was intended. Their own suggestion is that the amount of history the preview pulls in may simply need to be increased — recorded here as the reporter's hypothesis, not a confirmed cause.

Impact: the preview is the quick-look route for checking what's happening in a session without attaching to it. For long-lived sessions — Claude sessions in particular, where the useful context is often well above the last screen — a preview capped at zero to two pages of history defeats much of its purpose, and the user has to attach to the session to see anything meaningful.

Relevant area: the TUI's `pagePreview` scrollback preview in `internal/tui`, which reads saved scrollback through the `state.TailScrollback` helper.

A separate bug covers scroll input leaking into the session list when the preview is dismissed (`preview-scroll-leaks-into-session-list`); it was observed in the same preview interaction but is logged on its own.
