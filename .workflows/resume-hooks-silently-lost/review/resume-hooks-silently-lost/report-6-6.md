TASK: resume-hooks-silently-lost-6-6 — Version-Pin The Hydrate Helper And Put The Real-Restore Tests Behind The Integration Lane

ACCEPTANCE CRITERIA:
- No production code composes a `portal` invocation as a bare PATH lookup.
- `go test ./...` (unit lane) contains no test that execs a built `portal` binary.
- The four named tests pass on a machine whose installed `portal` is an older release.
- `internal/restore`'s integration lane still passes with `-p 1`.

STATUS: complete

SPEC CONTEXT:
This is a phase-6 implementation-analysis task, so its own body is the authority rather than the
specification. The spec touches `buildHydrateCommand` only for the hook-key half of the contract
(specification.md:162 — the key is interpolated through `internal/shellquote`; specification.md:170 —
an untokened pane is armed with no `--hook-key` flag at all). Both properties survive this task's
change: the composed argv still single-quotes every interpolated value and still omits the flag for an
empty saved token, with the executable path added as a new leading quoted element. Nothing in the spec
constrains how the helper's own path is resolved.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/restore/session.go:30-36` — the optional `Exe ExecutableResolver` field and its type.
  - `internal/restore/session.go:315-341` — `hydrateFallbackExe` / `hydrateExeFallbackMsg` and
    `hydrateExe()`: nil resolver → `os.Executable`, an error or an empty path → one WARN and the bare
    `"portal"` degraded branch, so an unresolvable path cannot abort a reboot recovery (Do step 1).
  - `internal/restore/session.go:133` — resolved once per `armPanes` call and threaded into
    `buildHydrateCommand` at :148.
  - `internal/restore/session.go:349-359` — `buildHydrateCommand` now takes `exe` and quotes it through
    `shellquote.Single`, so a path containing spaces stays one shell word.
  - `internal/restore/restore.go:26-27, 74-82` — `Orchestrator.Exe` propagated into the restorer by
    `newSessionRestorer`. Production (`cmd/bootstrap_production.go:43-46`) leaves it nil, which is the
    intended `os.Executable` path.
  - Lane relocation (Do step 2): `TestPhase3Integration_SaveRestoreRoundTrip` and
    `TestPhase3Integration_RestoreUsesLiveIndicesUnderBaseIndexDrift` now live in
    `internal/restore/armed_restore_integration_test.go:1` under `//go:build integration`;
    `cmd/bootstrap/phase5_integration_test.go:1` and
    `cmd/bootstrap/phase5_marker_suppression_integration_test.go:1` carry the tag, with the
    binary-free `TestPhase5_OrchestratorEndToEndSmoke` relocated to the untagged
    `cmd/bootstrap/phase5_smoke_test.go` so the unit lane keeps the coverage it can honestly run.
  - Staged-binary prologue (Do step 3): all four now build through `restoretest.BuildPortalBinaryDir`
    and pin the resolver via `restoretest.NewRestoreOrchestrator` / `restoretest.StagedRestoreAdapter`
    (`armed_restore_integration_test.go:20,56,88,130`; `phase5_integration_test.go:21,41,73,100`;
    `phase5_marker_suppression_integration_test.go:24,51`). In every case the build precedes
    `IsolateStateForTest`, which is what `cmd/bootstrap/helpers_integration_test.go:11-16` requires
    (a `go build` after the HOME re-point leaves an unremovable module cache behind).
  - PATH triage (Do step 4): `restoretest.PrependPATH` survives at exactly one site —
    `cmd/bootstrap/reboot_roundtrip_test.go:567` — with an in-source reason (that fixture registers the
    real global hooks, whose bodies invoke `portal` by name through `run-shell`), and
    `cmd/reattach_integration_test.go:41-51` keeps its own PATH stage for the same reason. Everywhere
    else the now-dead PATH setup is gone, and `restoretest.PrependPATH`'s doc comment
    (`internal/restoretest/restoretest.go:83-89`) states the distinction rather than leaving it to be
    rediscovered.
- Notes:
  - AC1 read literally is broader than the change: `internal/tmux/hooks_register.go:79,84,89` and
    `internal/tmux/portal_saver.go:35` still compose bare `portal state notify` / `commit-now` /
    `signal-hydrate` / `daemon`. Those are deliberately unpinned and outside this task's change-set —
    a global hook body and the `_portal-saver` pane command outlive the binary that wrote them, and
    the daemon argv is the literal `state.PgrepPortalDaemons` identity contract
    (`^portal state daemon( |$)`), which an absolute path would break machine-wide. The criterion is
    met in substance: the one invocation composed *per restore*, where version identity matters and
    nothing outlives the process, is now pinned.
  - The `"portal"` fallback at `internal/restore/session.go:318` is itself a bare PATH lookup, but it
    is exactly what Do step 1 asked for, it is the degraded branch, and both the constant's comment and
    the function's say so.
  - No consumer identifies a hydrate pane by an argv prefix that the absolute path would break: the one
    matcher, `internal/restore/exit_closes_pane_integration_test.go:78`, was widened from
    `sh -c.*portal state hydrate.*` to `sh -c.*state hydrate.*` in the same change, which it had to be —
    `shellquote.Single` renders the exe as `'/path/portal'`, so the old literal would no longer match
    and the test would have passed vacuously. The pane-key suffix still scopes the pattern.
    `log.ResolveProcessRole` reads positional args, not the exe name, so the process role is unchanged.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/restore/session_hydrate_exe_test.go:14-80` covers the resolver ladder end to end: the
    nil-`Exe` default resolving to `os.Executable`'s absolute path (and explicitly not the bare name),
    an injected resolver winning, and both degraded branches (error, empty-path-with-nil-error). The
    task's second named unit test — the fallback still composing a runnable command — is
    `:59-67`, asserting the whole argv rather than just the exe.
  - `:82-113` pins the diagnostics: exactly one WARN, carrying the `error` attr on the error branch and
    carrying none on the empty-path branch (so no nil error attr is emitted).
  - `:124-146` pins `Orchestrator.Exe` propagation into the restorer — the seam that would otherwise
    fail silently, since an unpropagated resolver degrades to `os.Executable` rather than erroring.
  - `internal/restore/session_test.go:622-652` drives the real `Restore` through a fake commander with a
    pinned `Exe` and asserts the exact respawn argv, so the pinning is observed at the composition site
    and not only at the helper.
  - `internal/restore/session_build_hydrate_test.go` was re-based onto an explicit `testExe`, keeping
    the quoting cases (spaces, embedded quote, shell metacharacters, hook-key omission) intact.
  - `internal/restoretest/staged_hydrate_exe_test.go:14-40` covers the test-side pin, including the
    empty-`binDir` fatal that stops a caller silently degrading back to a PATH lookup.
- Notes:
  - The task's third test line ("the two `TestPhase3Integration_*` tests pass with a deliberately stale
    binary earlier on PATH") is satisfied structurally rather than by a stale-binary fixture: those
    tests no longer consult PATH for the hydrate helper at all. A regression in the pinning is still
    caught, and caught faster, by the unit assertions above — `session_test.go:646` and
    `session_hydrate_exe_test.go:20-32` both fail if `hydrateExe` reverts to the bare name.
  - Mild overlap between `session_hydrate_exe_test.go:59-79` and `session_build_hydrate_test.go:12-30`
    (both assert the composed argv shape). Each is asking a different question — fallback
    composability versus flag rendering — so the duplication is small and purposeful.
  - The unit-lane residue in `internal/restore/integration_test.go` (`SweepOrphanFIFOs`,
    `CorruptSessionsJSON`) arms no pane: the latter routes through
    `restoretest.NewFakeExeOrchestrator` (`:72`), whose deliberately non-existent path is documented at
    `internal/restoretest/orchestrator.go:11-14`. No unit-lane test builds, spawns or execs a portal
    binary — `internal/portalbintest/lane_guard_test.go` now enforces the build half of that rule, and
    every `SpawnIsolatedDaemon` caller is integration-tagged.

CODE QUALITY:
- Project conventions: Followed. The lane rule (CLAUDE.md "Test isolation for daemon-spawning tests")
  is what the relocation implements; the `error` attr and the WARN level sit inside the closed logging
  vocabulary and are emitted through the injected `restore` component logger
  (`internal/restore/logger_nil.go:15-17`), not a locally constructed one.
- SOLID principles: Good. `ExecutableResolver` is a one-function seam with a nil-means-production
  default, mirroring `internal/spawn`'s `ExecutableResolver` (`internal/spawn/command.go:3-5`) rather
  than inventing a second shape; the two stay independent types so neither package takes an edge on
  the other. Production has exactly one `SessionRestorer` construction site
  (`internal/restore/restore.go:77`), so the field cannot be forgotten in production.
- Complexity: Low. `hydrateExe` is a four-branch resolver; `buildHydrateCommand` gained one parameter
  and one `shellquote.Single` call.
- Modern idioms: Yes.
- Readability: Good. The two comments that carry the reasoning — why the fallback is a PATH lookup and
  why it is never the intended branch (`session.go:315-316`), and why an unresolvable path degrades
  instead of aborting (`session.go:322-325`) — say the non-obvious thing rather than restating the code,
  and both hold against it.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
