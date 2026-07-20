TASK: cli-verb-surface-redesign-2-6 — Command (`-e`/`--`) is mint-scoped — reject on attach targets

ACCEPTANCE CRITERIA (plan row 2-6):
- command + a session-resolving target (attach) → usage error, for a bare session name or a `-s` pin
- single-target command + zero mint targets → usage error (multi-target zero-mint deferred to Phase 3)
- both `-e` and `--` → usage error (preserved)
- empty command value (`-e ""` / `--` with no args) → usage error (preserved)
- command + mint target threads into the mint (Phase-1 wiring, regression)

STATUS: Complete

SPEC CONTEXT:
Spec § "Command passthrough (`-e` / `--`) — mint-scoped" (lines 123-139): `-e`/`--` are two spellings of one command; specifying both is a usage error; the command targets mint surfaces only because an existing (attach) session has no safe injection channel (the command-injection-safety note: send-keys corrupts a busy pane, respawn-pane -k destroys running work — a safety floor, not a chosen restriction); zero mint targets + a command ⇒ usage error (erroring beats silently dropping); a command with no target opens the Projects picker (that is Task 2-7, not this task). Phase 2 phase-level acceptance (planning.md lines 54-55) restates the same rule.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/open.go:345-363 `openResolved` — the *SessionResult arm rejects a command (`if len(command) > 0 { return NewUsageError(commandAttachOnlyMessage) }`) BEFORE the ack write and attach handoff; the *PathResult arm threads `command` into `openPathFunc` (mint). Single shared dispatch point covers BOTH the bare-positional session hit and the `-s` pin (which routes through resolvePinAndOpen → openResolved), so one guard enforces the rule for both entry points.
  - cmd/open.go:464-504 `parseCommandArgs` — both `-e` and `--` present → usage error (471-473); empty `-e ""` → usage error (476-478); `--` with no trailing args → usage error (488-490).
  - cmd/open_burst.go:95-100 `commandAttachOnlyMessage` const — the sole authoring site for the wording, shared with the multi-target zero-mint guard (open_burst_run.go:151, Phase 3 arity of the same rule) so the two cannot drift.
  - main.go:83-105 `classify` maps *cmd.UsageError → exit code 2, satisfying the "usage error" contract.
- Notes: At single-target arity a command targeting an attach session IS the zero-mint case, so one guard (the *SessionResult arm) covers both the "reject on attach" and "single-target zero-mint" acceptance bullets — correct per spec (the two coincide at N=1). Multi-target zero-mint is correctly out of scope here (Phase 3; already present at open_burst_run.go:151, reusing the same const). No drift from plan or spec.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/open_test.go:325 `TestOpenCommand_BareSessionAttach_WithCommand_UsageError` — `open dev -e claude` (dev = existing session) → exact message, asserts *UsageError (exit 2), and asserts openSessionFunc is NOT called (no attach on the reject path).
  - cmd/open_test.go:370 `TestOpenCommand_SessionPin_WithCommand_UsageError` — `open -s dev -e claude` → same guard fires via the pin path; asserts *UsageError and no attach.
  - cmd/open_test.go:1507 `TestParseCommandArgs` table — "both -e and -- produces exit code 2", "-e with empty string produces exit code 2", "-- with no arguments produces exit code 2" (plus "-- with destination but no command args"), each asserting the exact message and *UsageError type.
  - cmd/open_test.go:2347 `TestOpenCommand_CommandThreadsIntoMintedTarget` — `open api -- vim .` (alias→mint) → openPathFunc receives path + `[vim .]` unchanged. Reinforced by the pin-threading regressions: `TestOpenCommand_PathPin_ThreadsCommandIntoMint` (634), `TestOpenCommand_AliasPin_ThreadsCommandIntoMint` (827), `TestOpenCommand_ZoxidePin_ThreadsCommandIntoMint` (1067).
- Notes: Every acceptance bullet has a direct test; each would fail if the feature broke (they assert the concrete error string, the *UsageError type for exit 2, AND the negative — no attach fires). Not over-tested: one focused test per behaviour; the multi-target rows in TestParseCommandArgs are explicitly labelled Task 3-7 regressions, not redundant 2-6 coverage. The command-on-attach guard is exercised only via the `-e` spelling; the `--` spelling for that guard is not directly tested (both spellings converge on the same `command` slice in parseCommandArgs, so the guard is spelling-agnostic — genuine but low-value gap).

CODE QUALITY:
- Project conventions: Followed. UsageError → exit-2 contract (main.classify) used correctly; DI seams (openSessionFunc/openPathFunc/openDeps) restored via t.Cleanup per the codebase's package-level-mutable-state test pattern; no t.Parallel.
- SOLID principles: Good. Single guard in openResolved's *SessionResult arm serves both the bare-positional and `-s`-pin entry points (single responsibility, no duplication); the mint-vs-attach outcome switch is the open/closed extension point.
- DRY: Good. commandAttachOnlyMessage is single-sourced as a const, shared with the Phase-3 multi-target zero-mint guard; the wording cannot drift between arities.
- Complexity: Low. The guard is a single length check; parseCommandArgs is a linear branch on hasExec/hasDash.
- Modern idioms: Yes. Type-switch dispatch, errors.As in tests.
- Readability: Good. Comments at open.go:339-353 explain the mint-scoped rationale and the guard-before-ack-write ordering (a command+attach usage error must fire without writing a marker).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/open_test.go:325 — add an attach-guard case using the `--` spelling (e.g. `open dev -- claude`) alongside the existing `-e` case, so both command spellings are proven to hit the command-on-attach reject; today only `-e` exercises that guard (both spellings converge on the same parsed `command` slice, so this is confirming convergence, not new logic).
