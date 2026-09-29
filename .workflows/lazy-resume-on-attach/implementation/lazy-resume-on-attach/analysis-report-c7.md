# Analysis Report: Lazy Resume On Attach (Cycle 7)

## Stats

- Total findings: 1
- Deduplicated findings: 1
- Proposed tasks: 1

## Summary
The duplication and standards passes found nothing above the floor. Standards confirmed that the phase-12 commit lock and the tick consuming `save.requested` conform to the specification, with `golangci-lint` clean. Architecture found one structural gap: `RunCommitCycle` holds `commit.lock` correctly, but the unlocked primitives it composes (`Commit`, `CaptureAndRefile`, `RefilePendingScrollback`) are still exported with no production caller outside `internal/state`. The rule protecting a waiting pane's transcript therefore rests on one CLAUDE.md sentence rather than on the API. It is staged as one proposal. That proposal settles the finding's fork for `Commit` on a source guard, because unexporting `Commit` would strand the commit-now fixture's stand-in cycle, which commits through the real `Commit`.

No settled direction is reversed. Phase 4, cycle 4 and cycle 6 each settled that the capture cycle has one entry point a caller cannot commit around. Phase 12 wrote that rule into CLAUDE.md as prose. This proposal completes those directions in code.

## Discarded Findings
- None. The duplication and standards passes filed no findings, and the candidates they weighed and left below the floor were not written as findings.
