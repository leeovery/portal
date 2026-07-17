TASK: Fix 1 (ghostty-spawn-zero-windows-1-1) — Correct the native Ghostty AppleScript template (internal/spawn/ghostty.go)

ACCEPTANCE CRITERIA:
1. ghosttyScriptTemplate is the single-statement `tell application "Ghostty" / new window with configuration {command:"%s", wait after command:true} / end tell` form — no `make`, no `with properties`, no `set surfaceConfig`.
2. Record literal carries exactly two fields: command (text) and wait after command (boolean true); the single %s is the ghosttyEmbed-escaped, space-joined composed argv supplied as a fmt.Sprintf argument.
3. The false 'validated (Ghostty 1.3.1)' comment is corrected to describe the actual `new window with configuration` form.
4. A % in the payload stays inert (format argument, not a verb source), and the backslash-before-quote escape order is confirmed unchanged in the relocated command:"..." context.
5. ghosttyEmbed, ghosttyOpenScript, ghosttyOpenArgv, osascriptRunner, and mapGhosttyResult unchanged.
6. ghostty_command_test.go (TestGhosttyOpenScript) drops 'surface configuration' and asserts new window, with configuration, command:"...", and wait after command.
7. go test ./... passes.

STATUS: Complete

SPEC CONTEXT:
Spec §Fix 1 (specification.md L28-48): the shipped template targeted a non-existent Ghostty scripting API (`make new surface configuration with properties {…}` + `make new window with properties {…}`), which fails to compile against the 1.3.1 sdef (osascript -2741), exits non-zero without opening a window, and maps to SpawnFailed → `opened 0/N`. The corrective form is the single-statement `new window with configuration {command:"%s", wait after command:true}`, passing a `surface configuration` record literal directly to `new window`'s `with configuration` parameter — no `make`, no `with properties`, no intermediate `set surfaceConfig`. Spec §Scope Verify-during-fix (L141-143): ghosttyEmbed escaping must be CONFIRMED (not assumed) to hold under the relocated %s. Spec §Testing L166-168: ghost­ty_command_test.go must drop the stale "surface configuration" expectation and assert the corrected terminology. Downstream (ghosttyOpenScript/Argv, osascriptRunner, mapGhosttyResult) is correct and stays unchanged (L48). Note: the merge-gating live validation (manual test + real ≥3-session burst) is owned by task 1-5, not this task.

IMPLEMENTATION:
- Status: Implemented — matches every acceptance criterion.
- Location: internal/spawn/ghostty.go:20-22 (template const), :8-19 (corrected doc comment), :32-37 (ghosttyEmbed unchanged), :42-44 (ghosttyOpenScript unchanged).
- Notes:
  - Criterion 1 — met exactly. Const at L20-22 is verbatim the required single-statement form. `git show 6d92561e` confirms the two-statement `set surfaceConfig ... / make new window with properties {configuration:surfaceConfig}` body was replaced by the single `new window with configuration {command:"%s", wait after command:true}` line. No `make`, no `with properties`, no `set surfaceConfig` remain anywhere in the file.
  - Criterion 2 — met. Record literal `{command:"%s", wait after command:true}` carries exactly two fields. ghosttyOpenScript (L43) supplies `ghosttyEmbed(command)` as the sole fmt.Sprintf argument for the single %s.
  - Criterion 3 — met. The doc comment (L8-19) no longer claims "validated (Ghostty 1.3.1)"; it now reads "sdef-correct … passes a `surface configuration` record literal directly to `new window`'s `with configuration` parameter — the only form Ghostty's scripting dictionary defines (there is no `make` command and no `with properties` terminology)". Accurate and spec-faithful (spec itself calls it a `surface configuration` record literal, L32).
  - Criterion 4 — confirmed by reading. The %s payload is a runtime fmt.Sprintf argument, so a literal `%` in the payload can never be a verb source (proven by the percent subtest). ghosttyEmbed (L32-37) is byte-for-byte unchanged: backslash (`\`->`\\`, L34) still runs before quote (`"`->`\"`, L35). The relocated payload sits inside the same double-quoted `command:"…"` string context, so the escape order remains correct.
  - Criterion 5 — met. The commit diff touches only the template const and its comment in ghostty.go. ghosttyEmbed, ghosttyOpenScript, ghosttyOpenArgv, osascriptRunner (+ execOsascriptRunner), and mapGhosttyResult (clean exit → Success; -1743/-1712 → PermissionRequired; other non-zero → SpawnFailed) are all unchanged.
  - No scope creep: only ghostty.go, ghostty_command_test.go, and workflow bookkeeping (.tick/tasks.jsonl, manifest.json) changed. Riders #1/#2 and the compile guard belong to sibling tasks 1-2/1-3/1-4 and are correctly absent here.

TESTS:
- Status: Adequate.
- Location: internal/spawn/ghostty_command_test.go (TestGhosttyOpenScript, TestGhosttyEmbed, TestGhosttyOpenArgv).
- Coverage (verified by reading; assertions traced by hand, suite not executed per the no-execution rule):
  - TestGhosttyOpenScript / "new window with configuration carrying a command property" (L40-64): asserts presence of `tell application "Ghostty"`, `new window`, `with configuration`, `command:"`, `wait after command`, `end tell`, AND an explicit NEGATIVE assertion that `surface configuration` is absent (L61-63). This directly satisfies criterion 6 and adds a regression tripwire against reverting to the old form — stronger than the criterion requires.
  - "it keeps a percent in the payload inert" (L66-75): feeds `echo 100%done` and asserts the literal `command:"echo 100%done"` survives — covers criterion 4's %-inertness.
  - "it embeds the composed attach argv verbatim after escaping" (L77-90): asserts the quote/backslash-free argv space-joins verbatim into `command:"…"`.
  - "it is pure — identical output for same input" (L92-98).
  - TestGhosttyEmbed (L101-121): `a\b"c` -> `a\\b\"c`, proving backslash-before-quote order, and confirms the escaped form embeds intact in the script — covers criterion 4's escape-order confirmation.
  - TestGhosttyOpenArgv (L18-37): unchanged; still valid (wraps script as `osascript -e <script>`).
  - Manual trace of ghosttyOpenScript(realAttachArgv()) confirms every `wants` substring is present and `surface configuration` is absent, so the updated assertions pass. go vet is satisfied: the const format string carries exactly one %s against one arg.
- Notes: Not under-tested — every acceptance criterion has a corresponding assertion, including the two Verify-during-fix items (%-inertness and escape order). Not meaningfully over-tested — each subtest targets a distinct behaviour. The "it is pure" subtest is a minor, pre-existing redundancy (Go's fmt.Sprintf is deterministic by construction) but is harmless and documents intent.

CODE QUALITY:
- Project conventions: Followed. Const template + small pure helpers + 1-method DI seam (osascriptRunner) matches the codebase's spawn-adapter pattern (OS specifics quarantined behind the typed Result taxonomy). Table-driven `wants` assertions and behaviour-named subtests match golang-testing conventions.
- SOLID principles: Good. Single responsibility preserved; the mapping/exec seams stay untouched and decoupled.
- Complexity: Low. Change is a const swap + comment rewrite; no new branches.
- Modern idioms: Yes. strings.ReplaceAll, fmt.Sprintf with a const format string (vet-analysable).
- Readability: Good. The rewritten doc comment is accurate and explains the sdef rationale and the load-bearing escape order.
- Security: The only injection surface is the AppleScript literal; ghosttyEmbed escapes backslash and double-quote (the characters that could break out of the `command:"…"` context). Payload is a controlled env-argv + `portal attach <session>`. No new exposure introduced by this task. (ghosttyEmbed handles only `\` and `"`, matching the double-quoted-string context — pre-existing and explicitly declared unchanged; not a finding for this task.)
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/spawn/ghostty_command_test.go:92-98 — the "it is pure — identical output for same input" subtest is a low-value pre-existing redundancy (fmt.Sprintf is deterministic by construction). Optional to drop; touches test logic so routed as quickfix rather than do-now. Not introduced by this commit.
