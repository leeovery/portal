TASK: skip-bootstrap-when-warm-6-1 — Correct the stale "cleanup steps 9-11" step-range in the bootstrap_progress.go race-review invariant comment (tick-25c865)

ACCEPTANCE CRITERIA:
- cmd/bootstrap_progress.go:34 reads "cleanup steps 9-10" (no "9-11").
- No remaining reference to a step 11 or an "eleven-step" sequence exists anywhere in cmd/bootstrap_progress.go.
- The change is comment-only — no production code, signatures, or asserted behaviour is modified.
- go build ./... succeeds.
- No test change required (comment-only); go build + go test ./cmd/... as regression guard.

STATUS: Complete

SPEC CONTEXT:
This feature (skip-bootstrap-when-warm) reduced the bootstrap orchestrator from eleven steps to ten by removing the former step 11 (CleanStale). Confirmed against cmd/bootstrap/bootstrap.go: package doc now reads "ten-step" (line 1), totalSteps == 10 (line 59), and the cleanup tail is step 9 = CleanStaleMarkers and step 10 = SweepOrphanFIFOs (the final step) — verified via the StepName consts and the "Step 9"/"Step 10 (the final step)" interface docs (bootstrap.go:76-77, 183-204). The invariant-1 race-review block in bootstrap_progress.go documents the @portal-restoring suppression window: Set (step 3) before Sweep/Saver/Restore, Clear (step 8) before the cleanup steps. With the step-11 deletion, the cleanup tail is now steps 9-10.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/bootstrap_progress.go:33-34 — invariant 1 now reads "Clear step 8 BEFORE cleanup steps 9-10". HOLDS.
- Notes: Factually correct against the ten-step orchestrator. Cross-checked the rest of the file: package doc says "ten-step" (line 5), bufferSize comment "Ten real steps" (line 76), Step.Index doc "Index 1..10" (line 86), consumer-side "10->5 mapping" (line 247), and the fatal-step enumeration "(1, 2, 3, or 8)" (lines 108, 152) — all internally consistent with the corrected count. grep -iE "9-11|eleven|11" over cmd/bootstrap_progress.go returns NO MATCHES, satisfying acceptance criterion 2 (this was the last stale occurrence). Change is comment-only; no import, signature, struct, or control-flow change is present, and the surrounding Go is well-formed and unchanged.

TESTS:
- Status: Adequate (no test appropriate)
- Coverage: A comment-only edit has no observable behaviour to assert; a behavioural test cannot fail on a comment change. No test is warranted and none was added — correct call, neither under- nor over-tested. The acceptance criteria's regression guard (go build + go test ./cmd/...) is a compile/green sanity check, not a new test.
- Notes: Test execution is out of this verifier's scope. Judged by reading: the file is a valid Go source with a single comment string altered, so compilation is unaffected.

CODE QUALITY:
- Project conventions: Followed (golang-documentation) — comment stays a full-sentence, intent-revealing invariant explanation; the edit preserves the surrounding prose verbatim and touches only the stale numeric range.
- SOLID principles: N/A (comment-only)
- Complexity: N/A (comment-only)
- Modern idioms: N/A
- Readability: Good — the corrected range removes a dangling reference to deleted code, so a maintainer reasoning about the clear-before-cleanup ordering is no longer sent to a nonexistent step 11.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (Out-of-scope observation, not a finding for this task: the project CLAUDE.md still narrates an "eleven-step" orchestrator with steps 1-11; that doc lives outside cmd/bootstrap_progress.go and outside this task's boundary. Flagging here only for the plan-completion pass; no action assigned to task 6-1.)
