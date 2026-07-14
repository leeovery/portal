TASK: restore-host-terminal-windows-8-5 — Extract the shared burst test-model construction prefix helper

ACCEPTANCE CRITERIA:
- The sessions-builder loop and the wire→resolve→enter→mark-all prefix exist once as shared helpers; the four constructors no longer duplicate them.
- Each constructor's distinct tail is preserved and its tests behave identically to today.
- The `Windows`-index convention (`Windows: i + 1`) is defined in exactly one place.
- Regression: all four burst suites (input-lock, cancel, self-attach, partial-failure) pass unchanged.
- The shared helpers are exercised by all four constructors (no dead helper).

STATUS: Complete

SPEC CONTEXT: This is a Phase 8 analysis-cycle (duplication-source) TEST-code refactor, not a spec-behaviour task. The four burst test-model constructors — `burstPendingModel` (input-lock), `realCancellableBurst` (cancel), `setupConfirmingBurst` (self-attach), `newPendingBurstModel` (partial-failure) — each repeated the same ~10-line setup (build `[]tmux.Session` with `Windows: i + 1`, FakeAck/FakeAdapter, NewModelWithSessions, wireBurstSeams, resolveDetection, enter multi-select, mark-all). The named risk is the `Windows`-index convention drifting across the four builders as the burst suite grows. The sibling-shared-helper convention (`wireBurstSeams`, `resolveDetection`, `markRow`, `allPresent`, `ghosttyIdentity`, `driveBurstToTerminal`) is already established in these files.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/burst_dispatch_test.go:93-99` — new `sessionsFromNames(names []string) []tmux.Session`, the single home of the `Windows: i + 1` convention (line 96).
  - `internal/tui/burst_dispatch_test.go:109-121` — new `markedSupportedBurstModel(t, names) (Model, *FakeAdapter, *FakeAckChannel)`, the shared confirm-all wire→resolve→enter→mark-all prefix.
  - `internal/tui/burst_input_lock_test.go:36-44` — `burstPendingModel` now calls `markedSupportedBurstModel`, keeps only its tail (termWidth/Height, `Select(0)`, `burstPending=true`).
  - `internal/tui/burst_selfattach_test.go:37-44` — `setupConfirmingBurst` now calls `markedSupportedBurstModel`, keeps only its precondition-check tail.
  - `internal/tui/burst_cancel_test.go:96-115` — `realCancellableBurst` reuses `sessionsFromNames` (its custom all-false `Confirm` adapter means it correctly keeps its own wire/resolve/enter/mark path, as the helper's doc-comment explicitly permits); removed now-unused `tmux` import.
  - `internal/tui/burst_partial_failure_test.go:50-66` — `newPendingBurstModel` reuses `sessionsFromNames`; keeps its own mark-all + direct burst-pending state build (no goroutine).
- Notes: Confirmed against `git show bfcd3085` — the extracted `markedSupportedBurstModel` body is byte-for-byte the prefix that `burstPendingModel`/`setupConfirmingBurst` previously inlined (same ack/adapter build, same `wireBurstSeams(…, ResolutionNative, allPresent, ack)`, same `resolveDetection(…, ghosttyIdentity())`, same mark-all). Term dimensions are correctly preserved: the helper does not set them, and `burstPendingModel` still sets 80×24 in its tail exactly as before; `setupConfirmingBurst` never set them (unchanged). This is a pure mechanical dedup — no logic touched. `Windows: i + 1` now grep-verifies to exactly one occurrence (`burst_dispatch_test.go:96`); the remaining literal `Windows: 1/2/3` slices are pre-existing per-test session fixtures outside the four constructors and out of scope.

TESTS:
- Status: Adequate (this task IS test code; the "tests" are the four regression suites the refactored constructors feed).
- Coverage: `sessionsFromNames` is exercised by `realCancellableBurst`, `newPendingBurstModel`, and (transitively) `markedSupportedBurstModel`; `markedSupportedBurstModel` is exercised by `burstPendingModel`, `setupConfirmingBurst`, and `burst_dispatch_test.go:490`. No dead helper — both are live and reachable. All four burst suites still construct their models through these helpers, so a break in either helper fails multiple suites loudly.
- Notes: Behaviour-preservation is verifiable by reading the diff (mechanical extraction, no assertion or state change). Import hygiene is consistent: `burst_cancel_test.go` correctly drops the now-unused `tmux` import; the other three retain `tmux` because they still hold explicit `tmux.Session` fixtures in test bodies. No unused-import or dead-code hazard.

CODE QUALITY:
- Project conventions: Followed. Matches the golang-testing sibling-shared-helper pattern already in this package (`t.Helper()` on both helpers, table-free constructor style, white-box `package tui`, no `t.Parallel`).
- SOLID principles: Good — single-responsibility split (`sessionsFromNames` = fixture builder; `markedSupportedBurstModel` = full prefix), and the helper doc-comment cleanly documents the seam between the two so a custom-adapter constructor reuses only the builder.
- Complexity: Low. Both helpers are trivial loops/wiring.
- Modern idioms: Yes — `for i, n := range names` and `for i := range names`.
- Readability: Good. The `markedSupportedBurstModel` doc-comment is unusually clear about why two constructors reuse only `sessionsFromNames`.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/burst_dispatch_test.go:116-119, burst_cancel_test.go:105-109, burst_partial_failure_test.go:53-56 — the enter-multi-select + mark-all loop (`pressSession(t, m, pressM)` then `for i := range names { m = markRow(t, m, i) }`) still lives in three places because the two custom-adapter constructors can't call `markedSupportedBurstModel`. Could extract an `enterAndMarkAll(t, m, names) Model` helper those three share. Low value / genuinely optional: the loop carries no magic constant (unlike `Windows: i + 1`, which was the actual drift risk and is now deduped), so residual drift risk is near-zero; the task's primary goal is met.
- [do-now] internal/tui/burst_selfattach_test.go:20-21 and internal/tui/burst_cancel_test.go:30-31 — the file-header "helpers live in the sibling files" prose lists seam helpers but not the newly-consumed `markedSupportedBurstModel` / `sessionsFromNames`; add them for orientation completeness (pure comment edit, no logic).
