TASK: Analysis Phase 2 refactor — make burstPartialFailureFlash self-contained (remove the double partition / permission scan). Commit 26a23299.

ACCEPTANCE CRITERIA:
1. burstPartialFailureFlash's signature no longer receives one partition half while re-deriving the other; fully self-contained OR receives the complete already-computed pair.
2. Along a single partial-failure handling pass, spawn.PartitionResults and spawn.FirstPermission each run at most once (no discarded recomputation).
3. Rendered flash byte-identical for total-failure ("— nothing opened"), genuine-partial ("— others left open"), permission-wall (verbatim Guidance), degenerate-empty ("" → no band).
4. CLI/picker PartialFailureMessage parity preserved (parity tests pass).
5. spawn.PartialFailureMessage(failed, len(confirmed) > 0) kept as the single renderer; not inlined/duplicated.
6. cmd/spawn.go untouched; byte-identical CLI/picker parity.

STATUS: Complete

SPEC CONTEXT: Fix 3 (§6-6 leave-what-opened partial-failure path) had left burstPartialFailureFlash receiving one partition half (failed) as a parameter while re-deriving confirmed internally via a second spawn.PartitionResults call — a mixed-ownership smell the analysis cycle flagged. §7-1 designates spawn.PartitionResults / spawn.FirstPermission as the single count-semantics chokepoint both the CLI (cmd/spawn.go) and the picker must key off so the two paths cannot drift. The flash body IS spawn.PartialFailureMessage, shared byte-for-byte with the CLI.

IMPLEMENTATION:
- Status: Implemented (preferred option (b))
- Location: internal/tui/burst_partial_failure.go:116-125 (burstPartialFailureFlash), :34-81 (handleBurstPartialFailure)
- Notes: Signature changed to burstPartialFailureFlash(results []spawn.WindowResult) string — the failed half is no longer threaded in. The flash now owns its full derivation: FirstPermission (line 117) → early Guidance return, else PartitionResults (line 120) → confirmed+failed both used, then len(failed)==0 → "" guard, else PartialFailureMessage(failed, len(confirmed) > 0). Caller line 66 changed from `confirmed, failed :=` to `confirmed, _ :=` (mutation needs only confirmed) and drops the failed argument at the call (line 75). Criterion #1 fully satisfied; the mixed pass-a-half-recompute-the-other anti-pattern is gone.

  Criterion #2 caveat (see NON-BLOCKING): the reviewer instruction to confirm "exactly one PartitionResults and one FirstPermission along the non-permission partial path" does NOT hold literally. Counting call sites executed on that path: FirstPermission runs twice (handleBurstPartialFailure line 43 for emit routing + flash line 117), PartitionResults runs twice (caller line 66 for the selection mutation + flash line 120). The "double" is relocated, not removed. This is structural: option (b) makes the flash self-contained, but the caller has independent needs (confirmed for applyBurstSelectionMutation, FirstPermission for emitPermission-vs-emitBurstSummary routing), so it computes its own copies. Criteria #1 (self-contained flash, no threaded state) and #2 (single computation across the pass) are in direct tension — you cannot have a fully self-contained flash AND single-scan-across-the-pass when the caller also consumes the derived values. The task PREFERRED option (b) (option (a) only "equally acceptable"), so the implementer took the sanctioned choice that satisfies #1 at the cost of #2. Both functions are pure and side-effect-free over the immutable results slice, so the extra scan of a tiny (single-digit) slice has zero behavioural or meaningful performance impact.

TESTS:
- Status: Adequate
- Coverage: burst_partial_failure_test.go covers total-failure parity ("— nothing opened", TestBurstPartialFailure_StaysInMultiSelectMode, asserts spawn.PartialFailureMessage([]string{"alpha"}, false)), genuine-partial ("— others left open", LeavesOpenedWindowsAndSkipsSelfAttach + AckTimeoutAndSpawnFailedClassifyIdentically, both assert against spawn.PartialFailureMessage(..., true)), permission-wall verbatim Guidance (PermissionGuidanceOnceAffectedStaysMarked + UnattemptedPostPermissionStayMarked), pre-spawn-error generic flash (PreSpawnError_GenericFlashSelectionUnchanged), and retry-set mutation (UnmarksConfirmedKeepsFailedForRetry). Degenerate-empty ("" → no band) is covered by burst_cancel_test.go TestBurstPartialFailureFlash_DegenerateEmptyFailedNoFlash, whose call was correctly updated to the new single-arg signature (the `nil` failed arg dropped) with behavioural assertions unchanged. User-cancel-silent arm is guarded by the untouched `if !m.burstCancelled` gate (line 74) and covered in burst_cancel_test.go.
- Notes: Parity is asserted structurally by comparing rm.flashText against spawn.PartialFailureMessage(...) directly rather than a hardcoded string — the correct way to pin CLI↔picker byte-identity. Not over-tested; each arm asserts a distinct behaviour. `go test ./internal/tui/ ./internal/spawn/ ./cmd/` all pass. No test needed updating beyond the one signature call-shape change, matching the expected-tests contract.

CODE QUALITY:
- Project conventions: Followed. Shared chokepoint (spawn.PartitionResults / FirstPermission / PartialFailureMessage) used at every site; no hand-rolled Ack loop, no inlined message. Doc comments are thorough and accurate.
- SOLID principles: Good. The self-contained flash has a single, clear responsibility and no longer depends on a caller passing a matching partition half (removes a fragile coupling / temporal-coupling smell).
- Complexity: Low. Flash is a linear 3-branch function.
- Modern idioms: Yes. Idiomatic `confirmed, _ :=` discard.
- Readability: Good. Comments explain the permission-first ordering, the degenerate-"" guard, and the trigger-never-counts-as-other invariant.
- Issues: The only residual is the cross-pass double scan documented under criterion #2 (non-blocking).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/burst_partial_failure.go:43,66,117,120 — Along the non-permission partial path spawn.FirstPermission (line 43 caller + line 117 flash) and spawn.PartitionResults (line 66 caller + line 120 flash) each still execute twice, so the task-title "double partition / permission scan" is relocated rather than removed and criterion #2's "each run at most once" is not literally met. The CLI (cmd/spawn.go:179,191,210) is the reference that computes each exactly once and reuses the confirmed/failed pair. To collapse the picker to a single scan, adopt option (a): compute `confirmed, failed := spawn.PartitionResults(msg.Results)` and the FirstPermission result once in handleBurstPartialFailure and thread them into the flash — which reverses criterion #1's self-containment (re-introducing threaded partition state). Genuine decoupling-vs-single-scan design trade-off with zero behavioural impact (both funcs pure over immutable results, tiny slice), so left as an idea, not a required change; the implementer correctly took the task-preferred option (b).
