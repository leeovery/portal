TASK: skip-bootstrap-when-warm-2-3 — Latch-read three-way branch + abridged-path wiring in PersistentPreRunE

ACCEPTANCE CRITERIA:
- Compute latchSatisfied exactly once via state.BootstrappedLatchSatisfied(client, version) (guarded by client != nil), after buildBootstrapDeps() and upstream of shouldRunConcurrentBootstrap; never re-read.
- Satisfied -> ensureSaverLiveness(client, stateDir) runs, serverStartedKey=false + tmuxClientKey injected, NO deferredBootstrapKey, return nil; runBootstrap and shouldRunConcurrentBootstrap never reached.
- Satisfied + CLI (!isTUIPath) -> bootstrapWarnings.EmitTo(cmd.ErrOrStderr()) flushes SaverDownWarning before RunE.
- Satisfied + TUI (open, 0 args) -> warnings NOT emitted in PersistentPreRunE (left for openTUI); serverStarted=false -> no loading page -> instant picker.
- Not satisfied (absent / version-mismatch / read-error / nil client) -> existing full-bootstrap path, routed by shouldRunConcurrentBootstrap(cmd, args, client, false).
- Version-mismatch -> not satisfied -> full bootstrap (re-stamps).
- attach (not in skipTmuxCheck) + satisfied -> abridged sync path.
- go build passes; go test ./cmd/... passes; golangci-lint clean.

STATUS: Complete

SPEC CONTEXT:
Spec § "Latch-Check Placement & Abridged-Path Wiring" (specification.md:136-178) defines a single latch read driving a three-way branch: compute latchSatisfied once (line 151), abridged gate first with early return (line 152), full-bootstrap routing via shouldRunConcurrentBootstrap only on the not-satisfied path (line 153). The abridged branch sits upstream of the concurrent decider so the design stays one-read (line 155). Abridged reuses the sync plumbing (warning sink + serverStartedKey=false + tmuxClientKey) and — load-bearing — sets NO deferredBootstrapKey so serverStarted=false survives to the instant-picker gate (line 176). Outcome matrix (lines 165-170) enumerates open/attach/CLI × satisfied/not-satisfied. Implementation matches the spec verbatim, including the "unreadable/down-server folds into not-satisfied" contract (BootstrappedLatchSatisfied at internal/state/markers.go:195-204).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/root.go:156-196 — verdict computed once (line 173: `latchSatisfied := client != nil && state.BootstrappedLatchSatisfied(client, version)`), block comment documenting the three-way branch (lines 158-172), abridged gate (lines 186-196): stateDir via state.Dir() (error swallowed per task), ensureSaverLiveness, CLI-only EmitTo, serverStartedKey=false + tmuxClientKey injection, early return nil; NO deferredBootstrapKey, NO registerHooks.
  - cmd/root.go:213 — shouldRunConcurrentBootstrap called with the 4-arg signature threading latchSatisfied (never re-read).
  - cmd/root.go:228-263 — synchronous full-bootstrap block left unchanged below the abridged gate.
  - internal/state import added (cmd/root.go:13).
- Notes: All DO items satisfied. client != nil guard correctly folds nil-client into not-satisfied. tmuxClientKey injected unconditionally on the abridged branch (safe — latchSatisfied requires client != nil). registerHooks correctly skipped on the abridged branch. version resolves to package-level cmd.version (cmd/version.go:12 `var version = "dev"`), the same var the saver adapter reads. No scope creep — exactly the wiring + doc comments the task specified.

TESTS:
- Status: Adequate
- Coverage: All 9 micro-acceptance tests present and mapped:
  1. "abridged path when latch satisfied" -> TestPersistentPreRunE_LatchedTUI_TakesAbridgedPath (concurrent_bootstrap_route_test.go:80) — runner.calls==0, serverStarted==false, deferredSeen==false.
  2. "computes verdict exactly once" -> TestPersistentPreRunE_LatchedTUI_ReadsLatchExactlyOnce (concurrent_bootstrap_gate_test.go:127) — asserts countOp(show-option)==1.
  3. "full bootstrap when latch absent" -> TestPersistentPreRunE_FullBootstrap_WhenNotSatisfied/latch absent (abridged_route_test.go:131) — runner.calls==1.
  4. "full bootstrap on version mismatch" -> same table /version mismatch (version swapped under t.Cleanup).
  5. "folds latch read error into full bootstrap" -> same table /latch read error.
  6. "emits abridged warnings to stderr on CLI path" -> TestPersistentPreRunE_Abridged_EmitsWarningsToStderrOnCLIPath (abridged_route_test.go:197) — asserts stderr == rendered SaverDownWarning, runner.calls==0.
  7. "leaves abridged warnings for openTUI on TUI path" -> TestPersistentPreRunE_Abridged_LeavesWarningsForOpenTUIOnTUIPath (abridged_route_test.go:232) — asserts sink still pending at openTUI.
  8. "abridged path for attach when satisfied" -> TestPersistentPreRunE_Abridged_AttachTakesAbridgedPath (abridged_route_test.go:272) — runner.calls==0 AND attach proceeds (connectedTo).
  9. Integration layer: abridged_integration_test.go covers the outcome matrix (TestAbridged_OutcomeMatrix_OpenSatisfied_AbridgedInstantPicker), version-mismatch re-stamp, and real-tmux BootstrappedLatchSatisfied assertions.
  Tests correctly use deterministic mock commanders (notSatisfiedLatchClient/optionAbsentErr avoid coupling to the developer's live tmux latch), recordingRunner for the orchestrator seam, package-version swap under t.Cleanup for mismatch, no t.Parallel().
- Notes: Would fail if the feature broke — e.g. removing the early return would flip runner.calls to 1; re-reading the latch would flip the show-option count. Edge cases (nil client, read-error fold, attach-not-in-skipTmuxCheck) all covered. Mild overlap between TakesAbridgedPath and ReadsLatchExactlyOnce (both drive open+satisfied, assert runner.calls==0 + serverStarted==false) — but each carries a distinct primary assertion (no-deferred-bootstrap vs single-read count) and both are separately mandated by the task's micro-acceptance list, so not over-tested.

CODE QUALITY:
- Project conventions: Followed. Small-interface DI seam (RestoringChecker) and package-level *Deps test seams honoured; error-swallow on state.Dir() matches the task's grounding and the codebase's best-effort idiom; comment density matches the surrounding sync block. No bare os.Exit, no new slog construction.
- SOLID principles: Good. Single-read verdict threaded (not re-derived) into shouldRunConcurrentBootstrap preserves single-responsibility and the single-source-of-truth invariant; the decider takes latchSatisfied as a parameter rather than recomputing (dependency inversion on the verdict).
- Complexity: Low. Linear three-way branch with one early return; no nesting beyond the guard.
- Modern idioms: Yes. Short-circuit `client != nil && ...` guard, blank-identifier error swallow with intent documented.
- Readability: Good. The block comment (root.go:158-172) precisely names the four not-satisfied fold cases and the load-bearing no-deferredBootstrapKey precondition; self-documenting.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (Considered the TakesAbridgedPath / ReadsLatchExactlyOnce overlap; consolidation would contradict the task's explicit separate-test mandate and each has a distinct primary assertion, so no action proposed.)
