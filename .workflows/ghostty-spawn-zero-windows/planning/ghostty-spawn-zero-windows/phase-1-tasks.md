---
phase: 1
phase_name: Restore Native Ghostty Spawn + Close Diagnosis/Copy/Guard Gaps
total: 5
---

## ghostty-spawn-zero-windows-1-1 | approved

### Task 1.1: Correct the native Ghostty AppleScript template

**Problem**: Multi-window spawn via the native Ghostty adapter is entirely non-functional — selecting ≥2 sessions and pressing Enter (or running `portal spawn <sessions…>`) opens **zero** host-terminal windows (`spawn: opened 0/N`, no `portal attach` process starts, the trigger never self-attaches). The root cause is that `ghosttyScriptTemplate` in `internal/spawn/ghostty.go` is written against a **non-existent** Ghostty scripting API: `make new surface configuration with properties {…}` + `make new window with properties {…}`. Ghostty 1.3.1's scripting dictionary has **no `make` command** and **no `with properties`** terminology, and `surface configuration` is a record-type (not a makeable class). The script fails to compile (osascript `-2741`), exits non-zero in milliseconds opening nothing, and `mapGhosttyResult` maps that non-zero exit (no `-1743`/`-1712` in output) to `SpawnFailed` — so every external window in every burst fails identically → `opened 0/N`.

**Solution**: Replace the invalid two-statement `make new … with properties {…}` template with the single-statement, sdef-correct form that passes a `surface configuration` record literal directly to `new window`'s `with configuration` parameter; correct the false "validated (Ghostty 1.3.1)" comment; and confirm (do not assume) that the `ghosttyEmbed` escaping still holds under the relocated `%s`.

**Outcome**: `ghosttyOpenScript(argv)` emits an AppleScript that compiles against the installed Ghostty dictionary and opens a window running the composed command; on a clean osascript exit `mapGhosttyResult` returns `Success` and the burst proceeds normally (acks land, trigger self-attaches). The unit lane is green with the updated `ghostty_command_test.go`.

**Do**:
- In `internal/spawn/ghostty.go`, replace the `ghosttyScriptTemplate` const body with exactly:
  ```
  tell application "Ghostty"
  	new window with configuration {command:"%s", wait after command:true}
  end tell
  ```
  No `make`, no `with properties`, no intermediate `set surfaceConfig` variable. The record literal carries exactly the two sdef-defined `surface configuration` fields: `command` (text) and `wait after command` (boolean `true` — keeps the window up after its command exits, the normal-detach lifecycle for a spawned session). The single `%s` remains the sole `fmt.Sprintf` format placeholder.
- Rewrite the doc comment above the const so it describes the actual sdef-correct `new window with configuration` form and no longer claims "validated (Ghostty 1.3.1)" (that validation never happened). Keep the note that the single `%s` is a `fmt.Sprintf` **argument** (never a verb source) so a `%` in the payload is inert.
- Leave `ghosttyEmbed`, `ghosttyOpenScript` (`fmt.Sprintf(ghosttyScriptTemplate, ghosttyEmbed(command))`), `ghosttyOpenArgv`, the `osascriptRunner` seam, and `mapGhosttyResult` **unchanged**. Confirm the `ghosttyEmbed` escape order (`\` → `\\` before `"` → `\"`) still holds: the payload now sits inside the record literal's double-quoted `command:"…"` string — the same double-quoted AppleScript string context as before — so the escape order is expected unchanged. Confirm via the existing `TestGhosttyEmbed` case, do not assume.
- Update `internal/spawn/ghostty_command_test.go` (`TestGhosttyOpenScript`) in lockstep: drop the now-stale `"surface configuration"` expectation from the `wants` slice (that keyword only existed in the old invalid form) and assert the corrected terminology instead — `new window`, `with configuration`, the still-present `command:"…"` embedding, and `wait after command`. Rename the sub-test description away from "surface configuration" so it reads honestly (e.g. `"it builds a new window with configuration carrying a command property"`).

**Acceptance Criteria**:
- [ ] `ghosttyScriptTemplate` is the single-statement `tell application "Ghostty" / new window with configuration {command:"%s", wait after command:true} / end tell` form — no `make`, no `with properties`, no `set surfaceConfig`.
- [ ] The record literal carries exactly two fields: `command` (text) and `wait after command` (boolean `true`); the single `%s` is the `ghosttyEmbed`-escaped, space-joined composed argv supplied as a `fmt.Sprintf` argument.
- [ ] The false "validated (Ghostty 1.3.1)" comment is corrected to describe the actual `new window with configuration` form.
- [ ] A `%` in the payload stays inert (it is a format argument, not a verb source), and the `\`-before-`"` escape order is confirmed unchanged in the relocated `command:"…"` context.
- [ ] `ghosttyEmbed`, `ghosttyOpenScript`, `ghosttyOpenArgv`, `osascriptRunner`, and `mapGhosttyResult` (clean exit → `Success`; `-1743`/`-1712` → `PermissionRequired`; other non-zero → `SpawnFailed`) are unchanged.
- [ ] `ghostty_command_test.go` (`TestGhosttyOpenScript`) drops `"surface configuration"` and asserts `new window`, `with configuration`, `command:"…"`, and `wait after command`.
- [ ] `go test ./...` passes.

**Tests**:
- `TestGhosttyOpenScript` — `"it builds a new window with configuration carrying a command property"` (asserts `new window`, `with configuration`, `command:"…"`, `wait after command`; no `surface configuration`).
- `TestGhosttyOpenScript` — `"it embeds the composed attach argv verbatim after escaping"` (unchanged: `command:"<space-joined argv>"` appears).
- `TestGhosttyEmbed` — `"it AppleScript-escapes embedded double quotes and backslashes"` (unchanged: `a\b"c` → `a\\b\"c`, backslash escaped before quote, confirming escape order holds under the relocated `%s`).
- `TestGhosttyOpenScript` — `"it keeps a percent in the payload inert"` (new/edge: `ghosttyOpenScript([]string{"echo 100%done"})` contains literal `command:"echo 100%done"` — the `%` survives `fmt.Sprintf` because it rides in as an argument).
- `TestGhosttyOpenArgv` — `"it wraps the script as osascript -e <script>"` (unchanged; confirms downstream argv wrapping still holds).

**Edge Cases**:
- `%` in the payload stays inert because it is a `fmt.Sprintf` argument, never a format verb source.
- Backslash-before-quote escape order holds under the relocated `%s` (same double-quoted string context).
- Downstream `mapGhosttyResult` mapping (clean exit → `Success`) is unchanged — the fix is confined to the emitted script.
- The false "validated" comment is corrected so it no longer claims validation the template never had.

**Context**:
> Spec §Fix 1: "Replace the invalid two-statement `make new … with properties {…}` template … with the single-statement, sdef-correct form … `new window with configuration {command:"%s", wait after command:true}`." The record literal "carries exactly the two fields the sdef defines on `surface configuration`: `command` (text) and `wait after command` (boolean `true`)." "Everything downstream of the template is correct and stays unchanged." Spec §Scope (Verify-during-fix): "`ghosttyEmbed` escaping still holds under the relocated `%s` (same double-quoted string context; expected unchanged) — confirm as part of Fix 1."

**Spec Reference**: `.workflows/ghostty-spawn-zero-windows/specification/ghostty-spawn-zero-windows/specification.md` §Fix 1; §Scope & Non-Goals (Verify-during-fix); §Testing & Validation Requirements (lockstep `ghostty_command_test.go`).

## ghostty-spawn-zero-windows-1-2 | approved

### Task 1.2: Surface per-window failure reason at WARN (Rider #1)

**Problem**: `LogWindowResults` (`internal/spawn/logemit.go`) emits **every** external-window record — success *and* failure — at `DEBUG` (message `"external window"`, attrs `session`/`ack`/`detail`). At production-default `INFO` the batch summary `opened 0/N` is visible but the per-window `detail` (the osascript error text — the actual diagnosis) is not, so the log records *that* windows failed but never *why*. The root cause of the primary defect could only be found by reproducing osascript outside portal — the log gave no diagnosis.

**Solution**: In the `LogWindowResults` per-window loop, split by outcome. A window that **failed** (`!r.Confirmed()`, i.e. `r.Ack` is `AckTimeout` or `AckFailed`) **and** whose `r.Result.Outcome` is **not** `OutcomePermissionRequired` emits at **`WARN`** with a distinct message **`"external window failed"`** carrying the existing closed attrs `session`/`ack`/`detail`. Every other window — a **confirmed** window, or the **permission-required** window — emits at **`DEBUG`** with the existing message `"external window"`, attrs unchanged.

**Outcome**: A total-failure batch logs both `opened 0/N` at INFO **and** one `external window failed` WARN (carrying the osascript `detail`) per non-permission failed window, so an operator at default INFO sees why each window failed. The permission window is not double-reported (`LogPermission`'s INFO remains its single authority). The closed `spawn` catalog gains exactly one new message string (`external window failed`, WARN) and no new attr keys.

**Do**:
- In `internal/spawn/logemit.go`, rewrite the `LogWindowResults` loop body to branch per `r`:
  - `failed := !r.Confirmed()` (true when `r.Ack` is `AckTimeout` or `AckFailed`).
  - `nonPermission := r.Result.Outcome != OutcomePermissionRequired`.
  - If `failed && nonPermission` → `logger.Warn("external window failed", "session", r.Session, "ack", string(r.Ack), "detail", r.Result.Detail)`.
  - Else → `logger.Debug("external window", "session", r.Session, "ack", string(r.Ack), "detail", r.Result.Detail)` (unchanged shape).
- The failed set deliberately spans **both** non-permission failure modes: `AckFailed` (adapter reported no window opened: `OutcomeSpawnFailed`, `detail` = the osascript error — the observed bug) and `AckTimeout` (adapter opened the window `OutcomeSuccess` but its token never arrived within budget: `detail` = a benign success string). Both are genuine window failures the operator must see at INFO; the `ack` attr distinguishes the mode (`ack=failed` vs `ack=timeout`).
- The permission-required window is excluded from the WARN because a permission window is `AckFailed` (`!Confirmed()`) but its detail is already carried by the dedicated `permission required — nothing self-attached` INFO event (`LogPermission`), and the CLI's permission arm calls `LogWindowResults` before `LogPermission` — the exclusion prevents a double-report.
- Do not change `LogBatchSummary`'s structure (it still calls `LogWindowResults` then emits the one INFO summary) or any other helper. No new attr keys — `session`/`ack`/`detail` are already in the closed `spawn` vocabulary. The only catalog amendment is the single new WARN message string `external window failed`.
- Update `internal/spawn/logemit_test.go` in lockstep (the DEBUG-per-window count is no longer `len(results)`):
  - `TestLogWindowResults_OneDebugPerWindow`: the `bravo` case is `Ack: AckTimeout, Result: SpawnFailed("boom bravo")` — a non-permission failure — so it now emits `WARN external window failed session=bravo ack=timeout detail=boom bravo` instead of a DEBUG line. Update the golden `want` body accordingly (alpha stays `DEBUG external window session=alpha ack=confirmed detail=opened alpha`; bravo becomes the WARN line). Rename the test if its name implies "one DEBUG per window" (it no longer holds).
  - `TestLogBatchSummary_OpenedDerivedFromPartitionResults`: results are alpha (confirmed), bravo (`AckTimeout`, `Success("")`), charlie (`AckFailed`, `SpawnFailed("boom")`). The final assertion `len(debugRecords(...)) == len(results)` is now wrong — only confirmed windows emit DEBUG. Change it to expect the confirmed count (1 DEBUG record for alpha) and 2 WARN `external window failed` records (bravo, charlie). Add a `warnRecords` filter helper mirroring the existing `debugRecords`/`infoRecords` helpers.
  - `TestLogBatchSummary_FullSuccessBody`: all windows confirmed → all DEBUG → this golden body is **unchanged** (verify it still passes; do not edit).
- Add the new lockstep assertions the spec §Testing requires (either into `TestLogWindowResults_OneDebugPerWindow` sub-tests or a new `TestLogWindowResults_FailedWindowsWarn`):
  - An `AckFailed` (open-failure) window emits `WARN external window failed` carrying `session`/`ack=failed`/`detail`.
  - An `AckTimeout`-after-`OutcomeSuccess` window emits `WARN external window failed` carrying `ack=timeout` and the benign success `detail`.
  - A confirmed window emits `DEBUG external window`.
  - A permission-required window (`Result: PermissionRequired(...)`, whose Ack is `AckFailed`) does **not** emit the WARN — it emits `DEBUG external window`.
  - Reuse `assertClosedKeys` to prove the WARN carries no non-closed attr key.

**Acceptance Criteria**:
- [ ] `LogWindowResults` emits `WARN "external window failed"` (attrs `session`/`ack`/`detail`) for any non-permission failed window — both `AckFailed` and `AckTimeout`.
- [ ] A confirmed window emits `DEBUG "external window"` unchanged; a permission-required window emits `DEBUG "external window"` (excluded from the WARN, no double-report).
- [ ] No new attr keys are introduced; exactly one new message string (`external window failed`, WARN) is added to the closed `spawn` catalog.
- [ ] `ack=failed` vs `ack=timeout` distinguishes the two failure modes, so the record stays honest even when `detail` is a benign success string (the `AckTimeout` case).
- [ ] Both surfaces (CLI via `logSpawnSummary`/permission arm → `LogWindowResults`; picker via `LogBatchSummary` → `LogWindowResults`) get identical WARN behaviour — no per-caller divergence beyond the pre-existing permission-arm asymmetry.
- [ ] `logemit_test.go` updated: the `ack=timeout` non-permission case now expects `WARN external window failed`; the DEBUG-per-window count is no longer `len(results)`; `TestLogBatchSummary_OpenedDerivedFromPartitionResults` reflects 1 DEBUG (confirmed) + 2 WARN (failed).
- [ ] `go test ./...` passes.

**Tests**:
- `"it emits WARN external window failed for an AckFailed open-failure window carrying ack=failed and the osascript detail"`.
- `"it emits WARN external window failed for an AckTimeout-after-OutcomeSuccess window carrying ack=timeout and the benign success detail"`.
- `"it emits DEBUG external window for a confirmed window"`.
- `"it does NOT emit the WARN for a permission-required window (permission INFO carries its detail)"`.
- Updated `TestLogWindowResults_OneDebugPerWindow` golden body (alpha DEBUG confirmed, bravo WARN failed).
- Updated `TestLogBatchSummary_OpenedDerivedFromPartitionResults` (1 DEBUG confirmed + 2 WARN failed; INFO summary `opened 1/4` unchanged).

**Edge Cases**:
- `AckTimeout`-after-`OutcomeSuccess` emits the WARN even though `detail` is a benign success string — `ack=timeout` distinguishes the mode; restricting the WARN to open-failures (`OutcomeSpawnFailed`) only would re-introduce the exact invisibility gap (a batch whose windows open but whose acks never land would show `opened 0/N` at INFO with no WARN).
- The permission-required window is excluded from the WARN (its detail is carried by the `LogPermission` INFO event) — no double-report.
- A confirmed window stays at DEBUG `external window`.
- The DEBUG-per-window count is no longer `len(results)` — only confirmed windows are DEBUG.

**Context**:
> Spec §Fix 2 (Rider #1): "A window that failed (`!r.Confirmed()` … `AckTimeout` or `AckFailed`) and whose `r.Result.Outcome` is not `OutcomePermissionRequired` → emit at `WARN` with a distinct message `external window failed`, carrying the existing closed attrs `session`, `ack`, `detail`. Every other window — a confirmed window, or the permission-required window … → emit at `DEBUG` … attrs unchanged." "The closed `spawn` component gains one new message string, `external window failed`, at `WARN`. It introduces no new attr keys." Definitions confirmed in source: `WindowResult.Confirmed()` (`internal/spawn/classify.go`) is `r.Ack == AckConfirmed`; the permission window is assigned `AckFailed` in `internal/spawn/burst.go`, so it is `!Confirmed()` — hence the explicit `Outcome != OutcomePermissionRequired` guard is required to exclude it.

**Spec Reference**: `.workflows/ghostty-spawn-zero-windows/specification/ghostty-spawn-zero-windows/specification.md` §Fix 2 (Rider #1); §Testing & Validation Requirements (Rider #1 + lockstep `logemit_test.go`).

## ghostty-spawn-zero-windows-1-3 | approved

### Task 1.3: Honest total-failure banner copy (Rider #2)

**Problem**: `PartialFailureMessage(failed []string)` in `internal/spawn/message.go` hard-codes the suffix `— others left open`, e.g. `'s2' failed to open — others left open`. On a **total** failure (every external window failed, nothing confirmed) the "others left open" clause is false — nothing opened, and the trigger self-attach is always skipped on partial failure. The observed banner `'portal-EfVRkk', 'portal-agent-first-3' failed to open — others left open` was emitted with `opened=0`, producing misleading copy.

**Solution**: Make the suffix conditional on whether any **other external window** actually opened. Change the signature to `PartialFailureMessage(failed []string, othersOpened bool) string`: `othersOpened == true` renders the unchanged `… failed to open — others left open`; `othersOpened == false` (total failure) renders `… failed to open — nothing opened`. Both callers derive `othersOpened = len(confirmed) > 0` from the shared `spawn.PartitionResults` chokepoint.

**Outcome**: A total-failure banner reads `'s2' failed to open — nothing opened` (single name) / `'s2', 's3' failed to open — nothing opened` (multiple), and a genuine partial still reads `… — others left open`. The copy stays single-sourced in `message.go`, and the CLI (`cmd/spawn.go`) and picker (`internal/tui/burst_partial_failure.go`) render byte-identical output.

**Do**:
- In `internal/spawn/message.go`, change `PartialFailureMessage` to `func PartialFailureMessage(failed []string, othersOpened bool) string`. Build the suffix conditionally:
  - `othersOpened` true → `fmt.Sprintf("%s failed to open — others left open", QuoteJoin(failed))`.
  - `othersOpened` false → `fmt.Sprintf("%s failed to open — nothing opened", QuoteJoin(failed))`.
  Keep the no-count-aware-verb property ("failed to open" agrees with one or several names), no `spawn:` prefix, and no ⚠ glyph. Update the doc comment to describe the conditional suffix and note the `— nothing opened` clause mirrors `GoneMessage`/`UnsupportedNoopMessage`.
- In `cmd/spawn.go`, the partial-failure branch already computes `_, failed := spawn.PartitionResults(results)` (line ~179). Change it to `confirmed, failed := spawn.PartitionResults(results)` and pass the signal at the call site (line ~210): `return fmt.Errorf("spawn: %s", spawn.PartialFailureMessage(failed, len(confirmed) > 0))`.
- In `internal/tui/burst_partial_failure.go`, `burstPartialFailureFlash(results, failed)` already has `results`. In its final branch (after the permission and empty-`failed` guards), compute `confirmed, _ := spawn.PartitionResults(results)` and return `spawn.PartialFailureMessage(failed, len(confirmed) > 0)`.
- The trigger self-attach is never in the confirmed set (it is not an external window) and is skipped on partial failure, so it never counts as an "other" — this falls out of using `PartitionResults(results)`, which only ever contains external-window results.
- Leave the permission-wall branch (`FirstPermission` → returns the driver Guidance) and the degenerate empty-`failed` branch (returns `""` so no band renders) **unchanged** — only the final `PartialFailureMessage(failed …)` call is affected.
- Lockstep test updates:
  - `internal/spawn/message_test.go` (`TestPartialFailureMessage`): update the two existing calls to pass the `othersOpened` argument, and add the total-failure cases. Assert:
    - `othersOpened=true` single → `'s2' failed to open — others left open`.
    - `othersOpened=true` multi → `'s2', 's3' failed to open — others left open`.
    - `othersOpened=false` single → `'s2' failed to open — nothing opened`.
    - `othersOpened=false` multi → `'s2', 's3' failed to open — nothing opened`.
    - The no-`spawn:`-prefix / no-⚠-glyph sub-test still holds for both variants.
  - `internal/tui/burst_partial_failure_test.go`: every assertion of the form `spawn.PartialFailureMessage([]string{...})` now needs the `othersOpened` argument matching the scenario:
    - `TestBurstPartialFailure_LeavesOpenedWindowsAndSkipsSelfAttach` (bravo confirmed, alpha failed → othersOpened=true): `spawn.PartialFailureMessage([]string{"alpha"}, true)`.
    - `TestBurstPartialFailure_AckTimeoutAndSpawnFailedClassifyIdentically` (alpha confirmed; bravo+charlie failed → othersOpened=true): `spawn.PartialFailureMessage([]string{"bravo", "charlie"}, true)`.
    - Add a total-failure picker parity assertion (extend or add a test): an N=2 burst where the single external window fails and nothing else confirmed (e.g. `TestBurstPartialFailure_StaysInMultiSelectMode`'s `external=[alpha]`, alpha `AckTimeout`) → the flash body equals `spawn.PartialFailureMessage([]string{"alpha"}, false)` = `'alpha' failed to open — nothing opened`, proving byte-identical parity with the CLI total-failure copy.

**Acceptance Criteria**:
- [ ] `PartialFailureMessage` takes `othersOpened bool`: `false` renders `… failed to open — nothing opened` (single and multi-name), `true` renders the unchanged `… failed to open — others left open`.
- [ ] Both callers derive `othersOpened = len(confirmed) > 0` from `spawn.PartitionResults`: `cmd/spawn.go` (CLI exit-1 error) and `internal/tui/burst_partial_failure.go` (`burstPartialFailureFlash`).
- [ ] The trigger self-attach never counts as an "other" (it is never in the `PartitionResults` confirmed set).
- [ ] The permission-wall branch (returns Guidance) and the degenerate empty-`failed` branch (returns `""`) are unchanged.
- [ ] The copy stays single-sourced in `message.go`; no `spawn:` prefix, no ⚠ glyph.
- [ ] Parity tests (`message_test.go` + `burst_partial_failure_test.go`) assert byte-identical CLI/picker output for total failure (single-name and multi-name → `— nothing opened`) and genuine partial (`— others left open`).
- [ ] `go test ./...` passes.

**Tests**:
- `TestPartialFailureMessage` — `"it renders — nothing opened for a total failure (single name)"` → `'s2' failed to open — nothing opened`.
- `TestPartialFailureMessage` — `"it renders — nothing opened for a total failure (multiple names)"` → `'s2', 's3' failed to open — nothing opened`.
- `TestPartialFailureMessage` — `"it renders — others left open for a genuine partial (single and multiple names)"`.
- `TestPartialFailureMessage` — `"it carries no spawn prefix and no glyph"` (both variants).
- `burst_partial_failure_test.go` — updated `TestBurstPartialFailure_LeavesOpenedWindowsAndSkipsSelfAttach` and `TestBurstPartialFailure_AckTimeoutAndSpawnFailedClassifyIdentically` (othersOpened=true), plus a total-failure picker parity assertion (othersOpened=false → `— nothing opened`).

**Edge Cases**:
- Total failure with a single failed external window (an N=2 burst: one external window + the trigger; the one external fails) → `'s2' failed to open — nothing opened`.
- Total failure with multiple failed external windows → `'s2', 's3' failed to open — nothing opened`.
- Genuine partial (`othersOpened == true`) still renders `— others left open`.
- The permission-wall branch (returns Guidance) and the degenerate empty-`failed` branch (returns `""` so no band renders) are unaffected.
- The trigger self-attach never counts as an "other".
- CLI and picker output byte-identical for every case.

**Context**:
> Spec §Fix 3 (Rider #2): "Make the suffix conditional on whether any other external window actually opened … the intended signature is `PartialFailureMessage(failed []string, othersOpened bool) string`. Both callers derive `othersOpened` from the shared `spawn.PartitionResults` chokepoint (`othersOpened = len(confirmed) > 0`…). The trigger self-attach is never in the confirmed set and is skipped on partial failure, so it never counts as an 'other'." Copy table: `othersOpened == true` → `'s2' failed to open — others left open` (unchanged); `othersOpened == false` → `'s2', 's3' failed to open — nothing opened` (single and multiple, incl. an N=2 burst with exactly one failed external window). "The `— nothing opened` suffix mirrors the established spawn copy in `GoneMessage` and `UnsupportedNoopMessage`." "The permission-wall branch … and the degenerate empty-`failed` branch … are unchanged."

**Spec Reference**: `.workflows/ghostty-spawn-zero-windows/specification/ghostty-spawn-zero-windows/specification.md` §Fix 3 (Rider #2); §Testing & Validation Requirements (Rider #2 parity across `message_test.go` + `burst_partial_failure_test.go`).

## ghostty-spawn-zero-windows-1-4 | approved

### Task 1.4: Compile-check regression guard (ghosttycompile-tagged)

**Problem**: The "why it shipped" root cause is that the only test exercising the real osascript boundary is `//go:build manual` (`TestManual_OpenWindow_OpensRealGhosttyWindow`) — a test nobody ran before tagging 0.9.1 — so no automatable lane could catch a wrong AppleScript template, and the feature reached tagged release with a template that fails to compile. Process-discipline-only prevention is the exact guard that already failed once; it was explicitly rejected.

**Solution**: Add an **automated** compile-check test in `internal/spawn`, fenced behind a dedicated `ghosttycompile` build tag, that feeds `ghosttyOpenScript(<representative composed argv>)` through a compile-only osascript path (`osacompile`) and asserts a **zero exit**. The broken template fails this with the observed `-2741` terminology error; the corrected `new window with configuration {…}` form compiles clean. The test `t.Skip`s when not macOS or when Ghostty is absent.

**Outcome**: A machine-in-the-loop tripwire exists that catches AppleScript terminology drift without a human: running `go test -tags ghosttycompile ./internal/spawn/` on a Mac with Ghostty installed compiles the emitted script clean (exit 0) with the corrected template and fails loudly with the captured `-2741` output if the template regresses. The test is excluded from both default lanes and skips cleanly off-platform.

**Do**:
- Create a new test file in `internal/spawn`, e.g. `internal/spawn/ghostty_compile_ghosttycompile_test.go`, with `//go:build ghosttycompile` as its first line and `package spawn`.
- Write the guard test (e.g. `TestGhosttyOpenScript_CompilesAgainstInstalledDictionary`):
  - `t.Skip` when `runtime.GOOS != "darwin"` (skips cleanly on non-Mac).
  - `t.Skip` when `Ghostty.app` is not present — `os.Stat("/Applications/Ghostty.app")` (also probe `~/Applications/Ghostty.app` if the primary path is absent); skip cleanly rather than hard-fail.
  - Build the representative composed argv as a fixed literal mirroring the shape the spawn layer composes (env-self-sufficient, `TMUX`/`TMUX_PANE` stripped): `[]string{"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE", "/bin/sh", "-c", "echo probe"}`, so the template **and** `ghosttyEmbed` escaping are exercised together.
  - `script := ghosttyOpenScript(argv)`.
  - `out := filepath.Join(t.TempDir(), "probe.scpt")` — `osacompile` requires an output target (it does not parse-and-discard like `osascript -e`); `t.TempDir()` auto-cleans it.
  - Run `osacompile -e <script> -o <out>` via `exec.Command`, capturing combined stdout+stderr.
  - Assert the process exits `0`; on a non-zero exit, fail with the captured compiler output (the broken template yields `-2741`).
- **Live-Mac confirmation (spec-mandated, resolve during implementation on the Mac):** `osacompile`'s terminology resolution against Ghostty's dictionary and its no-window property are already evidenced (the investigation reproduced the `-2741` this way). The one genuinely-open nuance is whether Ghostty must be **running** for terminology resolution — the `t.Skip`-on-*installed* gate does not cover that. Confirm on the live Mac: if resolution requires Ghostty running, or launches it as a side effect, document the observed behaviour in the test and adjust the precondition so the guard never produces a false failure unrelated to the template — either ensure/require Ghostty is running as part of the gate, or `t.Skip` when terminology cannot be resolved. Record the confirmed behaviour in a comment.
- Ensure the tag isolates the test: it compiles into **neither** `go test ./...` (unit) nor `go test -tags integration ./...` (integration), and is separable from the window-opening `manual` test. It runs only via `go test -tags ghosttycompile ./internal/spawn/`.

**Acceptance Criteria**:
- [ ] A new `//go:build ghosttycompile`-gated file exists in `internal/spawn`.
- [ ] The test feeds `ghosttyOpenScript(<representative composed argv>)` through `osacompile -e <script> -o <t.TempDir()/probe.scpt>` and asserts a zero exit; a non-zero exit fails with the captured compiler output.
- [ ] It `t.Skip`s cleanly when not macOS or when `Ghostty.app` is absent (no hard failure).
- [ ] The representative argv is the env-self-sufficient shape (`/usr/bin/env -u TMUX -u TMUX_PANE …`) so the template and `ghosttyEmbed` escaping are exercised together.
- [ ] `osacompile` opens no window (compile-only) — the guard is a terminology tripwire, not a functional proof.
- [ ] The `ghosttycompile` tag is excluded from `go test ./...` and `go test -tags integration ./...`; the guard runs only via `go test -tags ghosttycompile ./internal/spawn/`.
- [ ] The live-Mac assumption about whether Ghostty must be running for terminology resolution is confirmed and the precondition adjusted (or documented as unnecessary) so the guard never produces a false failure unrelated to the template.
- [ ] With Task 1.1's corrected template, the guard compiles clean (exit 0) on a Mac with Ghostty; the pre-fix template would fail it with `-2741`.

**Tests**:
- `TestGhosttyOpenScript_CompilesAgainstInstalledDictionary` — `"it compiles the emitted Ghostty script against the installed dictionary with a zero exit"` (the deliverable itself).
- `"it skips cleanly when not macOS"` (via `t.Skip` on `runtime.GOOS != "darwin"`).
- `"it skips cleanly when Ghostty.app is absent"` (via `t.Skip` on missing `Ghostty.app`).

**Edge Cases**:
- `t.Skip` (not fail) when not macOS or when `Ghostty.app` is absent, so invoking the tag on a machine without Ghostty skips cleanly.
- `osacompile` requires a throwaway output target (`-o <path>`) — use a path under `t.TempDir()` (auto-cleaned); it does not parse-and-discard like `osascript`.
- Confirm/adjust the gate if Ghostty must be running for terminology resolution (avoid a false failure unrelated to the template).
- The compile-check opens no window and proves only that the emitted script compiles — the functional proof remains the mandatory live validation (Task 1.5); the two are complementary, not substitutes.

**Context**:
> Spec §Fix 4: "add an automated compile-check that catches a wrong AppleScript template without a human in the loop … Feed `ghosttyOpenScript(<representative composed argv>)` through a compile-only osascript path (`osacompile` …) and assert a zero exit." "A dedicated build tag (proposed name `ghosttycompile`), so it compiles into neither `go test ./...` nor `go test -tags integration ./...` … It runs via `go test -tags ghosttycompile ./internal/spawn/`." "Within the test, `t.Skip` when not macOS or when `Ghostty.app` is not present." "Compile via `osacompile -e <script> -o <out>`, where `<out>` is a throwaway path under `t.TempDir()` … Assert the process exits 0; a non-zero exit fails the test with the captured compiler output (the current broken template yields `-2741`)." "The representative argv is a fixed literal of the composed shape, e.g. `[]string{"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE", "/bin/sh", "-c", "echo probe"}`." Assumption to confirm on live Mac: whether Ghostty must be running for terminology resolution (the `t.Skip`-on-installed gate does not cover this). Accepted limitation: "it does not prove a window opens and runs the command."

**Spec Reference**: `.workflows/ghostty-spawn-zero-windows/specification/ghostty-spawn-zero-windows/specification.md` §Fix 4 (Prevention); §Testing & Validation Requirements (Prevention compile-check).

## ghostty-spawn-zero-windows-1-5 | approved

### Task 1.5: Merge-gating live validation (manual Ghostty test + real ≥3-session burst)

**Problem**: The absence of live validation is exactly what let this defect ship to 0.9.1 — the compile-check (Task 1.4) proves the script parses, not that a window opens and runs its command. Compile-only validation is **insufficient**. The fix is therefore not "done" until the functional path is proven on a live Mac inside Ghostty; per spec this live gate **blocks the merge**.

**Solution**: A manual acceptance gate (not automatable — it requires a live Mac inside Ghostty) with two concrete checks: (1) the `-tags manual` real-window Ghostty test passes, and (2) a real ≥3-session picker multi-select burst confirms `opened 3/3` with token acks landing and the trigger self-attaching. This task also verifies both default automated lanes stay green with the lockstep updates from Tasks 1.1–1.3.

**Outcome**: On a live Mac inside Ghostty, a real Ghostty window opens and runs the command, and a real ≥3-session burst opens net-N windows (never N+1) reporting `opened 3/3` with acks landing and the trigger self-attaching. Both default lanes pass; the `manual` and `ghosttycompile` tags remain excluded from both. The merge is unblocked only once all of these hold.

**Do**:
- Prerequisites: Tasks 1.1–1.4 complete and merged into the working branch; a live Mac, running inside a **Ghostty** terminal, with Automation permission granted for Ghostty → Ghostty (self-exempt in the normal flow). No `terminals.json` Ghostty override (so the native adapter path is exercised).
- **Check 1 — real-window manual test.** Build with Fix 1 applied, then run:
  `go test -tags manual -run TestManual_OpenWindow_OpensRealGhosttyWindow -v ./internal/spawn/`
  Confirm the test PASSES **and** visually confirm a **new Ghostty window** opens and runs the marker command (`echo …; sleep 5`) — the window persists after the command exits (the `wait after command:true` property). The automated assertion only checks `OpenWindow` reported success; the real-window-opened check is the human's eyes.
- **Check 2 — real ≥3-session burst.** Build the binary (`go build -o portal .`). Launch the picker (`portal` / `portal open`), enter multi-select mode (`m`), mark **≥3** sessions, press Enter. Confirm:
  - N−1 external Ghostty windows open, plus the trigger window self-attaches — **net-N windows, never N+1** (the trigger window is reused as one session; only the N−1 others are externally spawned).
  - The token acks land (each spawned `portal attach --spawn-ack` writes its `@portal-spawn-<batch>-<token>` marker; the burster confirms each within budget).
  - The log reports `spawn: opened 3/3` at INFO (for a 3-session burst), and on a clean run there is **no** `external window failed` WARN.
- **Check 3 — default lanes green.** Run `go test ./...` (unit) and `go test -tags integration -p 1 ./...` (integration); both must pass, inclusive of the lockstep test updates from Tasks 1.1–1.3. Confirm the `manual` and `ghosttycompile` tags remain excluded from both lanes (they compile only under their own tags).
- Record the outcome of all three checks (pass/fail + observed `opened N/N`, window count, ack landing). Do **not** consider the fix done — and do **not** merge — until Checks 1, 2, and 3 all pass. If any check fails, file the observed behaviour back against the relevant fix task (1.1–1.4) rather than papering over it here.

**Acceptance Criteria**:
- [ ] `go test -tags manual -run TestManual_OpenWindow_OpensRealGhosttyWindow ./internal/spawn/` passes on a live Mac inside Ghostty, and a real Ghostty window is visually confirmed to open and run the command.
- [ ] A real ≥3-session picker multi-select burst confirms `opened 3/3`, the token acks land, and the trigger self-attaches.
- [ ] The burst opens **net-N** windows (N−1 external + the reused trigger), never N+1.
- [ ] `go test ./...` (unit) and `go test -tags integration -p 1 ./...` (integration) both pass, inclusive of the lockstep updates; the `manual` and `ghosttycompile` tags remain excluded from both lanes.
- [ ] Compile-only validation (Task 1.4) is **not** accepted as a substitute for this functional gate.
- [ ] The merge is blocked until all checks above pass.

**Tests**:
- (Manual gate — not an automated lane test.) `go test -tags manual -run TestManual_OpenWindow_OpensRealGhosttyWindow -v ./internal/spawn/` PASSES with a visually-confirmed new Ghostty window.
- (Manual gate.) Real ≥3-session picker burst → `opened 3/3`, acks land, trigger self-attaches, net-N windows.
- (Automated confirmation.) `go test ./...` and `go test -tags integration -p 1 ./...` both green with the lockstep updates.

**Edge Cases**:
- The `manual` and `ghosttycompile` tags stay excluded from both default lanes (they must not leak into unit/integration).
- Compile-only validation (Task 1.4) is insufficient — the functional proof (a window opens and runs its command) is required and is what this gate adds.
- The burst must land acks and self-attach the trigger, producing net-N windows (not N+1) — verify the trigger window is reused, not double-spawned.

**Context**:
> Spec §Testing & Validation Requirements: "Mandatory live validation (merge-gating, load-bearing). The absence of live validation is what let this ship, so the fix is not 'done' until, on a live Mac inside Ghostty: (1) `go test -tags manual -run TestManual_OpenWindow_OpensRealGhosttyWindow ./internal/spawn/` passes (a real Ghostty window opens and runs the command). (2) A real ≥3-session picker multi-select burst confirms `opened 3/3`, the token acks land, and the trigger self-attaches. Compile-only validation is insufficient … This live gate blocks the merge." Spec §Scope (Release posture): "The mandatory live validation is an in-scope acceptance gate of this fix … its two checks … are acceptance criteria for this work and must be planned/tracked as part of it." The `TestManual_OpenWindow_OpensRealGhosttyWindow` test lives in `internal/spawn/ghostty_openwindow_manual_test.go`; net-N behaviour ("net N windows, never N+1") is the hard spawn invariant — the trigger window is reused as one session, so only the N−1 others are externally spawned.

**Spec Reference**: `.workflows/ghostty-spawn-zero-windows/specification/ghostty-spawn-zero-windows/specification.md` §Testing & Validation Requirements (Mandatory live validation; Existing lanes stay green); §Scope & Non-Goals (Release posture).
