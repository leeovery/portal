---
topic: restore-host-terminal-windows
cycle: 1
total_proposed: 6
---
# Analysis Tasks: restore-host-terminal-windows (Cycle 1)

## Task 1: Hoist result classification into internal/spawn and add the missing picker permission event
status: pending
severity: medium
sources: architecture

**Problem**: The spec designates a single "count-semantics chokepoint" — a `WindowResult` is "opened" exactly when `Ack==AckConfirmed`, the confirmed/failed partition drives leave-what-opened retry, and "first permission wins" drives the burst-stop. That rule is a pure function of `[]spawn.WindowResult`, yet it is re-implemented at ≥5 sites across the two callers: `cmd/spawn.go` `tallyWindowResults` (opened/failed partition) + `firstPermission` (permission scan); `internal/tui` `emitBurstSummary` (opened count), `handleBurstPartialFailure` (confirmed/failed partition), `burstAllConfirmed` (all-confirmed check), and `burstPartialFailureFlash` (permission scan). The code comments themselves concede the paths must be kept "byte-identical" by hand. Separately, the permission-required outcome — a distinct entry in the closed `spawn` event catalog — diverges: the CLI emits a dedicated INFO `spawn: permission required — nothing self-attached` (`logSpawnPermission`) and skips the opened/total summary, but the picker's `handleBurstPartialFailure` unconditionally calls `emitBurstSummary`, so a permission burst logs the generic `spawn: opened 0/N` and never emits the permission event (it survives only in the DEBUG per-window line). `emitBurstSummary`'s own header lists the CLI mirrors it claims parity with — `logSpawnPermission` is silently absent. The picker is the dominant real path, so a spec-catalogued event is effectively unreachable in production.

**Solution**: Add the classification to `internal/spawn` as the single source both callers derive from — a `WindowResult.Confirmed()` predicate plus `PartitionResults(results) (confirmed, failed []string)` and `FirstPermission(results) (WindowResult, bool)`. Rewire `cmd/spawn.go`'s `tallyWindowResults`/`firstPermission` and the TUI's `emitBurstSummary`/`handleBurstPartialFailure`/`burstAllConfirmed`/`burstPartialFailureFlash` to call these instead of re-looping over `Ack`/`Outcome`. Give the picker an `emitPermission` mirror (resolution/terminal/bundle_id/detail, no counts) and route the permission arm of `handleBurstPartialFailure` through it instead of `emitBurstSummary`, matching the CLI branch. `FirstPermission` naturally serves both the flash and the log, so detection lives in one place.

**Outcome**: opened/failed/permission semantics live in one place in `internal/spawn` alongside the existing `PreflightMissing`/`QuoteJoin`/`GoneVerb` helpers; a future change to what "confirmed" means is a single edit; the permission-required event is emitted identically on both the CLI and picker paths, restoring one-service lockstep for the dominant (picker) path.

**Do**:
1. In `internal/spawn` (add to `burst.go` or a new `classify.go`): add `func (r WindowResult) Confirmed() bool { return r.Ack == AckConfirmed }`; `func PartitionResults(results []WindowResult) (confirmed, failed []string)` returning session names in list order (confirmed = `Confirmed()`; failed = everything else — keep the current unification of `AckFailed` + `AckTimeout` into "failed"); `func FirstPermission(results []WindowResult) (WindowResult, bool)` scanning for `Result.Outcome == OutcomePermissionRequired`.
2. `cmd/spawn.go`: keep `tallyWindowResults`' per-window DEBUG loop but derive `opened`/`failed` via the shared helper (e.g. `PartitionResults` + `len(confirmed)`); replace `firstPermission`'s body with `spawn.FirstPermission` (or delete it and call `spawn.FirstPermission` at the call site).
3. `internal/tui/burst_observability.go`: derive `emitBurstSummary`'s `opened` count via `r.Confirmed()`; add an `emitPermission` method mirroring `logSpawnPermission` — `Info("permission required — nothing self-attached", "resolution", ..., "terminal", ..., "bundle_id", ..., "detail", ...)` with NO opened/total/batch attrs.
4. `internal/tui/burst_partial_failure.go`: in `handleBurstPartialFailure`, detect the permission arm via `spawn.FirstPermission`; when present, call `emitPermission` (not `emitBurstSummary`) — matching the CLI's skip-the-summary branch — then continue the existing leave-what-opened mutation + guidance flash. Replace the local confirmed/failed loop with `spawn.PartitionResults`; make `burstPartialFailureFlash`'s permission scan use `spawn.FirstPermission`.
5. `internal/tui/burst_progress.go`: rewrite `burstAllConfirmed`'s per-result `Ack` check via `r.Confirmed()` (keep the `len(msg.Results)==len(m.burstExternal)` and `msg.Err` guards).

**Acceptance Criteria**:
- opened/failed/permission classification is derived from the new `spawn` helpers at every site; no residual hand-rolled `r.Ack == spawn.AckConfirmed` / `Outcome == OutcomePermissionRequired` loop remains in `cmd/spawn.go` or `internal/tui` outside the per-window DEBUG-emit loops.
- A permission-required burst on the picker path emits the `spawn: permission required — nothing self-attached` INFO event with the closed resolution/terminal/bundle_id/detail attrs and does NOT emit the generic `opened 0/N` summary — byte-identical to the CLI's `logSpawnPermission` output.
- The leave-what-opened selection mutation, guidance flash, and exit/quit behaviour of both callers are unchanged for all non-permission outcomes.
- Only the closed `spawn` attr keys appear on the new event; all existing spawn unit + integration tests remain green.

**Tests**:
- Unit (spawn): table test for `PartitionResults` over mixed `AckConfirmed`/`AckTimeout`/`AckFailed` slices (order preserved; timeout + failed both land in `failed`); `FirstPermission` returns the first permission window and `false` when none; `Confirmed()` truth table.
- Unit (tui): a picker permission-required `spawnCompleteMsg` emits exactly the `emitPermission` record (assert via the logtest sink) and NOT an opened/total summary; a partial-failure with no permission still emits `emitBurstSummary` as before.
- Unit (cmd): existing `tallyWindowResults`/`firstPermission` tests pass unchanged against the rewired implementations (or are updated to call the shared helpers).
- Cross-caller parity: assert the CLI `logSpawnPermission` and picker `emitPermission` produce the same rendered body + attr set for the same identity/resolution/detail.

## Task 2: Extract shared gone-session / unsupported-terminal message renderers into internal/spawn/message.go
status: pending
severity: medium
sources: duplication

**Problem**: The pre-flight gone-session sentence `"%s %s gone — nothing opened"` (rendered by threading the already-shared `spawn.QuoteJoin` + `spawn.GoneVerb`) is repeated verbatim at five production sites: the CLI abort error (`cmd/spawn.go:134`, with a `spawn: ` prefix), the CLI outcome log (`cmd/spawn.go:256` `logSpawnGone`), the picker outcome log (`internal/tui/burst_observability.go:83` `emitPreflightAbort`), the picker abort banner (`internal/tui/burst_preflight_abort.go:46`), and the capture-harness seed banner (`internal/tui/burst_preflight_abort.go:85`). The team extracted the sub-primitives (`QuoteJoin`/`GoneVerb` live in `message.go` precisely so the two callers name sessions identically) but stopped short of the full sentence, so a copy edit in one site silently drifts the other four — violating the spec's byte-identical-naming mandate. The same hazard, at lower multiplicity, exists for the unsupported-terminal `— nothing opened` line: `cmd/spawn.go:245` `unsupportedSpawnMessage` and `internal/tui/burst_progress.go:422` `unsupportedFlashText` hand-build the same `"unsupported terminal — %s · %s — nothing opened"` / `"no host-local terminal — nothing opened"` copy, differing only in the CLI's `spawn: ` prefix (the `burst_progress.go` comment documents it as a manual mirror). This is genuine copy-paste-drift, distinct from the sanctioned parallel log-emission pair (`logSpawnSummary`/`emitBurstSummary` emit structurally-parallel records by design — the concern here is the message TEXT, not the emission structure).

**Solution**: Add `spawn.GoneMessage(names []string) string` and `spawn.UnsupportedNoopMessage(id Identity) string` to `internal/spawn/message.go` — the established home for cross-caller message parity, alongside `QuoteJoin`/`GoneVerb` — each returning the bare body with no `spawn:` prefix and no ⚠ glyph. All five gone sites and both unsupported sites call the single renderer; the CLI wraps the returned body with its `spawn: ` prefix where it needs it.

**Outcome**: The spec's identical-naming requirement is structurally enforced by one renderer per message rather than five (+two) hand-kept copies; a wording change is a single edit.

**Do**:
1. `internal/spawn/message.go`: add `GoneMessage(names []string) string` = `fmt.Sprintf("%s %s gone — nothing opened", QuoteJoin(names), GoneVerb(len(names)))`; add `UnsupportedNoopMessage(id Identity) string` returning `"no host-local terminal — nothing opened"` when `id.IsNull()` else `fmt.Sprintf("unsupported terminal — %s · %s — nothing opened", id.Name, id.BundleID)`. (`Identity` is already an in-package type used by the burster.)
2. `cmd/spawn.go`: replace the inline `fmt.Sprintf` in the pre-flight abort return (~line 134) with `fmt.Errorf("spawn: %s", spawn.GoneMessage(gone))`; replace `logSpawnGone`'s body with `logger.Info(spawn.GoneMessage(gone))`; replace `unsupportedSpawnMessage`'s body with `"spawn: " + spawn.UnsupportedNoopMessage(id)`.
3. `internal/tui/burst_observability.go`: replace `emitPreflightAbort`'s inline `fmt.Sprintf` with `spawn.GoneMessage(gone)`.
4. `internal/tui/burst_preflight_abort.go`: replace the picker abort banner (~line 46) and the capture-harness seed banner (~line 85) gone-message sites with `spawn.GoneMessage`.
5. `internal/tui/burst_progress.go`: replace `unsupportedFlashText`'s body with `spawn.UnsupportedNoopMessage(id)`.
6. Confirm the ⚠ glyph is still added by the notice band (`statusGlyph`), not the returned body — `GoneMessage`/`UnsupportedNoopMessage` carry no glyph and no prefix.

**Acceptance Criteria**:
- The literal `"gone — nothing opened"` and `"unsupported terminal — %s · %s — nothing opened"` / `"no host-local terminal — nothing opened"` strings each appear exactly once, inside `internal/spawn/message.go`; no other production site hand-builds them.
- The CLI's `spawn: ` prefix is applied at the CLI call sites only; picker/banner sites render the bare body; the notice band still prepends ⚠ exactly once.
- Rendered output at all seven sites is byte-identical to today.

**Tests**:
- Unit (spawn): `GoneMessage` for one name (`'s2' is gone — nothing opened`) and ≥2 names (`'s2', 's4' are gone — nothing opened`); `UnsupportedNoopMessage` for `IsNull` vs a named identity (name + bundle id, U+00B7 middot).
- Regression: existing CLI pre-flight-gone and unsupported-noop message assertions and the picker flash/banner assertions pass unchanged (byte-identical copy).

## Task 3: Extract the shared exec-boundary and failure-detail helpers for the two spawn adapters
status: pending
severity: low
sources: duplication

**Problem**: The `osascriptRunner` and `recipeRunner` seams are deliberately separate (per the design note) and must stay separate, but the concrete plumbing behind them is duplicated. `execOsascriptRunner.Run` (`ghostty.go:78-90`) and `execRecipeRunner.Run` (`configadapter.go:41-53`) have byte-identical bodies (`exec.Command(argv[0], argv[1:]...)` → `log.CombinedOutputWithContext` → nil-err fast path → `errors.As(*exec.ExitError)` → `combineOutput`+code → else err), already jointly consuming the shared `combineOutput`. Separately, `failureDetail` (`ghostty.go:174-186`) and `recipeFailureDetail` (`configadapter.go:89-101`) are near-identical 13-line blocks differing only in the never-empty fallback label (`"ghostty osascript exit %d"` vs `"recipe exit %d"`). Each pair is two instances (borderline against the Rule of Three), but byte-identical exec-boundary + error-formatting logic is exactly the silent-drift hazard where one side gets a fix or behaviour tweak and the other does not, and one half (`combineOutput`) is already extracted.

**Solution**: Extract two package-private helpers in `internal/spawn`: `runArgvCombined(argv []string) (out string, exitCode int, err error)` holding the shared exec→combined-output→exit-code body, and `execFailureDetail(out string, exitCode int, err error, fallbackLabel string) string` holding the shared detail-formatting logic parameterised by the fallback label. Both runner `Run` methods call the former; both `failureDetail`/`recipeFailureDetail` call the latter (passing their respective labels). The two Adapters and their two distinct runner interfaces stay fully separate — only the identical plumbing is shared, preserving the separate-seam design intent.

**Outcome**: One exec-boundary body and one failure-detail formatter, so a fix or behaviour tweak lands once; the deliberately-separate runner/adapter seams are untouched.

**Do**:
1. `internal/spawn`: add `runArgvCombined(argv)` reproducing the current identical `Run` body (`exec.Command(argv[0], argv[1:]...)` → `log.CombinedOutputWithContext` → nil → `(string(out),0,nil)`; `errors.As` `*exec.ExitError` → `(combineOutput(out,err), exitErr.ExitCode(), nil)`; else → `(string(out),0,err)`).
2. Replace `execOsascriptRunner.Run` and `execRecipeRunner.Run` bodies with `return runArgvCombined(argv)`.
3. Extract `execFailureDetail(out, exitCode, err, fallbackLabel)` with the shared body from the two `failureDetail` funcs; have `failureDetail` call it with `"ghostty osascript exit %d"` and `recipeFailureDetail` with `"recipe exit %d"`.
4. Keep both runner interfaces and both Adapter types separate — no seam merge.

**Acceptance Criteria**:
- Both runner `Run` methods delegate to `runArgvCombined`; no duplicated exec body remains.
- Both failure-detail functions delegate to `execFailureDetail` with only their fallback label differing.
- The `osascriptRunner` / `recipeRunner` interfaces and their Adapter implementations remain distinct (no seam merge).
- Adapter unit tests (fake runners) and the real-exec paths behave identically to today.

**Tests**:
- Unit (spawn): `runArgvCombined` over a clean exit (`out,0,nil`), a non-zero exit (combined out + code, nil err), and a missing-binary/non-exit failure (err surfaced). `execFailureDetail` returns the same string as the two originals for representative inputs, including the never-empty fallback for each label.
- Regression: existing ghostty + configadapter adapter tests pass unchanged.

## Task 4: Remove or unexport the dead spawn.AttachCommand public API
status: pending
severity: low
sources: architecture

**Problem**: `spawn.AttachCommand` (`command.go:24`) is a public function that resolves the executable and composes the attach argv, but no production caller uses it. `Burster.Run` resolves `os.Executable` once up front and calls the pure `composeAttachArgv` per window (documented at `burst.go:113` as "not `AttachCommand`"); both the CLI `runSpawn` and the picker `dispatchBurst` go through the burster. Grep shows the only remaining references are a doc-comment and a `spawntest` comment. It is public surface exercised by nothing, so it can drift from the real composition path (`composeAttachArgv`) without any test noticing a behavioural mismatch.

**Solution**: Remove `AttachCommand` (preferred) — or unexport it to a package-private helper only if a test genuinely needs the resolve+compose combination. Do NOT make the burster compose through it: `Burster.Run` deliberately resolves the executable once up front, and routing per-window through `AttachCommand` would reintroduce a redundant per-window `os.Executable` read. Keep the spawn package's public surface to what is actually reached — `composeAttachArgv` already carries the tested composition and `ExecutableResolver` is still used by the burster.

**Outcome**: No untested public composition path that can silently diverge from `composeAttachArgv`; the spawn package's exported surface reflects what is actually reached.

**Do**:
1. Grep the repo for `AttachCommand` references (expect: the definition, its doc-comment, a `spawntest` comment; no production caller). Confirm no test asserts its behaviour beyond mere existence.
2. Remove `AttachCommand` (or unexport only if a real caller remains — removal preferred, since `Burster.Run` already resolves once up front).
3. Update/remove the stale doc-comment and `spawntest` comment references.
4. Keep `ExecutableResolver` (still used by the burster) and `composeAttachArgv` unchanged.

**Acceptance Criteria**:
- `spawn.AttachCommand` is no longer exported (removed, or unexported only if a real caller remains).
- `ExecutableResolver` and `composeAttachArgv` are unchanged and remain the sole production composition path.
- The package builds and all spawn tests pass; no dangling reference to the removed symbol remains.

**Tests**:
- Build/compile: `go build ./...` and `go test ./internal/spawn/...` green after removal.
- If unexported instead of removed: a unit test exercises the retained helper so it is no longer dead.

## Task 5: Re-derive the marked set at burst decision time so a deferred N≥2 Enter cannot open a stale selection
status: pending
severity: low
sources: architecture

**Problem**: When an N≥2 Enter lands before async terminal detection resolves, `beginBurst` stashes `m.orderedMarkedSessions()` into `pendingBurstOrdered` and returns WITHOUT setting `burstPending` (`burst_progress.go:370`). The input-lock guard (`updateSessionList`: `if m.burstPending`) is therefore not engaged during the defer window, so a subsequent `m` toggle is processed normally and mutates `selectedSessions` — but not the stashed snapshot. When `terminalDetectedMsg` fires it calls `decideBurst(m.pendingBurstOrdered)`, spawning the STALE set (a just-unmarked session still opens; a just-marked one is skipped). Detection begins on Sessions-page entry and is near-instant, so the window is tiny and requires a mark change between Enter and the detection reply — but correctness rests on caller/detection timing rather than being self-contained, the same "correctness depends on caller discipline" smell the spec warns against.

**Solution**: Re-derive `orderedMarkedSessions()` from the live `selectedSessions` at the detection-resolved point (feeding `decideBurst`) rather than replaying the `pendingBurstOrdered` snapshot — the cheaper fix that keeps the burst self-contained and makes the spawned set a pure function of the live selection at decision time. (Alternative: extend the burst input-lock to cover the pre-detection defer so the marked set cannot change under it — but the re-derive is preferred.)

**Outcome**: The burst opens exactly the marked set as it stands when detection resolves; a mark toggle during the tiny defer window is honoured, removing the timing-dependent stale-snapshot path.

**Do**:
1. In `internal/tui/burst_progress.go`, at the `terminalDetectedMsg` resolution point that consumes `pendingBurstEnter`/`pendingBurstOrdered` (see `model.go:2466`, `model.go:3280`), re-derive the ordered set via `m.orderedMarkedSessions()` (from live `selectedSessions`) instead of passing `m.pendingBurstOrdered` to `decideBurst`.
2. Retain `pendingBurstEnter` as the "an Enter is deferred" flag; drop the reliance on the stale `pendingBurstOrdered` payload for the spawned set (remove the field if it becomes unused, or verify no other reader depends on it).
3. Verify `beginBurst`'s non-resolved branch still dispatches detection when needed so the defer resolves.

**Acceptance Criteria**:
- A mark toggle between a deferred Enter and `terminalDetectedMsg` is reflected in the spawned set: a session unmarked during the window is NOT opened; one newly marked IS opened.
- The already-resolved (non-deferred) path is behaviourally unchanged.
- `pendingBurstOrdered` is either removed or demonstrably no longer the source of the spawned set.

**Tests**:
- Unit (tui): dispatch an N≥2 Enter with `detectResolved=false`; toggle a mark; fire `terminalDetectedMsg`; assert `BurstExternal`/`BurstTrigger` reflect the post-toggle live selection, not the pre-toggle snapshot.
- Regression: the already-resolved `beginBurst`→`decideBurst` path and existing burst dispatch tests pass unchanged.

## Task 6: Resolve the spawn-failure/permission flash vs multi-select banner notice-slot precedence
status: pending
severity: low
sources: standards

**Problem**: The spec (§ Mode affordance → "Notice-band precedence (single slot, highest wins)") enumerates one ordered slot where a transient error/guidance flash — grouped as "pre-flight abort / spawn-failure / permission" — outranks and REPLACES the multi-select banner. The implementation realizes this across two physical rows and treats the three flash-tier members inconsistently. The pre-flight abort is a section-header claimant (`applySectionHeader`) checked ABOVE the multi-select `N selected` banner, so it replaces it (matching its delivered Paper frame). But the spawn-failure and permission outcomes route through `setFlash` → the §11 ▌-barred notice band (`activeNoticeBand` returns the flash regardless of `multiSelectMode`) — a SEPARATE row that does not suppress the section-header multi-select banner (`notice_band.go:347-364`, `model.go:4676-4739`, `burst_partial_failure.go:36-78`). Since `handleBurstPartialFailure` keeps `multiSelectMode` true, after a partial/permission failure the picker renders BOTH the `N selected` banner AND the `⚠ … failed to open` flash at once — the flash never "wins the slot." Impact is visual-only and arguably informative (you see the error and that you remain in multi-select with the retry set marked), and there is no delivered design frame for this flash (the spec records it as a design residual), so exact placement was underspecified. Flagged because it is a literal divergence from an explicitly-decided precedence and is inconsistent with how the sibling pre-flight-abort flash was realized.

**Solution**: Confirm whether the two-row split is the intended reading of "single slot, highest wins." If it is (it matches the delivered frames and is arguably better), make NO behavioural change but add an explicit in-code note at the precedence seam so a later reader does not "fix" it back to strict single-slot. If strict single-slot was intended, suppress the multi-select section-header banner while a warning/permission flash owns the notice band — mirroring how `unsupportedBannerActive` already steps aside for multi-select — so the flash presents alone. This task requires a decision first; the code change is small either way.

**Outcome**: The precedence behaviour is a deliberate, documented decision rather than an undocumented divergence — either an explicit "two-row is intended" note at the seam, or a single-slot suppression that matches the spec's literal precedence.

**Do**:
1. Surface the decision for approval: two-row (document) vs strict single-slot (suppress banner) — the load-bearing choice.
2. If two-row is confirmed: add a comment at the `activeNoticeBand` / `applySectionHeader` precedence seam (`notice_band.go` ~347-364, `model.go` ~4676-4739) recording that the spawn-failure/permission flash deliberately co-renders with the `N selected` multi-select banner (informative: error + retained multi-select + marked retry set), that this is the intended reading of "single slot, highest wins" for this flash tier, and to NOT collapse it to strict single-slot.
3. If strict single-slot is confirmed: while a warning/permission flash owns the notice band, suppress the multi-select section-header banner (mirror the existing `unsupportedBannerActive` step-aside), so the flash presents alone; keep the retry-set selection mutation unchanged.

**Acceptance Criteria**:
- The chosen behaviour is explicit — either a documenting comment at the precedence seam (two-row) or a banner-suppression path (single-slot), not the current silent divergence.
- If suppression is chosen: after a partial/permission failure the notice slot shows the flash alone (no concurrent `N selected` banner), and multi-select mode + the marked retry set are otherwise unchanged.
- If documentation is chosen: no behavioural change; the note references the spec precedence clause and the pre-flight-abort sibling for contrast.

**Tests**:
- If suppression: unit (tui) asserting a partial/permission `spawnCompleteMsg` renders the flash and NOT the `N selected` banner, while `selectedSessions` still holds the retry set.
- If documentation only: no new test required; existing render tests remain green (a comment needs no assertion).
