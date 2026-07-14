TASK: restore-host-terminal-windows-7-3 — Extract the shared exec-boundary and failure-detail helpers for the two spawn adapters (tick-473b1a)

ACCEPTANCE CRITERIA:
- Both runner Run methods delegate to runArgvCombined; no duplicated exec body remains.
- Both failure-detail functions delegate to execFailureDetail with only their fallback label differing.
- The osascriptRunner / recipeRunner interfaces and their Adapter implementations remain distinct (no seam merge).
- Adapter unit tests (fake runners) and the real-exec paths behave identically to today.

STATUS: Complete

SPEC CONTEXT: Phase 7 is an analysis/refactor cycle. This task removes byte-identical duplication behind the two deliberately-separate spawn runner seams. The native Ghostty adapter (osascriptRunner) and the config-recipe adapters (recipeRunner) must stay separate seams per the design note, but the concrete exec plumbing and failure-detail formatting behind them were duplicated — the classic silent-drift hazard where one side gets a fix and the other does not. The extraction shares only the identical plumbing.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/spawn/exec_boundary.go:21 runArgvCombined (shared exec→combined-output→exit-code body)
  - internal/spawn/exec_boundary.go:56 execFailureDetail (label-parameterised failure formatter)
  - internal/spawn/exec_boundary.go:40 combineOutput (relocated here from ghostty.go; already-shared half)
  - internal/spawn/ghostty.go:73 execOsascriptRunner.Run → return runArgvCombined(argv)
  - internal/spawn/ghostty.go:143 failureDetail → execFailureDetail(..., "ghostty osascript exit %d")
  - internal/spawn/configadapter.go:37 execRecipeRunner.Run → return runArgvCombined(argv)
  - internal/spawn/configadapter.go:75 recipeFailureDetail → execFailureDetail(..., "recipe exit %d")
- Notes: The implementing commit (a63881d9) diff confirms a faithful, behaviour-preserving extraction. The two prior Run bodies were byte-identical and are reproduced exactly in runArgvCombined (only local var names differ: out→combined, err→runErr — semantics unchanged). The two failure-detail bodies differed only in the fallback label; execFailureDetail reproduces the identical branch logic parameterised by fallbackLabel. Both runner interfaces (osascriptRunner, recipeRunner) and all three Adapter types (ghosttyAdapter, argvRecipeAdapter, scriptRecipeAdapter) remain distinct — no seam merge. combineOutput was cleanly relocated into exec_boundary.go, consolidating all shared plumbing in one file. No orphaned/stale duplicate exec body remains (grep confirms both Run methods delegate). Out of scope and correctly untouched: successDetail's hardcoded "ghostty osascript exit 0" (success path, not part of this failure-detail task) and the pre-existing empty-argv precondition (both callers guarantee a non-empty argv).

TESTS:
- Status: Adequate
- Coverage:
  - internal/spawn/exec_boundary_test.go:12 TestRunArgvCombined — real hermetic exec (sh, and a missing binary; no tmux/daemon/built binary, unit-lane safe) covering all three contract branches: clean exit (out,0,nil), non-zero exit (combined stdout+stderr + code, nil err), missing binary (err surfaced, code 0). Matches the tick's Tests list exactly.
  - internal/spawn/exec_boundary_test.go:58 TestExecFailureDetail — all four formatter branches (trimmed output wins; error-only; detail+error joined; empty→never-empty fallback) with the fallback verified against BOTH the ghostty and recipe labels.
  - internal/spawn/exec_boundary_test.go:94 TestFailureDetailWrappersDelegate — pins that each wrapper delegates with its own correct label; directly verifies acceptance criterion 2 and would catch a label swap or a broken delegation.
  - Regression: existing ghostty/configadapter adapter tests (ghostty_openwindow_test.go, configadapter_argv_test.go, configadapter_script_test.go, and integration variants) were NOT modified by the commit — fake-runner mapping and real-exec paths unchanged, satisfying acceptance criterion 4.
- Notes: Well balanced — not under-tested (every branch of both helpers plus both labels), not over-tested. TestFailureDetailWrappersDelegate overlaps slightly with the fallback-label case in TestExecFailureDetail, but the split is intentional and clean: TestExecFailureDetail pins the formatter's literal output, TestFailureDetailWrappersDelegate pins that each wrapper binds the right label. Not redundant.

CODE QUALITY:
- Project conventions: Followed. Small package-private helpers, DI seams preserved, doc comments explain the WHY (drift hazard, seam-separation intent, three-way contract) rather than restating code. Named return values on runArgvCombined document the contract. Unit-lane hermeticity respected (no build:integration needed; no tmux/daemon/binary).
- SOLID principles: Good. Single responsibility per helper; the two seam interfaces stay segregated (no merge), exactly as the design mandates. Shared plumbing extracted without collapsing the abstractions.
- Complexity: Low. Both helpers are short, linear, single-purpose.
- Modern idioms: Yes — errors.As for *exec.ExitError, strings.Builder-free simple join, fmt.Sprintf with a label format string.
- Readability: Good. Intent is self-documenting; comments are accurate and non-redundant.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
