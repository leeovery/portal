---
topic: built-in-session-resurrection
cycle: 5
total_proposed: 1
---
# Analysis Tasks: built-in-session-resurrection (Cycle 5)

## Task 1: Fix stale "step 7" comment in phase5_integration_test.go CleanStale assertion
status: approved
severity: low
sources: standards

**Problem**: `cmd/bootstrap/phase5_integration_test.go:125` carries the inline comment `// CleanStale step: marker MUST be cleared by step 6 before step 7 runs.`. Under the canonical nine-step bootstrap ordering, CleanStale is step 8 (step 7 is SweepOrphanFIFOs, wired as `NoOpFIFOSweeper` at line 99 of the same test). Cycle 4 corrected the same file's lines 39 and 74 from "step 7 (CleanStale)" to "step 8 (CleanStale)" but missed this third occurrence, leaving an internal inconsistency in a file that already documents the nine-step sequence.

**Solution**: Update the line-125 comment to match the wording used at lines 39 and 74 so all three CleanStale references in this file agree on the step number.

**Outcome**: All three CleanStale-related comments in `cmd/bootstrap/phase5_integration_test.go` consistently identify CleanStale as step 8. Assertion behaviour unchanged.

**Do**:
1. Open `cmd/bootstrap/phase5_integration_test.go`.
2. On line 125, change `// CleanStale step: marker MUST be cleared by step 6 before step 7 runs.` to `// CleanStale step: marker MUST be cleared by step 6 before step 8 (CleanStale) runs.` (mirroring lines 39 and 74).
3. Run `go build ./...` and `go test ./cmd/bootstrap/...` to confirm no regressions.

**Acceptance Criteria**:
- Line 125 references "step 8 (CleanStale)" rather than "step 7".
- All three CleanStale comments in the file (lines 39, 74, 125) use consistent step-number wording.
- `go test ./cmd/bootstrap/...` passes.
- No other lines modified.

**Tests**:
- Existing `cmd/bootstrap/phase5_integration_test.go` suite continues to pass — comment-only change, no new tests required.
