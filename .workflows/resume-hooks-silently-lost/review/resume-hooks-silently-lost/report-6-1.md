TASK: resume-hooks-silently-lost-6-1 — Give The Stale-Hook Sweep One Decline Ladder And One Outcome Value

ACCEPTANCE CRITERIA:
1. No guard decision is spelled out in more than one function; adding a fourth guard requires editing one place.
2. All five decline paths populate the outcome's reason and emit under the `hooks` component.
3. `pruneDoctorStaleHooks` prints exactly one line for every possible outcome of the sweep, including both read failures.
4. The seam interface embeds `PaneHookLister` and `state.RestoringChecker` rather than restating either, and carries one copy of the enumeration doc paragraph.
5. `runHookStaleCleanup` takes no more than four parameters.
6. The three `Skipped stale hook prune:` phrases match the specification verbatim: `restore in progress` / `hooks.json is locked` / `could not read live panes`.

STATUS: issues_found

SPEC CONTEXT:
Spec §5 ("Stale Cleanup") governs the reaper: §5.1 fixes the `Skipped stale hook prune:` copy, §5.2 makes deletion shape-aware with one implementation of the rule, §5.4 holds the restore-marker and empty-live-set stand-downs. Two later signed-off corrigenda move the target this task was written against:

- **2026-09-01**: §5.1's third line, `could not read live panes`, is **withdrawn, not reassigned** — the guard it covered is a *successful* read that returned no panes, so the empty read renders `live pane list came back empty` and the failed enumeration renders `could not enumerate live panes`. `notEvaluableDetails` also gains its `lock-timeout` entry.
- **2026-09-04**: the decline set grows from five reasons to **six**, adding `marker-read-failed` with its own phrase (`could not read the restore marker`), because `doctor` is bootstrap-exempt and a machine with no tmux server is the ordinary path into that branch.

AC6's literal wording is therefore superseded by the record: the current phrases (`restore in progress` / `hooks.json is locked` / `could not enumerate live panes` / `live pane list came back empty`) are what the corrigenda prescribe, and the withdrawn phrase is pinned *absent* by `cmd/doctor_stand_down_copy_test.go:422-435`. Not a finding.

IMPLEMENTATION:
- Status: Implemented, then re-homed. Task 6-1 landed at `0eb1d55d` in `cmd/run_hook_stale_cleanup.go`; task 9-12 (`a4898f41`) moved the whole cycle to `internal/hooksweep` with its emission and vocabulary intact. Judged against the current tree, per "the code is the source of truth".
- Location:
  - `internal/hooksweep/sweep.go:71` `StalenessStandDown`, `:85` `JudgeAgainstLivePanes` — the one decline ladder, reached by both consumers.
  - `internal/hooksweep/sweep.go:152` `Run(reader, store) (Outcome, error)` — two parameters, one returned value.
  - `internal/hooksweep/sweep.go:45-60` `View` / `Outcome`; `internal/hooksweep/reason.go:18-42` the closed `Reason` set plus `Reasons`.
  - `internal/hooksweep/standdown.go:35-49` the single emission site over `logger = log.For("hooks")` (`sweep.go:14`).
  - `cmd/doctor.go:197-212` `pruneDoctorStaleHooks`; `:363-392` `checkStaleHooks`; `:220-254` the two phrase vocabularies.
  - `cmd/state_daemon.go:214` the daemon discards the outcome, as the task prescribes.
- Notes, criterion by criterion:
  - **AC1 — met.** Both the reaper (`sweep.go:155`, `:139`) and the diagnostic (`doctor.go:375`, `:379`) take `StalenessStandDown` then `JudgeAgainstLivePanes`; neither restates a guard. The deliberate ordering difference is preserved (the sweep gates on the marker *before* the store read at `sweep.go:155`; the check loads first at `doctor.go:370`) and is pinned by `internal/hooksweep/sweep_test.go:289` ("it skips before loading the store").
  - **AC2 — met, and widened to six.** Every decline reaches `standDownOutcome` (`sweep.go:199`), which emits and sets `Outcome.DeclineReason`: `restoring`/`marker-read-failed` via `StalenessStandDown`, `pane-read-failed`/`empty-pane-read` via `declinedError` unwrapped at `sweep.go:179`, `lock-timeout` at `:181`, `store-read-failed` at `:186`. All ride the `hooks` component.
  - **AC3 — met in substance.** Every decline (`doctor.go:209`) and every unclassified failure (`:202`) reaches a line; both read failures are covered by rows in `cmd/doctor_stand_down_copy_test.go:80-101`, each of which asserts the rendered `--fix` line via `assertStandDownRepair` (`:313`). Two paths still print nothing: a sweep that ran and removed nothing, and `errNothingPersisted` (`sweep.go:177`) — neither is a decline, and printing for them would be noise. A nil `deps.HookStore` (`doctor.go:198`) also prints nothing, but that is a `loadHookStore` failure rather than an outcome of the sweep, and the post-repair diagnosis prints `stale hooks: could not read hooks.json` immediately below it.
  - **AC4 — the embedding clause is met, the one-copy clause is not.** `hooksweep.Reader` (`sweep.go:37-40`) embeds `PaneHookLister` and `state.RestoringChecker` rather than restating either. But the enumeration doc paragraph now exists twice, verbatim, over two declarations of the same one-method interface — see FINDINGS.
  - **AC5 — met.** `Run` takes two parameters.
  - **AC6 — superseded.** See SPEC CONTEXT. `restoreStandDownPhrase` is `"restore in progress"` (`doctor.go:221`), the hedge the task named is gone, and `lockStandDownPhrase` is `"hooks.json is locked"` (`:225`).
  - Do-item 5's rename is done: no `AllPaneLister` survives anywhere in the tree.

TESTS:
- Status: Adequate — thorough, and each file's subject is distinct.
- Coverage:
  - Every decline reason yields a distinct non-empty `DeclineReason`: `internal/hooksweep/sweep_test.go:72,92,272,340,359,423` plus the per-reason table at `internal/hooksweep/sweep_move_test.go:47-80`.
  - `doctor --fix` stdout on the two previously-silent read failures: `cmd/doctor_stand_down_copy_test.go:80-101` (rows `hooks.json unreadable` and `pane enumeration failed`), each driven end-to-end through `runDoctorWith(t, deps, "--fix")` at `:319` and asserted on the whole rendered line.
  - Reaper/diagnostic verdict parity over exactly the table the task named: `cmd/hook_prune_verdict_parity_test.go:21-24` (empty rows with entries present, enumeration error, restore marker set, live rows with unstamped panes), comparing `checkStaleHooks` against `hooksweep.Run` on the same reader.
  - Exact-equality on the restore line: `cmd/doctor_fix_hook_prune_move_test.go:35` asserts `pruneDoctorStaleHooks` writes exactly `"Skipped stale hook prune: restore in progress\n"`, with the file deliberately spelling the literal rather than composing it from the vocabulary.
  - One-component / one-record-per-event: `cmd/hook_prune_one_component_test.go:41-107` compares the daemon's and `doctor --fix`'s emitted `(component, message)` sequences for equality and pins exactly one WARN for an unclassified failure; `cmd/hook_prune_single_report_test.go:18-66` pins one WARN per decline with none on the injected daemon logger.
  - Exhaustiveness is structural, not enumerated by hand: `internal/hooksweep/reason_enumeration_guard_test.go:22` fails a `Reason`-typed const omitted from `Reasons`, and `cmd/doctor_stand_down_copy_test.go:342-358` fails a declared reason with no copy row — which together are what make AC1's "adding a fourth guard requires editing one place" hold rather than being asserted in prose.
- Notes: not over-tested for the risk. The nearest overlap is `sweep_move_test.go:47-80` re-asserting per-reason declines that `sweep_test.go` already covers, but its stated subject is the cross-package contract after the re-homing, which the in-package suite does not exercise. Test placement respects the lane rule — every file here is unit-lane and none builds or execs a portal binary.

CODE QUALITY:
- Project conventions: Followed. One `log.For("hooks")` binding per package (`sweep.go:14`), consistent with CLAUDE.md's bind-once-per-package rule and with `theme` already spanning two packages; the closed `op=clean-stale-skipped` vocabulary is honoured at `standdown.go:12-16`; no new component or attr key is invented.
- SOLID principles: Good. `Reader` is a 2-method seam composed from two 1-method ones; emission is owned solely by the cycle, and both callers keep only their own rendering.
- Complexity: Low. `declinedSweep` (`sweep.go:174-194`) is a flat classification switch; `Run` is three statements plus a guard.
- Modern idioms: Yes — `errors.As` on a typed `declinedError` carrying the stand-down rather than a reason variable travelling beside the error.
- Readability: Good. `View.PaneRows`'s "meaningful only where `Enumerated` is set" caveat (`sweep.go:47-48`) holds at the one site that reads it (`doctor.go:384`), which is reachable only past a non-declined view.
- Issues: the duplicated interface declaration below.

BLOCKING ISSUES:
- None.

FINDINGS:
- [in-scope] [contained] cmd/hooks.go:21 — `PaneHookLister` is declared twice with a byte-identical four-line doc paragraph, at `cmd/hooks.go:21-27` and `internal/hooksweep/sweep.go:26-32` (verified identical by diff). This is the exact condition the task set out to remove — its Do-item 5 named "both declarations and both copies of the same … doc paragraph must be edited together" — and 6-1 did remove it (`0eb1d55d` left one declaration, embedded by `staleSweepReader`); the phase-9 re-homing at `a4898f41` reintroduced it by copying the interface into the new package without touching `cmd/hooks.go`. Fix: keep the `hooksweep` declaration as the single home and alias the `cmd` one — `type PaneHookLister = hooksweep.PaneHookLister` — dropping the duplicated paragraph. The edit is safe as written: `cmd` already imports `internal/hooksweep` (`cmd/doctor.go:12`) and `hooksweep` imports only `hooks`/`log`/`state`/`tmux`, so there is no cycle; `cmd/hooks.go` keeps its `tmux` import (still used at `:18`, `:31`, `:54`, `:58`); and an alias leaves every consumer typechecking unchanged, including the `var _ PaneHookLister = (*tmux.Client)(nil)` assertion at `cmd/hooks.go:36`, `HooksDeps.PaneLister` at `:44`, and the test fakes at `cmd/hookkey_vocabulary_test.go:84` and `cmd/hooks_seams_test.go:55`. — FAILS: the paragraph states the contract that the *row count* answers whether the tmux read succeeded while the *non-empty tokens* answer which panes are protected — the distinction the empty-pane-read guard (`internal/hooksweep/sweep.go:98`) turns on. With two copies, a change to `tmux.ListAllPaneHookKeys`'s contract can be recorded in one and not the other, leaving a reader of the stale copy conflating the two questions this work unit exists to keep apart. The whole remedy is declaration/comment text, so this is non-blocking.
