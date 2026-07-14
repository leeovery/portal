TASK: restore-host-terminal-windows-2-4 — Ghostty driver: pure osascript command construction

ACCEPTANCE CRITERIA:
- ghosttyOpenArgv(cmd)[0] == "osascript" and [1] == "-e"; [2] is the script.
- The script contains a `surface configuration` reference, embeds the composed command inside the `command:` property, contains `wait after command`, and issues `new window`.
- The embedded command is AppleScript-escaped: a `"` in an input element appears as `\"`, a `\` appears as `\\`; no unescaped payload `"` prematurely closes the AppleScript string literal.
- ghosttyOpenScript is pure — identical input yields identical output, no I/O / no process exec.
- The composed argv (/usr/bin/env … attach <session>) is embedded verbatim (post-escape).

STATUS: Complete

SPEC CONTEXT:
Spec §Build-time residuals (line 543) pins the validated Ghostty 1.3.1 AppleScript shape: make a `surface configuration` record with a `command` property and a `wait after command` property (governs whether the window persists after its command exits — the normal-detach window lifecycle), then `new window` with it; it is a preview API that may churn in 1.4 (pin/watch). Spec §Testing Strategy (line 485) mandates that pure command-construction is unit-tested by asserting the built command, with the real osascript exec deferred to manual/integration only (line 487). Spec §Config Schema recipe contract (line 372) assigns embedding-context escaping to the recipe author; here the Ghostty driver is that author and owns the AppleScript escaping. The composed argv is env-self-sufficient (`/usr/bin/env … PATH=… <exe> attach <session> --spawn-ack …`, composeAttachArgv in command.go) and Ghostty execs it as an argv in a bare PATH.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/ghostty.go:18-49 (ghosttyScriptTemplate, ghosttyEmbed, ghosttyOpenScript, ghosttyOpenArgv). Lines 51-148 are the driver struct + exec boundary + result mapping, which belong to Task 2.5 and are out of scope for this task.
- Notes:
  - ghosttyEmbed (ghostty.go:31-36) joins the argv with single spaces, then escapes `\`→`\\` BEFORE `"`→`\"`. The order is correct and load-bearing (escaping the quote first would double the escape backslash and corrupt the literal); it is documented in a comment and pinned by the escape-order test.
  - ghosttyOpenScript (ghostty.go:41-43) renders a constant template via fmt.Sprintf with the escaped payload as the single format ARGUMENT — so a `%` in the payload is inert (documented at ghostty.go:15-17). No format-injection surface.
  - Template (ghostty.go:18-21) matches the validated 1.3.1 shape: `tell application "Ghostty"` → `make new surface configuration with properties {command:"%s", wait after command:true}` → `make new window with properties {configuration:surfaceConfig}` → `end tell`. Task "Do" phrasing said "new window using that configuration"; the implementation uses the with-properties form — functionally the same intent, and the same ghosttyOpenScript path is what the Task 2.5 manual gate (ghostty_openwindow_manual_test.go) drives against real Ghostty, so the validated shape and the constructed shape cannot drift.
  - ghosttyOpenArgv (ghostty.go:47-49) returns exactly ["osascript", "-e", ghosttyOpenScript(command)]. No execution — matches "no execution" constraint.

TESTS:
- Status: Adequate
- Location: internal/spawn/ghostty_command_test.go (unit lane — no build tag, correct per the lane rule since no daemon/binary is spawned).
- Coverage:
  - AC1 (argv shape): TestGhosttyOpenArgv asserts len==3, [0]=="osascript", [1]=="-e", [2]==ghosttyOpenScript(cmd).
  - AC2 (script contents): TestGhosttyOpenScript "surface configuration…new window" checks the tell block, "surface configuration", "command:", "new window", "end tell"; a dedicated subtest checks "wait after command".
  - AC3 (escaping, the load-bearing edge case): TestGhosttyEmbed feeds `a\b"c` and asserts `a\\b\"c`, and that the script carries `command:"a\\b\"c"` — proving both escapes, the escape ORDER, and that no payload quote prematurely closes the literal.
  - AC4 (purity): calls ghosttyOpenScript twice, asserts identical output.
  - AC5 (verbatim embed): asserts ghosttyEmbed == the plain space-join for a quote/backslash-free argv, and that the script contains command:"<embedded>".
  - Tests assert on returned strings only; nothing runs osascript. Matches "asserted WITHOUT running osascript".
- Notes: Well-balanced — one focused assertion per criterion, no redundant happy-path duplication, no unnecessary mocking. The escape-order edge case (the genuinely fiddly part) is exercised with a combined backslash+quote input, which is exactly right. Not over-tested.

CODE QUALITY:
- Project conventions: Followed. Small pure functions, strings.Join/ReplaceAll, fmt.Sprintf on a const template — idiomatic Go and consistent with the package's seam/pure-construction split. Test uses table of substring wants and t.Run subtests per the repo's style; no t.Parallel (correct for this package).
- SOLID principles: Good. Pure construction is cleanly separated from the exec boundary (2.5), single responsibility per function.
- Complexity: Low. Straight-line, no branching in the construction functions.
- Modern idioms: Yes.
- Readability: Good. Comments explain the load-bearing escape order and the format-argument inertness of `%`.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/spawn/ghostty_command_test.go:80 — the purity subtest name "it is pure — identical output for the same input and no I/O" claims "no I/O" but the body only asserts identical output. Reword the subtest name to drop the unasserted "and no I/O" clause (the function visibly performs no I/O; the name should match what it verifies).
- [quickfix] internal/spawn/ghostty_command_test.go:10-16 — realAttachArgv omits the trailing `--spawn-ack <batch>:<token>` elements that the real composeAttachArgv (command.go:27-34) appends. Escaping is content-agnostic so coverage is unaffected, but appending the ack pair would make the fixture match Task 2.3's actual composed argv that AC5 references ("/usr/bin/env … attach <session>"). Low value; fidelity only.
