# Standards Findings — killed-sessions-resurrect-on-restart (cycle 3)

```
AGENT: standards
FINDINGS:
- FINDING: EagerSignalCore doc-comment names a deleted private fake (recordingFIFOSignaler)
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate.go:28
  DESCRIPTION: The doc-comment on `EagerSignalCore.Signaler` (line 28) closes with "Tests inject recordingFIFOSignaler." After cycle-2 Task 1 (T5-1) landed, the per-package private fake `recordingFIFOSignaler` in `cmd/bootstrap/eager_signal_hydrate_test.go` was deleted and consumers were migrated to the shared `statetest.RecordingFIFOSignaler` (`internal/statetest/fifo_signaler_recorder.go`). The line-28 sentence is the only remaining production-code reference to the now-deleted private name — `grep -rn "recordingFIFOSignaler" cmd internal` returns this single hit outside the cycle-2 analysis docs. Mirrors the same class of doc-drift the cycle-2 finding flagged for `CLAUDE.md` (live behaviour described inaccurately in a load-bearing reference); cosmetic, does not affect runtime, but the file's package doc-comment is the canonical reader-facing description of the seam and the drift directly contradicts the post-Task-1 promotion that just landed in cycle 2.
  RECOMMENDATION: Edit `cmd/bootstrap/eager_signal_hydrate.go:28` to replace `Tests inject recordingFIFOSignaler.` with `Tests inject statetest.RecordingFIFOSignaler.` — keeps the doc-comment in lock-step with the post-T5-1 shared fake at `internal/statetest/fifo_signaler_recorder.go`. One-line cosmetic edit, no behavioural change.

SUMMARY: Cycle 2's four cleanup tasks (T5-1 statetest.RecordingFIFOSignaler promotion, T5-2 redundant EagerSignaler wiring drop, T5-3 CLAUDE.md primitive rename, T5-4 restoretest package-doc reconcile) all landed cleanly — every cycle-2 standards / spec drift surface is closed. One residual sub-line standards drift remains: the doc-comment at `cmd/bootstrap/eager_signal_hydrate.go:28` still names the deleted private `recordingFIFOSignaler` fake. All implementation surfaces remain in conformance with the spec's Fix 1 / Fix 2 / Fix 3 requirements and the eight acceptance criteria.
```
