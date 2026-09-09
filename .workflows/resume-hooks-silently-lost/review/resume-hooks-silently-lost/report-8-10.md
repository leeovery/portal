TASK: resume-hooks-silently-lost-8-10 — "Integration Fixtures Disagree About Which State Directory They Isolated"
(have `IsolateStateForTest` own `PORTAL_STATE_DIR` in both the process env and the returned env slice, and point the
three divergent `internal/restore` fixtures at the helper's returned `stateDir`)

ACCEPTANCE CRITERIA (from the plan task):
- `IsolateStateForTest` sets `PORTAL_STATE_DIR` in the process env and carries it in the returned slice, both naming
  the returned `stateDir`.
- The sandbox registry file's contents match the `PORTAL_STATE_DIR` the slice carries.
- The three fixtures write to, register and tear down one directory.
- `cmd/bootstrap`'s fixtures still resolve their own state dir by setting it after the helper.

STATUS: complete

SPEC CONTEXT: This is a phase-8 implementation-analysis task, so its authority is its own body rather than the
specification (per the shared verifier context). The governing project context is `CLAUDE.md`'s ABSOLUTE INVARIANT on
test isolation and its three boundaries — filesystem/state, tmux, processes. The property at stake is boundary 3: the
daemon-pgrep sandbox is default-deny and carries ownership across the process boundary through a registry file listing
test-owned state dirs, so a subprocess orphan sweep can only SIGKILL a daemon whose state dir the registry names. A
fixture that registered directory A and pointed its daemon at directory B put its own daemon outside the registry —
exactly the recognition the registry exists to provide.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/portaltest/isolated_env.go:80` — `t.Setenv("PORTAL_STATE_DIR", stateDir)`, placed after the state dir is
    created (`:72-75`) and before the `os.Environ()` read at `:94`, so the derived slice carries it.
  - `internal/portaltest/isolated_env.go:97-98` — filter-then-append of `PORTAL_STATE_DIR=<stateDir>`, mirroring the
    `XDG_CONFIG_HOME` (`:94-95`) and `TMUX` (`:103-104`) handling.
  - `internal/portaltest/isolated_env.go:82` (in-process registration) and `:88-92` (registry file written with
    `stateDir` and pointed at by `state.SandboxRegistryEnv`) — the same value the slice now names.
  - `internal/portaltest/isolated_env.go:12-21` — doc comment states the helper owns `PORTAL_STATE_DIR` in both the
    process env and the slice, and documents the override route (process env via `t.Setenv`, slice by appending, with
    `exec.Cmd` dedupe last-wins). Do-list item 2 satisfied.
  - Divergent fixtures now take the helper's `stateDir`:
    `internal/restore/prefix_sibling_integration_test.go:31,35`; and for the rename-reboot and multipane suites,
    `internal/restore/reboot_fixture_test.go:58,78` (`_, fx.stateDir = portaltest.IsolateStateForTest(t)` followed by
    `RegisterStateDirTeardownGuard(t, fx.stateDir)`), which is the arrange
    `internal/restore/multipane_legacy_integration_test.go:30,63` and
    `internal/restore/rename_reboot_durability_integration_test.go:21` route through. No `t.TempDir()` +
    `t.Setenv("PORTAL_STATE_DIR", …)` pair survives at any of the three sites; `rename_reboot_shared_test.go` now holds
    only shared constants and assertions, with no preamble at all.
  - `cmd/bootstrap` fixtures still set their own after the call:
    `cmd/bootstrap/orphan_sweep_integration_test.go:56-57`, `:113-114`, `:168-169`;
    `cmd/bootstrap/composition_e2e_harness_integration_test.go:114-115`. Criterion 4 holds.
- Notes:
  - The named line numbers in the task body (`rename_reboot_shared_test.go:52` etc.) no longer locate the preamble
    because a sibling consolidation task moved it into `newRebootFixture`. That is not a loss for this task: the
    coverage rule follows exactly one hop into a same-package arrange
    (`internal/portaltest/teardown_guard_coverage_test.go:120-137`, `:296`), so the two suites routed through the
    fixture are still judged rather than skipped, and Do-list item 4's concern (a helper putting the pairing out of
    the rule's reach) does not materialise.
  - Checked for collateral breakage from the helper now owning the variable: `SpawnIsolatedDaemon` appends its own
    per-orphan `PORTAL_STATE_DIR` *after* the passed slice (`internal/portaltest/spawn_daemon.go:31-33`), so
    last-wins still gives each orphan its own directory; and the backstop's `resolveDevStateDir`
    (`internal/portaltest/fingerprint.go:286-294`) reads only `XDG_CONFIG_HOME`/`HOME`, so the new variable cannot
    re-point the fingerprint snapshot. The value the helper sets equals `<configDir>/portal/state`, which is what
    `XDG_CONFIG_HOME`-based resolution already produced, so fixtures that resolve through `state.EnsureDir()` land on
    the same directory as before.

TESTS:
- Status: Adequate
- Coverage: `internal/portaltest/isolated_env_test.go:247-308` (`TestStateDirEnv`) carries all four named micro
  acceptance tests:
  - `:248` "it sets PORTAL_STATE_DIR to the returned state dir" — asserts the process env against the returned value.
  - `:256` "it carries PORTAL_STATE_DIR in the returned env slice" — seeds a decoy via `t.Setenv` first, then asserts
    exactly one entry (so the filter is exercised, not just the append) naming the returned dir. This is the test that
    fails if either half of the change is dropped.
  - `:270` "it registers the same directory with the sandbox registry as the slice names" — reads the registry file
    through the slice's `state.SandboxRegistryEnv` entry and compares its body to the slice's `PORTAL_STATE_DIR`,
    which is criterion 2 stated as an equality between the two artefacts rather than against a literal.
  - `:287` "it lets a caller override the state dir after the call" — pins the documented escape hatch the
    `cmd/bootstrap` fixtures rely on, across both the process env and a real subprocess.
- Notes: the override subtest is the weakest of the four — it appends after the helper, so it would still pass if the
  helper stopped setting the variable entirely; its `:256` sibling is what pins that. It is not redundant (nothing else
  covers the last-wins contract the doc comment promises) and it was explicitly asked for by the plan, so no change is
  called for. No over-testing: the four subtests cover four distinct properties, and the pre-existing env assertions in
  the file were not duplicated for the new variable.

CODE QUALITY:
- Project conventions: Followed. The helper stays test-only with the structurally-mandatory `*testing.T` first
  parameter; the new write is a `t.Setenv` (auto-restored) rather than an `os.Setenv`, so it cannot leak into a later
  test; the unit/integration lane split is untouched (the changed fixtures keep their `//go:build integration` tags).
- SOLID principles: Good — the change consolidates one responsibility (naming the isolated state dir) in the one place
  that creates that directory, which is the stated fix for the divergence.
- Complexity: Low. Two added statements plus a two-line filter/append, reusing the existing `filterEnvKeys` variadic.
- Modern idioms: Yes.
- Readability: Good. The comments at `:77-79` and `:84-87` state the ordering constraint that makes the slice carry
  both values, and both hold against the code (`:80` and `:92` precede the `os.Environ()` read at `:94`).
- Issues: none rising to a defect. The `filterEnvKeys(env, "PORTAL_STATE_DIR")` at `:97` is strictly redundant given
  `t.Setenv` at `:80` already replaced any inherited entry before `os.Environ()` is read — it is defensive symmetry
  with the `XDG_CONFIG_HOME` case (where the process value is deliberately empty and the filter *is* load-bearing),
  costs nothing, and is asserted by the `:261` count check. Not reported as a finding.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
