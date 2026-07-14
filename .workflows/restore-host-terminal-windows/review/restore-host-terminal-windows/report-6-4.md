TASK: restore-host-terminal-windows-6-4 — Full-success self-attach (net N) + marker self-clean

ACCEPTANCE CRITERIA:
- A spawnCompleteMsg with every WindowResult.Ack == AckConfirmed sets Selected() == burstTrigger and returns tea.Quit (drives the existing connector via processTUIResult).
- The batch markers are cleaned (AckChannel.Clean(batch) recorded) before the terminal spawnCompleteMsg is produced — i.e. before the self-attach exec handoff.
- No success flash / N/N ✓ banner renders on full success (silent self-attach).
- An includes-self selection self-attaches to the trigger (one marked session) with the rest spawned externally — the origin session ends up attached.
- A session confirmed via ack while already attached elsewhere (fake ack writes the token) still self-attaches correctly (no dup guard; the ack confirms our new window).
- The self-attach uses the existing AttachConnector/SwitchConnector path (via Selected()+tea.Quit), not a spawn-adapter call.

STATUS: Complete

SPEC CONTEXT:
Spec §Burst & Partial-Failure Contract — "Spawn, then self-attach LAST — gated on ALL N−1 confirming": all confirm → the trigger window self-attaches silently (no "14/14 ✓" nag). §Cleanup — "the picker self-cleans its batch markers before self-exec". §The N vs N−1 split — the picker always self-attaches to exactly one of the N; only the N−1 others are externally spawned (net N, never N+1). §Trigger-Context Matrix — includes-self (trigger becomes one attached session, rest spawn, origin ends up attached either way) and already-attached-elsewhere (allowed, no dup guard; the token ack confirms our new window regardless of other clients). §Confirmation mechanism — the explicit token ack is what makes a session already attached elsewhere confirm correctly.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:2497-2528 — the spawnCompleteMsg arm. Full-success gate `if m.burstAllConfirmed(msg) && !m.burstCancelled`: emits the batch summary (§6-10), sets `m.selected = m.burstTrigger`, calls `resetBurstState()`, returns `m, tea.Quit`. The non-all-confirmed / cancel outcome falls through to `handleBurstPartialFailure(msg)` (§6-6) — the clear split the task required.
  - internal/tui/burst_progress.go:250-253 — `burstAllConfirmed` derives the verdict from `spawn.PartitionResults` (the single count-semantics chokepoint) plus the `msg.Err == nil` and `len(msg.Results) == len(m.burstExternal)` arity guards. Uses `len(failed) == 0`, identical to the CLI's runSpawn gate, so the two orchestrations cannot drift.
  - internal/tui/burst_progress.go:188-206 — `burstRunner.run` calls `r.ackChannel.Clean(batch)` (line 197) STRICTLY before emitting the terminal event (line 198), so markers are unset by the time the handler issues tea.Quit — the self-clean-before-self-exec ordering.
  - internal/tui/burst_progress.go:261-268 — `resetBurstState` is the single reset chokepoint clearing burst-pending/pipe/cancel/counters/cancelled.
  - internal/tui/dispatchBurst (burst_progress.go:475-513) derives external/trigger via `spawn.SplitNetN` (the shared net-N computation).
- Notes:
  - Shape parity with the single-attach commit is exact: model.go:2429-2430 (preview-Enter) and handleSessionListEnter both do `m.selected = …; return m, tea.Quit` → processTUIResult drives the already-resolved AttachConnector/SwitchConnector. No spawn-adapter is used for the self-attach, matching the spec and criterion 6.
  - No goroutine/pipe leak: the full-success arm returns tea.Quit without re-issuing the receiver; the goroutine has already emitted the terminal event and its deferred close runs with no waiting receiver. resetBurstState nils the pipe/cancel refs.
  - `emitBurstSummary` (a §6-10 log emission, NOT a flash) is invoked before resetBurstState; it reads `len(m.burstExternal)` which reset does not touch, so ordering is immaterial to correctness. This is legitimate observability, not a flash — consistent with the "no N/N ✓ nag" requirement.

TESTS:
- Status: Adequate
- Location: internal/tui/burst_selfattach_test.go
- Coverage:
  - TestBurst_FullSuccess_SelfAttachesToTriggerAndQuits — asserts Selected()==trigger ("charlie"), isQuitCmd(follow), burst-pending cleared, adapter.Calls==2 (N-1 external only), and that the trigger is never externally opened. Covers criteria 1 and 6.
  - TestBurst_FullSuccess_CleansMarkersBeforeSelfAttachHandoff — reads ack.Cleaned==1 and Selected()=="" at the pre-apply point (returned by driveBurstToTerminal before the terminal message is applied), proving Clean ran strictly before the self-attach handoff; also asserts no re-Clean after apply. Covers criterion 2. The read of ack.Cleaned is race-free: the channel receive establishes happens-before over the goroutine's Clean mutation.
  - TestBurst_FullSuccess_RendersNoSuccessFlash — asserts rm.flashText=="". Covers criterion 3.
  - TestBurst_FullSuccess_IncludesSelfSelectionSelfAttaches — Selected()==trigger, quit, adapter.Calls==1, trigger not externally opened. Covers criterion 4.
  - TestBurst_FullSuccess_ConfirmedWhileAttachedElsewhere — Results[0].Ack==AckConfirmed then Selected()==trigger + quit. Covers criterion 5.
  - TestBurst_NotAllConfirmed_ClearsPendingWithoutQuit — a spawn-failed external (AckFailed) leaves Selected()=="" , nil cmd, pending cleared, failed session stays marked, multi-select stays active. Correctly pins the full-success/partial-failure split so this task's arm does NOT fire on a non-all-confirmed terminal.
- Notes:
  - Would fail if the feature broke: yes — Selected/quit/flash/Clean-ordering are all directly asserted.
  - The confirm-all fake path (FakeAdapter parses --spawn-ack out of the composed argv and Writes the real token via spawn.ParseSpawnAckFlag) keeps the ack honest to the wire format, so AckConfirmed is genuinely earned rather than stubbed.
  - Mild over-lap: TestBurst_FullSuccess_ConfirmedWhileAttachedElsewhere is structurally near-identical to the includes-self test — both mark ["alpha","bravo"] via the default confirm-all path and assert Selected==trigger + quit. There is no fake "attached elsewhere" state at the model layer (the model has no dup guard, so the only observable behaviour is "confirmed ack → self-attach"), so the "attached elsewhere" scenario is not genuinely distinguishable in a unit test — it is truly exercisable only at the integration layer with a second real client. The test's value is spec-criterion traceability plus the distinct Ack==AckConfirmed assertion; this is defensible but worth a reviewer decision (see notes).

CODE QUALITY:
- Project conventions: Followed. White-box package tui tests, no t.Parallel (per the cmd/tui mutable-seam rule), nil-tolerant Option seams, shared spawn chokepoints (PartitionResults / SplitNetN) reused rather than re-implemented.
- SOLID principles: Good. burstAllConfirmed is a thin derivation over the single count-semantics chokepoint; resetBurstState is the single reset point; the full-success and partial-failure arms are cleanly separated.
- DRY: Good — verdict, split, and reset each route through one shared function, explicitly to prevent CLI/picker drift.
- Complexity: Low. The arm is a single guarded branch with an early return.
- Modern idioms: Yes (slices.Clone, range-over-int in tests, small interface seams).
- Readability: Good — the arm and its helpers carry precise spec-anchored comments.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/burst_selfattach_test.go:210-238 — TestBurst_FullSuccess_ConfirmedWhileAttachedElsewhere is structurally near-identical to TestBurst_FullSuccess_IncludesSelfSelectionSelfAttaches and cannot model a real attached-elsewhere state at the unit layer (no dup guard exists to exercise). Decide whether to keep it for spec-criterion traceability, consolidate the two, or add a one-line comment pointing to the integration layer as the true home of the multi-client edge.
