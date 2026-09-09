TASK: resume-hooks-silently-lost-8-14 (tick-691213) — "The Restore-Binary Exe Pin Has Two Unguarded Routes"

ACCEPTANCE CRITERIA:
- No integration-tagged fixture composes a `SessionRestorer` without a pinned `Exe`.
- The unit lane's bare literals still compile and pass, unguarded.
- The adapter constructor cannot produce an orchestrator with an empty `Exe`.
- The new guard fails on a staged offending literal and passes on the real tree.

STATUS: complete

SPEC CONTEXT:
Phase 8 is an implementation-analysis cycle task, so its authority is its own body rather than the
specification (per the shared verifier context). The underlying hazard it protects is real and stated in
the tree: `restore.SessionRestorer.Exe` / `restore.Orchestrator.Exe` are optional
(`internal/restore/session.go:30-31`), and `hydrateExe` treats a nil resolver identically to an absent
field, falling back to `os.Executable` (`internal/restore/session.go:326-330`). Under `go test` that is
the test binary, so a live-server restore driven by an unpinned restorer respawns each pane into the
suite and the session disappears with no error — documented at
`internal/restoretest/restoretest.go:54-65`. Task 8-14 closes the two routes past the existing
Orchestrator guard: the bare `SessionRestorer` fixture and the adapter constructor.

Note on drift: the two guard files this task created (`session_restorer_literal_guard_test.go`,
`orchestrator_literal_guard_test.go`) were later folded by task 9-20 into the parameterised pair
`internal/restoretest/literal_guard_test.go` + `literal_guard_scan_test.go`. Verification below is against
the current tree, which is the source of truth; the substance of every criterion survives the fold.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/restoretest/session_restorer_staged.go:19-28` — `NewSessionRestorer(t, client, stateDir, binDir)`,
    integration-tagged, pinning `Exe` through `StagedHydrateExe` and taking `binDir` rather than a resolver
    so the field cannot be forgotten at the call site.
  - `internal/restore/prefix_sibling_integration_test.go:43` — the one live-server `SessionRestorer` fixture
    now routes through the constructor; its hand-rolled four-field literal and its `internal/restore` import
    are gone.
  - `internal/restoretest/literal_guard_scan_test.go:48-54` — the `sessionRestorerGuard` descriptor, scoped
    to integration-tagged files via `isIntegrationTagged` (`:64-67`), which evaluates the file's build
    constraint with `integration` as the only satisfied tag rather than matching a filename.
  - `internal/restoretest/literal_guard_test.go:24-38` — the standing guard over both descriptors.
  - `internal/bootstrapadapter/adapters.go:63-86` — `NewRestoreAdapter` now takes
    `exe restore.ExecutableResolver` and returns `(nil, ErrRestoreExeRequired)` for a nil one;
    `internal/restoretest/orchestrator_staged.go:34-41` is its one caller and fatals on the error.
- Notes:
  - AC1 holds on the real tree: no `restore.SessionRestorer` composite literal exists in any
    integration-tagged file. The only qualified literals are in six unit-lane `restore_test` files
    (`session_test.go`, `session_geometry_test.go`, `session_geometry_summary_test.go`,
    `session_markers_test.go`, `session_exact_target_test.go`, `commander_fake_loudness_test.go`) plus the
    guard's own string fixtures; the only unqualified ones are in the untagged in-package
    `internal/restore/session_hydrate_exe_test.go`. All nine integration-tagged files in `internal/restore`
    are `package restore_test`, so the guard's qualified-selector rule reaches every one of them.
  - Every other live-server restore route is pinned too: `armed_restore_integration_test.go:56,130` and
    `exit_closes_pane_integration_test.go:153` take `NewRestoreOrchestrator`, and
    `restoretest.RestoreFromState` (`internal/restoretest/reboot.go:55-58`) composes through the same
    constructor — so the constructor set, not just the guard, is what holds the property.
  - AC3 holds at the seam the task named. Production is deliberately outside it:
    `cmd/bootstrap_production.go:43-55` composes `&restore.Orchestrator{…}` and wraps it as a
    `RestoreAdapter{Inner: …}` literal, where the `os.Executable` fallback *is* the intended pin — and
    `adapters.go:72-73` says so. `NewRestoreAdapter`'s sole caller is `restoretest.StagedRestoreAdapter`;
    that pre-existing placement (an exported constructor in a production package serving tests alone) is
    what the task directed work at, not something it introduced.
  - The guard is scoped by rule to the qualified `restore.X` selector form written in an integration-tagged
    file. An aliased import, or an integration test written inside `package restore`, would be outside it.
    That is the orchestrator guard's pre-existing shape, inherited deliberately, and has no live instance
    today.

TESTS:
- Status: Adequate
- Coverage:
  - "it pins the staged binary on the restorer it returns" —
    `internal/restoretest/session_restorer_staged_test.go:11-33`: asserts `Exe()` resolves to
    `<binDir>/portal`, plus `StateDir` and a non-nil `Logger`.
  - "it refuses to build the restore adapter without an exe" —
    `internal/bootstrapadapter/adapters_test.go:107-119`: `errors.Is(err, ErrRestoreExeRequired)` and a nil
    adapter alongside the error; the sibling subtest at `:121-141` proves the resolver is pinned onto the
    inner orchestrator rather than merely accepted.
  - The offending-literal cases live in the folded table at `internal/restoretest/literal_guard_test.go`:
    `:96-112` flags an integration fixture composing a `SessionRestorer` (the task's
    "…without an Exe" case, renamed by 9-20), `:117-131` flags one whose `Exe` is an explicit `nil` — the
    hole a key-presence rule would have left, and the reason the rule is "never compose the struct" —
    `:133-147` passes the fixture routing through the constructor, and `:151-165` is the task's
    "it ignores a unit-lane composite literal".
  - "passes on the real tree" is `TestNoTestComposesAPaneArmingRestoreType`
    (`literal_guard_test.go:24-38`), and the vacuous-pass tripwire is covered per descriptor at `:184-236`
    (empty tree, and a non-empty tree holding no integration-tagged file, each asserting the descriptor's
    own wording).
- Notes: the rule tests would fail if the guard broke — each case pins the scan's verdict, not just its
  absence of output — and the fixture set is AST-based, so the guard's own fixture strings are not
  self-findings. No redundancy worth flagging: the explicit-nil case and the unit-lane case each pin a
  distinct property the others do not.

CODE QUALITY:
- Project conventions: Followed. The new constructor is integration-tagged (it must be — `StagedHydrateExe`
  lives behind `//go:build integration`), its test with it; the guard and its rule tests are untagged so
  the unit lane polices both lanes, matching the repo's ~20 source guards and CLAUDE.md's lane rule. The
  guard is routed through `sourceguardtest` (`RepoSources`/`Rooted`/`BuildConstraint`/`SatisfiedWith`/
  `ParsedSource.Position`) rather than re-authoring the walk, and reports through
  `harnesstest.TestingT`/`Recorder` so its own fatal paths are testable.
- SOLID principles: Good. `NewSessionRestorer` mirrors `NewRestoreOrchestrator` per type rather than being
  abstracted into one generic constructor — the right call, since the two types' field sets are what the
  constructors exist to fix.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. The doc comments state the failure mode (silent test-binary arming) and the reason for
  each choice — why `binDir` rather than a resolver, why the session-restorer half is integration-scoped,
  and why a set `Exe` proves nothing.
- Comment accuracy: The claims hold against the code. `adapters.go:66-73`'s "an unset one is silent … falls
  back to os.Executable" matches `session.go:326-330`; the earlier false "logger must be non-nil" line was
  removed, and both `adapters_test.go` subtests pass a nil logger without issue.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
