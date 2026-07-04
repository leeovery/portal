TASK: skip-bootstrap-when-warm-2-2 — Re-key shouldRunConcurrentBootstrap to latch-not-satisfied + reword openTUI comment

ACCEPTANCE CRITERIA:
- shouldRunConcurrentBootstrap takes (cmd, args, client, latchSatisfied bool) and returns isTUIPath(cmd,args) && client != nil && !latchSatisfied.
- The client.ServerRunning() call is removed; the decider issues zero tmux round-trips on every path.
- Warm-unlatched portal open (zero args, non-nil client, latchSatisfied==false) -> true (concurrent + loading).
- Latch-satisfied portal open (latchSatisfied==true) -> false (non-concurrent).
- nil client -> false; non-TUI command -> false; direct-path open -> false — regardless of latchSatisfied.
- openTUI still unconditionally force-sets serverStarted=true on the deferred route; comment reads "full bootstrap in progress" (not "cold by construction").
- TestWithServerStarted_GatesLoadingPage still passes unchanged.
- go build / go test ./cmd/... / golangci-lint clean.

STATUS: Complete

SPEC CONTEXT:
Spec § "Latch-Check Placement & Abridged-Path Wiring" → "Loading-screen trigger: latch-absent, not server-down" (spec lines 157-161): the concurrent/loading path is re-keyed off latch-not-satisfied rather than a ServerRunning() server-down probe. "Loading screen" now means exactly "a full bootstrap is in progress." A hand-started warm-unlatched tmux server hit by `x` should now get the loading screen + progress during its first full bootstrap (previously a synchronous no-progress stall). serverStarted force-true stays correct on this route because its sole effect is parking the model on the loading page; the "cold by construction" justification becomes stale and is reworded to "full bootstrap in progress." The abridged branch sits upstream of shouldRunConcurrentBootstrap, so the decider is only reached on the not-satisfied path (spec lines 147-155).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/root.go:303-308 — shouldRunConcurrentBootstrap re-signed to (cmd, args, client, latchSatisfied bool); body reduced to `if !isTUIPath(cmd, args) || client == nil { return false }` then `return !latchSatisfied`. No ServerRunning() call anywhere in the body.
  - cmd/root.go:280-302 — godoc rewritten: drops the "Cold = server NOT running ... ServerRunning() IS that probe" paragraph; restates the trigger as latch-not-satisfied on the TUI path, notes "loading screen now means exactly a full bootstrap is in progress," keeps the isTUIPath / nil-client guards, and documents that the decider is only reached on the not-satisfied path (latchSatisfied threaded explicitly to preserve the single-read invariant + self-describing contract).
  - cmd/root.go:213 — call site updated to shouldRunConcurrentBootstrap(cmd, args, client, latchSatisfied); latchSatisfied is the single upstream verdict computed at root.go:173 via state.BootstrappedLatchSatisfied(client, version). Task 2-3 has landed (upstream computation present), so no TODO(2-3) marker was needed.
  - cmd/root.go:198-212 — the block comment above the call reworded: drops the has-server / ServerRunning() cold-probe language and restates the trigger as latch-not-satisfied on the TUI path.
  - cmd/open.go:452-459 — openTUI force-true comment reworded to "Full bootstrap in progress: the loading page must show ..." with the serverStarted-sole-effect / warm-unlatched explanation. The `serverStarted = true` assignment stays unconditional on the deferred route (open.go:459).
  - cmd/open.go:437-447 — the surrounding §10.2 route comment already carries the matching warm-unlatched rationale (consistent, no stale "cold" assumption).
- Notes: Zero residual `client.ServerRunning()` calls; the only "ServerRunning()" mentions are comments documenting the retirement. Implementation matches the acceptance criteria exactly; no drift.

TESTS:
- Status: Adequate
- Location: cmd/concurrent_bootstrap_gate_test.go
- Coverage:
  - TestShouldRunConcurrentBootstrap (lines 35-65) — five subtests, one per boolean axis, matching the plan's named cases verbatim: (a) TUI + !satisfied -> true; (b) TUI + satisfied -> false; (c) nil client -> false; (d) non-TUI command -> false; (e) direct-path open -> false. All acceptance-criteria branches covered.
  - TestShouldRunConcurrentBootstrap_IssuesNoProbe (lines 67-92, renamed from ...ProbesOnlyOnTUIPath) — records recordingCommander.Calls across non-TUI / direct-path / TUI invocations and asserts len(Calls)==0 in every case. Correctly proves the decider issues zero tmux round-trips even on the TUI path (the previously-sanctioned single info probe is gone). Would fail if a ServerRunning()/info probe were reintroduced.
  - TestWithServerStarted_GatesLoadingPage (lines 94-117) — retained unchanged; still verifies serverStarted=false -> PageSessions and serverStarted=true -> PageLoading. Satisfies the "still passes unchanged" criterion.
  - probeClient() helper (lines 31-33) — comment correctly documents that the backing commander is never called because the decider is pure.
  - TestPersistentPreRunE_LatchedTUI_ReadsLatchExactlyOnce (lines 119-165) — bonus single-read/abridged-route coverage (Task 2-3 territory) confirming the satisfied path never reaches this decider and reads the latch exactly once.
- Notes: The plan flagged that TestPersistentPreRunE_WarmDirectTUI_RunsSynchronously would need a tmux-call-count retune owned by Task 2-3. No test by that name remains; entry-path tmux-call behaviour is covered by the LatchedTUI/abridged route tests (2-3's surface). Not this task's responsibility and not a coverage gap. Tests are focused, non-redundant, and NO t.Parallel() (consistent with the cmd-package mutable-state constraint).

CODE QUALITY:
- Project conventions: Followed. Small pure decider, heavy explanatory godoc (matches the codebase's documentation-dense style), spec-section references (§10.2, "see Task 2-3") consistent with existing comments (e.g. root.go:234 "task 6-10").
- SOLID principles: Good. Single-responsibility routing predicate; the ServerRunning() side-effect/dependency removed makes it a pure function of its inputs (improved testability).
- Complexity: Low. Two-line body, one guard + one boolean return.
- Modern idioms: Yes. Idiomatic Go boolean predicate; parameter threading over hidden re-probe preserves the single-read invariant.
- Readability: Good. The godoc explicitly explains why latchSatisfied is threaded even though it is effectively always false at the production call site (single-read invariant + self-describing contract + unit-testability of the satisfied branch), pre-empting the obvious "why not inline this?" question.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (The godoc's "(see Task 2-3)" plan-task-ID reference at root.go:299 was considered; it matches an established codebase convention — e.g. "task 6-10" at root.go:234 — so singling it out here would be inconsistent scope creep rather than a defect.)
