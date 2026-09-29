# Analysis Report: lazy-resume-on-attach (Cycle 5)

## Stats

- Total findings: 3
- Deduplicated findings: 3
- Proposed tasks: 0

## Summary

The duplication and standards passes found nothing. They covered the phase-10 additions (the token match across the restore-to-wait hand-over, `CaptureAndRefile` reading the skeleton markers itself, and the hard link for a moved mid-restore pane) and found that each rule still has one owner and the tree conforms to the specification and its corrigenda. The architecture pass raised three structural findings, one medium and two low, all in the same vein: a seam that is correct only in its current narrow context. None becomes a proposal. The medium finding's silent half needs a future edit, and its other half already fails the per-tail lazy suite. The two low findings stand alone, and each needs a mutation path or a line reorder that the tree does not have. One of the low findings also reverses the settled phase-1 direction. No spec defects and no comment corrections were raised.

## Discarded Findings

- **The hydrate helper's lazy decision reaches the exec through a mutable pointer and a side effect each tail has to remember** (architecture, medium). Discarded: neither failure it names can happen in the tree as it stands, and the one that could follow an edit would not be silent. Re-verified against the current tree:
  - `runHydrate` sets `cfg.Decision` at `cmd/state_hydrate.go:112`, before every one of its exec sites (`:121`, `:151`, `:166`, `:181`).
  - Production wires the two replaceable tails to `handleHydrateTimeout` and `handleHydrateFileMissing` (`:424-425`). Both run `markPendingThenUnsetSkeletonMarker` (`:320`, `:340`), and so does the replay (`:177`). The seam replacements the finding points to exist only in tests, and those tests default an unnamed seam to a failing one.
  - A tail that reached the exec without marking fails `TestHydrateLazy_MarksPendingBeforeClearingMidRestoreMarker` (`cmd/state_hydrate_lazy_test.go`). That test drives all three tails through `lazyTails()`, with the production handlers wired in, and fatals on "no pending marker write". So the ordering rule is guarded, not left to discipline alone.
  - The nil-decision branch in `execShellOrHookAndExit` (`:243-244`) has no production caller. `TestHydrateLazy_NilDecisionFallsBackToItsOwnLookup` is the only thing that reaches it. Firing a lazy registration eagerly through that branch would first need a new caller that builds a config outside `runHydrate`.

  Cycles 1–3 discarded the same substance on the same evidence, and nothing in the tree has moved it since.
- **Registration keeps two sources of truth, and MarshalJSON trusts the stale one** (architecture, low). Discarded: it is low severity and does not cluster with any other finding. The failure needs a store mutation that edits a loaded registration in place and saves the snapshot without going through `Set`. No such path exists: `Set` is the only writer of a registration value (`internal/hooks/store.go:163-166`), and `Remove`, `Discard` and `CleanStale` only delete. The recommendation would also delete `Set`'s re-projection line, which is the settled phase-1 direction (the store re-projects what it stores before storing it). The finding offers no measured defect, spec rule or project rule as grounds for that reversal. Cycle 1 discarded the same finding.
- **The recover tail's pane binding cannot be seen from runResumeRecover, so the tests of the $TMUX_PANE fallback check a field nothing reads** (architecture, low). Discarded: it is low severity and does not cluster with any other finding. The gap is real: `executeResumeRecover` replaces the cobra body's `ReadMarker`/`ClearMarker` closures with the probe's, so no test observes the target those closures address. But the `$TMUX_PANE` tests do check the fallback's result. The variable they assert on is the same `pane` that `target := tmux.PaneIDTarget(pane)` is composed from one line below it (`cmd/state_resume_recover.go:76-81`). The failure needs a future reorder of those lines inside that one function body. Even then it is reached only under version skew, where a chain was parked by a build whose `--pane` the answering build no longer recognises, because the current chain always passes `--pane`. The failed clear on that route logs `unset resume pending marker failed` at WARN (`:53`) rather than passing silently. Changing the seam signatures builds on the settled cycle-1 fallback and does not reverse it, so the ledger does not bar it. The discard rests on severity and the lack of a cluster.
