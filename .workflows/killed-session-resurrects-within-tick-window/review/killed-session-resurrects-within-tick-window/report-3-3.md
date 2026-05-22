TASK: Extract runPortalSubprocess helper to consolidate runPortalCommitNow and runPortalList (killed-session-resurrects-within-tick-window-3-3)

ACCEPTANCE CRITERIA:
- `runPortalSubprocess` helper exists, centralising env wiring + subprocess exec.
- `runPortalCommitNow` and `runPortalList` remain as thin trampolines retaining their names.
- `t.Helper()` propagation preserved.
- Byte-equivalent failure-message format.

STATUS: Complete

SPEC CONTEXT: Phase 3 (Analysis Cycle 2) cleanup. The symptom integration test (task 1-6) originally had two near-identical subprocess wrappers differing only by argv. Pure refactor — no behavioural surface change.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/state_commit_now_symptom_integration_test.go:448-501`
  - `runPortalSubprocess` at L448-462: takes `args ...string`, builds `exec.Command(binary, args...)`, wires `TMUX`/`PORTAL_STATE_DIR`/`PATH`, calls `t.Helper()`, fatals on non-zero exit with diagnostic.
  - `runPortalCommitNow` at L477-480: trampoline calling `runPortalSubprocess(t, binary, f, "state", "commit-now")`. `t.Helper()` retained.
  - `runPortalList` at L498-501: trampoline calling `runPortalSubprocess(t, binary, f, "list")`. `t.Helper()` retained.
- Notes:
  1. `t.Helper()` called in the helper AND in both trampolines — failure line numbers correctly attribute to caller of trampoline.
  2. Failure message format `"portal %s subprocess failed: %v\n--- output ---\n%s\n%s"` with `strings.Join(args, " ")` yields byte-equivalent strings ("portal state commit-now subprocess failed: ...", "portal list subprocess failed: ...").
  3. Trampoline names retained — all 4 callsites (L158, L387, L401) compile unchanged.

TESTS:
- Status: Adequate (no new tests required; pure refactor)
- Coverage: Helper exercised transitively by `TestCommitNowSymptom`'s three sub-tests; each invokes both trampolines.
- Notes: Failure path (non-zero exit) implicitly covered by diagnostic `t.Fatalf` — explicit coverage would require injecting a broken binary, out of scope for refactor.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Single responsibility per helper.
- Complexity: Low. Helper ~14 lines; trampolines 4 lines each.
- Modern idioms: Variadic `args ...string` plus `exec.Command(binary, args...)` is idiomatic.
- Readability: Good. Doc comments explain intent.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] `runPortalSubprocess` swallows stdout/stderr unless command fails. Optional `-v` log under `testing.Verbose()` would aid future triage. Same behaviour as pre-refactor — no regression.
- [quickfix] Doc comment on `runPortalSubprocess` (L440-447) says "A non-zero exit is treated as a fixture failure" — but helper is also called from inside sub-test bodies (L158) where failure is an assertion-level concern. Tweak to "test failure" for precision.
