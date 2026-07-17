---
status: complete
created: 2026-07-17
cycle: 1
phase: Traceability Review
topic: Ghostty Spawn Zero Windows
---

# Review Tracking: Ghostty Spawn Zero Windows - Traceability

## Result: CLEAN — no findings

The plan is a complete and faithful translation of the specification in both
directions. No missing spec content, no hallucinated plan content.

## Direction 1: Specification → Plan (completeness)

Every specification element is represented in the plan with implementer-ready depth:

| Spec section | Plan coverage | Verdict |
|--------------|---------------|---------|
| Fix 1 — Correct the Ghostty AppleScript template (`internal/spawn/ghostty.go`) | Task 1.1 (ghostty-spawn-zero-windows-1-1) | Covered: single-statement `new window with configuration {command:"%s", wait after command:true}` form; exactly two sdef fields; no `make`/`with properties`/`set surfaceConfig`; false "validated (Ghostty 1.3.1)" comment corrected; `%s` as `fmt.Sprintf` arg (`%` inert); `ghosttyEmbed` escape order re-confirmed via `TestGhosttyEmbed`; downstream `ghosttyOpenScript`/`ghosttyOpenArgv`/`osascriptRunner`/`mapGhosttyResult` unchanged; lockstep `ghostty_command_test.go` |
| Fix 2 (Rider #1) — Surface per-window failure at WARN (`logemit.go` `LogWindowResults`) | Task 1.2 (ghostty-spawn-zero-windows-1-2) | Covered: `!r.Confirmed()` && `Outcome != OutcomePermissionRequired` → WARN `external window failed` (attrs `session`/`ack`/`detail`); both `AckFailed` and `AckTimeout`; permission window excluded (LogPermission is authority, no double-report); confirmed → DEBUG `external window`; one new message string, zero new attr keys; parity across CLI + picker; lockstep `logemit_test.go` |
| Fix 3 (Rider #2) — Honest total-failure banner copy (`message.go` `PartialFailureMessage`) | Task 1.3 (ghostty-spawn-zero-windows-1-3) | Covered: signature `(failed []string, othersOpened bool)`; both callers derive `othersOpened = len(confirmed) > 0` from `spawn.PartitionResults` (`cmd/spawn.go`, `internal/tui/burst_partial_failure.go`); exact copy table (`— nothing opened` vs `— others left open`, single + multi name); trigger self-attach never an "other"; permission-wall + empty-`failed` branches unchanged; single-source parity tests |
| Fix 4 (Prevention) — Compile-check regression guard | Task 1.4 (ghostty-spawn-zero-windows-1-4) | Covered: new `//go:build ghosttycompile` file in `internal/spawn`; `osacompile -e <script> -o <t.TempDir()/probe.scpt>` zero-exit assertion on `ghosttyOpenScript(<representative env-self-sufficient argv>)`; `t.Skip` when non-macOS or `Ghostty.app` absent; live-Mac running-state assumption confirmed/adjusted; compile-only limitation acknowledged (not a functional-proof substitute) |
| Testing & Validation — Mandatory live validation (merge-gating) | Task 1.5 (ghostty-spawn-zero-windows-1-5) | Covered: Check 1 `-tags manual TestManual_OpenWindow_OpensRealGhosttyWindow` real window; Check 2 real ≥3-session burst → `opened 3/3`, acks land, trigger self-attaches, net-N not N+1; Check 3 both lanes green with `manual`/`ghosttycompile` excluded; merge blocked until all pass |
| Testing & Validation — Automated tests (Rider #1, Rider #2 parity, prevention compile-check) | Tasks 1.2 / 1.3 / 1.4 | Covered in the respective fix tasks with named test assertions |
| Testing & Validation — Lockstep existing-test updates (`ghostty_command_test.go`, `logemit_test.go`, `message_test.go`, `burst_partial_failure_test.go`) | Tasks 1.1 / 1.2 / 1.3 | Covered — each fix task carries its own lockstep test-update block with named tests |
| Scope & Non-Goals — Out of scope (config `terminals.json` path, detection/pre-flight/token-ack/selection/notice-band, single-session open/attach, no new adapters) | No task (correctly) | Out-of-scope items require no tasks; none present in the plan |
| Scope & Non-Goals — Verify-during-fix (`ghosttyEmbed` escaping; riders separate but in-scope) | Tasks 1.1 / 1.2 / 1.3 | Covered — escaping confirmation in 1.1; riders as 1.2 and 1.3 |
| Release posture — live validation gating the merge | Task 1.5 | Covered — task explicitly blocks merge until all checks pass |

## Direction 2: Plan → Specification (fidelity / anti-hallucination)

Every task's Problem, Solution, Do steps, acceptance criteria, tests, and edge
cases trace to a specific specification section. No untraceable content found.

Implementation-level elaborations were reviewed and are all grounded, not invented:

- Task 1.2's "Source-verified" note (`WindowResult.Confirmed()` in `classify.go` is
  `r.Ack == AckConfirmed`; the permission window is assigned `AckFailed` in `burst.go`)
  grounds the spec's stated fact that the permission window is `AckFailed` and that the
  explicit `Outcome != OutcomePermissionRequired` guard is load-bearing. Traces to Fix 2.
- Task 1.3's line-number hints (`cmd/spawn.go` ~179/~210) and named existing tests
  (`TestBurstPartialFailure_*`) are implementation guidance for the spec-mandated lockstep
  updates. Traces to Fix 3 + Testing & Validation.
- Task 1.4's secondary `~/Applications/Ghostty.app` probe is a faithful realization of the
  spec's "when `Ghostty.app` is not present" skip condition, not added scope.
- Task 1.5's prerequisites (Automation permission granted, no `terminals.json` override) and
  the description of the existing manual test's marker command (`echo …; sleep 5`) describe
  the existing test harness / spawn permission model, not new behaviour. The Check-2 "no
  `external window failed` WARN on a clean run" assertion ties Fix 2 into the live gate and
  traces to Fix 2 + Testing & Validation.
- Task 1.1's new "keeps a percent in the payload inert" test traces directly to the Fix 1
  requirement that a `%` in the payload stays inert as a `fmt.Sprintf` argument.

## Findings

None.

---

*No changes required. The plan faithfully and completely translates the specification.*
