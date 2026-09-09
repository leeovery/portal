TASK: resume-hooks-silently-lost-8-32 — internal/restore Fixture Residue From This Phase (tick-7cd6e6)

ACCEPTANCE CRITERIA:
- One fixture type, parameterised by pane count; both former constructors are gone.
- No helper in the package takes an argument nothing reads.
- The integration lane passes with both suites' assertions unchanged.

STATUS: complete

SPEC CONTEXT:
This is a phase-8 implementation-analysis (duplication) task, so its authority is its own body
rather than the specification. The suites it touches are the ones the spec's §7.2 migration
notes name: `internal/restore/multipane_legacy_integration_test.go` (per-pane hook routing keyed
on the durable `@portal-pane-id` token, plus the legacy un-stamped-pane degradation to a bare
shell) and the rename-reboot family (a hook keyed on a token must survive a rename and one or
more reboots). Both subjects are preserved verbatim by this refactor; nothing in the spec
prescribes the fixture's shape.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/restore/reboot_fixture_test.go:1-177` (new file, `//go:build integration`) — the merged
    `rebootPane` / `rebootFixture` / `newRebootFixture` (`:25`, `:32`, `:46`) with the shared
    `assertLivePanes` (`:104`), `captureAndPersist` (`:119`), `persist` (`:141`), `rebootAndHydrate`
    (`:153`) and `paneIndices` (`:167`).
  - `internal/restore/rename_reboot_shared_test.go:27` — `renameRebootPane(t) (rebootPane, string)`,
    the rename suite's one-pane descriptor plus its hook-fire file.
  - Re-pointed call sites: `internal/restore/multipane_legacy_integration_test.go:30,63`,
    `internal/restore/rename_reboot_durability_integration_test.go:20-21`,
    `internal/restore/rename_reboot_hook_integration_test.go:70-71`.
- Notes:
  - AC1 met. `newLegacyFixture`, `legacyFixture`, `legacyPane`, `newRenameRebootFixture` and
    `renameRebootFixture` appear nowhere in the Go tree (grep across the repo returns only
    `.workflows/` and `.tick/` prose). The fixture is parameterised by a `[]rebootPane` slice rather
    than a bare int — a sound divergence from the Do list's literal wording, since the multipane
    suite varies the per-pane token and hook vocabulary as well as the count, and a count alone
    could not carry it.
  - AC2 met. `openTestLogger` is absent from the whole Go tree; it was deleted from
    `internal/restore/restore_test.go` by the earlier task 7-15 (d7ae4889), so this task inherited
    the criterion already satisfied. I re-read the surviving helpers in the package
    (`rename_reboot_shared_test.go:27,36,44`, `reboot_fixture_test.go`, `restore_test.go:46,62,69,451`,
    `exit_closes_pane_integration_test.go:98,161,167,191`, `prefix_sibling_integration_test.go:66`)
    and every declared parameter is read.
  - AC3 (judged by reading; no suite was executed). The refactor is behaviour-preserving or
    strictly stronger at every point:
    * The rename fixture's post-restore pane check moved from `slices.Contains(coords, "0:0")` to
      `assertLivePanes`'s exact-equality against `["0:0"]`. `restoretest.TryLivePaneCoords`
      (`internal/restoretest/live_pane_coords.go:34`) reads `list-panes -s -t =<session>:`, so the
      result is session-scoped and a single-pane restore can only produce `["0:0"]` — the tightening
      is safe, and it still covers what the old containment check covered.
    * The rename fixture gains an arrange-time `assertLivePanes` (`:89`) and a `verifyHookKeyed`
      per hooked pane (`:74`) that it did not have. Both are additions, neither can weaken the suite.
    * The captured-token check widened from pane 0 only to every pane plus the window/pane-count
      topology (`:127-135`); the rename suite's one-pane case is a strict subset.
  - Cleanup ordering is preserved and slightly improved. `renameRebootPane`'s `t.TempDir()` now runs
    before `portaltest.IsolateStateForTest`, so under LIFO the hook-fire directory is removed *after*
    the tmux kill-server rather than before it — the hook can no longer write into a directory that
    is already gone. `RegisterStateDirTeardownGuard` (`:78`) still sits after `IsolateStateForTest`
    and before `tmuxtest.New` (`:80`), which is the ordering CLAUDE.md prescribes.
  - Compilation: every symbol the new file names resolves —
    `restoretest.BuildPortalBinaryDir/LivePaneCoords/FindCapturedSession/SeedScrollback/WriteIndex/
    ANSIScrollback/RebootServer/RestoreFromState/DriveSignalHydrate/WaitForSkeletonMarkersCleared/
    HydrateBudget/HydrateTick` and `tmuxtest.Socket.{Run,TryRun,Client,WaitForSession,StampPaneToken,
    ReadPaneToken}`. `StampPaneToken` takes a `tmux.Target`, and `:95` passes `tmux.PaneTargetExact`
    (which returns one); `:87` passes `tmux.PaneTarget`, which returns a plain `string`, matching
    `Socket.Run`'s variadic. Import sets in all four touched files are exactly what their bodies use
    — no orphaned or missing import.

TESTS:
- Status: Adequate
- Coverage: This is a declared pure refactor of test scaffolding, so the correct outcome is no new
  tests and no changed subjects. Both suites keep their subjects: multipane keeps the per-pane
  routing assertions (`multipane_legacy_integration_test.go:51-54`, four `AssertMarkerCount` calls
  proving each hook fired once in its own pane and zero times in the other) and the un-stamped
  bare-shell case (`:63-73`); the rename family keeps its rename-immune live-token check
  (`rename_reboot_hook_integration_test.go:78`), its single hook fire (`:89`), and the durability
  suite's two-cycle re-persist / re-fire pair (`rename_reboot_durability_integration_test.go:33,42,65`).
- Notes: The fixture would still fail loudly if the behaviour under test broke — the token is asserted
  in the capture (`reboot_fixture_test.go:131-135`), on the live pane after rename, and by the marker
  counts after each hydrate. The empty-`panes` guard at `:49-51` keeps a mis-parameterised fixture
  from silently arranging nothing. No redundant new assertions were added; the arrange-time
  `verifyHookKeyed` at `:74` and the post-rename loop at `multipane_legacy_integration_test.go:41-43`
  ask different questions (the write landed / it survived the rename and capture).

CODE QUALITY:
- Project conventions: Followed. The new file carries `//go:build integration`, which is required
  both by CLAUDE.md's lane rule (it reaches `restoretest.BuildPortalBinaryDir`, which builds the
  portal binary) and because every helper it calls in `internal/restoretest` is itself
  integration-tagged. Isolation is intact: `portaltest.IsolateStateForTest`, the state-dir teardown
  guard in the prescribed position, a per-fixture `tmuxtest` socket prefix, and no `t.Parallel()`.
- SOLID principles: Good. `rebootPane` is a value description; the fixture owns the arrange and the
  reboot cycle; `renameRebootPane` keeps the rename suite's vocabulary out of the shared fixture
  rather than adding a mode flag to it.
- Complexity: Low. Two short loops over `panes`, no branching beyond the empty-token / empty-hook
  skips.
- Modern idioms: Yes. `for range panes[1:]` for the split count is idiomatic; the `%v`/`%q` diagnostics
  carry both got and want.
- Readability: Good. Every helper's doc comment states why it exists rather than restating its body,
  and each failure message names the consequence (`"a capture that lost one would restore a hookless
  pane and prove nothing"`, `"a fixture that silently built the wrong topology fails here"`).
- Issues: None. I checked every comment in the changed code against it: the `rebootPane`/`rebootFixture`/
  `newRebootFixture` headers, the LIFO teardown note at `:77`, and the `assertLivePanes`/
  `captureAndPersist`/`persist`/`rebootAndHydrate` headers all hold true, and none references a task
  id, phase or spec section.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
