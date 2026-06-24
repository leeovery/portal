TASK: spectrum-tui-design-5-3 — Restore per-session progress callback (the N/M source)

ACCEPTANCE CRITERIA (from tick-5be26c):
- The callback fires once per session in the loop with N advancing 1..M against a fixed M = len(idx.Sessions)
- Live-skipped sessions (already live, underscore-prefixed, invalid topology) still advance N so the counter reaches M/M
- M=0 (early return) fires zero callbacks
- A nil callback (synchronous warm/CLI path) leaves Restore behaviour byte-for-byte unchanged — same return values, same logs, same per-session sequence
- Restore() (bool, error) signature and the Restorer contract unchanged (additive field only)
- Non-visual plumbing — vhs-exempt; verification is behavioural

STATUS: Complete

SPEC CONTEXT:
§10.2 — on the cold+TUI path only, the orchestrator runs in a goroutine and "a progress
callback is injected at the restore per-session loop." §10.4 — "Restoring sessions (N/M)" is
the ONLY label carrying an N/M counter because the restore per-session loop is the one real
per-item progress source; M = len(idx.Sessions). Empty restore (M=0) suppresses the counter
and ticks immediately. The callback must be additive/nil-tolerant so the synchronous warm/CLI
path is byte-for-byte unchanged. This is a flagged prior-incident area (restore/daemon race),
so the bar is provable behaviour parity.

IMPLEMENTATION:
- Status: Implemented (additive, correct)
- Location:
  - internal/restore/restore.go:51 — new optional `Progress func(n, m int)` field on Orchestrator (well-documented godoc, lines 37-50).
  - internal/restore/restore.go:87 — `m := len(idx.Sessions)`.
  - internal/restore/restore.go:89-97 — loop changed `for _, sess` → `for i, sess`; guarded `if o.Progress != nil { o.Progress(i+1, m) }` fired BEFORE restoreOne, so N advances on every iteration regardless of outcome (live-skip / underscore / invalid topology / swallowed restore failure). Inline comment documents the before-restoreOne placement decision per the task's "Decide and document" instruction.
  - The two M=0 paths (len==0 early return at :67-69; ReadIndex skip/corrupt at :62-65) never reach the loop → zero callbacks.
  - Restore() (bool, error) signature unchanged; restoreOne / SessionRestorer / geometry / scrollback untouched.
- Production wiring (note: deviates from the plan's literal instruction, for the better):
  - Plan line 111 said wire the closure directly in buildProductionOrchestrator. The implementation instead leaves restoreInner.Progress nil there (cmd/bootstrap_production.go:158-170, with an explanatory comment) and installs the callback at Run-time via the optional RestoreProgressSink seam — bootstrap step 6 (cmd/bootstrap/bootstrap.go:375-381) calls sink.SetProgress(...) ONLY when a ctx progress emitter is wired (the cold/TUI route). RestoreAdapter.SetProgress (internal/bootstrapadapter/adapters.go:118-126) sets a.Inner.Progress. The forwarder emits StepEvent{Index:6, Name:stepRestore, RestoreN:n, RestoreM:m} onto the same ctx emitter the step events ride.
  - This is a sound refinement, not drift: it keeps the synchronous route's Progress structurally nil (SetProgress is never called when emit==nil), keeps the seam off the Restorer interface so Restore() (bool,error) is preserved, and is consistent with the task note's own forward-reference to the SetProgress seam. The plan's `buildProductionOrchestrator:158` is the orchestrator-build site; the closure now lives one layer up at Run-time. Behavioural outcome is identical.
  - The >63-session naked-send race flagged in the carry-forward note IS fixed: cmd/bootstrap_progress.go:209-214 send() now guards `select { case p.ch <- ev: case <-ctx.Done(): }`, so a restore burst exceeding the 64-buffer against an early-Quit TUI cannot wedge the orchestrator goroutine.
- Notes: Prior-incident concern cleared — the callback is pure read-side instrumentation inside Run's single goroutine; it mutates no tmux/state, perturbs no step ordering. The four race invariants are independently re-traced in cmd/bootstrap_progress.go's header comment.

TESTS:
- Status: Adequate (well-targeted, not over- or under-tested)
- Location: internal/restore/restore_progress_test.go
- Coverage (one test per acceptance criterion, named to the AC):
  - TestProgress_FiresOncePerSessionWithNAdvancingAgainstFixedM — asserts exact sequence {1,3},{2,3},{3,3}.
  - TestProgress_AdvancesNOnLiveSkippedSessionsSoCounterReachesMM — mixes live-skip, underscore-prefix, zero-windows, zero-panes, and one restorable; asserts {1,5}..{5,5}. Directly covers the "counter reaches M/M despite skips" AC.
  - TestProgress_AdvancesNOnSwallowedPerSessionRestoreFailure — drives a new-session failure inside restoreOne (logged+swallowed) and asserts N still ticks. Covers the edge case the task explicitly calls out.
  - TestProgress_FiresZeroCallbacksWhenMIsZero — table-driven (named subtests) across absent / empty / corrupt sessions.json. Covers all three M=0 entry paths.
  - TestProgress_NilCallbackLeavesRestoreOutcomesUnchanged — runs the same fixture twice (nil vs no-op callback) over ONE shared state dir and diffs the recorded tmux call sequence arg-for-arg. This is the strongest available proof of byte-for-byte parity for the additive-instrumentation AC; asserting the tmux call stream (observable behaviour) rather than internal state is the right approach.
  - Cross-route coverage exists upstream: cmd/bootstrap/progress_emitter_restore_test.go pins that the emitter route installs the forwarder and the synchronous route never calls SetProgress.
- Notes:
  - Tests assert observable contracts (callback sequence + tmux call parity), not implementation details — aligned with golang-testing review-mode guidance.
  - Correctly no t.Parallel() (consistent with the rest of internal/restore; the package's mocks are per-test but the suite convention is non-parallel).
  - The parity test's two runs share one temp dir specifically so absolute FIFO/scrollback paths in recorded args are byte-identical — a deliberate, documented choice that makes the diff meaningful.
  - Minor: the parity test compares lengths then indexes element-by-element by hand; reflect.DeepEqual on the [][]string would be terser, but the hand-rolled diff yields better failure messages (call[i][j] differ) — defensible as-is.

CODE QUALITY:
- Project conventions: Followed. Optional behaviour as a nil-defaulting struct field + interface-segregated SetProgress seam (kept off the Restorer interface) matches the repo's small-interface DI pattern. Logging untouched (no new attrs/components invented). godoc on the new field is thorough and accurate.
- SOLID principles: Good. RestoreProgressSink is an optional, segregated seam; Restore()'s contract is preserved (ISP/OCP — additive, no signature churn). The cold-only install lives at the composition boundary (step 6), not baked into the production builder.
- Complexity: Low. One nil-guarded call added to an existing loop; no new branches in the per-session decision tree.
- Modern idioms: Yes. `for i, sess := range` + guarded callback is idiomatic; ctx-guarded select on the send is the correct Go pattern for the early-Quit drain race.
- Readability: Good. The before-restoreOne placement and nil-tolerance are documented at the call site and on the field.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
