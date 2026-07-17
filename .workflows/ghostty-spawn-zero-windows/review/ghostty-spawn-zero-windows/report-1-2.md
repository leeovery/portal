TASK: Rider #1 / Fix 2 — surface why each window failed at production-default INFO (rewrite LogWindowResults per-window loop to split by outcome; internal/spawn/logemit.go)

ACCEPTANCE CRITERIA:
- LogWindowResults emits WARN 'external window failed' (attrs session/ack/detail) for any non-permission failed window — both AckFailed and AckTimeout.
- failed := !r.Confirmed(); nonPermission := r.Result.Outcome != OutcomePermissionRequired; failed && nonPermission → Warn("external window failed"); else → Debug("external window") unchanged shape.
- Confirmed window emits DEBUG 'external window'; permission-required window emits DEBUG (excluded from WARN, no double-report).
- No new attr keys; exactly one new message string ('external window failed', WARN) added to closed spawn catalog.
- ack=failed vs ack=timeout distinguishes the two failure modes; record honest even when detail is a benign success string (AckTimeout case).
- Both surfaces (CLI logSpawnSummary/permission arm → LogWindowResults; picker LogBatchSummary → LogWindowResults) get identical WARN behaviour.
- LogBatchSummary structure unchanged (LogWindowResults then one INFO summary).
- logemit_test.go updated: ack=timeout non-permission now WARN; DEBUG-per-window count no longer len(results); TestLogBatchSummary reflects 1 DEBUG + 2 WARN.
- go test ./... passes.

STATUS: Complete

SPEC CONTEXT:
Spec §Fix 2 (Rider #1). At production-default INFO the batch summary `opened 0/N` was visible but the per-window `detail` (the osascript error text — the actual diagnosis) was DEBUG-only, so the operator could see THAT windows failed but not WHY (the root cause could only be found by reproducing osascript outside portal). The spec prescribes splitting the per-window loop by outcome: a failed non-permission window → WARN 'external window failed' (session/ack/detail); every other window (confirmed OR permission-required) → DEBUG 'external window' unchanged. The failed set deliberately spans BOTH AckFailed (open-failure, OutcomeSpawnFailed) AND AckTimeout (opened but token never landed, benign success detail) — restricting to open-failures would re-introduce the invisibility gap. Permission window excluded so it does not double-report with the dedicated LogPermission INFO event. Catalog amendment (spec-governed): closed spawn component gains exactly one new message string at WARN, no new attr keys.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/logemit.go:53-64 (LogWindowResults). LogBatchSummary at :77-95 unchanged in structure (still calls LogWindowResults at :79 then one INFO summary at :87).
- Verified against dependencies:
  - classify.go:15-17 — Confirmed() == (Ack == AckConfirmed); both AckTimeout and AckFailed are !Confirmed(). Matches the `failed := !r.Confirmed()` predicate.
  - burst.go:164-167 — a permission-required window gets Ack = AckFailed (default; OpenWindow's PermissionRequired result fails OK() so awaitToken is never called). adapter.go:89-91 confirms OK() == (Outcome==OutcomeSuccess), so PermissionRequired.OK()==false. Therefore the permission window is !Confirmed() (AckFailed) — the `nonPermission := Outcome != OutcomePermissionRequired` guard is exactly what excludes it. The exclusion is load-bearing and correct: without it the permission window would double-report (WARN here + INFO LogPermission), and the CLI's permission arm calls LogWindowResults BEFORE LogPermission (logemit.go doc :48-51).
  - No new attr keys: WARN carries "session"/"ack"/"detail" — all already in the closed spawn vocabulary (confirmed against closedSpawnAttrKeys in logemit_test.go:33-43 and CLAUDE.md spawn attr list).
  - Exactly one new message string ("external window failed"). Catalog is single-sourced in logemit.go (no separate catalog file for message strings); the only new WARN string is at :59. No other call site emits it (grep across internal/cmd — only logemit.go emits, tests assert).
- Notes: `nonPermission` is assigned unconditionally before the `if`, but Go short-circuit means it is only meaningful when `failed` is true; negligible, idiomatic, no action. Doc comment (:34-52) is thorough and accurately explains the split, the both-modes span, and the permission exclusion rationale.

TESTS:
- Status: Adequate
- Coverage (internal/spawn/logemit_test.go):
  - TestLogWindowResults_FailedWindowsWarn — 4 subtests: ack_failed_open_failure_warns (WARN, session/ack=failed/detail=osascript boom), ack_timeout_after_success_warns (WARN, ack=timeout, benign success detail "opened y" still surfaced), confirmed_window_stays_debug (DEBUG, no WARN), permission_required_excluded_from_warn (AckFailed + PermissionRequired → DEBUG, no WARN). Directly covers every acceptance branch including the load-bearing permission exclusion.
  - TestLogWindowResults_SplitsByOutcome — pins the combined rendered body byte-for-byte (DEBUG confirmed + WARN timeout, ordering, and NO INFO record).
  - TestLogBatchSummary_OpenedDerivedFromPartitionResults — asserts DEBUG count == 1 (confirmed only, no longer len(results)) and WARN count == 2 (the two failed windows) for a mixed confirmed/timeout/failed slice; the spec's lockstep requirement.
  - assertClosedKeys (:45-54) proves every emitted record — including the new WARN — carries no non-closed attr key.
  - Nil-logger tolerance retained (TestLogEmit_NilLoggerDoesNotPanic).
  - Parity witnesses outside this task's file, both green: internal/tui/burst_observability_test.go:191 (picker-side WARN split) and cmd/spawn_test.go:1250 (CLI permission path: window 1 confirmed + window 2 permission-excluded → 2 DEBUG, still valid).
  - The stale TestLogWindowResults_OneDebugPerWindow named in the spec's lockstep note is gone (renamed to _SplitsByOutcome); remaining `len(results)` mentions are comment/error-message text only, not stale count assertions.
- Not under-tested: benign-detail AckTimeout case, permission exclusion, closed-key invariant, and the DEBUG/WARN count shift are all covered.
- Not over-tested: SplitsByOutcome (byte-exact body) and FailedWindowsWarn (per-outcome attribute-level) overlap slightly but serve distinct purposes (rendered-order contract vs isolated-outcome attrs); reasonable, not bloated.
- Ran `go test ./internal/spawn/ ./internal/tui/ ./cmd/` — all pass. Targeted `go test ./internal/spawn/ -run 'TestLogWindowResults|TestLogBatchSummary|TestLogEmit' -v` — all subtests PASS.

CODE QUALITY:
- Project conventions: Followed. Uses log.OrDiscard entry guard (nil-tolerant), only closed spawn attr keys at the call site, greppable distinct message string, single-source helper both surfaces delegate to. No raw logger construction (stays within internal/log usage rules).
- SOLID principles: Good. Single responsibility preserved; LogWindowResults remains the one chokepoint, LogBatchSummary composition unchanged, no per-caller divergence introduced.
- Complexity: Low. A single loop with a two-boolean guard and a `continue`.
- Modern idioms: Yes. Idiomatic Go; `string(r.Ack)` conversion consistent with the pre-existing DEBUG line.
- Readability: Good. `failed`/`nonPermission` names are self-documenting; the doc comment accurately explains the design and the exclusion.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
