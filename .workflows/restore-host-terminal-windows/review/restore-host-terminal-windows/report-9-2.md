TASK: Delete the four dead burst-outcome fields on Model (restore-host-terminal-windows-9-2, tick-bf24fe)

ACCEPTANCE CRITERIA:
- burstBatch, burstResults, burstIdentity, burstResolution removed from Model, along with their resetBurstState and dispatchBurst assignments.
- No reference to any of the four symbols remains; go build ./... and go test ./internal/tui/... green.
- Burst terminal-outcome behaviour (full-success self-attach, partial-failure flash, permission arm, cancellation) unchanged — it already reads the outcome from spawnCompleteMsg.
- The Model doc comment no longer describes the removed fields as active.

STATUS: Complete

SPEC CONTEXT: Phase 9 is an analysis-cycle cleanup phase. This task is architecture-sourced: the ~13 burst-lifecycle fields on the god-object Model included four dead ones. burstBatch/burstResults were only ever zeroed in resetBurstState; burstIdentity/burstResolution were written once in dispatchBurst but never read. The live terminal outcome travels on spawnCompleteMsg (msg.Batch/Results/Identity/Resolution) plus len(m.burstExternal) — the model fields were a stale second home. Removing them makes "which burst fields are live" answerable from code, not grep against stale state.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/model.go:498-518 (field block + doc comment), internal/tui/burst_progress.go:261-268 (resetBurstState), internal/tui/burst_progress.go:475-500 (dispatchBurst).
- Notes: Whole-repo grep (--include="*.go", covers prod + tests) for burstBatch, burstResults, burstIdentity, burstResolution returns ZERO hits — all four declarations, both resetBurstState zeroing lines, both dispatchBurst write lines, and every consumer are gone. No dangling reference.
  - resetBurstState (burst_progress.go:261-268) now clears only live fields: burstPending, burstPipe, burstCancel, burstTotal, burstDone, burstCancelled. The removed `m.burstResults = nil` / `m.burstBatch = ""` lines are absent.
  - dispatchBurst (burst_progress.go:494-500) now assigns only live fields: burstPipe, burstCancel, burstPending, burstTrigger, burstExternal, burstTotal, burstDone. The removed `m.burstIdentity`/`m.burstResolution` lines are absent. Line 482 reads m.detectAdapter/m.detectResolution — the DETECTION cache (§10-1), distinct fields, correctly untouched (not to be confused with the deleted burstResolution).
  - All 9 live burst fields present and untouched: burstPending/burstPipe/burstCancel/burstTrigger/burstExternal/burstTotal/burstDone (model.go:505-511), burstCancelled (518), pendingBurstEnter (526).
  - Doc comment (model.go:498-504) is corrected: it now enumerates only the live fields and closes with "The resolved terminal outcome (batch, results, identity, resolution) travels on spawnCompleteMsg, never on the model." — the opposite of a stale "active" claim; it actively documents that those outcomes do NOT live on the model.
  - Terminal-outcome path confirmed message-driven: the full-success arm (model.go:2519-2526) calls burstAllConfirmed(msg) (reads msg.Results + m.burstExternal) and emitBurstSummary(msg.Batch, msg.Identity, msg.Resolution, msg.Results, true); handleBurstPartialFailure (burst_partial_failure.go:34-73) reads msg.Identity/msg.Resolution/msg.Batch/msg.Results for the permission, summary, and partial-failure-flash arms. No path reads the deleted model fields.

TESTS:
- Status: Adequate (regression-by-existing-suite; no new test warranted)
- Coverage: The message-driven terminal behaviour is exercised by the existing burst suites — burst_all_confirmed_test.go, burst_selfattach_test.go, burst_partial_failure_test.go, burst_cancel_test.go, burst_input_lock_test.go, burst_preflight_abort_test.go, burst_unsupported_noop_test.go, burst_dispatch_test.go, initial_burst_seed_options_test.go, plus multi_select_enter_test.go and build_test.go. All drive outcomes through spawnCompleteMsg / BurstPending / handleBurstPartialFailure, never the deleted fields (whole-repo grep confirms zero references, including in *_test.go).
- Notes: This is a pure dead-code removal, so the correct test posture is exactly what the tick specifies — the existing suites pass unchanged, and the compiler is the enforcer for dangling references. Adding a test asserting field-absence would be over-testing (it would pin an implementation detail, not behaviour). No under-testing: every burst terminal arm retains its behavioural regression coverage. (Per task constraints I did not run the suite/build; adequacy assessed by reading. The change is a strict deletion of unreferenced symbols, so compilation and the green suites follow structurally.)

CODE QUALITY:
- Project conventions: Followed. Aligns with the codebase's discipline of keeping Model fields live and single-homed; doc-comment-per-field-block convention respected and updated in lockstep.
- SOLID principles: Good — reduces the god-object Model's field count and removes a duplicate outcome home (single source of truth = spawnCompleteMsg).
- Complexity: Low — net removal, no new branches.
- Modern idioms: N/A (deletion).
- Readability: Improved — the corrected doc comment now tells the truth about where the outcome lives, closing the exact trap the tick describes.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/model.go:505-526 — The tick's optional step 5 (explicitly deferred): consider grouping the remaining ~9 cohesive burst fields (burstPending/burstPipe/burstCancel/burstTrigger/burstExternal/burstTotal/burstDone/burstCancelled/pendingBurstEnter) into a single burstState struct on Model so the sub-state-machine is one addressable unit. Requires a design decision on whether to widen scope; left as a future refactor, not required for this task.
