TASK: 2-1 — Rework Reactive-Backstop No-op Tests Onto The In-Flight Entry Path (chore, test-only; tick-68e162 / persistent-no-host-terminal-banner-2-1)

ACCEPTANCE CRITERIA:
1. Both TestBurstUnsupported_NonNullAtomicNoOp and TestBurstUnsupported_NullFlash call markTwo(t, m) BEFORE resolveDetection(...).
2. The two asserted flash strings and every assertAtomicNoOp invariant are byte-identical to the pre-rework versions.
3. go test ./internal/tui -run TestBurstUnsupported passes against the current code (unit lane, no tmux daemon).
4. TestBurstUnsupported_DeferredThenUnsupported and TestBurstUnsupported_SupportedStillDispatches are unchanged and still pass.
5. No production file is modified by this task (test-only commit).

STATUS: complete

SPEC CONTEXT:
Spec §3 Sub-fix 2 retains the reactive decideBurst backstop specifically to guard the "async in-flight window": the user can enter multi-select and press Enter while terminal detection has not yet resolved (detectResolved == false). Task 2.2 adds a proactive entry block in handleMultiSelectToggle that turns a post-resolve `m` (detection already resolved unsupported) into a no-op. The two reactive-backstop no-op tests previously entered multi-select AFTER resolution, so they would trip 2.2's new entry block and go red. §7 (Testing — Rework) mandates re-anchoring them onto the in-flight entry path so they exercise the retained reactive arm, staying green per-commit. This task is ordered FIRST (before 2.2) precisely so the suite never transiently fails.

IMPLEMENTATION:
- Status: Implemented (matches the task DO list exactly)
- Location: internal/tui/burst_unsupported_noop_test.go
  - NonNullAtomicNoOp: markTwo now at L133, precedes resolveDetection at L134 (was reversed).
  - NullFlash: markTwo now at L167, precedes resolveDetection at L171 (was reversed).
  - Doc comments for both tests gained the "A1 — markTwo runs BEFORE detection resolves, so the proactive entry block is inert" note (L120-123, L154-157).
- Correctness of the reordering (verified by reading production):
  - spawn_detect.go:117 DetectUnsupported() == detectResolved && detectResolution==ResolutionUnsupported → FALSE while detection in flight.
  - model.go:3554 entry block gates on m.DetectUnsupported(); with markTwo run pre-resolve it is inert, so enterMultiSelectEmpty + markRow*2 open the mode and mark alpha/bravo (markTwo asserts count==2 at L47-49 — a real precondition, not a smoke assert).
  - resolveDetection (burst_dispatch_test.go:47) then flips detectResolved via terminalDetectedMsg; the explicit `if !m.DetectUnsupported()` precondition (L135, L172) confirms the unsupported resolution before Enter.
  - pressEnter routes through decideBurst (burst_progress.go:425 DetectUnsupported() arm) → atomic no-op + re-asserted flash. This is the exact async-race path the retained backstop guards.
- Commit scope: `git show 71a9c67d --stat` shows only three files — internal/tui/burst_unsupported_noop_test.go (+12/-2, test-only), .tick/tasks.jsonl and .workflows/.../manifest.json (workflow bookkeeping, not production code). No production Go file touched. Criterion 5 satisfied.
- Notes: The later copy rewrite by task 3-1 (commit 81d09740) changed the flash literals to the current "can't open new windows …" wording; that is out of scope for this task and expected per the review note. This commit (71a9c67d) does not touch the const want lines.

TESTS:
- Status: Adequate (this is itself a test-rework task; verification is by completeness + still-holds, not new coverage)
- Coverage: The reworked pair now exercises the in-flight entry → post-resolve Enter path (the reactive backstop). DeferredThenUnsupported (L188-233) continues to cover the sibling in-flight-at-Enter defer→terminalDetectedMsg path; SupportedStillDispatches (L235-267) continues to guard the supported ghostty→native dispatch. Coverage of the three unsupported shapes (named non-NULL, NULL remote/mosh + transient-error fold, deferred) is preserved.
- Byte-identical assertions (criterion 2): the diff hunks do not modify assertAtomicNoOp, its call sites, or the two `const want` lines — only markTwo was relocated and doc comments added. assertAtomicNoOp (L56-81) still asserts the full invariant set: no burst-pending, no burstPipe, zero adapter.Calls, empty Selected(), still in multi-select mode, count==2, both marked names retained. Byte-identical to pre-rework confirmed.
- Unchanged tests (criterion 4): DeferredThenUnsupported and SupportedStillDispatches are absent from the diff — verbatim unchanged.
- Pass judgement (criterion 3, judged by reading — not executed): the reordering is sound because DetectUnsupported() is false during the in-flight window, so the entry path opens regardless of whether 2.2's block is present; the post-resolve Enter reaches decideBurst's unsupported arm. Tests are structurally green both before and after 2.2 lands.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel (per CLAUDE.md / the file header). Unit-lane, no tmux daemon, no built binary — correctly stays out of the integration lane. Uses the shared helpers (markTwo, resolveDetection, assertAtomicNoOp, wireUnsupportedBurstSeams) rather than duplicating setup.
- SOLID principles: N/A (test file); helper reuse is clean.
- Complexity: Low. A pure statement reorder plus doc-comment additions.
- Modern idioms: Yes. t.Helper() on helpers, const want for the asserted copy.
- Readability: Good. The added doc-comment notes make the in-flight-entry intent explicit and cite the A1 window and the decideBurst reactive arm, which aids future maintenance.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
