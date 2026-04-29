# Analysis Tasks: built-in-session-resurrection (Cycle 6)

```yaml
topic: built-in-session-resurrection
cycle: 6
total_proposed: 5
```

---

## Task 1: Extract shared reboot-round-trip + portal-binary test helpers into `internal/restoretest/`

- **status:** pending
- **severity:** high
- **sources:** duplication (D1, D2), architecture (A4 cross-ref)

**Problem**: Phase 12 added ~115 lines of byte-identical test helpers across `cmd/bootstrap/reboot_roundtrip_test.go` and `internal/restore/integration_full_test.go` (`driveSignalHydrate`, `openAndSignalFIFO`, `waitForSkeletonMarkersCleared`, `sortedKeySet`, `buildPortalBinaryDir`, `prependPathDir`, `projectRoot`), plus three near-identical "compile portal CLI" helpers across those two files and `cmd/reattach_integration_test.go` (`buildPortalBinaryForReattach`/`reattachProjectRoot`). The duplication is acknowledged in source comments ("duplicated here (rather than imported)") but no shared home exists — drift hazard if e.g. retry-budget tweaks land in one copy.

**Solution**: Create `internal/restoretest/` exporting the canonical implementations; update all three integration test files to import them.

**Outcome**: Single canonical home for ~150 lines of cross-file test infrastructure; both reboot round-trip files and the reattach integration test pass against the shared package; future round-trip tests reuse the harness.

**Do**:
1. Create `internal/restoretest/` (build-tagged `//go:build integration` if appropriate).
2. Add `restoretest.go` exporting:
   - `BuildPortalBinaryDir(t *testing.T) string` — t.TempDir-based, t.Fatal on error.
   - `BuildPortalBinaryStable() (string, error)` — `os.MkdirTemp`-based for `sync.Once.Do` callers.
   - `ProjectRoot() (string, error)` — single walk-up-to-go.mod implementation; both above call it.
   - `PrependPATH(t *testing.T, dir string)`.
   - `DriveSignalHydrate(t *testing.T, client *tmux.Client, stateDir string, sessions []string)`.
   - `OpenAndSignalFIFO(path string, delay, budget time.Duration) error`.
   - `WaitForSkeletonMarkersCleared(t *testing.T, client *tmux.Client, timeout time.Duration)`.
   - `SortedKeySet(m map[string]struct{}) []string`.
3. Replace all duplicated definitions in `cmd/bootstrap/reboot_roundtrip_test.go`, `internal/restore/integration_full_test.go`, and `cmd/reattach_integration_test.go` with calls into `internal/restoretest`.
4. Remove the now-unused private helpers from each test file.
5. Verify no import cycles (the new package depends only on `internal/tmux` + stdlib + testing).

**Acceptance Criteria**:
- `internal/restoretest/` exists with the eight exported symbols.
- The three integration test files no longer define their private duplicates.
- `go build ./...` succeeds; `go test ./...` (with the integration tag if applied) passes.
- No "duplicated here (rather than imported)" comments remain.

**Tests**:
- Existing reboot round-trip tests in both packages continue to pass against the shared helpers.
- Existing `cmd/reattach_integration_test.go` cases continue to pass with `BuildPortalBinaryStable` driving the `sync.Once`.
- Add `internal/restoretest/restoretest_test.go` covering at minimum `ProjectRoot()` (locates the repo's `go.mod`) and `SortedKeySet` ordering.

---

## Task 2: Exercise the production `client-attached` / `signal-hydrate` pathway end-to-end in reboot round-trip tests

- **status:** pending
- **severity:** medium
- **sources:** standards (S1)

**Problem**: Phase 5 task 5-9 acceptance bullets explicitly require round-trip tests to exercise `client-attached` (via `tmux attach-session` on a fresh PTY) and `client-session-changed` (via `tmux switch-client`). Both Phase 12 round-trip tests instead bypass the entire signal pathway by writing the FIFO byte directly through test-local `openAndSignalFIFO`, and the documented PTY fallback ("invoke `portal state signal-hydrate <session>` directly") is also not used — the test re-implements `writeFIFOSignal` inline. Net effect: the tmux hook → run-shell → `portal state signal-hydrate` argv → `runSignalHydrate` body → FIFO write pipeline is never end-to-end exercised, and `client-session-changed` has no coverage at all.

**Solution**: Replace direct FIFO writes in at least one round-trip path with an exec of the already-built portal binary (`portal state signal-hydrate <session>`), and add a `switch-client` subtest covering `client-session-changed`.

**Outcome**: The argv → `runSignalHydrate` → FIFO pipeline is exercised in CI; `client-session-changed` has at least one passing assertion; `client-attached` coverage upgrades from "FIFO byte appears" to "production binary writes the FIFO byte."

**Do**:
1. In `cmd/bootstrap/reboot_roundtrip_test.go` (and/or `internal/restore/integration_full_test.go`), replace at least one call to `driveSignalHydrate`/`openAndSignalFIFO` with `exec.Command(portalBinary, "state", "signal-hydrate", sessionName)` for each restored session.
2. Add a `switch-client` subtest variant: restore two sessions, attach to session A on a PTY, then `tmux switch-client -t <sessionB>`, asserting both sessions complete hydration (markers cleared) — exercising `client-session-changed`.
3. Keep at least one direct-FIFO path as a "fallback" mode in case the PTY/exec path proves CI-flaky, but ensure the production-binary path is the default coverage.
4. Update test godoc to map each subtest back to the Phase 5 task 5-9 acceptance bullet it satisfies.
5. Update the "test-side replacement for `portal state signal-hydrate`" comment to reflect that the helper is now a fallback, not the primary surface.

**Acceptance Criteria**:
- At least one reboot round-trip subtest invokes the built portal binary's `state signal-hydrate` subcommand to drive hydration (no direct FIFO write on that path).
- A new subtest exercises `tmux switch-client` between two restored sessions; both hydrate to completion.
- All existing round-trip assertions (live-structure, ANSI scrollback, session env) continue to pass against the production-binary path.
- Test godoc cites which Phase 5 task 5-9 acceptance bullet each subtest satisfies.

**Tests**:
- `TestRebootRoundTrip_*_ViaSignalHydrateBinary` (or extension of existing test): exec'ing the portal binary drives all sessions to "skeleton markers cleared" within the existing budget.
- `TestRebootRoundTrip_SwitchClientHydratesSecondSession`: attach to A, switch-client to B, both end with markers cleared and `verifyLiveStructure` passing.
- Existing `TestRebootRoundTrip_*` cases continue to pass.

---

## Task 3: Add the two missing Phase 5 task 5-10 acceptance assertions to `cmd/reattach_integration_test.go`

- **status:** pending
- **severity:** low
- **sources:** standards (S2, S3)

**Problem**: Two of seven acceptance bullets enumerated in `phase-5-tasks.md` for task 5-10 are not asserted in `cmd/reattach_integration_test.go`:
1. "saved_at is not advanced during a steady-state reattach window" (phase-5-tasks.md:947) — `TestReattachIntegration_SteadyStateReattachZeroStructuralRewrites` asserts pane-id preservation but never compares `sessions.json.saved_at` pre/post Run. Planning explicitly notes this duplication is intentional ("different trigger — normal command, not a test-only probe").
2. "portal open PATH resolves a session name present only in sessions.json" (phase-5-tasks.md:942) — `TestReattachIntegration_OpenLaunchesTUIAfterRestoredSkeleton` covers bare `portal open` (no positional arg) but never the path-arg branch through alias/zoxide/direct-path resolution into `cmd/open.go` attach hand-off against a saved-only session.

**Solution**: Add the two missing assertions/subtests in `cmd/reattach_integration_test.go`, each cited against its bullet in the planning doc.

**Outcome**: All seven Phase 5 task 5-10 acceptance bullets are exercised; the bullet/test mapping is explicit in test godoc.

**Do**:
1. In `TestReattachIntegration_SteadyStateReattachZeroStructuralRewrites` (or sibling), capture `sessions.json.saved_at` before bootstrap (`os.ReadFile` + JSON decode), Run, then re-read and assert `saved_at` is unchanged. Mirror `phase5_marker_suppression_integration_test.go:207-210`.
2. Add `TestReattachIntegration_OpenPathResolvesSavedOnlySession` (or equivalent): pre-seed an alias entry (or zoxide stub) resolving to a saved-only session name, invoke `portal open <path>`, assert connector dispatch reaches the saved session name.
3. Update test godoc / file header to map each test to its Phase 5 task 5-10 bullet.

**Acceptance Criteria**:
- `saved_at`-unchanged assertion exists in the steady-state reattach test (`before == after`).
- A test exercises `portal open <path>` against a saved-only session via alias or zoxide pre-seed and asserts attach dispatch reaches that session.
- Test godoc enumerates each Phase 5 task 5-10 bullet and its asserting test.

**Tests**:
- Updated `TestReattachIntegration_SteadyStateReattachZeroStructuralRewrites` (or sibling) asserts `saved_at` invariance.
- New `TestReattachIntegration_OpenPathResolvesSavedOnlySession` (or equivalent) covers the path-arg → saved-only-session branch.

---

## Task 4: Use `state.SanitizePaneKey` for round-trip hook fixtures and tighten the marker-suppression test scope godoc

- **status:** pending
- **severity:** low
- **sources:** standards (S4, S5)

**Problem**: Two narrow precision gaps:
1. `cmd/bootstrap/reboot_roundtrip_test.go:155-169` registers the on-resume hook via `tmux.PaneTarget(...)`. Per spec § "Helper hook lookup under index drift," the hook key is the saved structural identifier `<raw-session>:<saved-window>.<saved-pane>` produced by `state.SanitizePaneKey` and passed via `--hook-key` from `SessionRestorer.collectArmInfos`. `PaneTarget` is a tmux-target formatter, not a saved-key formatter; the test relies on coincidental string equality and would silently drift if `saveBase != 0` or formatter separators diverge.
2. `cmd/bootstrap/phase5_marker_suppression_integration_test.go:155-167` wires `bootstrap.NoOpSaver{}` and never starts a real `portal state daemon`. The `saved_at`-unchanged assertion proves Restore-side write discipline only — it does not exercise the spec's "Restoration guard" contract (daemon ticks observe `@portal-restoring=1` and skip the capture). Test godoc currently presents it as the suppression contract, over-claiming.

**Solution**: Replace `tmux.PaneTarget` with `state.SanitizePaneKey` in the round-trip hook registration; tighten the marker-suppression test's godoc to match what it actually proves.

**Outcome**: Test fixture hook keys share their canonical source with production; the marker-suppression test header documents its actual scope (Restore-side write discipline) and notes that daemon-tick suppression remains out of scope.

**Do**:
1. In `cmd/bootstrap/reboot_roundtrip_test.go:155-169`, replace `tmux.PaneTarget("alpha", cfg.saveBase+0, cfg.savePaneBase+0)` with `state.SanitizePaneKey("alpha", cfg.saveBase+0, cfg.savePaneBase+0)`. Add the `internal/state` import if missing.
2. Verify existing assertions still pass (the hook-key lookup will use the same canonical formatter as production).
3. In `cmd/bootstrap/phase5_marker_suppression_integration_test.go`, update the test/file godoc to scope precisely: the test covers "Restore-side write discipline (Restore itself does not write sessions.json mid-run)" and explicitly notes that "daemon-tick suppression (the spec's Restoration guard contract proper) is exercised by the daemon's own unit tests, not here."
4. Leave `NoOpSaver{}` wiring as-is — scope re-statement is sufficient.

**Acceptance Criteria**:
- `cmd/bootstrap/reboot_roundtrip_test.go` registers hook keys via `state.SanitizePaneKey` (no `tmux.PaneTarget` for the saved-key path).
- `cmd/bootstrap/phase5_marker_suppression_integration_test.go` test/file godoc explicitly scopes the test to Restore-side write discipline and acknowledges the daemon-tick path is out of scope.
- Existing assertions in both tests continue to pass.

**Tests**:
- Existing reboot round-trip tests (with the formatter change) continue to pass.
- Existing marker-suppression test (with godoc scope tightening) continues to pass.

---

## Task 5: Documentation precision cleanup across spec citations, error-warning wording, godoc scope, and historical record

- **status:** pending
- **severity:** low
- **sources:** standards (S6, S7), architecture (A2, A3, A5)

**Problem**: Five low-severity documentation precision issues, individually trivial but collectively a hygiene cleanup:
1. (S6) `cmd/state_signal_hydrate.go:18-21` cites a non-existent spec section "FIFO open-for-write semantics" — closest real titles are "Signal Mechanism: FIFO Per Pane" (spec L763) and "Helper Behavior on Startup."
2. (S7) Spec § Fatal Bootstrap Errors lists `@portal-restoring set-option fails` as fatal but does not enumerate the symmetric Clear (unset) case. Implementation, CLAUDE.md, and reconciled planning all classify Clear as fatal — spec text relies on readers extending "set" to "unset."
3. (A2) `CorruptSessionsJSONWarning()` headline at `cmd/bootstrap/errors.go:52` says "Portal state file is corrupt — restoration skipped." Now that permission-denied is wrapped under `ErrCorruptIndex`, a chmod-locked-but-otherwise-valid sessions.json triggers a "corrupt" headline that is technically wrong.
4. (A3) `cmd/state_cleanup.go:127-128` docstring ends with "RemoveAll follows the leaf inode and never traverses through a symlink target." Wrong as written — `os.RemoveAll` DOES traverse intermediate symlinked path components; only the *leaf*-symlink case is what the Lstat guard covers.
5. (A5) `.workflows/.../planning/.../review-integrity-tracking-c2.md` L251/L259 still say "log WARN and continue" for `Restoring.Clear`, contradicting the post-T12-9 fatal-on-failure rule.

**Solution**: Targeted text-only edits, each tightly scoped to the cited file/line.

**Outcome**: Spec citations match real section titles; user-facing warning wording covers permission-denied without misleading users; symlink-protection docstring scopes its claim correctly; historical review record carries an in-place reconciliation note.

**Do**:
1. In `cmd/state_signal_hydrate.go:18-21`, replace `Spec → "FIFO open-for-write semantics"` with the actual spec section title (e.g. `Spec → "Signal Mechanism: FIFO Per Pane"` or `Spec → "Helper Behavior on Startup"` — whichever the comment refers to).
2. In `cmd/bootstrap/errors.go:50-55`, generalise the headline of `CorruptSessionsJSONWarning()` from "Portal state file is corrupt — restoration skipped." to "Portal state file unusable — restoration skipped." Adjust any test asserting the old wording.
3. In `cmd/state_cleanup.go:127-128`, replace the trailing sentence with: *"The Lstat check below covers the leaf-symlink case (PORTAL_STATE_DIR resolves directly to a symlink). RemoveAll DOES traverse intermediate symlinked components — by design, since users may legitimately have `~/.config` symlinked to a different volume. Whatever leaf directory PORTAL_STATE_DIR points at (after intermediate resolution) is what gets purged."*
4. In `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` § Fatal Bootstrap Errors (around L1394), add an explicit bullet for "Clear `@portal-restoring` fails (unset)" so spec/planning/implementation alignment is authoritative.
5. In `.workflows/built-in-session-resurrection/planning/built-in-session-resurrection/review-integrity-tracking-c2.md` at L251 and L259, append an in-place note: "(Resolved by T12-9: step 6 Clear `@portal-restoring` is fatal on failure — see bootstrap.go:227-229.)"

**Acceptance Criteria**:
- `cmd/state_signal_hydrate.go` retry comment cites a spec section title that exists in the spec.
- `CorruptSessionsJSONWarning()` headline is accurate for both corrupt-decode and permission-denied cases.
- `cmd/state_cleanup.go` docstring no longer claims `RemoveAll` "never traverses through a symlink target"; the new wording matches what `TestStateCleanup_PurgeAllowsSymlinkedIntermediatePathComponents` asserts.
- Spec § Fatal Bootstrap Errors enumerates Clear `@portal-restoring` failures explicitly.
- `review-integrity-tracking-c2.md` L251/L259 carry the T12-9 reconciliation note.

**Tests**:
- Update any test asserting the old "Portal state file is corrupt" headline (search `cmd/bootstrap/` test files).
- `TestStateCleanup_PurgeAllowsSymlinkedIntermediatePathComponents` continues to pass (no behavior change — docstring only).
- `go test ./...` continues to pass after the wording adjustments.
