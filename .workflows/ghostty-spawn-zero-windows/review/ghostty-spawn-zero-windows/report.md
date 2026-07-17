# Implementation Review: Ghostty Spawn Zero Windows

**Plan**: ghostty-spawn-zero-windows
**QA Verdict**: Request Changes

## Summary

The four coordinated code fixes are all implemented correctly and are well-tested. Fix 1 (the primary root cause) replaces the invalid `make new … with properties` template with the sdef-correct single-statement `new window with configuration {command:"%s", wait after command:true}` form, corrects the false "validated" comment, and leaves every downstream seam (ghosttyEmbed/OpenScript/OpenArgv/osascriptRunner/mapGhosttyResult) untouched. Rider #1 (WARN split), Rider #2 (honest banner copy), the ghosttycompile compile-guard, and the two Phase-2 analysis refactors (self-contained flash; -2741 signature-classified guard) each meet their acceptance criteria with adequate, non-redundant tests, and the unit lane is reliably green. The single outstanding item is the topic's own merge-gating acceptance gate (Task 1-5): the real ≥3-session live burst (CHECK 2) was **not run** — deferred by explicit user choice to a code-analysis substitution — while the spec is unambiguous that this live burst is "merge-gating, load-bearing" and that non-functional validation "is insufficient" and "blocks the merge." That deferral is the reason for Request Changes: it is a user-authorized decision the user must consciously stand behind, not an implementation defect. The code-analysis basis is also not airtight — a spaced session name would be shredded by the same round-trip that broke the manual-test marker — which is precisely the class of end-to-end defect the live burst exists to catch.

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

1. **Run the merge-gating live ≥3-session burst (CHECK 2), or make an explicit, recorded decision to override the spec's merge-gate.** The spec designates this live burst as load-bearing and states compile/analysis-only validation is insufficient to merge. It was deferred by user choice. Either (a) perform the real burst on a live Mac inside Ghostty and confirm `opened 3/3` with acks landing and net-N (not N+1) windows, or (b) consciously accept the fix-forward posture with the residual risk on record. The code-analysis substitution recorded in the task note is **not** a clean substitute — it over-states `composeAttachArgv` safety (see Recommendations → Bugs), so the risk it claims to retire is not fully retired.

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

7. `internal/spawn/command.go:27` (`composeAttachArgv`) + `internal/session/naming.go:31` (`SanitiseProjectName`) — a session name containing a space (spaced directory basename → `My Project-abc123`; `SanitiseProjectName` strips only `.`/`:`, leaving spaces) is embedded whole into the `ghosttyEmbed` space-joined `command:"…"` string and then re-split by Ghostty's `bash -c`, shredding the argv — the identical round-trip failure that broke the manual-test marker. Latent, edge-case, and in the Fix-1/ghosttyEmbed layer (out of Task 1-5's scope). The sound fix is per-argv quoting rather than a naive space-join, which is a design decision. Flagged because it is exactly the class of defect the deferred live burst (Required Change 1) would expose (Report 1-5)
