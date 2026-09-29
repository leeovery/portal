# Analysis Report: lazy-resume-on-attach (Cycle 8)

## Stats

- Total findings: 2
- Deduplicated findings: 2
- Proposed tasks: 1

## Summary

The duplication and standards passes found nothing that clears the floor. `golangci-lint run ./...` reports 0 issues, and every rule more than one task relies on still has one declaration. The architecture pass raised two seams that depend on caller discipline. The first is the hydrate helper's resume decision: a mutable pointer threaded through by-value handler seams, where only the mark step makes it safe to park. It is staged as a medium proposal. The second is the capture cycle's dump-skip predicate. It is low-severity, stands alone and has one production declaration, so it is discarded. Standards collected two doc-comment corrections on the resume panel's exported renderers.

## Comment Corrections

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

## Discarded Findings
- The freeze's scrollback skip is a contract every CommitCycle.Dump must reassemble (architecture, low) — discarded for low severity. It is a single finding and forms no cluster. The production rule already has one declaration: `paneSkipsScrollback` (`cmd/state_daemon.go:343`), which the tick and the shutdown flush both reach through the one daemon dump. The only other restatement is in the lazy-panel integration fixture, and a test file's reuse falls outside the floor. The failure needs a production dumper that does not exist: `commit-now` dumps nothing, and every committer outside `internal/state` is fenced into `RunCommitCycle`. The proposal would also move part of the skip into `internal/state`. Phase 4 and cycle 4 deliberately left the dump and its skip with the caller.
