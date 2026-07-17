# Plan: Ghostty Spawn Zero Windows

## Phases

### Phase 1: Restore Native Ghostty Spawn + Close Diagnosis/Copy/Guard Gaps
status: approved
approved_at: 2026-07-17

**Goal**: Make native Ghostty multi-window spawn functional again by replacing the invalid `make new … with properties` AppleScript with the sdef-correct `new window with configuration {…}` form, and close the three gaps the defect exposed: per-window failure reasons invisible at INFO (Rider #1), dishonest total-failure banner copy (Rider #2), and the absence of an automated guard against terminology drift (Fix 4). The phase ends with both default test lanes green and the merge-gating live validation confirmed.

**Why this order**: The specification defines four coordinated changes with a single root cause, all confined to `internal/spawn` and its CLI/picker seams. The impact area is one package; each change is surgical; regression verification is delivered as lockstep updates to existing tests rather than a separate phase. Splitting into per-fix phases would create trivial single-task phases (an anti-pattern) with no independently valuable intermediate state — the riders and the guard have no hard ordering dependency that demands separate checkpoints. A single vertical phase delivers the complete working fix in one increment.

**Acceptance**:
- [ ] `ghosttyScriptTemplate` in `internal/spawn/ghostty.go` uses the single-statement `tell application "Ghostty" / new window with configuration {command:"%s", wait after command:true} / end tell` form — no `make`, no `with properties`, no intermediate `set surfaceConfig` variable
- [ ] The record literal carries exactly the two sdef-defined `surface configuration` fields: `command` (text) and `wait after command` (boolean `true`); the single `%s` is the `ghosttyEmbed`-escaped, space-joined composed argv supplied as a `fmt.Sprintf` argument
- [ ] The false "validated (Ghostty 1.3.1)" comment is corrected to describe the actual sdef-correct `new window with configuration` form
- [ ] `ghosttyEmbed` escaping is confirmed (not assumed) to hold under the relocated `%s`: a `%` in the payload stays inert and the backslash-before-quote escape order is unchanged in the double-quoted `command:"…"` string context
- [ ] `ghosttyOpenScript`, `ghosttyOpenArgv`, the `osascriptRunner` exec seam, and `mapGhosttyResult` outcome mapping remain unchanged
- [ ] `internal/spawn/ghostty_command_test.go` (`TestGhosttyOpenScript`) drops the stale `"surface configuration"` expectation and instead asserts `new window`, `with configuration`, `command:"…"`, and `wait after command`
- [ ] `LogWindowResults` (`internal/spawn/logemit.go`) emits WARN with the distinct message `external window failed` (attrs `session`/`ack`/`detail`) for any non-permission failed window — both `AckFailed` and `AckTimeout` — and DEBUG `external window` for confirmed windows and the permission-required window; the closed `spawn` catalog gains exactly one new message string (`external window failed`, WARN) and no new attr keys
- [ ] `internal/spawn/logemit_test.go` is updated in lockstep: the `ack=timeout` non-permission case now expects WARN `external window failed`; `TestLogWindowResults_OneDebugPerWindow` and the mixed-case assertions in `TestLogBatchSummary` reflect that the DEBUG-per-window count is no longer `len(results)`
- [ ] `PartialFailureMessage` (`internal/spawn/message.go`) takes the `othersOpened bool` signal: `othersOpened == false` renders `… failed to open — nothing opened` (single and multi-name), `othersOpened == true` renders the unchanged `… failed to open — others left open`; the permission-wall and empty-`failed` branches are unchanged
- [ ] Both callers derive `othersOpened = len(confirmed) > 0` from the shared `spawn.PartitionResults` chokepoint: `cmd/spawn.go` (CLI exit-1 error) and `internal/tui/burst_partial_failure.go` (`burstPartialFailureFlash`); the trigger self-attach never counts as an "other"
- [ ] Parity tests (`message_test.go` + `burst_partial_failure_test.go`) assert byte-identical CLI/picker output for total failure (single-name and multi-name → `— nothing opened`) and genuine partial (`— others left open`)
- [ ] A new `//go:build ghosttycompile`-gated test in `internal/spawn` feeds `ghosttyOpenScript(<representative composed argv>)` through `osacompile -e <script> -o <t.TempDir()/probe.scpt>` and asserts a zero exit; it `t.Skip`s cleanly when not macOS or when `Ghostty.app` is absent; the live-Mac assumption about whether Ghostty must be running for terminology resolution is confirmed and the precondition adjusted so the guard never produces a false failure unrelated to the template
- [ ] `go test ./...` (unit) and `go test -tags integration -p 1 ./...` (integration) both pass, inclusive of the lockstep updates; the `manual` and `ghosttycompile` tags remain excluded from both lanes
- [ ] Merge-gating live validation passes on a live Mac inside Ghostty: `go test -tags manual -run TestManual_OpenWindow_OpensRealGhosttyWindow ./internal/spawn/` passes (a real window opens and runs the command), and a real ≥3-session picker multi-select burst confirms `opened 3/3` with token acks landing and the trigger self-attaching

#### Tasks
status: approved
approved_at: 2026-07-17

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| ghostty-spawn-zero-windows-1-1 | Correct the native Ghostty AppleScript template | % in payload stays inert (format arg), backslash-before-quote escape order holds under relocated %s, downstream mapGhosttyResult mapping (clean exit → Success) unchanged, false "validated" comment corrected |
| ghostty-spawn-zero-windows-1-2 | Surface per-window failure reason at WARN | AckTimeout-after-OutcomeSuccess emits WARN with benign success detail (ack=timeout distinguishes mode), permission-required window excluded from WARN (no double-report), confirmed window stays DEBUG, DEBUG-per-window count no longer len(results) |
| ghostty-spawn-zero-windows-1-3 | Honest total-failure banner copy | total failure single-name (N=2, one failed external window), total failure multi-name, permission-wall branch unchanged, degenerate empty-failed branch returns "" unchanged, trigger self-attach never counts as an "other", CLI/picker byte-identical parity |
| ghostty-spawn-zero-windows-1-4 | Compile-check regression guard (ghosttycompile-tagged) | t.Skip when not macOS or Ghostty.app absent, osacompile requires throwaway output target under t.TempDir, confirm/adjust gate if Ghostty must be running for terminology resolution, opens no window |
| ghostty-spawn-zero-windows-1-5 | Merge-gating live validation (manual Ghostty test + real ≥3-session burst) | manual and ghosttycompile tags stay excluded from both default lanes, compile-only validation insufficient (functional proof required), acks land and trigger self-attaches (net-N, not N+1) |

### Phase 2: Analysis (Cycle 1)

**Goal**: Address findings from Analysis (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| ghostty-spawn-zero-windows-2-1 | Make burstPartialFailureFlash self-contained (remove the double partition / permission scan) | PartitionResults/FirstPermission each run at most once (no discarded recomputation); byte-identical flash for total-failure ("— nothing opened"), genuine-partial ("— others left open"), permission-wall (verbatim Guidance), degenerate-empty ("" → no band); CLI/picker parity preserved; cmd/spawn.go untouched; helper signature no longer mixes passed-in half with re-derived half |
| ghostty-spawn-zero-windows-2-2 | Close the Fix 4 compile-guard's installed-but-not-running precondition gap | installed-but-not-running cannot produce a false t.Fatalf template-drift (passes on clean resolution or t.Skips/precondition-gated); genuine -2741 drift to pre-fix `make new surface configuration` form still fails; corrected committed template still passes; no default/integration-lane behaviour change (compiles into neither lane); executor chooses path (a) live-Mac evidence or (b) defensive code change at implementation time |
