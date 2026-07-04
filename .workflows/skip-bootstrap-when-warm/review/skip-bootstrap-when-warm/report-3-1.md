TASK: skip-bootstrap-when-warm-3-1 — Wire the hooks store and lastCleanup into daemonDeps

ACCEPTANCE CRITERIA (from tick-5a9eb4 / planning line 84):
- daemonDeps has a HookStore *hooks.Store field and a lastCleanup time.Time field.
- stateDaemonCmd.RunE builds the store exactly once via loadHookStore() and assigns it to deps.HookStore.
- A loadHookStore() error at startup is returned from RunE (wrapped) — daemon does not proceed with a nil HookStore that silently disables cleanup. [SUPERSEDED — see below]
- deps.lastCleanup is initialised to time.Now() (daemon-start instant), not the zero time.Time.
- The lister for later cleanup is the existing deps.Client (*tmux.Client satisfies AllPaneLister via ListAllPanes) — no new client field, no new seam.
- go build passes; go test ./cmd/... green; golangci-lint clean.

STATUS: Complete

SPEC CONTEXT:
Spec § Daemon-Owned Hooks Cleanup → Operational contract (specification.md lines 251-259). The _portal-saver daemon becomes the sole automatic home for hooks stale-cleanup (removed from the orchestrator in Phase 1). Dependency wiring is pinned: the store is a *hooks.Store built ONCE at daemon startup via loadHookStore() (→ hooksFilePath → configFilePath("PORTAL_HOOKS_FILE","hooks.json")) and carried on daemonDeps, so it resolves the SAME hooks.json foreground commands mutate (relies on daemon env inheritance — same rule as PORTAL_STATE_DIR). lastCleanup is initialised to daemon-start time so the first cleanup fires ~10s after start, not on the first ~1s idle tick. Client is reused as the AllPaneLister — no new seam.

IMPLEMENTATION:
- Status: Implemented (one AC intentionally superseded by task 4-2)
- Location:
  - cmd/state_daemon.go:16 — internal/hooks added to import block.
  - cmd/state_daemon.go:44 — HookStore *hooks.Store field, with a thorough doc comment matching the task's wording (env-inheritance caveat + lister note).
  - cmd/state_daemon.go:50 — lastCleanup time.Time field, doc comment explains the daemon-START anchor rationale.
  - cmd/state_daemon.go:715-719 — RunE builds the store once via loadHookStore().
  - cmd/state_daemon.go:729-730 — deps literal sets HookStore: hookStore and lastCleanup: time.Now().
  - Client reused as lister (cmd/state_daemon.go:429 maybeRunHookCleanup passes deps.Client); AllPaneLister satisfaction pinned by cmd/bootstrap_production_test.go:126 (var _ AllPaneLister = (*tmux.Client)(nil)). No new field/seam added.
- Notes: The AC "a loadHookStore() error at startup is returned from RunE (wrapped)" is NOT met by the current code — RunE logs a WARN ("load hook store failed; hooks stale-cleanup disabled") and proceeds with a nil store (lines 715-719). This is INTENTIONAL and correct: task skip-bootstrap-when-warm-4-2 ("Decouple daemon capture startup from best-effort hooks-cleanup store resolution", planning line 98, git commit 3c9c20ac) explicitly reversed 3-1's error-surfacing decision so the daemon's PRIMARY job (scrollback capture) is never gated on the best-effort cleanup store. The code comment (lines 704-719) and the test comment (state_daemon_test.go:998 "RECONCILED (analysis cycle 1, task 4-2)") both document the supersession. Final state is internally consistent and reflects the completed plan. Not a defect. The error-surfacing behaviour and its test belong to task 4-2's verification, not 3-1's.

TESTS:
- Status: Adequate
- Location: cmd/state_daemon_test.go:903-1045, TestStateDaemon_HooksCleanupWiring (four subtests), using the withImmediateRun(t) deps-capture seam + withDaemonLockFileReset(t); no daemon subprocess spawned (in-process unit tests, tick loop short-circuited).
- Coverage:
  - "it builds the hook store from loadHookStore at startup" (912) — asserts deps.HookStore != nil.
  - "it initialises lastCleanup to a non-zero start time" (933) — asserts !IsZero() AND within a loose 2s window of time.Now() (bounded loosely to absorb CI jitter, as the task specified).
  - "it resolves the same hooks.json path foreground commands use" (960) — seeds an entry via hooks.NewStore(path).Set through the SAME PORTAL_HOOKS_FILE path, then asserts deps.HookStore.Load() reads it back — a store pointed at a different file would visibly fail. This is the load-bearing "same hooks.json" verification, not a redundant happy-path duplicate.
  - "it disables cleanup with a WARN rather than aborting the daemon on a loadHookStore error" (1008) — the task-4-2 reconciled inverse of 3-1's original error test: blanks $HOME so os.UserHomeDir()/loadHookStore errors, asserts RunE returns nil, deps.HookStore == nil, exactly one WARN under component=daemon.
- Notes: Each subtest verifies a distinct behaviour; would fail if the respective wiring broke. Not over-tested — no redundant assertions, no excessive mocking. Test #4 exercises the 4-2 behaviour but lives inside the 3-1 wiring function; clearly labelled "RECONCILED (analysis cycle 1, task 4-2)" so there is no ambiguity.

CODE QUALITY:
- Project conventions: Followed. Reuses loadHookStore() verbatim (no bespoke hooks.NewStore path), reuses the existing Client as lister (no new seam), import added without a cycle (cmd already imports internal/hooks). Matches golang-dependency-injection / golang-design-patterns conventions.
- SOLID principles: Good. Single struct field addition, no new responsibility leaked; the store is built at the composition root (RunE) and injected via daemonDeps.
- Complexity: Low. Two struct fields plus a five-line build-store block in RunE.
- Modern idioms: Yes. Standard fmt.Errorf wrap style, time.Now()/time.Since.
- Readability: Good. Doc comments are thorough and accurately describe the env-inheritance caveat and the daemon-START anchor rationale.
- Issues: The struct-level header comment (lines 26-29) enumerates the tick-mutable fields (HashMap, PrevIndex, LastSaveAt) but omits lastCleanup, which maybeRunHookCleanup mutates every cleanup cadence — a minor doc-completeness gap. Minor naming asymmetry: lastCleanup is unexported while its sibling timing anchor LastSaveAt is exported (both loop-mutated throttle anchors on the same package-private struct).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] cmd/state_daemon.go:26-29 — the struct-level header comment lists the tick-mutable fields (HashMap, PrevIndex, LastSaveAt) but omits lastCleanup, which maybeRunHookCleanup rewrites each cleanup cadence (line 432). Add lastCleanup to that enumeration so the comment stays accurate.
- [quickfix] cmd/state_daemon.go:50 — lastCleanup is unexported while its sibling loop-mutated timing anchor LastSaveAt (line 54) is exported; for consistency rename lastCleanup → LastCleanup (touches references at lines 400/409/418/426/432/727/730 and in state_daemon_test.go:950/955). Low priority — cosmetic on a package-private struct; access is unaffected either way.
