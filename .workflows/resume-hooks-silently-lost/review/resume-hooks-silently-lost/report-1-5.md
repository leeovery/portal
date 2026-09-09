TASK: resume-hooks-silently-lost-1-5 — The Stale-Hook Check Tells The Truth (`checkStaleHooks` reads `@portal-restoring` before the live enumeration and reports not-evaluable instead of counting)

ACCEPTANCE CRITERIA:
1. With the marker set, the stale-hooks check reports `checkNotEvaluable` with a restore-window detail and no count is computed
2. A failed marker read produces the identical result
3. The marker read is ordered before the live enumeration, before the empty-live-set branch and before the stale count
4. Not-evaluable never drives the exit code — an otherwise-healthy install in a restore window still exits 0
5. Outside a restore window, a genuinely stale token-shaped key still fails the check with its existing `N stale hook entr…` detail, even with retained non-token-shaped entries alongside
6. With no server running, the marker read fails and the check reports not evaluable, starting no tmux server
7. After a `--fix` stand-down, the post-repair diagnosis reports the check not evaluable rather than counting, and the exit code comes from the rest of the diagnosis

STATUS: complete

SPEC CONTEXT:
§5.4 (specification.md:296-311) requires both readers of hook staleness to take the same reading of `@portal-restoring`: the sweep must not delete what it cannot judge, and `checkStaleHooks` must not count during the window in which restored panes carry a full pane list and no tokens — where every token-shaped key would read as stale on the one command whose job is to report whether hooks were lost. §5.2 (:279) keeps `portal doctor`'s "exit 0 iff all pass" contract with retained old-format entries present. §9.2 (:504) names the unit-lane test row for the sweep-and-check stand-down.

Two corrigenda govern this task's own wording decision and are authoritative over both the spec body and the task text:
- 2026-09-04 (:568): the cycle declines for six reasons, not five — a failed marker read takes the same *posture* as a set marker but under its own reason and its own phrase (`could not read the restore marker`), because a read-only `portal doctor` on a machine with no server is the ordinary path into that branch and a phrase asserting a restore would be a fresh false statement.
- 2026-09-01 (:552): the withdrawn-phrase precedent the above applies.

IMPLEMENTATION:
- Status: Implemented (delivered past the task text, under a recorded corrigendum)
- Location: `cmd/doctor.go:363-392` (`checkStaleHooks`), `cmd/doctor.go:350-354` (`staleHooksNotEvaluable`), `cmd/doctor.go:219-262` (the shared phrase consts, the two surface vocabularies and `phraseFor`), `internal/hooksweep/sweep.go:72-79` (`StalenessStandDown` — the single home of the failed-read-counts-as-set rule), `internal/state/markers.go:108-110` (`RestoreWindowActive`), `internal/hooksweep/reason.go:19-41` (the closed reason vocabulary and its enumerable `Reasons`).
- Notes:
  - The stand-down sits exactly where the task ordered it: after the `store == nil` (`:367`) and `store.Load` (`:370-373`) guards, at `:375`, and before `hooksweep.JudgeAgainstLivePanes` (`:379`), the empty-live-set branch (`:384`) and the `hooks.StaleKeys` count (`:387`). A declined reading returns before `ListAllPaneHookKeys` is called at all, so no count exists to be overridden.
  - The reading is taken through the same widened seam as the enumeration: `DoctorDeps.HookLister` is a `hooksweep.Reader` (`cmd/doctor.go:64`), which composes `PaneHookLister` with `state.RestoringChecker` (`internal/hooksweep/sweep.go:36-39`). No nil guard was added for `lister`, as specified.
  - Sanctioned divergence from criteria 1 and 2 — the detail wording. The task fixed `restore may be in progress (not evaluable)` for both a set marker and a failed read; the delivered code renders `restore in progress (not evaluable)` for `ReasonRestoring` and `could not read the restore marker` for `ReasonMarkerReadFailed` (`cmd/doctor.go:220-221`, `:247-249`). This is not drift: the 2026-09-04 corrigendum records the split, and the reason the task hedged the phrase ("a failed marker read counts as set, so a down server takes this branch") is precisely what the split removes — with the failed read carrying its own reason, the restore phrase is only rendered when the marker genuinely reads as set. Both branches still report `checkNotEvaluable` with no count, which is the substance the criteria protect. The posture criterion 2 was written to guarantee is preserved; only the printed prose is separated.
  - No duplication of the restore-window rule: `state.IsRestoringSet` is reached from one place for hook-staleness work (`internal/hooksweep/sweep.go:72`), so the sweep and the diagnosis structurally cannot disagree about the window. `StaleKeys` has exactly two callers (`cmd/doctor.go:387`, `internal/hooks/store.go:332`), both gated by that reading.
  - The diagnosis emits no log line for its stand-down — `StandDown.emit` is called only from `standDownOutcome` in the sweep (`internal/hooksweep/sweep.go:196-199`) — which matches the documented rule that a diagnosis that pruned nothing must not claim a stand-down in the reaper's vocabulary.
  - `doctor` remains bootstrap-exempt (`cmd/root.go:26`), and the added read is `TryGetServerOption` → `GetServerOption` → `show-options -s`, which does not carry tmux's start-server flag, so criterion 6's "starting no tmux server" is unaffected by the new read.

TESTS:
- Status: Adequate
- Coverage:
  - Criterion 1: `cmd/doctor_test.go:1070-1076` ("it reports not evaluable while the restore marker is set"), asserting through `assertRestoreWindowResult` (`:1163-1171`).
  - Criterion 2 (as superseded): `cmd/doctor_test.go:1080-1088` ("it reports a failed marker read as its own reason"), which also pins zero enumerations.
  - Criterion 3: `cmd/doctor_test.go:1086-1091` (marker before the empty-live-set branch) and `:1096-1105` ("it reads the marker before counting"), the latter asserting `lister.calls == 0` — the rendered result alone cannot distinguish a read that came first from one that overrode a count, and the call counter is what closes that.
  - Criterion 4: `cmd/doctor_test.go:1133-1147` ("it keeps portal doctor at exit 0 in a restore window"), Executing the real command body over a healthy fixture; backed by `doctorUnhealthy` (`cmd/doctor.go:570-577`) counting only `checkFail`/`checkUnknown`.
  - Criterion 5: `cmd/doctor_test.go:1120-1131`, one reapable token beside two `UnjudgeableSeed*` old-format keys, asserting `1 stale hook entry`.
  - Criterion 6: `cmd/doctor_test.go:1107-1118`, both seams answering a down-server error. "Starts no server" is pinned separately by `cmd/doctor_fix_theme_test.go:348-356` (zero bootstrap-orchestrator runs on `doctor --fix`).
  - Criterion 7: `cmd/doctor_stand_down_copy_test.go` — the "restore window" row (`:57-67`) with `postRepairNotEvaluable: true` runs through `assertStandDownRepair` (`:307-334`), which asserts the skipped-prune line, the post-repair not-evaluable line, that no `stale hook entr` count appears, and that hooks.json was left byte-identical; `"it leaves the exit code to the post-repair diagnosis for every stand-down"` (`:440-468`) pins both exit codes per reason.
  - The exact user-visible strings are pinned once, as whole rendered lines, in the copy table (`:64-65`, `:73-74`) via `renderStaleHooksLine`; the behaviour suites assert through `phraseFor(notEvaluableDetails, …)` so a re-wording is one edit. That split is deliberate and holds — the copy is not self-fulfilling because the table holds the literal.
  - Completeness is guarded rather than asserted in prose: `cmd/doctor_stand_down_phrase_guard_test.go:53-69` fails a reason missing from either vocabulary or an undeclared key in one, `cmd/doctor_stand_down_copy_test.go:340-378` fails a reason with no copy case and two reasons that render the same phrase, and `internal/hooks/cleanstale_staleness_guard_test.go:26` pins that `checkStaleHooks` routes its count through `hooks.StaleKeys`.
- Notes: There is overlap between `TestDoctorStaleHooksCheck`'s restore subtests and the copy table's restoring row, but they assert different things (ordering and exit code vs. the rendered line and the untouched file), so this is not redundant coverage. Nothing here is over-tested; the ordering assertions would fail if the stand-down were moved after the enumeration, and the exit-code assertions would fail if not-evaluable started counting.

CODE QUALITY:
- Project conventions: Followed. Unit-lane placement is correct (no binary built, no daemon spawned, no real tmux); `hooksweep` binds the `hooks` log component and the diagnosis adds no second emission for the same event, per CLAUDE.md's closed-vocabulary rule; the check reads `hooks.json` through `hooks.ViaDoctor`, the `via` value the spec assigns it.
- SOLID principles: Good. `hooksweep.Reader` is a two-method composition of the exact surface the work reads; the stand-down rule, the phrase vocabulary and the rendering are each in one place, with the `cmd` side keeping only what it prints.
- Complexity: Low. `checkStaleHooks` is a flat guard ladder, one condition per rung.
- Modern idioms: Yes.
- Readability: Good. Each rung's comment states why it sits where it does rather than restating the code, and the phrase consts carry the reason the two conditions do not share words.
- Issues: None. Every comment in the changed region holds against the code: the ladder ordering, the "no second shape filter here" claim (`StaleKeys` carries the shape rule, `internal/hooks/store.go:248-263`), and the "emission is the sweep's" claim (no `emit` call on the diagnosis path).

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
