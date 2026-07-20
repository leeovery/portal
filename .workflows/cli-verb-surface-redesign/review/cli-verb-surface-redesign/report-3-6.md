TASK: cli-verb-surface-redesign-3-6 — Net-N dispatch: trigger absorbs first, spawns N−1 external, connects last

ACCEPTANCE CRITERIA (plan row 3-6 + Phase 3 AC bullet):
- Trigger = first target in command-line order; N−1 spawned first; trigger self-connects LAST (load-bearing outside tmux).
- First-element split distinct from the legacy trailing-trigger SplitNetN.
- Current session never special-cased (window only if in set; first → no-op switch; elsewhere → moves + own window; absent → left detached).
- Duplicates honored, never deduped.
- Inside/outside split only selects the trigger's connector.
- N=1 degenerates to a plain single connect.

STATUS: Complete

SPEC CONTEXT:
Spec § "The trigger absorbs the first target, unconditionally; no dedup" (lines 168-182) and its "Execution order — the trigger connects last" clause. The trigger takes the first target left-to-right; the N−1 non-trigger surfaces spawn first; the trigger self-connects last because outside tmux `exec attach` replaces the Portal process — connecting first would destroy the burster and open only one surface. No current-session detection: a session gets a window only if it is a target; duplicates are intent, not collapsed. The inside/outside-tmux split selects only the trigger's connector (switch-client inside / exec attach outside); the rest run the spawned `portal open …`.

IMPLEMENTATION:
- Status: Implemented (matches AC, no drift)
- Location:
  - internal/spawn/split.go:39-41 — SplitTriggerFirst(ordered) returns ordered[0] (trigger) + ordered[1:] (external); deliberately distinct from SplitNetN (split.go:18-20, trailing-trigger, picker-only) with cross-referencing doc comments.
  - cmd/open_burst_run.go:144-232 — runOpenBurstWithDeps: zero-mint-command guard → SplitTriggerFirst → detect+resolve → unsupported atomic-no-op gate → spawn N−1 via Burster.Run(external) → Ack.Clean → report externals → connectTrigger LAST (line 231).
  - cmd/open_burst_run.go:253-258 — connectTrigger: mint trigger → LocalMint(dir, command); attach trigger → Connector.Connect(value). Sole site the inside/outside connector is selected (Connector defaults to buildSessionConnector(tmuxClient) at cmd/open_burst_run.go:95).
  - cmd/open.go:136-141 — buildSessionConnector selects SwitchConnector (inside tmux) / AttachConnector (outside). Externals always run out-of-tmux argv (TMUX stripped, spawn layer).
  - cmd/open_burst.go:147-151 — dispatchOpenBurst: len(surfaces)==1 degenerates to openResolved (single connect); 2+ → runOpenBurstFunc.
- Notes:
  - "Trigger connects last" is load-bearing and correctly ordered: Burster.Run (all N−1 spawns) fully returns before connectTrigger. A pre-spawn Burster error returns before any connect.
  - "Current session never special-cased" is satisfied by construction — there is NO current-session detection anywhere in the burst path; surfaces are taken literally. The three emergent behaviors (first → no-op switch; elsewhere → own window; absent → detached) are properties of tmux switch-client + the spawned `open --session` argv, not of this function, so there is nothing to special-case. Correct.
  - Trigger-independence honored: external failures / permission wall do not gate the trigger connect (open_burst_run.go:188-231); only the trigger's OWN connect failure propagates (connectTrigger return).
  - Zero-mint-command guard (line 150) fires before detect/spawn — the multi-target arity of the Task 2-6 attach-command rule, message single-sourced via commandAttachOnlyMessage.
  - Scope is clean: commit f1c43139 touches exactly cmd/open_burst.go, cmd/open_burst_run.go, internal/spawn/split.go + their tests. No scope creep.

TESTS:
- Status: Adequate
- Coverage:
  - internal/spawn/split_test.go — TestSplitTriggerFirst (two/three/single element), TestSplitTriggerFirst_DistinctFromSplitNetN (proves the two splits pick OPPOSITE ends of the same slice; mirror-image externals) → directly covers "first-element split distinct from legacy trailing-trigger SplitNetN".
  - cmd/open_burst_run_test.go — TriggerFirst_ExternalRestInOrder (trigger absorbs first, externals surfaces[1:] in order), TriggerConnectsLast (event sequence proves both spawns precede the connect), TriggerAttach_RoutesToConnector, TriggerMint_RoutesToLocalMint, DuplicatesHonored_NoDedup, UnsupportedTerminal_AtomicNoop (trigger must NOT half-connect), PreSpawnBursterError_TriggerNotConnected, TriggerOwnConnectFails_PropagatesError, TriggerMintOwnConnectFails_PropagatesError.
  - cmd/open_multitarget_test.go:306 — SingleGlobExpandingToOne_SingleConnectNotBurst covers "N=1 degenerates to a plain single connect" (glob→1 surface routes to openResolved, not the burst).
  - cmd/open_burst_seams_test.go — buildOpenBurstDeps defaulting (injected wins, unset defaults, novel LocalMint→openPathFunc).
- Notes:
  - Well-balanced: the core Task-3-6 ACs each have a focused assertion (order, connects-last, distinct-split, duplicates, N=1 degenerate, trigger connector routing). Deterministic fakes + manual clock, no real tmux/osascript.
  - Several tests in open_burst_run_test.go (partial failure, per-window ack timeout, log outcomes, permission, markers-cleaned, command-rides-mint) exercise Task 3-7 / 3-8 behavior of the same function; not redundant with the Task-3-6 assertions and appropriate for the function's home test file — not over-testing.
  - The emergent current-session behaviors (first→no-op switch; elsewhere→own window; absent→detached) are not unit-asserted, but they arise entirely from already-tested units (spawn argv composition + connector) and from tmux itself; asserting them here would test tmux, not Portal. No under-testing finding.

CODE QUALITY:
- Project conventions: Followed. Small-interface DI with a *Deps struct + package-level seam var and lazy production defaulting (buildOpenBurstDeps), matching the codebase's bootstrapDeps/openDeps pattern. Shared production seams memoised lazily so a fully-injected test never resolves a tmux client. Tests avoid t.Parallel (package-level mutable state), per CLAUDE.md.
- SOLID: Good. connectTrigger (trigger connect), hasMintSurface (pure predicate), SplitTriggerFirst (pure split) are cleanly separated; runOpenBurstWithDeps is the orchestration seam.
- Complexity: Acceptable. runOpenBurstWithDeps is linear with clearly-delineated, heavily-commented phases; branching is shallow.
- Modern idioms: Yes (method-value seams, slices helpers in tests).
- Readability: Good — load-bearing ordering (spawn-before-connect, clean-before-connect, connect-last) is documented in-source with spec references; SplitTriggerFirst vs SplitNetN distinction is explicitly justified as a per-caller convention, not accident.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] cmd/open_burst_run.go:49-50 — the OpenBurstDeps.Logger comment "(Task 3-8 adds the batch summary)" is a now-stale forward-reference (Task 3-8 is complete and the batch summary is emitted through this Logger). Reword to describe current behavior, e.g. "receives the unsupported-terminal line, the batch summary, and per-window records."
