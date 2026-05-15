# Visual distinction for quick preview system

When the quick preview opens (Space on a session in the TUI), it visually looks indistinguishable from a fully-attached session. At a glance there is no signal that this is a read-only preview rather than the real thing — the scrollback fills the screen and looks identical to the attached state. That ambiguity is a problem: users need to be able to tell instantly that they are in a transient preview mode and not actually inside the session.

Two possible directions for fixing this:

1. **Dim the preview content slightly** — render the scrollback text at a reduced contrast / lower opacity so it reads as inactive or "not-quite-live." Cheap, minimal layout change, communicates "this isn't the real thing" without taking up screen real estate.

2. **Mount the preview inside a bordered chrome** — wrap the preview content in a visible frame, with space at the top/bottom/edges for affordances like a title, the session name, and a footer showing the available controls (Enter to attach, Esc/Space to dismiss, etc). More explicit and discoverable, gives a natural place to surface the keybindings, but eats screen space.

Could also be a combination — e.g. a subtle border + slightly dimmed text — though the goal is just "obviously a preview" rather than maximally decorated.

Relevant area: `internal/tui` preview page (`pagePreview` arm), which renders via the `previewModel` injected with `TmuxEnumerator` + `ScrollbackReader` seams. The dimming approach would likely live in the lipgloss styling layer; the chrome approach would mean wrapping the content area inside an outer layout with header/footer regions.

Worth thinking about alongside the broader discoverability question for the preview feature — if we go the chrome route, the footer naturally doubles as the keybinding hint surface.
