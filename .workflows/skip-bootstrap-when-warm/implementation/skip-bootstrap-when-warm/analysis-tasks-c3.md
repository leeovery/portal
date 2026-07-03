---
topic: skip-bootstrap-when-warm
cycle: 3
total_proposed: 1
---
# Analysis Tasks: Skip Bootstrap When Warm (Cycle 3)

## Task 1: Correct the stale "cleanup steps 9-11" step-range in the bootstrap_progress.go race-review invariant comment
status: approved
severity: low
sources: standards

**Problem**: This feature reduced the bootstrap orchestrator from eleven steps to ten by removing step 11 (CleanStale). The 11-to-10 relabelling was applied thoroughly across the codebase — `bootstrap.go`'s package doc + `totalSteps`, `loading_progress.go`'s `stepLabelTable`/`totalBootstrapSteps`, `model.go`'s "1..10", `adapters.go`'s step-10-final wording, and three other comments in `cmd/bootstrap_progress.go` itself ("eleven-step"→"ten-step", "Eleven real steps"→"Ten real steps", "Index 1..11"→"1..10"). But one comment was missed: the invariant-1 race-review block comment at `cmd/bootstrap_progress.go:34` still reads "Clear step 8 BEFORE cleanup steps 9-11". The orchestrator now has only cleanup steps 9-10, so this comment points a maintainer at a step 11 that this very feature deleted — a factually wrong reference to removed code inside a load-bearing ordering-invariant explanation, contradicting the corrected step count documented everywhere else in the same file.

**Solution**: Change "cleanup steps 9-11" to "cleanup steps 9-10" at `cmd/bootstrap_progress.go:34`. Comment-only edit; no behaviour, control flow, or signature changes.

**Outcome**: The invariant-1 race-review comment matches the ten-step orchestrator; no reference to the removed step 11 (or an "eleven-step" sequence) remains anywhere in `cmd/bootstrap_progress.go`, and a maintainer reasoning about the clear-restoring-before-cleanup ordering is no longer sent looking for a nonexistent step.

**Do**:
- Open `cmd/bootstrap_progress.go` and locate the invariant-1 race-review block comment near line 34 (the "Clear step 8 BEFORE cleanup steps 9-11" line).
- Replace "cleanup steps 9-11" with "cleanup steps 9-10". Preserve the rest of the comment verbatim.
- Grep `cmd/bootstrap_progress.go` for any residual "9-11", "eleven", or standalone step-"11" reference to confirm this was the last stale occurrence (the standards finding already inventoried the other references in this file as corrected).

**Acceptance Criteria**:
- `cmd/bootstrap_progress.go:34` reads "cleanup steps 9-10" (no "9-11").
- No remaining reference to a step 11 or an "eleven-step" sequence exists anywhere in `cmd/bootstrap_progress.go`.
- The change is comment-only — no production code, signatures, or asserted behaviour is modified.
- `go build ./...` succeeds.

**Tests**:
- No test change is required (comment-only edit). Run `go build ./...` and `go test ./cmd/...` as a regression guard to confirm the edit is inert and the package still compiles and passes green.
