TASK: resume-hooks-silently-lost-7-9 — Integration-Lane Timing Budgets Fail On A Normally-Loaded Developer Machine (tick-4351fc)

ACCEPTANCE CRITERIA:
- No `6 * time.Second` convergence constant survives in `cmd/bootstrap`.
- Every one of the seven sites reaches its assertion through the shared progress-tolerant wait.
- The wait extends its deadline on an observed change and fails at a stated absolute ceiling.
- A process making no progress fails within the ceiling, with the last observation in the message.
- The seven tests pass on a machine under load comparable to the measured ~18 load average on 10 cores.
- No assertion's subject changes — the same end states are still asserted, only the waiting differs.

STATUS: complete

SPEC CONTEXT:
This is a phase-7 implementation-analysis task, not specified work — its authority is its own body
(per the shared verifier context, phases 6-10 are consolidation/quality cycles the implementation
generated). The governing project convention is CLAUDE.md's "Cleaning up after a test run" section:
there is no CI, every run competes with the developer's real workload, and leaked/ambient load
"manufactures flakes and misattributes them to the code under test". A fixed wall-clock budget with
~2% headroom is exactly that failure mode, which is what this task removes.

IMPLEMENTATION:
- Status: Implemented (delivered at 75cfc86d; the helper was subsequently re-homed by a later
  phase-9 task — see Notes)
- Location:
  - `internal/harnesstest/progress.go:9-13` — `DefaultProgressStall`/`Ceiling`/`Tick`.
  - `internal/harnesstest/progress.go:18-27` — `ProgressWait{Stall, Ceiling, Tick}`, the two-budget
    separation.
  - `internal/harnesstest/progress.go:45-56` — `ProgressResult[T]` + its `String()`, which renders
    `last=`/`reached=`/`stalled=`/`elapsed=`/`changes=` for the caller's failure message.
  - `internal/harnesstest/progress.go:62-97` — `AwaitProgress`; deadline extension at `:78`
    (`stallDeadline = time.Now().Add(w.Stall)` on every observed change), absolute ceiling at
    `:86`, stall verdict at `:90`.
  - `cmd/bootstrap/orphan_sweep_integration_test.go:34-38` — `pgrepConvergenceWait`
    (Stall 10s / Ceiling 45s / Tick 50ms); `:304-308` — `waitForPgrepCount` now returns a
    `ProgressResult[pgrepObservation]` instead of a bool+timeout; `:288-302` — the comparable
    observation carrying count, pids and the enumeration error.
  - The six 6s constants are gone and every call site routes through `waitForPgrepCount`:
    `composition_abc_integration_test.go:46,68`, `composition_e2e_convergence_integration_test.go:42`,
    `composition_e2e_f_observables_integration_test.go:31`,
    `composition_e2e_fresh_acquire_integration_test.go:28`,
    `composition_e2e_self_eject_integration_test.go:44`, `upgrade_path_integration_test.go:68`
    (plus `composition_e2e_harness_integration_test.go:170`, an eighth site converted in the same
    pass when its own 3s `compositePreStatePGrepTimeout` was deleted).
  - `cmd/state_daemon_integration_test.go:45-49` — `panePopulationWait` (Stall 10s / Ceiling 120s)
    replacing the fixed 10s budget; `:246-256` — the per-pane `AwaitProgress` observing accumulated
    line count. The sub-2s aggregate skip is kept verbatim at `:110-115`.
  - Rename done: `TestCompositeBootstrap_ConvergesPgrepToOneWithin6s` →
    `TestCompositeBootstrap_ConvergesPgrepToOneDaemon`
    (`composition_e2e_convergence_integration_test.go:18`).
- Notes:
  - Criterion 1 verified by scan: no `6 * time.Second` remains anywhere in `cmd/bootstrap`. The
    three surviving occurrences in the tree are `internal/harnesstest/progress.go:10`
    (a default), `internal/tmux/portal_saver_endstate_integration_test.go:31` (a `Stall`, not a
    fixed budget) and `cmd/state_daemon_self_supervision_integration_test.go:30` — none in scope.
  - Criterion 6 (subject unchanged) holds. Each converted site still asserts the same end states —
    e.g. `composition_e2e_convergence_integration_test.go:59-73` still pins exactly one survivor and
    that the survivor is the saver-pane PID, and `:75-92` still pins the forbidden log strings. The
    one thing deliberately dropped is the implicit `remaining := budget - time.Since(start)` bound on
    the bootstrap slice's own wall time, which is the very structure the task directed be removed;
    it is recorded as intended-not-a-defect in the task's own manifest entry.
  - **Criterion 5 is recorded unmet, deliberately and correctly.** The executor's measurement
    disproved the task's premise: the composite failures are bimodal (36-42ms or never; `changes=0`
    for a full 60s stall in 3 of 24 runs), i.e. a product race in the saver respawn, not a near-miss
    budget. It is logged as its own work at
    `.workflows/.inbox/bugs/2026-08-31--saver-respawn-leaves-no-daemon.md` and captured in the
    manifest for this task. Recording it unmet rather than weakening the criterion is the right
    call — this is a divergence with a stated, evidenced reason, not a loss.
  - Home drift, and it is sound: the task said "beside `tmuxtest.PollUntil`", and the commit put
    both `AwaitProgress` and `PollUntil` there. Phase 9 (`analysis-tasks-c3.md`) then moved the pair
    into the neutral `internal/harnesstest` leaf, on the reasoning that neither references anything
    tmux-related while three packages now depend on them. No stale `tmuxtest.AwaitProgress` /
    `tmuxtest.PollUntil` reference survives anywhere in the tree.

TESTS:
- Status: Adequate
- Coverage: `internal/harnesstest/progress_test.go:11-123` — five subtests, one per property:
  reach-fast (`:12`, asserts `Reached`, `Last == 3`, and elapsed well under the stall budget),
  deadline extension (`:35`, asserts `!Reached && !Stalled && Changes > 0` with
  `Elapsed >= 3*Stall` under a permanently-changing observation, bounded by `2*Ceiling`),
  stall inside the ceiling (`:69`, asserts `Stalled`, `Changes == 0`, `Stall <= Elapsed < Ceiling`),
  last-observation reporting (`:93`, A-B-B sequence, asserts `Last == "two"` and that `String()`
  carries `last=`/`reached=false`/`stalled=true`/`changes=`), and zero-value defaults (`:116`).
  The four named micro-acceptance tests all map onto these; the two integration-level ones map onto
  the renamed `TestCompositeBootstrap_ConvergesPgrepToOneDaemon` and
  `TestDaemon_MidTickSIGHUP_ExitsWithinBoundedWindow`.
- Notes:
  - The `Changes` counter is pinned from both sides (`>0` under a changing observation, `==0` under
    a constant one), which closes the gap the fix-tracking round opened
    (`fix-tracking-resume-hooks-silently-lost-7-9.md:4-22`).
  - The reviewer-banked concern that `progress_test.go` reintroduced a fixed wall-clock upper bound
    (`Ceiling + 300ms`) was addressed: the current assertion is `2*wait.Ceiling` at
    `progress_test.go:63`, with the reasoning stated in-source at `:58-62`.
  - Not over-tested: no two subtests pin the same property, and none reaches into unexported state
    (the suite is `package harnesstest_test`).
  - Lane placement is correct — the new unit tests are untagged and hermetic (no tmux, no daemon,
    no binary); every converted call site is already `//go:build integration`.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()` (CLAUDE.md prohibits it). `harnesstest` stays
  stdlib-only, which `internal/harnesstest/leaf_guard_test.go:16-20` enforces across both lanes.
  `AwaitProgress` takes the shared `harnesstest.TestingT` rather than `*testing.T`, matching the
  package's stated contract. No production code imports the helper. The CLAUDE.md `harnesstest`
  architecture row describes `ProgressWait`/`ProgressResult[T]` accurately.
- SOLID principles: Good. The wait knows nothing about its subject — `observe`/`reached` are
  injected, the observation type is a parameter, and it deliberately reports rather than fails, so
  each caller owns its own diagnostic. `pgrepObservation` and `panePIDObservation` implement
  `String()` so `%v` on `Last` renders them without the helper knowing anything about pgrep or tmux.
- Complexity: Low. One loop, three exits (reached / ceiling / stall), no goroutines, no channels,
  no time-source seam to get wrong.
- Modern idioms: Yes — generics over a `comparable` observation, `for first := true; ; first = false`
  rather than a duplicated pre-loop probe, `%s` on a `Stringer` result.
- Readability: Good. Every non-obvious budget carries a comment saying why the two budgets are
  separate rather than restating the field
  (`progress.go:15-17`, `orphan_sweep_integration_test.go:30-33`,
  `state_daemon_integration_test.go:41-44`). All three read true against the code beneath them —
  the two that previously denied the ceiling's existence were corrected in the fix round.
- Issues: None found. Checked specifically for the two ways this shape can mask a real failure and
  neither is present: (a) a flapping observation still fails at the ceiling because `Changes` feeds
  no control flow; (b) a spuriously-changing observation cannot burn the ceiling here, because
  `state.PgrepPortalDaemons` (`internal/state/pgrep.go:40-51`) preserves pgrep's ascending pid
  order, so a stable population renders a stable `Pids` string.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
