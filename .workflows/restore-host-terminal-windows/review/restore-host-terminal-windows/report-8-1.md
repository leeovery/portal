TASK: Extract the spawn log-emission shapes into internal/spawn shared helpers (tick-6f32ac / restore-host-terminal-windows-8-1)

ACCEPTANCE CRITERIA:
- The four spawn log-emission shapes exist once in internal/spawn; neither cmd/spawn.go nor internal/tui/burst_observability.go hand-rolls the message strings or attr lists.
- The summary's opened/total is derived from spawn.PartitionResults on BOTH the CLI and picker paths (no residual inline confirmed-count loop in emitBurstSummary).
- Rendered log output (message + closed attr keys/values) at every emission site is byte-identical to today for the summary, permission, unsupported, and gone events.
- Only the closed spawn attr keys appear; all existing spawn unit + integration tests remain green.

STATUS: Complete

SPEC CONTEXT: The `spawn` log component is a closed, spec-governed vocabulary (Observability & State Footprint §"Attr keys (closed set)"). The two spawn surfaces — the CLI (cmd/spawn.go, the test seam) and the picker (internal/tui/burst_observability.go, the dominant production path) — previously each carried a byte-for-byte hand-written copy of the same closed emission, kept in sync by hand via a "MIRROR …" comment. This is a pure duplication-removal refactor (severity high, source: duplication), mirroring the earlier Cycle-1 extraction of the renderers (message.go) and count-semantics (classify.go) so the two paths cannot drift. A latent drift already existed: emitBurstSummary computed `opened` inline (`if r.Confirmed() { opened++ }`) while the CLI derived it from spawn.PartitionResults.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/logemit.go:34-104 (LogWindowResults, LogBatchSummary, LogPermission, LogUnsupported, LogGone); cmd/spawn.go:229-254 (wrappers) + :196 (direct LogWindowResults call); internal/tui/burst_observability.go:41-70 (thin model-bound wrappers).
- Notes:
  * All four required emission shapes now live once in internal/spawn/logemit.go, plus a fifth extracted helper LogWindowResults (the shared per-window DEBUG loop) — a legitimate factoring of the common "external window" record loop, not scope creep.
  * The residual inline count is gone: `rg` for `if r.Confirmed() { opened++ }` / `opened++` across internal/tui and cmd returns nothing. emitBurstSummary (burst_observability.go:41-44) now delegates to spawn.LogBatchSummary, which derives opened via PartitionResults (logemit.go:56-60) on both paths. total=N is passed through (len(m.burstExternal)+1 on the picker, n on the CLI) — correctly NOT derived from len(results), preserving the cancelled/pre-abort-burst semantics.
  * The tick's sketched signature (LogBatchSummary(logger, results, id, batch)) was correctly expanded to (logger, id, resolution, results, total, triggerAttached, batch) — the extra params (resolution attr, total, triggerAttached) are exactly what preserving byte-identical output requires. Divergence from the sketch is right; the acceptance criteria (byte-identical + PartitionResults-derived) are what matter and are met.
  * Do-step 4 done: burst_observability.go's header comment no longer says "MIRROR cmd/spawn.go's…" — it now points at internal/spawn/logemit.go as the single shared source.
  * All call sites route correctly: picker via emitBurstSummary/emitPermission/emitUnsupportedNoop/emitPreflightAbort (model.go:2523, burst_partial_failure.go:44/46, burst_progress.go:434/484, burst_preflight_abort.go:41); CLI via logSpawn* wrappers + spawn.LogWindowResults direct.
  * Only closed attr keys emitted: batch/terminal/bundle_id/resolution/session/ack/opened/total/detail — enforced by the assertClosedKeys/assertClosedSpawnKeys test guards; baseline pid/version/process_role are handler-injected, never at these sites.

TESTS:
- Status: Adequate
- Coverage:
  * Unit (internal/spawn/logemit_test.go): full-success body golden (TestLogBatchSummary_FullSuccessBody), opened-derived-from-PartitionResults over a mixed AckConfirmed/AckTimeout/AckFailed slice with a PartitionResults precondition assert (TestLogBatchSummary_OpenedDerivedFromPartitionResults), standalone per-window loop (TestLogWindowResults_OneDebugPerWindow), permission body (TestLogPermission_Body), unsupported body incl. no opened/total/batch (TestLogUnsupported_Body), gone single+plural (TestLogGone_Body), and nil-logger tolerance for every helper (TestLogEmit_NilLoggerDoesNotPanic). Each asserts the exact rendered body AND stays within the closed attr set.
  * Cross-caller parity: wantPermissionBody is pinned as a byte-identical literal at all three sites (logemit_test.go:173, cmd/spawn_test.go:1269, internal/tui/burst_observability_test.go:344), with TestLogSpawnPermission_ParityBody (CLI) and TestEmitPermission_ParityWithCLI (picker) both anchoring to it. For summary/unsupported/gone the parity anchor is the shared-helper golden in logemit_test.go itself: because both callers delegate to the single helper, that central golden IS the parity guarantee — the picker (opened 3/3 / opened 1/3) and CLI (opened 3/3) independent goldens confirm the delegation end-to-end.
  * Regression: existing cmd/spawn_test.go and internal/tui burst-observability assertions (permission-skips-summary, partial-failure-emits-summary, total-includes-trigger) are retained and assert against the delegated output.
- Notes: Body() renders `<LEVEL> <msg> key=value…` (capture.go:161-181), so the goldens legitimately pin message + ordered attrs. Not over-tested — the unit goldens and the per-caller behavioural tests target distinct concerns (helper correctness vs. caller routing) with no redundant restating. Would fail if a message string, attr key, attr order, or the opened-count mechanism drifted.

CODE QUALITY:
- Project conventions: Followed. Component logger bound once (spawnLogger = log.For("spawn")); nil-tolerance via log.OrDiscard matches the codebase's Discard idiom; internal/spawn importing internal/log (never the reverse) respects the leaf-log rule; closed vocabulary honoured with a compile-adjacent test guard. Matches the message.go/classify.go extraction pattern.
- SOLID principles: Good — single source of truth for the emission vocabulary; the count chokepoint (PartitionResults) is honoured on both paths.
- Complexity: Low. LogBatchSummary is the only multi-branch helper and it is linear.
- Modern idioms: Yes — stdlib *slog.Logger threaded through, no reflection or over-abstraction.
- Readability: Good; doc comments are thorough (verbose, consistent with house style).
- Issues: None blocking. Minor: the four CLI wrappers (logSpawnGone/logSpawnUnsupported/logSpawnPermission/logSpawnSummary, cmd/spawn.go:229-254) are now one-line pass-throughs to spawn.LogX, while runSpawn calls spawn.LogWindowResults directly (cmd/spawn.go:196) — a small routing inconsistency. The wrappers exist mainly to preserve the CLI-local test seam (TestLogSpawnPermission_ParityBody calls logSpawnPermission).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] cmd/spawn.go:229-254 — the four thin logSpawn* wrappers now only forward to spawn.LogX; consider inlining them into runSpawn (which already calls spawn.LogWindowResults directly) and retargeting the CLI parity tests to spawn.LogX, removing the extra indirection layer. Requires deciding whether to keep the CLI-side test seam as a separate parity anchor vs. relying on the central logemit_test.go golden — hence a design decision, not a mechanical edit.
