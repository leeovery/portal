TASK: skip-bootstrap-when-warm-1-4 — Retune loading_progress.go to ten bootstrap steps

ACCEPTANCE CRITERIA:
- totalBootstrapSteps == 10.
- stepLabelTable has exactly keys 1..10 with no gaps and no key 11; steps 9 and 10 retain their LabelRunningResumeCommands mapping (no renumber).
- LabelForStep(BootstrapProgressMsg{Index: 11}) returns "" (step 11 out of range / unmapped).
- After ten distinct completed step indices the bar fraction is exactly 1.0; after nine it is < 1.0.
- Renamed drift-guard test asserts the table covers exactly 1..10 and passes; out-of-range guard now also treats 11 as unmapped.
- No "eleven" / "11 real steps" / "1..11" residue in the file or its test comments.
- go build passes; go test ./internal/tui/... green; golangci-lint clean.

STATUS: Complete

SPEC CONTEXT:
Spec "Affected Code Surface → Orchestrator → internal/tui/loading_progress.go" (line 324-327) mandates that two INDEPENDENT constants move 11 → 10 because internal/tui must not import cmd/bootstrap (the two step-count encodings drift independently, each with its own drift guard). (1) stepLabelTable drops the CleanStale 11: entry as a drop-key (not a renumber — steps 9/10 keep their indices); the NoGaps drift-guard retunes to 1..10. (2) totalBootstrapSteps is the loading-bar denominator (BarFraction = len(completedSteps) / totalBootstrapSteps) and must become 10 or the bar tops out at 10/11 ≈ 91% and never reaches 100% on a full successful bootstrap. Spec also asks to verify the real-step→label mapping and the N/M counter (Restoring sessions only) still hold at 10 steps. This pairs with task 1-3 (orchestrator totalSteps 11→10 + CleanStale step removal).

IMPLEMENTATION:
- Status: Implemented (exact match to the DO list)
- Location: internal/tui/loading_progress.go, internal/tui/loading_progress_test.go (task commit ae628474)
- Details verified against acceptance criteria:
  - totalBootstrapSteps == 10 (loading_progress.go:41), doc comment retuned "ten increments" + drift-guard reference "1..10".
  - stepLabelTable (loading_progress.go:84-95) has exactly contiguous keys 1..10, no gap, no key 11; step 9 (CleanStaleMarkers) and step 10 (SweepOrphanFIFOs) keep LabelRunningResumeCommands — genuine drop-key, not renumber. The removed `11: … // CleanStale` line is gone.
  - LabelForStep(Index:11) returns "" because Index 11 is absent from stepLabelTable and the map read yields the zero-value string (loading_progress.go:108-113). Apply (loading_progress.go:174-177) also early-returns unchanged for any unmapped index, so a stale Index:11 producer event advances no bar.
  - BarFraction = len(completedSteps) / 10 (View loading_progress.go:214; FailedView:242). labelDone loop bound `idx <= totalBootstrapSteps` (loading_progress.go:327) auto-follows the const — no numeric literal, verified.
  - All prose retuned: file-header "10 real bootstrap steps"/"10 internal steps"/"collapses the 10 real steps"/"Index (1..10)"; stepLabelTable "(1..10)"; cleanup-step range "(8–10)"; View "(distinct completed steps)/10"; labelState "once all ten steps"/"steps 8–10". No "eleven"/"11 real steps"/"1..11" residue remains (grep-confirmed clean; the only surviving "11" tokens are the intentional removed-step-11 out-of-range assertions in the test file).
  - Build constraint holds: no cmd/bootstrap import — the two `cmd/bootstrap` matches in the file are doc-comment prose only; the mapping keys off the raw BootstrapProgressMsg.Index. No new import.
- Peripheral fan-out in same commit (correctly reconciled, not scope creep): model.go:173 doc "(1..10)"; bootstrap_progress_test.go + inert_during_loading_test.go loop bounds 11→10; loading_style_consolidation_test.go golden snapshots regenerated for the legitimate 5/11→5/10 mid-restore bar-width change (filled bar grows 18→20 cells of 39: round(5/10·39)=20; light/dark colour + colourless variants all updated consistently).

TESTS:
- Status: Adequate
- Coverage (all named plan tests present and correctly retuned in loading_progress_test.go):
  - TestBarAdvancesEveryStep (74-96): loop 1..10, want float64(step)/10.0, asserts <1.0 for step<10 and exactly 1.0 after step 10. Directly pins the "reaches 100% only after the tenth step / <1.0 after nine" criterion.
  - TestMappingCoversAllTenStepsNoGaps (321-374, renamed from …AllElevenSteps…): loop 1..10 all map to a valid canonical label; out-of-range set now {0, 11, 12, 99} — each returns "" and advances no bar; asserts exactly five labels in stable order.
  - TestRemovedStep11IsUnmapped (376-388): dedicated "it does not map removed step 11" — LabelForStep(Index:11)=="" and feeding Index:11 leaves the bar at 0.
  - TestStepMapsToFriendlyLabel (46-72): step-11 case deleted; 1..10 including step-6 RestoreM discriminator.
  - TestLabelStateTransitions (98-151): frontier walk loops 6..10; "last real step" now 10.
  - TestRestoringSessionsCounter (178-234) + TestEmptyRestoreSuppressesCounterAndTicksDone (240-263): step-6 fractions retuned to 5/10 and 6/10; counter/M=0 behaviour otherwise unchanged.
  - TestRunningResumeCommandsTicksDoneWithNoItems (268-295): frontier loop 6..9 (active at 9), step 10 ticks the group done, no per-item counter.
  - TestIdempotentPerStepIndex (299-319): duplicate/out-of-order fractions retuned to /10.
- Would fail if the feature broke: yes — a stray 11-step denominator would fail TestBarAdvancesEveryStep (1.0 never reached at step 10) and a re-added key 11 would fail both TestMappingCoversAllTenStepsNoGaps's out-of-range loop and TestRemovedStep11IsUnmapped.
- Not over-tested: TestRemovedStep11IsUnmapped and the {…,11,…} out-of-range case in TestMappingCoversAllTenStepsNoGaps both assert LabelForStep(Index:11)=="" + bar-stays-0. This is a small overlap, but BOTH were explicitly required by the plan (DO: "add 11 to the out-of-range []int"; TESTS: named "it does not map removed step 11"). Intentional belt-and-braces drift guard, not redundant bloat — no change warranted.
- Not under-tested: every acceptance criterion and edge case (bar exactly 1.0 after tenth; contiguous 1..10 no gaps/no phantom key 11; Index:11 out-of-range no-op) has a direct assertion.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (cmd-package mutable-mock constraint respected across the tui tests). Table-driven subtests, terse helpers (feed/labelState/counterText). internal/tui stays free of cmd/bootstrap. golang-code-style/naming idioms honoured.
- SOLID principles: Good. Single-source-of-truth mapping preserved; the change is a pure data/const edit with no structural drift.
- Complexity: Low. Pure accumulator; no new branches, no numeric literals leaked past the two intended constants.
- Modern idioms: Yes. reflect.TypeFor / range-over-func in the sibling guard test; map zero-value read for out-of-range → "".
- Readability: Good. Doc comments kept in lockstep with the numeric change; the drop-key-not-renumber rationale is spelled out in-source and in the commit message.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (The step-11 double-coverage across TestRemovedStep11IsUnmapped and the TestMappingCoversAllTenStepsNoGaps out-of-range loop is plan-mandated defence-in-depth, not redundancy to remove — no action.)
