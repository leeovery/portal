TASK: cli-verb-surface-redesign-3-8 — Leave-what-opened partial failure + per-window ~8s ack timeout + `portal.log` outcomes

ACCEPTANCE CRITERIA:
- opened windows stay (no teardown); failed/un-acked surfaces don't auto-retry
- per-window ~8s ack timeout timed from each window's OWN spawn
- trigger connects independent of other windows' failures; skipped only if its own target fails at connect (outside tmux returns to the shell)
- each outcome recorded in portal.log; stderr summary swallowed on attach (log is the durable surface)
- permission-required stops the burst, surfaced once
- batch markers cleaned on every terminal path before self-connect

STATUS: Complete

SPEC CONTEXT:
Spec §"Atomic pre-flight & partial failure" (specification.md:208-213) governs this task. Past the read-only resolve, per-window failure is leave-what-opened: opened windows stay (Portal doesn't own/tear-down host windows), failures don't auto-retry, and the trigger connects to its own first-target surface independent of other windows' ack failures — skipped only if its own target fails at connect (outside tmux → returns to the shell without attaching). portal.log is the durable surface; a best-effort stderr summary is swallowed by a successful attach and directly visible only in the trigger-skip case. Per-window ack timeout ~8s, the timer starting at that window's own spawn so cumulative sequential delay never eats a later window's budget. Phase-3 AC (planning.md:90) mirrors these.

IMPLEMENTATION:
- Status: Implemented (matches acceptance criteria, no drift)
- Location:
  - internal/spawn/ack.go:30 — `spawnAckTimeout = 8 * time.Second` (per-window budget, documented rationale).
  - internal/spawn/burst.go:194-225 — `awaitToken` captures `start := b.Now()` right after this window's OpenWindow, so the timer starts at each window's own spawn; a Collect error is treated as "not present yet" (bounded by the timer → AckTimeout, never a false confirm/panic).
  - internal/spawn/burst.go:163-191 — `Run` loop: no teardown, no retry; the sole early-stop is `OutcomePermissionRequired` (break at :187); timeout/spawn-failed do NOT break.
  - internal/spawn/classify.go:26-51 — `PartitionResults` (confirmed/failed), `FirstPermission` (burst-stop signal).
  - internal/spawn/logemit.go — `LogWindowResults` (per-window DEBUG confirmed / WARN failed), `LogBatchSummary` (per-window loop + one INFO `opened N/N`), `LogPermission`, `LogUnsupported`, `LogGone` — single source for the closed spawn log vocabulary, nil-logger tolerant.
  - internal/spawn/message.go:44-66 — `PartialFailureMessage` (othersOpened-keyed).
  - cmd/open_burst_run.go:177-231 — orchestration: burster Run → unconditional `Ack.Clean(batch)` (:186, before self-connect) → report (permission arm / leave-what-opened / full success) → `connectTrigger` LAST, independent of external outcomes; only the trigger's OWN connect error propagates.
- Notes: "Cleaned on every terminal path" is correctly scoped to every POST-BURST path: the unsupported-terminal (:166) and pre-spawn-abort (:178) early returns never spawn a window and never write a marker (the burster returns batch="" on error), so there is nothing to clean and no leak — sound.

TESTS:
- Status: Adequate (thorough; not over-tested)
- Coverage:
  - Leave-what-opened / no teardown / no retry: cmd/open_burst_run_test.go TestRunOpenBurst_PartialFailure_LeavesOthersOpen_StillConnectsTrigger (e1 spawn-fails, e2 confirms, both attempted, trigger still connects, no error); burst_test.go "continues spawning the remaining windows after a middle window fails".
  - Per-window ~8s timer from own spawn: cmd TestRunOpenBurst_PerWindowAckTimeout_TimedFromOwnSpawn + spawn burst_test.go "it starts each window's ack timer at its own spawn" — both use a delaying-ack + manual clock with a delay >= Poll and < Timeout so the global clock is already past a full Timeout by window 2's spawn, making per-window vs global timer discriminating. NewBurster default Timeout==spawnAckTimeout pinned by TestNewBurster_Defaults.
  - Trigger independence + skip-only-on-own-failure: TestRunOpenBurst_TriggerOwnConnectFails_PropagatesError, TestRunOpenBurst_TriggerMintOwnConnectFails_PropagatesError, and the still-connects cases in the partial-failure/permission tests.
  - portal.log outcomes: TestRunOpenBurst_RecordsOutcomesInLog (per-window DEBUG ack + one INFO opened 3/3 with resolution/terminal/bundle_id/opened/total/batch); logemit_test.go pins full-success/partition/WARN-split/permission/unsupported/gone bodies byte-for-byte + closed-attr-key guard.
  - Permission stops burst, surfaced once: TestRunOpenBurst_PermissionRequired_GuidanceOnce_StillConnectsTrigger (later windows never spawn, one LogPermission INFO, no batch summary, guidance once verbatim, driver detail quarantined off stderr) + burst_test.go permission-stop.
  - Markers cleaned before self-connect: TestRunOpenBurst_MarkersCleanedBeforeSelfConnect (cleanOrderConnector snapshots cleaned-count==1 at Connect time).
  - Renderer branches: message_test.go covers both PartialFailureMessage branches (others-left-open / nothing-opened).
- Notes: "stderr swallowed on attach" is architectural (the connector exec-replaces / switches away), not this unit's responsibility, so its absence from unit tests is correct — the code emits exactly one best-effort Fprintln, which is asserted. No redundant/over-mocked tests observed; fakes are minimal and behaviour-focused.

CODE QUALITY:
- Project conventions: Followed. Small injectable seams + `*Deps` struct with production defaults; shared count/render/emit chokepoints in internal/spawn so the open burst and picker burst cannot drift; best-effort side-effects use `_ =` discards per house style; no t.Parallel.
- SOLID principles: Good. classify/message/logemit are single-responsibility pure functions; burster is DI-driven and clock-injected.
- Complexity: Low. Run is a single sequential loop; the report block is a clear 3-way branch.
- Modern idioms: Yes (slices, context, range-over-func in ack.go optionNames).
- Readability: Good; comments are verbose but load-bearing and accurate.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] cmd/open_burst_run.go:222,226,231 — LogBatchSummary is emitted with triggerAttached=true (opened = confirmed + 1) BEFORE connectTrigger runs, and this is documented as deliberate (:204-206). On the rare trigger-own-connect-failure path (attach session vanished between pre-flight and connect), portal.log therefore reports `opened N/N` counting a trigger that did not attach. Outside tmux a successful connect exec-replaces the process (no post-success log is possible), but the FAILURE path returns an error, so a corrective WARN could be emitted from the connectTrigger error path to keep the durable log honest. Whether the extra line is worth the noise on a vanishingly rare path is a judgment call.
