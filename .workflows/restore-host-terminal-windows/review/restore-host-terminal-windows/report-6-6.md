TASK: restore-host-terminal-windows-6-6 — Partial-failure leave-what-opened + selection mutation (non-all-confirmed arm of the terminal spawnCompleteMsg handler)

ACCEPTANCE CRITERIA:
1. One external window timing out among many → opened windows left in place (no teardown call), self-attach skipped (Selected()=="", no tea.Quit), picker stays in multi-select mode.
2. Confirmed sessions unmarked; failed/un-acked/un-attempted stay marked (a second Enter retries exactly the still-missing set).
3. An adapter spawn-failed and an ack timeout both classify as failed; both named in the one-line flash.
4. A permission-required result surfaces the driver's Result.Guidance verbatim once (not the generic failed-window flash); the affected session stays marked.
5. Because the burster stops on permission-required, post-wall windows are never attempted (stay marked).
6. No opened window is ever torn down (no teardown seam called from any burst path).
7. A Burster.Run pre-spawn error (msg.Err != nil, empty Results) → generic "could not start opening windows" flash, selection unchanged, burst-pending cleared, stays in multi-select mode — no degenerate empty-named "failed to open".

STATUS: Complete

SPEC CONTEXT:
Spec §"Burst & Partial-Failure Contract" (lines 156-170): all-or-nothing applies only at the pre-flight gate; once past pre-flight the only residual is a rare per-window hiccup handled by leave-what-opened rather than teardown. Portal "does not try to close or undo the windows that already opened … leaves them in place, skips the trigger window's self-attach, unmarks the sessions whose windows opened and keeps the failed/un-acked ones marked, so a second Enter retries exactly the missing set." §"Permissions & Error Quarantine" (line 420): within a burst a permission-required result is accounted like a failed window, stops the burst (grant is per-(source,target)), and surfaces the guidance once for the batch — not the generic one-line error. §"Multi-Select Mode → Mode affordance": notice-band precedence. Implementation aligns with all of these.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/burst_partial_failure.go (handleBurstPartialFailure, applyBurstSelectionMutation, burstPartialFailureFlash, burstPreSpawnErrorFlash const); internal/tui/model.go:2497-2528 (spawnCompleteMsg case routes the non-all-confirmed / cancelled arm to handleBurstPartialFailure); supporting: burst_progress.go:250-268 (burstAllConfirmed / resetBurstState), classify.go (PartitionResults / FirstPermission), message.go (PartialFailureMessage / QuoteJoin), burst_observability.go (emitBurstSummary / emitPermission), notice_band.go:347-377 (flash precedence over the multi-select banner).
- Notes: All 7 acceptance criteria are met.
  * Pre-spawn error (msg.Err != nil) handled FIRST after observability, returns without running the confirmed/failed split — matches the "empty Results, selection unchanged" requirement exactly.
  * confirmed/failed derived from the shared spawn.PartitionResults chokepoint, so the CLI (cmd/spawn.go) and picker cannot drift on count semantics (a strength — AC3's "classify identically" is structural, not duplicated logic).
  * "No teardown" is structural, not merely conventional: spawn.Adapter (internal/spawn/adapter.go:9) exposes ONLY OpenWindow — no close seam exists to call, so AC6 holds by construction.
  * Permission routing keys off the generic spawn.FirstPermission Outcome (never a driver detail string), preserving the permission-quarantine boundary. Permission window carries Ack=AckFailed (burst.go:164) so it lands in `failed` → stays marked (AC4).
  * refreshSessionDelegate() (not applyCanvasMode as the plan text loosely worded it) is the correct narrow path — it re-points the delegate at the live selectedSessions set so the ● clears from unmarked rows without re-touching footer/title. This is a better choice than the planned wording.
  * applyBurstSelectionMutation takes []string (the direct PartitionResults output) rather than the plan's map[string]struct{} — a cleaner signature with no behavioural difference.
  * Cancel path (§6-8, m.burstCancelled) deliberately converges here as a silent leave-what-opened mutation; the flash is guarded by !m.burstCancelled. Correct and documented.

TESTS:
- Status: Adequate
- Location: internal/tui/burst_partial_failure_test.go (7 tests)
- Coverage: Every acceptance criterion and every listed test scenario is covered:
  * LeavesOpenedWindowsAndSkipsSelfAttach — end-to-end spawn-fail; asserts Selected()=="", follow==nil, MultiSelectActive, burst-pending cleared, adapter.Calls does not grow (no teardown proxy), confirmed unmarked / failed+trigger marked, flashText == spawn.PartialFailureMessage (AC1, AC6).
  * PreSpawnError_GenericFlashSelectionUnchanged — injected msg.Err with nil Results; asserts generic flash, no "failed to open", selection count unchanged, all names still marked, no cmd, burst-pending cleared, multi-select preserved (AC7).
  * UnmarksConfirmedKeepsFailedForRetry — pins the exact post-mutation set {alpha:unmarked, bravo:marked, charlie:unmarked, delta(trigger):marked} (AC2).
  * AckTimeoutAndSpawnFailedClassifyIdentically — both classify as failed, both named in the flash (AC3).
  * PermissionGuidanceOnceAffectedStaysMarked — end-to-end permission burst; guidance verbatim, not the generic flash, affected session marked, confirmed unmarked, no quit, multi-select preserved (AC4).
  * UnattemptedPostPermissionStayMarked — asserts the burst stops (Results==2, adapter.Calls==2), un-attempted charlie stays marked (AC5).
  * StaysInMultiSelectMode — focused mode-preservation guard on an ack-timeout injection.
- Notes:
  * Balanced — no over-testing. The end-to-end vs injected-msg split is deliberate and documented (avoids the real ~8s ack timeout and background-goroutine race under -race). The mild overlap between StaysInMultiSelectMode and LeavesOpenedWindowsAndSkipsSelfAttach is justified: they exercise different classification paths (injected AckTimeout vs real spawn-fail).
  * Flash-body parity asserted through the shared spawn.PartialFailureMessage renderer rather than a hardcoded string — good (keeps CLI/picker copy in lockstep).
  * The defensive `len(failed) == 0 → ""` branch of burstPartialFailureFlash is not directly unit-tested. It is effectively unreachable on the non-cancel path (permission always yields a failed entry; all-confirmed is routed away by burstAllConfirmed; cancel is silenced by the !burstCancelled guard), so this is a low-value gap, not a real coverage hole.

CODE QUALITY:
- Project conventions: Followed. Small-interface DI, package-level shared chokepoints (spawn.PartitionResults/FirstPermission), single reset chokepoint (resetBurstState), no colour literals, matches the tui test-surface no-t.Parallel convention. Comments are dense but accurate and load-bearing (they encode cross-task ordering contracts).
- SOLID principles: Good. handleBurstPartialFailure has one responsibility (the non-all-confirmed arm); classification/rendering are delegated to the shared spawn package (interface segregation, DRY across CLI+picker).
- Complexity: Low. Linear control flow, one early return for the pre-spawn arm, one guarded flash.
- Modern idioms: Yes (slices.Clone in the test helper, range-over the confirmed slice, map delete).
- Readability: Good. Intent is self-documenting; the notice_band precedence decision is explicitly recorded to prevent a future single-slot regression.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/burst_partial_failure.go:43-47 — the observability emission (emitBurstSummary) runs BEFORE the msg.Err != nil early return, so a pre-spawn Burster.Run error (nothing opened, possibly empty msg.Batch) still emits a `spawn: opened 0/N` INFO batch summary. This is §6-10 observability territory, not 6-6, but flag it for the 6-10 reviewer: confirm a 0/N (possibly empty-batch) summary is the intended forensic breadcrumb for a burst that never started, vs. routing the pre-spawn abort to a distinct emission.
- [quickfix] internal/tui/burst_partial_failure.go:111-113 — add a direct unit test for burstPartialFailureFlash returning "" when failed is empty and no permission is present (the defensive leading-space guard). Currently only reachable defensively; a tiny table test would pin the "" contract at its source.
