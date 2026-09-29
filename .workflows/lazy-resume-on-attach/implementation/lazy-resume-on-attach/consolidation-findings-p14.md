# Consolidation Findings: lazy-resume-on-attach (Phase 14)

## Findings

None. The phase is one task (Tlazy-resume-on-attach-14-1, commit c22987df9) and the bank is empty. The swept surface has no cross-task duplication, near-miss helpers, drift, accretion or superseded code that clears the floor:
- The nil-Decision exec arm and `hydrateConfig.Decision` are gone. No production caller of the removed shape remains.
- The three tails each compose mark-then-exec from the one `parked` value. A tail that skipped the mark would fail `TestHydrateLazy_MarksPendingBeforeClearingMidRestoreMarker` across all three tails, so it would not go unnoticed.
- The handler-level "no settle sleep" timing checks are backed by the tail-level one through `runHydrate` (`cmd/state_hydrate_test.go:856-859`), so no guard was left vacuous.

## Spec Defects

### S1: §7.3 places the degraded tails' skeleton-marker clear inside their handlers
- **Claim**: §7.3, first consequence bullet (specification.md:328): "the tail it takes when the hydrate signal never arrives and the tail it takes when the saved scrollback file is missing both clear that marker inside their own handler and both fire the hook today."
- **Observed**: Neither handler touches either marker any more.
  - `handleHydrateTimeout` (`cmd/state_hydrate.go:305-317`) writes the preamble, removes the FIFO and logs.
  - `handleHydrateFileMissing` (`:321-334`) logs its per-cause WARN and the `scrollback missing` INFO.
  - The mid-restore clear now runs in `runHydrate`'s own tails, after the handler returns: the timeout tail at `:122`, `runFileMissingTail` at `:185`, and the replay tail at `:172`.
  - `TestHydrateHandlers_ReportWithoutTouchingEitherMarker` (`cmd/state_hydrate_lazy_test.go:566`) asserts that the handlers issue no tmux command.
  - `TestHydrateLazy_MarksAndParksWhateverTheHandlerSeamDoes` (`:454`) asserts that a tail marks and parks whatever the handler seam does.
- **Read**: Spec stale. The rule the sentence supports still holds on the landed tree: the mode is resolved once and the pending marker is written ahead of whichever clear runs, on every tail. Only the location clause is false. It sends a reader looking for the clear to the handlers, which are exactly the replaceable seams the phase took the clear out of. The fix is to drop "inside their own handler", or to state that each tail clears the marker after its handler has reported.
