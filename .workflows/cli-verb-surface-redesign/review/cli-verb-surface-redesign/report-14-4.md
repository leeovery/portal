TASK: cli-verb-surface-redesign-14-4 (chore) — Add an explicit sentinel at iota 0 for the doctor checkStatus enum

ACCEPTANCE CRITERIA:
- checkUnknown is the iota-0 value; checkPass and the other three statuses are shifted up.
- A zero-value checkResult{} does not render as a pass marker and does not contribute to a healthy (doctorUnhealthy == false) result.
- The four real statuses render and drive the exit code exactly as before.
- go build, go test ./..., and golangci-lint run are clean.

STATUS: Complete

SPEC CONTEXT:
Defensive/convention fix aligning the doctor health-diagnostic enum with the golang-naming skill's "explicit Unknown/Invalid sentinel at iota 0" rule (SKILL.md line 108, 139). For a diagnostic the zero value is the most dangerous default: a zero-value checkResult{} would silently classify as pass, and doctorUnhealthy (which drives the scriptable exit code — ErrDoctorUnhealthy, cmd/doctor.go:20-25) counts only checkFail, so a forgotten status assignment could mask a failure as green. No active bug — every production checkResult carries an explicit status.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/doctor.go:44-67 (const block), :663-676 (checkMarker), :678-691 (doctorUnhealthy)
- Notes:
  - checkUnknown is declared at iota 0 (line 55) with checkPass/checkFail/checkInfo/checkNotEvaluable shifted up one (lines 57/59/62/66). Verified via grep that these unexported constants are referenced ONLY symbolically and only within doctor.go / doctor_test.go — no file depends on the numeric ordinal, so the shift is safe.
  - checkMarker (line 663) has no case for checkUnknown; it falls through to the `default: return " "` arm — a blank, NOT the "✓" pass glyph. Acceptance ("does not render as a pass marker") satisfied.
  - doctorUnhealthy (line 684) was updated to count checkUnknown as unhealthy: `if r.status == checkFail || r.status == checkUnknown`. A zero-value result therefore actively fails the run rather than silently reading green. This is stronger than the plan's minimum ("does not contribute to a healthy result") — it makes a forgotten assignment loud, which is the correct choice for an exit-code contract.
  - The four real statuses are untouched in both switch/loop: checkPass→"✓", checkFail→"✗", checkNotEvaluable→"·", checkInfo→" ", and only checkFail (plus the new sentinel) drives the exit code — exactly as before for real results.
  - Doc comments (lines 48-54, 679-683) accurately describe the sentinel's defensive role and note no production path constructs it.

TESTS:
- Status: Adequate
- Coverage: cmd/doctor_test.go:202-228 TestDoctorZeroValueCheckResultNotHealthy pins all four guarantees on the zero value: status != checkPass, status == checkUnknown, checkUnknown == 0 (iota-0 anchor), checkMarker(zero) != "✓", and doctorUnhealthy([zero]) == true. Existing exit-code/marker tests (lines ~527, 664, 723, 816, 841) reference statuses symbolically and continue to assert the four real statuses' behaviour, so they remain valid with the shifted values.
- Notes:
  - The test would fail if the sentinel regressed (e.g. checkPass moved back to iota 0, or checkMarker/doctorUnhealthy stopped handling the zero value) — a genuine regression tripwire, not a tautology.
  - Slight overlap between `status == checkPass` and `status != checkUnknown` assertions, but they encode distinct guarantees (never-pass vs specifically-unknown) — not over-tested.

CODE QUALITY:
- Project conventions: Followed. Matches golang-naming's iota-0 Unknown-sentinel rule exactly (the very convention this task exists to satisfy).
- SOLID principles: Good — single-responsibility helpers unchanged; the sentinel is additive.
- Complexity: Low — one added const, one added `||` clause, one relied-upon default arm.
- Modern idioms: Yes — idiomatic Go enum with explicit zero-value sentinel.
- Readability: Good — thorough intent-revealing comments on the sentinel and on doctorUnhealthy explain WHY the zero value counts as unhealthy.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (I could not execute `go build` / `go test` / `golangci-lint` per the no-execution rule, but the change is a pure symbolic enum shift with all references confirmed symbolic, so a build/lint break is not credible.)
