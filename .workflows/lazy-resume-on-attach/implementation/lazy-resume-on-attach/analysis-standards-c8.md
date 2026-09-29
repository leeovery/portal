AGENT: standards
FINDINGS: none
COMMENT_CORRECTIONS:
- internal/tui/resume_panel.go:29 — the exported contract says the output is always exactly Width by Height, but renderPaneScreen swaps a non-positive dimension for the fallback terminal size, which is what runResumeDraw relies on when the pane size cannot be read
  OLD: // RenderResumePanel draws the waiting panel as a string of exactly Width by
// Height cells, as the card when the pane holds it and as the plain stack below
// that size.
  NEW: // RenderResumePanel draws the waiting panel as a string of exactly Width by
// Height cells, or a fallback size where either is not positive, as the card
// when the pane holds it and as the plain stack below that size.
- internal/tui/resume_discard_confirm.go:13 — the same exact-size claim, false for the same non-positive input
  OLD: // RenderResumeDiscardConfirm draws the discard confirmation as a string of
// exactly Width by Height cells. It is the picker's kill modal retitled, built
// through the same destructive-confirm builder, and it degrades with the pane
// exactly as the waiting panel does.
  NEW: // RenderResumeDiscardConfirm draws the discard confirmation as a string of
// exactly Width by Height cells, or a fallback size where either is not
// positive. It is the picker's kill modal retitled, built through the same
// destructive-confirm builder, and it degrades with the pane exactly as the
// waiting panel does.
SUMMARY: This was a full pass over the implementation set against the whole specification and its corrigenda, including phase 13's unexporting of captureAndRefile and refilePendingScrollback and its new commit guard. Everything checked still conforms, and `golangci-lint run ./...` reports 0 issues. Checked: both stored shapes with verbatim preservation; `hook set --resume-mode` and its refusals; the fifth `hook list` column; the two doctor lines that never drive the exit code; the tolerant `resume_mode` default; mode resolution and pending-marking ahead of every hydrate tail; the draw/wait/recover chain with probe-then-drop, trap and backstop; answer ordering and holding an answer when the clear fails; `op=discard` with `via=panel`; the token-matched freeze, link and re-file under the commit lock; the token-resolved preview; both screens' verbatim strings and degraded stacks; and the packed `A`/`P` indicators with their help legend. Four candidates earlier cycles judged below the floor still name no new failure: the set-noop keeping unmodelled attributes, the preview's empty-versus-absent fallback, the `via` tag on the Enter-time lookup, and the per-draw theme event volume. Two more were weighed and not raised. A wrong-typed sibling key in prefs.json zeroes `resume_mode`, but every other prefs field already behaves that way, which is what "like every other field there" asks for. The pending count doubles a waiting pane only when grouped sessions show its window twice, and that line is informational. Only two exported doc comments need correcting.
