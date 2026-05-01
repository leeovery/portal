---
phase: 1
phase_name: Fix signal-hydrate Argv Parse and Migrate Existing Hook Installations
total: 3
---

## scrollback-not-restored-with-non-zero-base-index-1-1 | approved

### Task 1: Add `--` Separator to signal-hydrate Hook Command

**Problem**: When tmux fires `client-attached` / `client-session-changed` for a session whose name begins with `-` (e.g. `-dotfiles-HM9Zhw`), the resolved hook command `portal state signal-hydrate -dotfiles-HM9Zhw` is parsed by cobra/pflag as a short-flag cluster, fails with `unknown shorthand flag: 'd'`, and exits non-zero before `runSignalHydrate` runs. No FIFO byte is written; the hydrate helper times out at 3s and exec's a bare `$SHELL` with no scrollback replay. Leading-dash session names arise naturally from `SanitiseProjectName` substituting `.` → `-` for projects whose basename starts with `.` (`.dotfiles`, `.config`, etc.).

**Solution**: Insert the `--` end-of-flags separator into `signalHydrateCommand` so the resolved hook command becomes `portal state signal-hydrate -- #{session_name}`. Tighten `signalHydrateSubstring` to `"portal state signal-hydrate --"` so the dedupe check in `RegisterHookIfAbsent` distinguishes the new fixed entry from any pre-existing un-separated entry — without this, the old broken entry would suppress the new install on idempotent re-registration. Add a content unit test that pins the constant's shape so future edits cannot silently regress.

**Outcome**: `signalHydrateCommand` and `signalHydrateSubstring` in `internal/tmux/hooks_register.go` carry the `--` separator. A cobra-level argv-parse test exercising `runSignalHydrate` via `Execute()` with a leading-dash session name passes (exit 0, FIFO byte written). A constant-shape unit test pins the `--` placement before `#{session_name}` so a future edit removing the separator fails the test.

**Do**:
- In `internal/tmux/hooks_register.go`, change the body of the `signalHydrateCommand` const literal from `portal state signal-hydrate #{session_name}` to `portal state signal-hydrate -- #{session_name}` (single space either side of `--`).
- In the same file, change `signalHydrateSubstring` from `"portal state signal-hydrate"` to `"portal state signal-hydrate --"`.
- Update the doc comment above `signalHydrateCommand` to mention the `--` end-of-flags separator and why (cobra/pflag short-flag interpretation of leading-dash session names).
- Add a unit test in `internal/tmux/hooks_register_test.go` (create if missing) named `TestSignalHydrateCommand_HasEndOfFlagsSeparator` asserting that `signalHydrateCommand` contains the substring ` -- #{session_name}` (with the spaces). Add a second assertion that `signalHydrateSubstring == "portal state signal-hydrate --"`.
- Add a cobra-level argv-parse test in `cmd/state_signal_hydrate_test.go` named `TestStateSignalHydrate_AcceptsLeadingDashSessionViaCobraExecute`. It must:
  - Inject a stub `signalHydrateRunFunc` (the existing package-level seam) that records the received `signalHydrateConfig.Session` and returns nil (no real FIFO/tmux work). Restore the original via `t.Cleanup`.
  - Invoke the cobra command tree the same way production does — `rootCmd.SetArgs([]string{"state", "signal-hydrate", "--", "-dotfiles-HM9Zhw"})` followed by `rootCmd.Execute()` (or the cmd-package equivalent helper). The `--` here is the argv separator the hook will pass; the test is asserting that, with the separator in place, cobra accepts `-dotfiles-HM9Zhw` as a positional argument and the run func is invoked with `Session == "-dotfiles-HM9Zhw"`.
  - Assert `Execute()` returned nil and the recorded session matches `-dotfiles-HM9Zhw`.
  - Add a second sub-case that omits the `--` and asserts `Execute()` returns a non-nil error containing `unknown shorthand flag` (regression guard: confirms the `--` is what unblocks the parse, not some unrelated change to cobra config).
- Do not alter `stateSignalHydrateCmd` itself (no `DisableFlagParsing`); the spec explicitly rejected that approach in Out of Scope.

**Acceptance Criteria**:
- [ ] `signalHydrateCommand` literal in `internal/tmux/hooks_register.go` resolves to `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate -- #{session_name}"`.
- [ ] `signalHydrateSubstring == "portal state signal-hydrate --"`.
- [ ] `TestSignalHydrateCommand_HasEndOfFlagsSeparator` passes and would fail if the `--` were removed from either constant.
- [ ] `TestStateSignalHydrate_AcceptsLeadingDashSessionViaCobraExecute` passes: cobra `Execute()` with `["state", "signal-hydrate", "--", "-dotfiles-HM9Zhw"]` returns nil and the run func receives `Session == "-dotfiles-HM9Zhw"`.
- [ ] The negative sub-case (without `--`) yields an `Execute()` error containing `unknown shorthand flag`.
- [ ] No test added uses `t.Parallel()`. Existing tests in `cmd/state_signal_hydrate_test.go` that bypass argv parsing remain unchanged.
- [ ] `go build ./...` and `go test ./...` pass.

**Tests**:
- `TestSignalHydrateCommand_HasEndOfFlagsSeparator` — pins the literal shape of `signalHydrateCommand` and `signalHydrateSubstring`; future regression of either losing the `--` fails the test.
- `TestStateSignalHydrate_AcceptsLeadingDashSessionViaCobraExecute` (positive) — leading-dash session name (`-dotfiles-HM9Zhw`) round-trips through cobra `Execute()` with `--` and the run func is invoked with the correct session.
- `TestStateSignalHydrate_AcceptsLeadingDashSessionViaCobraExecute` (negative sub-case) — same leading-dash name without `--` produces the historical `unknown shorthand flag` parse error, confirming the separator is load-bearing.
- Existing `cmd/state_signal_hydrate_test.go` cases for internal-dash-only names (e.g. `myrepo-AbCdEf`) continue to pass — confirms the change does not regress non-dash-prefixed names.

**Edge Cases**:
- **Leading-dash session name**: Primary failure mode; test asserts the new path makes it work via cobra `Execute()`.
- **Internal-dash-only session name** (e.g. `myrepo-AbCdEf`): Already worked; existing test coverage in `cmd/state_signal_hydrate_test.go` must continue to pass.
- **Future regression of the constant losing the `--`**: Pinned by `TestSignalHydrateCommand_HasEndOfFlagsSeparator`; deleting the separator fails the constant-shape assertion before the system test would catch it.

**Context**:
> Spec § "Primary Root Cause": cobra/pflag parses leading-`-` tokens as short-flag clusters; `stateSignalHydrateCmd` defines no flags but cobra inherits parent persistent flags so the parse is still attempted. Empirical verification:
> ```
> $ portal state signal-hydrate -dotfiles-HM9Zhw      → exit 1 (parse error)
> $ portal state signal-hydrate -- -dotfiles-HM9Zhw   → exit 0
> ```
> Spec § "Out of Scope" rejects `DisableFlagParsing = true` because it loses future flag capacity and is less intent-preserving than `--`. The chosen fix is the `--` separator on the hook command only — manual user invocations of `portal state signal-hydrate -<dashed-name>` from a shell prompt are intentionally unfixed.
>
> The package-level `signalHydrateRunFunc` seam already exists at `cmd/state_signal_hydrate.go:127` for exactly this kind of argv-only test (see `TestStateInternalSubcommandsAcceptValidArgv` in `cmd/state_test.go`).

**Spec Reference**: `.workflows/scrollback-not-restored-with-non-zero-base-index/specification/scrollback-not-restored-with-non-zero-base-index/specification.md` § "Primary Root Cause", § "Part 1 — Add `--` End-Of-Flags Separator to `signal-hydrate` Hook", § "Testing Requirements" items 1 and 3.

---

## scrollback-not-restored-with-non-zero-base-index-1-2 | approved

### Task 2: Migrate Pre-Existing Un-Separated Hook Entries on Bootstrap

**Problem**: Existing Portal installs already have `client-attached` / `client-session-changed` hook entries that lack the `--` separator. After Task 1, `RegisterHookIfAbsent`'s dedupe substring is tightened to `"portal state signal-hydrate --"`, so the new fixed entry would simply append alongside the old broken entry — both hooks fire on each event, the broken one still errors, and hydration still fails for leading-dash sessions. Without an explicit migration, upgraded users remain broken.

**Solution**: Before installing the fixed entry, scan every event in `hydrationTriggerEvents` and evict any entry whose command matches the full Portal-authored shape (`command -v portal >/dev/null 2>&1 &&` AND `portal state signal-hydrate`) but does NOT contain `portal state signal-hydrate --`. Process indices highest-first so earlier indices are not invalidated by removal. Per-index `UnsetGlobalHookAt` failures are best-effort — log a WARN and continue. Emit a single INFO line per bootstrap when at least one entry was evicted; bootstraps with no evictions are silent. The migration is idempotent: once an install is migrated, subsequent bootstraps perform no eviction work and emit no INFO line.

**Outcome**: A new free function in `internal/tmux/hooks_register.go` performs the eviction scan across `hydrationTriggerEvents` before `RegisterPortalHooks` reaches the hydration-trigger category. After bootstrap, for every event in `hydrationTriggerEvents` exactly one hook entry contains `portal state signal-hydrate`, and a back-to-back second bootstrap is a no-op (no evictions, no INFO line). Hand-authored user hooks lacking the `command -v portal` prefix are not touched. Per-index failures surface as WARN lines and never abort bootstrap.

**Do**:
- In `internal/tmux/hooks_register.go`, add a free function with this shape:
  ```go
  // migrateHydrationHooks evicts pre-existing signal-hydrate hook entries
  // that lack the `--` end-of-flags separator. Best-effort: per-index
  // UnsetGlobalHookAt failures are logged via the supplied logger and do
  // not abort. Returns the count of successfully evicted entries across
  // all events in hydrationTriggerEvents.
  func migrateHydrationHooks(c *Client, log MigrationLogger) (evicted int, err error)
  ```
  Where `MigrationLogger` is a tiny in-package interface (or accept `*state.Logger` directly if the import boundary allows; if not, define a 2-method interface `Info(component, format string, args ...any)` / `Warn(component, format string, args ...any)` that `*state.Logger` satisfies). Match the existing pattern in this package (no logger field on `Client`).
- The function body must:
  1. Call `c.ShowGlobalHooks()` once. On error, return `(0, fmt.Errorf("show-hooks failed: %w", err))` — the caller decides whether to surface as warning or fatal. No eviction is attempted.
  2. Parse via `ParseShowHooks(raw)`.
  3. For each event in `hydrationTriggerEvents`:
     - Collect the indices of entries whose `Command` satisfies the eviction predicate: `strings.Contains(cmd, "command -v portal >/dev/null 2>&1 &&") && strings.Contains(cmd, "portal state signal-hydrate") && !strings.Contains(cmd, "portal state signal-hydrate --")`.
     - Sort the collected indices in **descending** order.
     - For each index, call `c.UnsetGlobalHookAt(event, index)`. On success increment the local evicted counter. On error, log `WARN | bootstrap | failed to evict stale signal-hydrate hook on <event> at index <i>: <err>` via the supplied logger and continue with the remaining indices.
  4. After all events processed, if the local evicted counter is > 0, log a single `INFO | bootstrap | evicted N stale signal-hydrate hook(s) lacking '--' separator` line. If zero, emit nothing (no log noise on the steady-state path).
  5. Return `(evicted, nil)` — even on per-index failures the function returns nil error; failures are visible only via the WARN log lines and partial eviction count. The caller does not abort bootstrap.
- Update `RegisterPortalHooks(c *Client)` to accept (or be wrapped to provide) the logger seam needed by the migration. Two acceptable shapes — choose the one that minimises blast radius:
  - **Option A**: extend `RegisterPortalHooks` to take a logger argument and update call sites in `internal/bootstrapadapter/` accordingly. Preferred if `*state.Logger` already imports cleanly into `internal/tmux`.
  - **Option B**: add a sibling `RegisterPortalHooksWithLogger(c *Client, log MigrationLogger) error` and have the existing `RegisterPortalHooks` delegate to it with a no-op logger. The bootstrap adapter calls the sibling. Pick this if the existing `RegisterPortalHooks` has external callers that would otherwise need updating.
  - In either case, `migrateHydrationHooks` runs **before** the per-category register loop reaches the hydration-trigger category. The simplest placement: call `migrateHydrationHooks` once at the top of `RegisterPortalHooks`/`RegisterPortalHooksWithLogger` body, before the existing loop.
- The migration must scan **every event listed in `hydrationTriggerEvents`** — read the slice at runtime, do not hard-code event names. If the slice is later extended, the migration follows.
- Wire the new logger seam through `internal/bootstrapadapter` (the adapter that constructs hook-registration calls in production) so the bootstrap orchestrator's `*state.Logger` reaches the migration function. Keep the change tightly scoped to the hook-register call path.
- Add a unit test in `internal/tmux/hooks_register_test.go` (or a sibling `hooks_register_migration_test.go`) that — preferring the real-tmux fixture from `internal/tmuxtest` per the spec's testing requirement — verifies the full migration path. See Tests section below.

**Acceptance Criteria**:
- [ ] `migrateHydrationHooks` exists in `internal/tmux/hooks_register.go` with the contract above.
- [ ] Eviction predicate: command must contain BOTH `command -v portal >/dev/null 2>&1 &&` AND `portal state signal-hydrate`, AND must NOT contain `portal state signal-hydrate --`. Loose substring matching on `portal state signal-hydrate` alone is rejected.
- [ ] Indices are processed highest-first; per-index `UnsetGlobalHookAt` failures emit `WARN | bootstrap | failed to evict stale signal-hydrate hook on <event> at index <i>: <err>` and continue.
- [ ] When the eviction count for a given bootstrap is > 0, exactly one INFO line `INFO | bootstrap | evicted N stale signal-hydrate hook(s) lacking '--' separator` is written. When zero, no INFO line is emitted.
- [ ] `RegisterPortalHooks` (or its logger-aware sibling) calls `migrateHydrationHooks` before the hydration-trigger category install runs.
- [ ] After bootstrap on an install with pre-existing un-separated entries, for each event in `hydrationTriggerEvents` the count of entries whose command contains `portal state signal-hydrate` equals exactly 1.
- [ ] Two consecutive back-to-back bootstraps yield identical hook state and the second emits no INFO line (silent no-op).
- [ ] Hand-authored user hooks that reference `portal state signal-hydrate` but lack the `command -v portal >/dev/null 2>&1 &&` prefix are NOT evicted.
- [ ] If `hydrationTriggerEvents` is extended at compile time, the migration covers the new event without further code changes (i.e. the loop iterates the slice, not a hard-coded list).
- [ ] A `ShowGlobalHooks` failure causes `migrateHydrationHooks` to return a wrapped error; the caller in `RegisterPortalHooks` aggregates it via `errors.Join` alongside any per-event register errors. Bootstrap does not abort on this path; the orchestrator surfaces the result as a soft warning per the existing bootstrap-step error contract.
- [ ] `go build ./...` and `go test ./...` pass.
- [ ] No test added uses `t.Parallel()`.

**Tests**:
- `TestMigrateHydrationHooks_EvictsUnSeparatedThenInstallsFixed` (real-tmux fixture preferred via `internal/tmuxtest`):
  - Set up a tmux server.
  - For each event in `hydrationTriggerEvents`, append the legacy un-separated command verbatim: `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"` (no `--`).
  - Call `RegisterPortalHooks` once.
  - Assert that for each event, `ShowGlobalHooks` returns exactly one entry containing `portal state signal-hydrate`, and that entry contains `portal state signal-hydrate --`.
  - Assert one INFO line `evicted N stale signal-hydrate hook(s) lacking '--' separator` was written to a captured logger, with N equal to `len(hydrationTriggerEvents)`.
- `TestMigrateHydrationHooks_IdempotentNoOpOnSecondBootstrap`:
  - Run the same fixture; call `RegisterPortalHooks` twice in a row.
  - Assert hook state is unchanged after the second call.
  - Assert the second call's logger captured zero INFO lines and zero WARN lines.
- `TestMigrateHydrationHooks_ZeroPreExistingEntriesIsSilentNoOp`:
  - Fresh server with no pre-existing entries; call `RegisterPortalHooks` once.
  - Assert install proceeds normally, eviction count is 0, no INFO and no WARN lines emitted.
- `TestMigrateHydrationHooks_MultipleStaleEntriesOnSameEventEvictAllInOrder`:
  - Append three un-separated entries on `client-attached` (indices 0, 1, 2).
  - Call `migrateHydrationHooks` directly.
  - Assert all three are evicted (verify via `ShowGlobalHooks` post-call), eviction count == 3, and the call did not panic or return a partial-failure error. Index-shift safety is implicitly verified — descending order means later index removals do not invalidate earlier ones.
- `TestMigrateHydrationHooks_DoesNotEvictHandAuthoredHooksLackingCommandVPortalPrefix`:
  - Append a hand-authored entry: `run-shell "portal state signal-hydrate foo"` (no `command -v portal` prefix, no `--`).
  - Call `migrateHydrationHooks`.
  - Assert the hand-authored entry remains present (eviction predicate requires both fingerprints).
- `TestMigrateHydrationHooks_PartialFailureLogsWarnAndContinues`:
  - Use a mock `Client` (or `Commander`) that fails `UnsetGlobalHookAt` on a specific (event, index) pair and succeeds on others.
  - Append two un-separated entries on the same event so two indices are evicted in sequence.
  - Call `migrateHydrationHooks`.
  - Assert the captured logger contains one WARN line of the expected shape; assert the function returns `(1, nil)` (one successful eviction, error is nil because per-index failures are best-effort).
- `TestMigrateHydrationHooks_HydrationTriggerEventsSliceIsRespectedAtRuntime`:
  - Stub or temporarily extend `hydrationTriggerEvents` (via a test-only export or by exercising a function that takes the slice as a parameter — choose whichever fits the chosen Option A/B shape).
  - Append un-separated entries on every listed event.
  - Assert all are evicted. Confirms the migration loop reads the slice rather than hard-coding events.

**Edge Cases**:
- **Zero pre-existing entries**: Silent no-op; no INFO, no WARN.
- **One stale entry per event**: Standard upgrade case; both events are scrubbed and the fixed entries installed; one INFO line summarising the count.
- **Multiple stale entries on same event**: Descending-index iteration prevents index-shift bugs — verified by `TestMigrateHydrationHooks_MultipleStaleEntriesOnSameEventEvictAllInOrder`.
- **`UnsetGlobalHookAt` partial failure**: WARN + continue; final eviction count reflects only successful removals; bootstrap is not aborted.
- **Hand-authored user hooks lacking `command -v portal` prefix**: Not evicted — predicate requires both fingerprints.
- **Back-to-back bootstraps idempotent**: Once migrated, subsequent runs find zero predicate-matching entries and emit no INFO line.
- **`hydrationTriggerEvents` slice extended later**: Loop reads slice at runtime; new events are scrubbed automatically with no code change.

**Context**:
> Spec § "One-shot bootstrap migration" mandates: scan-then-evict-then-install; eviction-predicate must be the **full Portal-authored hook shape** (both `command -v portal >/dev/null 2>&1 &&` AND `portal state signal-hydrate`, NOT containing `portal state signal-hydrate --`); indices processed highest-first; per-index failures are WARN-and-continue; eviction loop's failure must NOT abort bootstrap; install step is itself idempotent so retry is automatic on the next bootstrap.
>
> Spec § "Operator visibility": exactly one INFO line per bootstrap on non-zero eviction; zero INFO on zero evictions to avoid steady-state log noise.
>
> Spec § "Code location": migration logic lives in `internal/tmux/hooks_register.go` alongside `RegisterHookIfAbsent`; expose as a new free function (suggested name `migrateHydrationHooks`); test prefers real-tmux socket fixture (`internal/tmuxtest`) over a mocked `Commander` because eviction logic depends on the precise format of `show-hooks` output and `set-hook -gu` index semantics.
>
> Existing hook-management surface (already exposed by `internal/tmux`): `ShowGlobalHooks`, `ParseShowHooks`, `AppendGlobalHook`, `UnsetGlobalHookAt` — do not introduce new tmux-client surface area unless these prove insufficient.
>
> Production-side reading aid: bootstrap step 2 in the orchestrator currently calls `RegisterPortalHooks` via `internal/bootstrapadapter`. The logger that should receive the INFO/WARN lines is the bootstrap-step `*state.Logger`.

**Spec Reference**: `.workflows/scrollback-not-restored-with-non-zero-base-index/specification/scrollback-not-restored-with-non-zero-base-index/specification.md` § "Part 1 — Add `--` End-Of-Flags Separator to `signal-hydrate` Hook" → "One-shot bootstrap migration" / "Migration mechanics (explicit)", § "Acceptance Criteria" item 3, § "Testing Requirements" item 4.

---

## scrollback-not-restored-with-non-zero-base-index-1-3 | approved

### Task 3: Reboot Round-Trip Integration Test with Leading-Dash Session Name

**Problem**: The current reboot round-trip integration tests (`cmd/bootstrap/reboot_roundtrip_test.go`) use session names "alpha" and "beta" — neither begins with `-`. Those tests would not have caught this bug, because tmux's `run-shell` never resolves a leading-dash session name into the hook command for them. A regression of the `--` separator (Task 1) or the migration eviction (Task 2) must fail end-to-end against a real tmux server with a leading-dash session, exercising tmux's actual `run-shell` argv resolution; a mock-based shape would not catch the bug because mocks bypass the very layer that fails in production.

**Solution**: Add a sibling reboot round-trip integration test in `cmd/bootstrap/reboot_roundtrip_test.go` that uses a leading-dash session name (e.g. `-dotfiles-test`) on a non-zero `base-index` / `pane-base-index` server. The test follows the existing fixture pattern: build the portal binary, set up isolated state + hooks dirs, capture topology, kill the test-only server, restart with non-zero base indices, run the bootstrap orchestrator with production hook-registration adapters (so the new `--` separator and migration both fire), drive `signal-hydrate` for the leading-dash session via the built binary (the production-binary path that exec's `portal state signal-hydrate -- -dotfiles-test`), and assert end-to-end scrollback replay with no `hydrate timeout` WARN in `portal.log`.

**Outcome**: A new sub-test in `cmd/bootstrap/reboot_roundtrip_test.go` (`//go:build integration`) creates a leading-dash session, performs a full reboot round-trip on an isolated test socket, and verifies scrollback replays. The test scans `portal.log` and asserts zero `hydrate timeout` WARN lines for the leading-dash session. The non-dash control path (`alpha`/`beta` in the existing `TestPhase5RebootRoundTripEndToEnd`) continues to pass unchanged. The developer's primary tmux server is never touched (`internal/tmuxtest` socket fixture only).

**Do**:
- Add a new test function in `cmd/bootstrap/reboot_roundtrip_test.go`:
  ```go
  func TestRebootRoundTrip_LeadingDashSessionName(t *testing.T) { ... }
  ```
  Guarded by the existing `//go:build integration` tag at the top of the file and the existing `testing.Short()` / `tmuxtest.SkipIfNoTmux(t)` early-skip pattern.
- The test must:
  1. Use `tmuxtest.New(t, "ptl-rt-leadingdash-")` to create an isolated socket (the existing fixture already enforces this — the developer's primary server is untouched).
  2. Build the portal binary via `restoretest.BuildPortalBinaryDir(t)` and prepend it to PATH so `command -v portal` resolves inside `run-shell`.
  3. Set up `PORTAL_STATE_DIR`, `PORTAL_HOOKS_FILE` per the existing fixture pattern; create the state dir.
  4. Open a portal logger pointed at `<stateDir>/portal.log` (the same path the production hydrate helper writes to via `state.OpenLogger`).
  5. `_seed` bootstrap session, then `applyBaseIndices(t, ts, 1, 1)` — non-zero `base-index` AND `pane-base-index` to satisfy the spec's coverage requirement and to confirm the misleading WARN is also gone (cross-references AC #4 for Phase 2 but does not depend on Phase 2 completion).
  6. Create a single session named `-dotfiles-test` (leading-dash) with a single pane, e.g. `ts.Run(t, "new-session", "-d", "-s", "-dotfiles-test", "-c", cwd, "sleep", "infinity")`. If tmux refuses the name via positional argv parse on the `tmux` CLI itself, fall back to passing the name via `--` separator on the tmux command (`new-session -d -s -- -dotfiles-test ...`). Validate at test setup time that the session was created (`ts.WaitForSession(t, "-dotfiles-test", 2*time.Second)`); if creation is impossible at the tmux-CLI layer the test must `t.Fatalf` with a clear message rather than silently degrade.
  7. Capture and commit via the existing `captureAndCommit` helper.
  8. Overwrite the pane's scrollback file on disk with a known ANSI fixture (e.g. `\x1b[31mred\x1b[0m\nbefore reboot\n`), mirroring the existing test's pattern.
  9. `ts.KillServer()`; assert `list-sessions` errors (server actually died).
  10. Restart server; `_seed` again; `applyBaseIndices(t, ts, 1, 1)` again (saved indices match restore indices — this test is about the leading-dash hook firing, not drift).
  11. Wire the bootstrap orchestrator with **production** hook-registration adapters this time — i.e. pass a real hooks adapter that calls `RegisterPortalHooks` (so the migration code from Task 2 and the `--` separator from Task 1 actually run). The existing test wires `bootstrap.NoOpHooks{}`; this new test must use the production wiring (`bootstrapadapter.HooksAdapter{...}` or whatever the production seam is named — discover via grep at implementation time).
  12. Run the orchestrator. After bootstrap completes, assert via `ShowGlobalHooks` + `ParseShowHooks` that for each event in `hydrationTriggerEvents`, exactly one entry contains `portal state signal-hydrate` and that entry contains `portal state signal-hydrate --`.
  13. Drive signal-hydrate via the built binary path: `restoretest.DriveSignalHydrateBinary(t, binDir, ts.SocketPath(), stateDir, hooksPath, []string{"-dotfiles-test"})`. This is argv-identical to what the registered tmux `client-attached` hook fires — exercising the full hook → CLI argv → cobra → `runSignalHydrate` → FIFO write pipeline against a leading-dash session.
  14. `restoretest.WaitForSkeletonMarkersCleared(t, client, 10*time.Second)`. If this times out the test fails — the marker not being cleared means hydration never completed.
  15. Read `<stateDir>/portal.log` and assert it contains zero lines matching the substring `hydrate timeout` for any pane belonging to `-dotfiles-test`. (Use `strings.Contains` line-by-line or a regex; substring match is sufficient because the message format is stable.)
  16. Verify scrollback bytes survived via `verifyANSIScrollback(t, ts, "-dotfiles-test", 1, 1)` (note: pane indices are restoreBase=1, restorePaneBase=1).
- Confirm `restoretest.DriveSignalHydrateBinary` already wraps the session arg with `--` (it should, since it builds the argv `portal state signal-hydrate <session>` and the production hook command also passes `--`). If it does not, update it to do so — the test must be argv-identical to the production hook. Read `internal/restoretest/` source at implementation time to verify, and adjust if needed (this is a test-only helper, so no production impact).
- Add a regression assertion sibling test (or sub-test) `TestRebootRoundTrip_NonDashSessionStillHydrates`:
  - Trivial wrapper around the existing `runRebootRoundTrip` flow with `useBinary: true` and session names that do not start with `-` (the existing test already covers this; this assertion may be a comment pointing to `TestPhase5RebootRoundTripEndToEnd` rather than a duplicate test, if the existing test is sufficient regression coverage). Confirm by inspection at implementation time; if `TestPhase5RebootRoundTripEndToEnd` already covers the non-dash case under Task 1's changes, no new test is required — note this in the test file's top-of-function comment.
- Verify the existing `armPanes:202` pane-count mismatch logging is preserved by NOT modifying any code in `internal/restore/session.go` for this task. (This task is test-only; logging preservation is a non-regression check, not an active assertion.)

**Acceptance Criteria**:
- [ ] New test `TestRebootRoundTrip_LeadingDashSessionName` exists in `cmd/bootstrap/reboot_roundtrip_test.go`, gated by `//go:build integration` and `testing.Short()`.
- [ ] Test creates a session named `-dotfiles-test` on an isolated `internal/tmuxtest` socket; the developer's primary tmux server is untouched.
- [ ] Test sets `base-index = 1` and `pane-base-index = 1` on the test server (non-zero) before creating the session.
- [ ] Test wires production hook-registration adapters into the bootstrap orchestrator (not `NoOpHooks{}`), so `RegisterPortalHooks` actually runs and installs the `--`-separated entry.
- [ ] After bootstrap, hooks state assertion: for each event in `hydrationTriggerEvents`, exactly one entry contains `portal state signal-hydrate`, and that entry contains `portal state signal-hydrate --`.
- [ ] Test drives `signal-hydrate` via `restoretest.DriveSignalHydrateBinary` (argv-identical to production hook); no direct FIFO bypass.
- [ ] After hydrate completes, all skeleton markers for `-dotfiles-test` panes are cleared (via `WaitForSkeletonMarkersCleared`).
- [ ] `<stateDir>/portal.log` contains zero `hydrate timeout` WARN lines after the run.
- [ ] ANSI scrollback fixture (`\x1b[31m...before reboot...`) is present in the live pane's `capture-pane -e -p -S -` output post-hydrate.
- [ ] Existing `TestPhase5RebootRoundTripEndToEnd` and `TestPhase5RebootRoundTripBaseIndexDrift` continue to pass unchanged (non-dash regression check).
- [ ] No `t.Parallel()`. No production code changes in this task — test-only.
- [ ] `go test -tags=integration ./cmd/bootstrap/...` passes.

**Tests**:
- `TestRebootRoundTrip_LeadingDashSessionName` — full reboot round-trip with `-dotfiles-test` on non-zero base indices, real-tmux socket, production hook adapters, binary-driven signal-hydrate, asserts marker clearance + zero `hydrate timeout` WARN + ANSI scrollback survival.
- (If new) `TestRebootRoundTrip_NonDashSessionStillHydrates` — sibling regression check confirming non-dash names still hydrate; SKIP and add a comment instead if `TestPhase5RebootRoundTripEndToEnd` already covers this under the new code (decided at implementation time).

**Edge Cases**:
- **Non-zero `base-index` / `pane-base-index` on test server**: Set to 1/1 to confirm hydration works under base-index drift AND under leading-dash naming simultaneously. Asserted via marker clearance, not via prediction-vs-live (the diagnostic WARN check moves to Phase 2).
- **Isolated test socket only**: `internal/tmuxtest` fixture creates a dedicated socket via `tmux -L <name>`; the test's `kill-server` targets only this socket. Assert at the top of the test (or rely on the fixture's docstring) that the developer's primary tmux server is untouched.
- **Non-dash session name regression**: The existing `TestPhase5RebootRoundTripEndToEnd` covers this; confirm at implementation time that it still passes against the Task 1 + Task 2 changes (`signalHydrateCommand` now contains `--`; the `RegisterPortalHooks` migration runs but is a no-op on a fresh server).
- **`tmux new-session` rejecting a leading-dash name at the tmux-CLI argv layer**: If reproducible, fall back to the `--` separator on the tmux CLI call (`new-session -d -s -- -dotfiles-test ...`). If even that fails, `t.Fatalf` with a clear diagnostic — do not silently weaken the test by switching to a different name.

**Context**:
> Spec § "Testing Requirements" item 2: must run against a real tmux server via `internal/tmuxtest`'s socket fixture; a mock-based shape would not exercise tmux's actual `run-shell` argv resolution and so would not catch the bug. The existing test's session names ("alpha", "beta") would not have surfaced the failure.
>
> Spec § "Testing Constraint — Do Not Restart The Active Tmux Server": the developer's working tmux server must NEVER be killed by tests. All round-trip tests must use a separate, isolated tmux socket via `internal/tmuxtest`. This applies to automated tests, manual repro steps, and debugging sessions.
>
> Spec § "Acceptance Criteria" item 1: hydration must succeed for leading-dash session names regardless of `base-index` / `pane-base-index` values. Item 5: no regression for non-dash names; `armPanes:202` pane-count mismatch logging preserved.
>
> Existing fixture pattern in `cmd/bootstrap/reboot_roundtrip_test.go`: see `runRebootRoundTrip` for the full template — binary build → state dir → seed session → base indices → topology → capture → kill → restart → seed+indices → orchestrator → drive signal-hydrate → wait for markers → assertions. The new test follows this template with the leading-dash session name swapped in and production hook adapters substituted for `NoOpHooks{}`.

**Spec Reference**: `.workflows/scrollback-not-restored-with-non-zero-base-index/specification/scrollback-not-restored-with-non-zero-base-index/specification.md` § "Acceptance Criteria" items 1, 5, § "Testing Requirements" item 2, § "Testing Constraint — Do Not Restart The Active Tmux Server".
