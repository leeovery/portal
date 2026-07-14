TASK: restore-host-terminal-windows-3-6 — Leave-what-opened partial-failure handling

ACCEPTANCE CRITERIA:
1. With one window among many timing out, the burst still spawns the remaining windows (no early stop on timeout/spawn-failed) and opened windows are left in place (no teardown invoked).
2. The trigger's self-attach (Connect) is skipped on any not-all-confirmed batch (trigger stays in its calling context).
3. An adapter spawn-failed and an ack timeout are classified identically as failed and both appear (by session name) in the one-line message.
4. The batch markers are Cleaned on the failure path (best-effort), as on the success path.
5. The command returns a plain error (exit 1) whose one-line stderr message names the failed window(s) and does not leak the opaque Result.Detail.
6. Multiple failed windows are all named in the single message.

STATUS: Complete

SPEC CONTEXT:
Spec "Burst & Partial-Failure Contract → Spawn, then self-attach LAST — gated on ALL N−1 confirming": after pre-flight passes, sequentially spawn the N−1 and collect acks; all-confirm → trigger self-attaches silently; any fails → Portal does NOT close/undo opened windows (no teardown; it doesn't own host windows), leaves them in place, skips the trigger self-attach, and shows a clean one-line error naming the failed window(s). "A missing marker at timeout = a failed spawn." Reporting & exit codes: partial spawn failure → exit 1 with "the same one-line message the picker would show", on stderr; nothing self-execs. Count semantics: total=N (incl. trigger target); opened counts each acked spawn plus the trigger self-attach only when it occurs — on the failure path the trigger self-attach is skipped and not counted. The selection-mutation half (unmark opened, keep failed marked) is explicitly a Phase 6 picker concern, out of scope for this CLI task.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/spawn/burst.go:154-181 — Run loop continues through all external windows; the only break paths are ctx cancellation (Task 6.8) and OutcomePermissionRequired (Task 3.7). A spawn-failed adapter result (result.OK()==false → AckFailed, never awaited) and an ack timeout (awaitToken → AckTimeout) both leave the loop running for later windows. Phase 2's stop-on-first placeholder is gone.
  - cmd/spawn.go:161-217 — runSpawn N≥2 branch: Ack.Clean(batch) runs unconditionally after the burst (line 169), BEFORE any branch; PartitionResults derives failed (line 179); the permission sub-case is checked first (191-199, Task 3.7); the generic leave-what-opened branch (209-210) logs the partial summary with triggerAttached=false and returns fmt.Errorf("spawn: %s", spawn.PartialFailureMessage(failed)); the success path (216-217) is the only path that reaches Connect(trigger).
  - internal/spawn/classify.go:15-34 — Confirmed()/PartitionResults chokepoint unifies AckTimeout + AckFailed into "failed", preserving list order; shared by CLI and picker so classification can't drift.
  - internal/spawn/message.go:44-56 — PartialFailureMessage shared renderer.
  - internal/spawn/logemit.go:52-70 — LogBatchSummary derives opened from PartitionResults (+1 only when triggerAttached), passes total=N through.
- Notes: No teardown seam exists anywhere in the codebase — "leave what opened" is satisfied by construction (there is simply no close-window call). Wording reconciliation worth recording: the plan's illustrative fmt.Errorf example was "failed to open window(s) for 's2' — others left open", but the implementation routes through the shared spawn.PartialFailureMessage renderer (introduced in Task 3-4), which renders "'s2' failed to open — others left open". This is a deliberate, better design — the spec pins only "the same one-line message the picker would show", the renderer is the single source both CLI and picker use, and its literal output is golden-tested (message_test.go:64-85). Not a defect; noting only because it diverges from the plan's example string.

TESTS:
- Status: Adequate
- Coverage:
  - Burst level (internal/spawn/burst_test.go): "classifies a non-OK adapter result as failed and still spawns the remaining windows" (248), "continues spawning the remaining windows after a middle window fails (no early stop)" (280) — asserts all 3 OpenWindow calls and per-window Ack outcomes with the middle window AckFailed. Test doubles (writingAdapter/delayingAck) are honest — they parse the real --spawn-ack flag via ParseSpawnAckFlag and drive a manual clock, so no real time/tmux/osascript.
  - CLI level (cmd/spawn_test.go TestSpawnPartialFailure, 946-1135): all 6 plan-named tests present — leaves opened windows in place on a timeout (954), no early stop after spawn-failed (984), ack-timeout and spawn-failed classified identically and both named in list order (1018), self-attach skipped / conn.calls==0 (1048), Ack.Clean called exactly once on the failure path (1071), exit-1 one-line message that is not a *UsageError, not a silent-exit, single-line, names 's2', and does NOT leak "osascript"/"-1743" (1094). Message asserted through spawn.PartialFailureMessage so CLI body == picker body.
  - Failure-path summary emission (opened count with triggerAttached=false, mixing AckTimeout+AckFailed → "opened 1/4", one DEBUG per window) is covered at the shared-helper level in internal/spawn/logemit_test.go TestLogBatchSummary_OpenedDerivedFromPartitionResults (101-144). This is the correct layer to test it — the CLI merely delegates — so the absence of a summary-log assertion in the CLI partial-failure tests is not a gap.
- Notes: Not over-tested. Each test isolates one behaviour; assertions are distinct (spawn count, connect count, clean count, message shape, classification, detail-leak). No redundant happy-path variations, no implementation-detail coupling — behaviour is asserted through the public seams (adapter Calls, connector calls, ack Cleaned, err.Error()).

CODE QUALITY:
- Project conventions: Followed. Small injectable seams (Adapter/Ack/Exe/Getenv/clock), package-level *Deps with production defaults, shared renderers (classify/message/logemit) as single sources of truth to prevent CLI↔picker drift, nil-logger-tolerant emission. Matches golang-design-patterns / testing conventions.
- SOLID principles: Good. Burster.Run has a single responsibility (open N−1 + classify); the classification/message/log concerns are each factored into their own pure functions; the CLI branch composes them without re-deriving semantics.
- Complexity: Acceptable. runSpawn is branch-heavy but each branch is guarded and linear; the burst loop has exactly two break conditions, both documented.
- Modern idioms: Yes (slices.Clone in the fakes, context between-window checks, pure partition returning nil for absent class).
- Readability: Good. Intent is explicit; the permission-precedence-before-generic ordering and the "no teardown by construction" stance are documented at the call sites.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
