TASK: open-with-forced-filter-4-5 (tick-781c51) — Pin the search decision behind the whole bootstrap against a real cold server

ACCEPTANCE CRITERIA:
- The decision closure is invoked exactly once across the whole boot
- All ten real step events, and the terminal complete event, were delivered before it ran — it is never invoked at the end of restore (step 6)
- `state.IsRestoringSet(client)` is false at the instant it runs
- The closure's own live read returns the restored sessions
- The model is on `PageLoading` after every step event and until the decision resolves
- On the single-match term the model quits with `Selected()` set, `SearchAttached()` true, `ActivePage()` still `PageLoading`
- On the shared-fragment term the closure answers with no session and the model transitions to `PageSessions`
- `FatalError()` is nil on both runs and no `BootstrapFatalMsg` is observed
- The fixture touches only its own socket, state dir and spawned daemon; the suite carries `//go:build integration`
- `go test -tags integration -p 1 ./...` passes, and `go test ./...` is unaffected

STATUS: complete

SPEC CONTEXT:
§3.4 ("When the count is taken") is the governing rule: on a cold server the sigil takes the picker's concurrent-bootstrap path, so K is evaluated "once that bootstrap has run to completion — every step of it, not merely the restore that reconstructs the saved sessions", because acting at the end of restore "would replace the process mid-bootstrap and abandon the steps that follow it — among them the clearing of the `@portal-restoring` marker". §7.1/§7.6 classify a sigil invocation as a picker invocation taking the concurrent bootstrap and the honest loading page, with the concurrent machinery itself unchanged. The task's subject is therefore an *ordering* property against a real orchestrator — the classification logic itself (`searchDecision`, `searchMatches`, `PickerSessions`) is pinned by the Phase 2/3 unit suites.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/concurrent_search_decision_integration_test.go` (whole file, 259 lines; added by 8d06028d8, updated by b1b809fc4 when task 12-3 moved the decision off the render goroutine). No production file was touched by this task — the plan's "Do" prescribes the fixture alone.
- Notes:
  - `driveSearchColdBoot` (`cmd/concurrent_search_decision_integration_test.go:54`) reuses `setupConcurrentColdBootEnv`, `buildConcurrentColdBootOrchestrator` and `concurrentBootDrainBudget` from `cmd/concurrent_coldboot_integration_test.go:28`, `:104` and `:24` exactly as the plan directs.
  - The pipe is the production one (`newBootstrapProgressPipe` / `pipe.start`, `cmd/bootstrap_progress.go:47`, `:55`) and the model is the production constructor (`tui.Build` with `Search: &tui.SearchForm{Term, Decide}`, `internal/tui/build.go:100`, `:132`), so the wiring under test is real end-to-end.
  - The step-classification (`m.RestoreM == 0 && m.RestoreN == 0`, line 105) is correct against the declared contract: `cmd/bootstrap/progress_emitter.go:9-10` states RestoreN/RestoreM are zero except on the restore step's per-session events, which set RestoreM > 0.
  - The two-consumer hazard is handled as the plan requires: the model's returned command is discarded for every message except the terminal complete (lines 118-126), and that one command is the single `searchDecisionCmd` — `dismissLoadingGate` (`internal/tui/model.go:1509`) returns it unbatched when `searchDecide != nil`, so `cmd()` yields exactly one `searchDecisionMsg`. The pipe's channel is buffered at 64 (`cmd/bootstrap_progress.go:12`, `:49`) and the orchestrator goroutine's last act is the Done send before `close`, so abandoning the loop the instant the decision answers leaks no goroutine.
  - The seeding works out as the fixture claims. `restoretest.SeedSessionsJSON` writes panes with no directory (`internal/restoretest/sessions_json.go:50-65`) and `internal/restore` never stamps `@portal-dir` (no occurrence in `internal/restore/`), so `Session.Dir` is empty and `resolver.SearchFields` (`internal/resolver/search.go:9`) reduces matching to the name alone — "solo" → 1, "shared" → 2. `parseSessionList` drops underscore-prefixed sessions (`internal/tmux/tmux.go:188-195`), so `_portal-bootstrap`/`_portal-saver` cannot perturb either count.
  - `tmuxtest.New` starts no server (`internal/tmuxtest/socket.go`, `New` only mkdtemps and registers cleanup), so step 1 is a genuine cold start — the "real cold server" premise holds.

TESTS:
- Status: Adequate
- Coverage: All seven micro-acceptance tests named in the plan are present as named subtests — "it takes the search decision only after every bootstrap step" (line 174), "…with the restoring marker cleared" (185), "it sees the restored sessions in the decision's own read" (195), "it invokes the decision exactly once" (207, 246), "it attaches the single match without painting the picker" (213), "it opens the picker when two sessions match" (234) — plus the per-step loading-page pin in `assertStandingOnLoadingPage` (line 144), which asserts both `ActivePage() == PageLoading` and `probe.calls == 0` after every real step event. `assertCleanSearchBoot` (156) covers `FatalError()` nil and no `BootstrapFatalMsg`.
- Notes:
  - The fixture fails loudly rather than hanging: the `concurrentBootDrainBudget` deadline arm (line 131) fatals with `portaltest.ReadPortalLogSafe(stateDir)` attached, matching the sibling suite's contract.
  - Would it fail if the feature broke? Yes, on every route I traced. A decision dispatched at the end of restore never runs (the harness discards non-complete commands) and `searchDecideInFlight` then makes the complete-event gate return nil, so the run ends with `calls == 0` — caught by both the "exactly once" and "after every bootstrap step" subtests. A decision run inline during a step is caught directly by `assertStandingOnLoadingPage`. A marker leak past step 8 is caught by `probe.restoring`. A pre-restore read is caught by the `slices.Contains` sweep over the seeds.
  - Not over-tested: two full boots is the minimum the K = 1 / K ≥ 2 split needs, and the duplicated "exactly once" / "after every step" subtests on the second run are assertions over a *different* boot, not restatements of the first.
  - Isolation satisfies the lane rules in full: `//go:build integration` (line 1); `setupConcurrentColdBootEnv` runs `portaltest.IsolateStateForTest` + `t.Setenv("PORTAL_STATE_DIR", …)`, registers `RegisterStateDirTeardownGuard` *before* `tmuxtest.New`, uses a disposable `-S` socket, and reaps the saver daemon in cleanup (`cmd/concurrent_coldboot_integration_test.go:37-54`). The binary is staged through `restoretest.BuildPortalBinaryStable` (`cmd/reattach_integration_test.go:44`), so the `-tags integration` pgrep sandbox is compiled in. The new fixture enumerates no process and signals nothing — it adds no `pgrep`/`kill` of its own. No `t.Parallel()` anywhere, per the cmd package's package-level-mutable-state rule.
  - The probe hand-rolls its classification (`searchMatches` on the raw `ListSessionsProbe` result, lines 69-80) rather than calling `searchDecision`, so it does not exercise `searchCandidates`/`PickerSessions`. That is what the plan prescribes and is right for this task's subject — the ordering — with the classification itself owned by the Phase 2/3 unit suites; the fixture is not attached to any session, so the excluded step is inert here regardless.

CODE QUALITY:
- Project conventions: Followed. Integration build tag, no `t.Parallel()`, `portaltest`/`restoretest`/`tmuxtest` helpers used by name rather than re-rolled, `t.Helper()` on every helper, failure messages that state the invariant rather than the expression.
- SOLID principles: Good. The fixture separates driving (`driveSearchColdBoot`), the mid-boot invariant (`assertStandingOnLoadingPage`) and the terminal invariant (`assertCleanSearchBoot`); the probe record is a plain value struct.
- Complexity: Acceptable. The drive loop carries one select, one type switch and one conditional command run — the irreducible shape of pumping a channel into a Bubble Tea model by hand.
- Modern idioms: Yes — `slices.Contains`, `errors`-free value plumbing, buffered `got` channel so the deadline arm cannot strand the receiving goroutine.
- Readability: Good. Every non-obvious decision carries its reason: why only the complete event's command is run (48-53), why zero-counter ticks alone are steps (103-104), why one seeding drives both counts (19-20).
- Comment accuracy: Every comment in the file holds against the code and against the machinery it describes — I checked the RestoreM/RestoreN claim against `cmd/bootstrap/progress_emitter.go:9-10` and the two-consumer claim against `bootstrapProgressPipe.receiver`. The header comment was correctly re-written by task 12-3 when the harness started running the complete event's command.
- Issues: None that clear the bar.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test -tags integration -p 1 ./...` passes, and `go test ./...` is unaffected" — needs the integration lane run serially, plus a unit-lane run to confirm the new file stays excluded (reading settles the build tag and the absence of identifier collisions in `package cmd`; only the run settles compilation and pass/fail).
- "The decision closure is invoked exactly once across the whole boot"; "All ten real step events, and the terminal complete event, were delivered before it ran"; "`state.IsRestoringSet(client)` is false at the instant it runs"; "The closure's own live read returns the restored sessions"; "The model is on `PageLoading` after every step event"; "On the single-match term the model quits with `Selected()` set, `SearchAttached()` true, and `ActivePage()` still `PageLoading`"; "On the shared-fragment term … the model transitions to `PageSessions`"; "`FatalError()` is nil on both runs and no `BootstrapFatalMsg` is observed" — these are statements about a live cold boot against real tmux. Reading settles that each is asserted, correctly targeted and would fail on the regression it names; observing them requires executing `go test -tags integration -p 1 ./cmd -run TestConcurrentColdBoot_SearchDecision`.
