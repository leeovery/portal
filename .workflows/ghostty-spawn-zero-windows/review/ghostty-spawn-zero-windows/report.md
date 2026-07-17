# Implementation Review: Ghostty Spawn Zero Windows

**Plan**: ghostty-spawn-zero-windows
**QA Verdict**: Approve (with one consciously-waived acceptance gate — see below)

## Summary

The four coordinated code fixes are all implemented correctly and are well-tested. Fix 1 (the primary root cause) replaces the invalid `make new … with properties` template with the sdef-correct single-statement `new window with configuration {command:"%s", wait after command:true}` form, corrects the false "validated" comment, and leaves every downstream seam (ghosttyEmbed/OpenScript/OpenArgv/osascriptRunner/mapGhosttyResult) untouched. Rider #1 (WARN split), Rider #2 (honest banner copy), the ghosttycompile compile-guard, and the two Phase-2 analysis refactors (self-contained flash; -2741 signature-classified guard) each meet their acceptance criteria with adequate, non-redundant tests, and the unit lane is reliably green.

**Post-review resolution (this session):** Two items that were open at review time are now settled. (1) The topic's merge-gating live-burst acceptance gate (Task 1-5, CHECK 2) was **consciously waived by the user** — it is testable at launch and the user has accepted the fix-forward posture on the record; it is a deliberate deferral, not an unmet requirement being ignored. (2) Bug #7 — the spaced-session-name shredding that the review surfaced (and that the code-analysis substitution for the live burst had failed to retire) — was **fixed in this session** (commits `36f1c130` + `e86be56a`): per-element POSIX shell-quoting was centralised in the shared `renderCommandString`, fixing both the native Ghostty path and the config `terminals.json` recipe path and preserving the documented "render {command} identically" invariant. With the live gate consciously waived and the surfaced defect fixed, no blocking item remains.

## QA Verification

### Specification Compliance

Implementation aligns with the specification across all four in-scope fixes:

- **Fix 1** (§Fix 1) — template corrected to the exact prescribed form; `ghosttyEmbed` escape order confirmed (not assumed) to hold under the relocated `%s`; downstream mapping unchanged. Verified.
- **Fix 2 / Rider #1** (§Fix 2) — `LogWindowResults` now emits WARN `external window failed` for any non-permission failed window spanning **both** `AckFailed` and `AckTimeout`; confirmed and permission-required windows stay at DEBUG. The permission exclusion (`Outcome != OutcomePermissionRequired`) is load-bearing and correct — permission windows are `AckFailed` (via `burst.go`), so without the guard they would double-report against the `LogPermission` INFO event. Exactly one new message string, no new attr keys. Matches the spec-governed catalog amendment.
- **Fix 3 / Rider #2** (§Fix 3) — `PartialFailureMessage(failed, othersOpened bool)` renders `— nothing opened` on total failure and the unchanged `— others left open` otherwise; both callers derive `othersOpened = len(confirmed) > 0` from the shared `PartitionResults` chokepoint; trigger self-attach never counts as an "other"; permission-wall and empty-failed branches untouched; copy single-sourced with byte-identical CLI/picker parity.
- **Fix 4** (§Fix 4) — `ghosttycompile`-tagged compile-guard feeds `ghosttyOpenScript(<env-self-sufficient argv>)` through `osacompile -e … -o <t.TempDir()/probe.scpt>`; verified excluded from both default lanes (via `go list` with/without tags) and compiling only under `-tags ghosttycompile`.

**Deviation (§Testing & Validation Requirements — Mandatory live validation):** CHECK 2 (real ≥3-session `opened 3/3` burst) was not performed. The recorded acceptance substitutes code analysis for the functional proof the spec says cannot be substituted, and re-defers the exact live-validation step whose absence §Context identifies as the root cause of the original ship. This is a user-authorized fix-forward deferral; it directly contradicts the spec text as written and must be an eyes-open decision.

### Plan Completion

- [x] Phase 1 acceptance criteria met — Fixes 1–4 (tasks 1-1…1-4) fully implemented and tested
- [~] Phase 1 Task 1-5 (merge-gating live validation) — **partially met**: CHECK 1 window-open live-confirmed (validates the Fix-1 template — the actual defect); CHECK 3 unit lane green and tag exclusion verified; **CHECK 2 not run** (user-deferred); CHECK 3 integration lane not deterministically green (pre-existing daemon-timing flake, not a regression)
- [x] Phase 2 acceptance criteria met — tasks 2-1 (self-contained flash) and 2-2 (installed-but-not-running guard gap) complete
- [x] All tasks marked completed in the manifest
- [x] No scope creep — changes confined to `internal/spawn`, its CLI (`cmd/spawn.go`) and picker (`internal/tui/burst_partial_failure.go`) seams, plus tests

### Code Quality

No blocking code-quality issues across any task. All changes follow project Go conventions (single-sourced renderers, closed spawn log vocabulary, DI seams, build-tag-in-filename isolation, table-driven behaviour-named tests). Complexity stays low; doc comments are accurate and load-bearing (notably the Fix-4 rationale block, which records the spec-required precondition discharge). One latent, out-of-scope defect was surfaced during Task 1-5 verification — see Recommendations → Bugs.

### Test Quality

Tests adequately verify requirements and are not over-tested:

- **Fix 1** — `TestGhosttyOpenScript` asserts the corrected terminology plus a negative guard on `surface configuration` (a regression tripwire stronger than required); percent-inertness and escape-order are covered.
- **Fix 2** — `TestLogWindowResults_FailedWindowsWarn` covers all four outcome branches including the load-bearing permission exclusion; `TestLogBatchSummary_OpenedDerivedFromPartitionResults` pins the shifted DEBUG/WARN counts; `assertClosedKeys` proves no non-closed attr key on the new WARN.
- **Fix 3** — five renderer subtests map 1:1 to the true/false × single/multi + prefix/glyph matrix; CLI/picker parity asserted structurally against the shared expression (cannot drift).
- **Fix 4 / Task 2-2** — the guard is its own self-verifying test; isolation verified empirically.
- **Task 2-1** — existing burst tests pass unchanged with the single call-shape update; parity asserted structurally.

Minor optional test-coverage suggestions (non-blocking) are in Recommendations.

### Required Changes

1. ~~**Run the merge-gating live ≥3-session burst (CHECK 2), or make an explicit, recorded decision to override the spec's merge-gate.**~~ **RESOLVED — waived by the user (2026-07-17).** The user consciously accepted the fix-forward posture: the live burst is deferred to launch, eyes-open, on the record. The residual functional risk this change had called out (spaced session names, which the code-analysis substitution failed to retire) has been separately closed by the bug #7 fix below, so the waiver no longer leaves an unmitigated gap.

## Recommendations

### Do now

1. `internal/spawn/ghostty_compile_ghosttycompile_test.go:20` — add a one-line pointer in the `driftDiscriminator` doc comment noting the `-2741`-only narrowing is Task 2-2's defensive-precondition decision, so a future reader connects it to the installed-but-not-running rationale block (Report 1-4)
2. `internal/spawn/ghostty_openwindow_manual_test.go:21` — the function docstring still says the window "prints the marker line and sleeps," but the corrected discrete-token marker no longer sleeps; drop the stale "and sleeps" clause (Report 1-5)

### Quick-fixes

3. `internal/spawn/ghostty_command_test.go:92-98` — the "it is pure — identical output for same input" subtest is a low-value pre-existing redundancy (`fmt.Sprintf` is deterministic by construction); optional to drop (Report 1-1)
4. `cmd/spawn_test.go:952` (`TestSpawnPartialFailure`) — optionally add a subtest that Executes the CLI body with every external window failing (`othersOpened=false`) and asserts `err.Error() == "spawn: " + spawn.PartialFailureMessage([]string{"s2"}, false)`, giving the CLI total-failure path the same end-to-end coverage the picker has; deliberately optional — Rider #2 scopes parity tests to `message_test.go` + `burst_partial_failure_test.go` and the path is covered by construction (Report 1-3)

### Ideas

5. Compile-guard `-2741`-only discriminator narrows sensitivity to novel future drift
   - `internal/spawn/ghostty_compile_ghosttycompile_test.go:112-134` — the guard now hard-fails only on the exact `-2741` string and `t.Skip`s every other resolution failure; a hypothetical future template drift yielding a *different* invalid-terminology code would be silently skipped rather than caught (Report 2-2)
   - same file — consider whether the discriminator should cover the general "not defined" / terminology-error class rather than the single literal code; requires either live not-running evidence (spec path a) or a positive compile-success signal (Report 1-4)
6. `internal/tui/burst_partial_failure.go:43,66,117,120` — along the non-permission partial path `spawn.FirstPermission` and `spawn.PartitionResults` each still execute twice (caller + self-contained flash), so the "double scan" is relocated rather than removed and criterion #2's "at most once" is not literally met; this is the sanctioned option-(b) trade-off (self-contained flash vs single-scan-across-the-pass are in direct tension) with zero behavioural impact — collapsing to one scan would reverse criterion #1's self-containment. Design decision, not a required change (Report 2-1)

### Bugs

7. ~~`internal/spawn/command.go:27` (`composeAttachArgv`) + `internal/session/naming.go:31` (`SanitiseProjectName`) — a session name containing a space is embedded whole into the `ghosttyEmbed` space-joined `command:"…"` string and then re-split by Ghostty's `bash -c`, shredding the argv~~ **FIXED (2026-07-17, commits `36f1c130` + `e86be56a`).** Root cause was the naive `strings.Join(command, " ")` in the shared rendering (`renderCommandString`, used by the config recipe paths, and duplicated in `ghosttyEmbed` for the native path) — **not** `composeAttachArgv`, which correctly keeps the session a discrete argv element. Confirmed pre-existing (introduced by the original `restore-host-terminal-windows` spawn feature, commits `8fef8b6e`/`a19a9924`), not a regression from this bugfix; it became reachable only because Fix 1 makes windows actually open. Fix: centralised per-element POSIX single-quoting in `renderCommandString`, with `ghosttyEmbed` building on it (then AppleScript-escaping) — so the native Ghostty path **and** both config `terminals.json` recipe paths reproduce the exact argv across the downstream shell word-split, preserving the documented "render identically" invariant. Lockstep test updates + new spaced-element / embedded-single-quote coverage; unit lane green, lint clean, `ghosttycompile`/`manual` builds compile. (Report 1-5)

   *Note for `terminals.json` users:* `{command}` (and a script recipe's `$1`) is now per-element single-quoted rather than a raw space-join — correct for any recipe that runs the command through a shell (the intended use), and strictly better for spaced names. A recipe whose terminal space-splits `{command}` *without* stripping shell quotes would need revisiting, but such a recipe was already broken for any multi-token command.
