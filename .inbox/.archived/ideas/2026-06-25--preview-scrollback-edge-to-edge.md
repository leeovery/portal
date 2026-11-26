# Render preview scrollback edge-to-edge inside its container

In the new Modern Vivid design, the session preview ("peek mode") panel wraps the
captured scrollback body in the same per-row inset as the rest of the chrome — a
2-cell L/R inset on every compartment row. In practice this looks like too much
padding around the scrollback, because most of the sessions being previewed
already carry their own internal padding in how they render (Claude Code, for
example, indents its own output a little). Portal's inset then stacks on top of
that built-in padding, so the body reads as over-padded inside the preview
window.

The idea: render the scrollback body up to the container edges (no Portal-added
inset on the body), letting the previewed app's own rendering supply whatever
breathing room it has. The result should feel like looking straight at the pane
rather than at a doubly-padded inset of it.

Worth noting for whoever picks this up: the inset is `panelRowInset = 2` in
`internal/tui/panel.go`, applied uniformly to every compartment row by the shared
`renderJoinedPanel`. That same helper backs all the modals (help, kill, delete,
rename, edit) as well as the preview (`internal/tui/pagepreview.go`), so the
inset can't simply be globalised away without touching modal chrome — they should
keep their padding. The scope here is the preview body specifically.

A design question is embedded and should be settled visually against the
reference frame: should *only* the scrollback body go flush to the side borders
(header `◉ preview` marker and footer nav hints keeping their inset), or should
the whole preview panel go flush? The former keeps chrome alignment consistent
with the rest of the UI but creates a step between the inset header and the flush
body; the latter is simpler but changes the header/footer look too. Either way
the body viewport width and the `previewFrameOverhead = 6` (2 borders + 4 inset)
arithmetic in `pagepreview.go` change, and the preview frame geometry is asserted
across roughly twenty test files, so it's a real change rather than a one-line
constant tweak — which is why this is captured as an idea rather than a quick fix.
