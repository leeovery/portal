TASK: resume-hooks-silently-lost-6-15 — Finish The Restore Reboot-Fixture Consolidation (tick-82c68a)

ACCEPTANCE CRITERIA:
- The arrange and the reboot-and-hydrate each exist once in `internal/restore`.
- `cmd/noncontiguous_window_reboot_integration_test.go` reaches the shared reboot sequence rather than reimplementing its first half.
- No test in `cmd/bootstrap` re-authors the scrollback seed.
- Deleting the `SetServerOption` line from `restore_marker.go` makes a test fail.

STATUS: complete

SPEC CONTEXT:
The work unit's defect is that a hook key was a volatile `<session>:<window>.<pane>` coordinate, so a
rename or a reboot orphaned the hook. The specification (§ "Tests that must change", spec line 516)
names `internal/restore/rename_reboot_hook_integration_test.go` and
`rename_reboot_durability_integration_test.go` as the suites re-pointed to prove the user-visible
guarantee under the new token key, with `internal/restore/rename_reboot_shared_test.go` (spec line 521)
as their declared shared home. This task is a phase-6 implementation-analysis task, so its authority is
its own body: it consolidates the duplicated arrange, the duplicated reboot-and-hydrate, the scrollback
seed, and covers the `@portal-restoring` bracket's set half. It changes test scaffolding only — no
production behaviour is touched.

IMPLEMENTATION:
- Status: Implemented (and legitimately carried further by later tasks in the same plan)
- Location:
  - Shared arrange + reboot-and-hydrate: `internal/restore/reboot_fixture_test.go:46`
    (`newRebootFixture`) and `:153` (`(*rebootFixture).rebootAndHydrate`). The task's Do list named
    `rename_reboot_shared_test.go` as the home; a later task generalised the fixture from
    "rename-reboot" to "reboot" (adding a `[]rebootPane` parameter so the multipane suite shares it too)
    and moved it to its own file. That is a strict superset of what the task asked for and a sound
    divergence — the substance (authored once per package) is delivered.
  - Three suites now arrange through it: `internal/restore/rename_reboot_hook_integration_test.go:71`,
    `rename_reboot_durability_integration_test.go:21`, `multipane_legacy_integration_test.go:30` and `:63`.
    `internal/restore/rename_reboot_shared_test.go` retains only what is genuinely shared vocabulary
    (`renameRebootPane`, `capturedPaneToken`, `verifyHookKeyed`).
  - Promoted reboot sequence: `internal/restoretest/reboot.go:22` (`OpenRebootGap`), `:40`
    (`RebootServer`), `:55` (`RestoreFromState`).
  - `cmd/noncontiguous_window_reboot_integration_test.go:361` calls `restoretest.RebootServer` and
    `:364` calls `restoretest.RestoreFromState`; its open-coded kill/guard/EnsureServer/Orchestrator
    copy is gone, and the `disableRenumberWindows` re-application still sits between the two, which is
    exactly what `RebootServer`'s doc (`internal/restoretest/reboot.go:36-39`) tells a caller to do for
    a server-lifetime option.
  - Scrollback seed: `internal/restoretest/scrollback.go:14` declares `ANSIScrollback`;
    `cmd/bootstrap/reboot_roundtrip_test.go:108-109` and `:594-595` both route through
    `restoretest.SeedScrollback` with it. `verifyANSIScrollback` is retained as the assertion at
    `cmd/bootstrap/reboot_roundtrip_test.go:405`, so the shared const's ANSI prefix has a reader.
  - Marker set half: `internal/restoretest/restore_marker.go:30` calls `assertRestoringSet` (`:40`)
    between the `SetServerOption` at `:22` and `o.Restore()` at `:31`, so every caller of
    `RestoreWithMarker` observes it.
- Notes:
  - I verified the consolidation lost no assertion. The pre-task `runRenameRebootFire` checked the
    captured pane token, `verifyHookKeyed`, a `strings.Contains(restoredPanes, "0:0")` post-restore
    check and the hook-fire count; all four survive, and the `0:0` substring check was replaced by
    `(*rebootFixture).assertLivePanes` (`internal/restore/reboot_fixture_test.go:104`), which pins the
    full ordered coordinate set through an exactly-pinned target — strictly stronger than what it
    replaced.
  - The dropped `restoretest.PrependPATH` call is correct, not an omission: these fixtures' only portal
    subprocess is the hydrate helper, which is pinned by path through `StagedHydrateExe`, exactly as
    `internal/restoretest/restoretest.go:83-90` documents.
  - `internal/restoretest/reboot.go` carries `//go:build integration`; `restore_marker.go` stays
    untagged because `internal/restore`'s untagged `integration_test.go` calls it. Lane rules hold —
    everything that builds or execs the portal binary remains integration-tagged.
  - No orphans left behind: `assertHookFireCount`, `rebootScrollback`, `persistIndex`,
    `newRenameRebootFixture` and the `renameHydrateBudget`/`renameHydrateTick` constants have no
    remaining occurrence anywhere in the tree.

TESTS:
- Status: Adequate
- Coverage:
  - AC4 is covered twice over, and independently. `internal/restoretest/restore_marker_test.go:74`
    (`TestRestoreWithMarker_BracketsTheRestore`, unit lane) drives `RestoreWithMarker` over a
    server-option recorder built on `commandertest` and asserts at `:91-97` that the
    `set-option -s @portal-restoring 1` call was recorded and ordered before the unset — deleting the
    `SetServerOption` line at `internal/restoretest/restore_marker.go:22` fails that subtest directly.
    The in-fixture `assertRestoringSet` would additionally fatal every caller.
  - The guard-on-the-guard is present at `internal/restoretest/restore_marker_test.go:120-130`: it runs
    `assertRestoringSet` against a `harnesstest.Recorder` with the marker unset and requires a fatal, so
    the assertion's own failing path is exercised rather than assumed. That is what makes the AC's
    "makes a test fail" a proven property rather than a claim.
  - The moved fixtures are exercised by the existing integration suites unchanged in subject
    (`TestRenameRebootHook_ExternalRename`, `_RenameSessionEquivalent`, `_PaneProcessKeptRunning`,
    `_DurableAcrossRepeatedReboots`, the two `TestMultiPaneLegacy_*` cases, the divergent-window reboot,
    and both reboot-round-trip cases).
- Notes:
  - `verifyHookKeyed` is asserted twice per run for the rename suites — once inside the arrange
    (`internal/restore/reboot_fixture_test.go:74`) and again after the capture
    (`rename_reboot_hook_integration_test.go:83`). Nothing between the two writes `hooks.json`, so the
    second is a re-assertion of a fact already held. It is cheap and reads as a deliberate
    checkpoint at the moment of interest; noted, not raised.

CODE QUALITY:
- Project conventions: Followed. Integration-only helpers are build-tagged; the fixture registers
  `portaltest.RegisterStateDirTeardownGuard` after `IsolateStateForTest` and before `tmuxtest.New`, which
  is the ordering CLAUDE.md mandates; the state dir is the one `IsolateStateForTest` owns and pins in
  `PORTAL_STATE_DIR` (`internal/portaltest/isolated_env.go:79`), so the fixture cannot register one
  directory and write to another; no test hand-rolls a `go build`.
- SOLID principles: Good. `OpenRebootGap` / `RebootServer` / `RestoreFromState` are three separable
  steps rather than one all-or-nothing helper, which is what lets
  `cmd/bootstrap/reboot_roundtrip_test.go:459` take the gap alone (its subject is the server start) while
  every other caller takes the whole reboot.
- Complexity: Low. The one branchy addition, `newRebootFixture`'s per-pane token/hook loops, guards both
  optional fields explicitly.
- Modern idioms: Yes.
- Readability: Good. Each moved helper carries a doc comment stating why the step exists (the kill
  confirmation, the server-lifetime-option caveat, the `binDir` pinning), not what the line does.
- Issues: None found. Comments in the changed code hold against it: `RebootServer`'s caveat about
  server-lifetime options matches both re-applying call sites, and `OpenRebootGap`'s claim that a
  separate caller exists is true (`cmd/bootstrap/reboot_roundtrip_test.go:459`).

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
