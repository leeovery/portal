---
phase: 3
phase_name: Integration Coverage for Tmux Transient and `portal clean`
total: 3
---

## bootstrap-cleanstale-wipes-hooks-on-tmux-transient-3-1 | approved

### Task 3-1: Shared integration scaffolding for transient `list-panes` reproduction

**Problem**: Tasks 3-2 and 3-3 both need to reproduce a tmux transient where `list-panes -a` returns exit ≠ 0 (failure mode (a)) or exit 0 with empty stdout (failure mode (b)) against a real tmux server, while keeping the rest of the production tmux command surface working. Without shared scaffolding, each test would re-implement Commander interception, hooks-store seeding, real-tmux fixturing, and `portal.log` reading — duplicating ~150 lines of brittle setup across two files and inviting drift between bootstrap-path and `portal clean`-path assertions. The spec's "Deterministic Repro Mechanism" mandates `Commander` injection at the `tmux.Client` boundary; this task builds that primitive once.

**Solution**: Create a single shared integration-test helper file under `cmd/bootstrap/` (alongside the existing `orphan_sweep_integration_test.go` and `composition_e2e_*` files which already pattern-match this co-location), guarded by `//go:build integration`, exposing: (i) a `transientListPanesCommander` that wraps `*tmux.RealCommander` and intercepts only `list-panes -a` calls — returning either an exit-≠-0 error or empty stdout based on per-test policy, with everything else pass-through; (ii) a `seedHooksJSON(t, stateDir, entries)` helper that writes a populated `hooks.json` into the isolated state dir; (iii) a `hooksJSONBytes(t, stateDir) []byte` helper for the byte-identical before/after assertion; (iv) a `tailPortalLog(t, stateDir) string` wrapper around `portaltest.ReadPortalLogSafe`. Isolation is provided by `portaltest.IsolateStateForTest(t)` per the project's daemon-test isolation rule.

**Outcome**: A shared `transient_listpanes_helpers_integration_test.go` file in `cmd/bootstrap/` compiles under `-tags integration`, exposes the four helpers listed above with stable signatures, and is exercised by a smoke-test subtest that builds the Commander stub, runs one intercepted `list-panes -a` call returning an error, runs one pass-through `list-windows` call returning real output, and verifies both behaviours. Both Tasks 3-2 and 3-3 import these helpers without further setup.

**Do**:
- Create file `cmd/bootstrap/transient_listpanes_helpers_integration_test.go` with `//go:build integration` and `package bootstrap_test` (matches the existing integration-test convention in this directory; see `cmd/bootstrap/orphan_sweep_integration_test.go:1-49` for the header / package shape to mirror).
- Define `transientListPanesCommander` as a struct wrapping an inner `tmux.Commander` (production `&tmux.RealCommander{}` by default) plus a policy field (`mode failureMode` where `failureMode` is one of `failExitNonZero`, `failEmptyStdout`, `passThrough`) and a one-shot/sticky toggle. Implement `Run(args ...string) (string, error)` and `RunRaw(args ...string) (string, error)`. Inside both: if `args[0] == "list-panes"` and `args` contains `"-a"`, apply the policy; otherwise delegate to the inner Commander verbatim. Track invocation count so the one-shot variant resets after the first intercepted call.
- For `mode == failExitNonZero`: return `("", fmt.Errorf("tmux list-panes -a: exit 1 (simulated transient)"))` so the wrapping at `ListAllPanesWithFormat` (Phase 1) produces a non-nil error to the adapter.
- For `mode == failEmptyStdout`: return `("", nil)`. This exercises failure mode (b) — `ListAllPanes` returns `([]string{}, nil)`, triggering the hazard guard at the adapter.
- Implement `seedHooksJSON(t *testing.T, env []string, entries map[string]string)` — write a valid `hooks.json` to the resolved config path. **Resolve from the `env` slice returned by `IsolateStateForTest(t)`, not from the global process env**, so the seed lands under the isolated tree regardless of which env vars the helper actually overrides today. Concretely: scan `env` for `PORTAL_HOOKS_FILE=...` first (use it verbatim if present); otherwise scan for `XDG_CONFIG_HOME=...` and join with `portal/hooks.json`; otherwise `t.Fatalf` (signals isolation regression — preferable to silently corrupting). Mirrors `cmd/config.go`'s `configFilePath` resolution but consumes the test-isolated env. Use the production `internal/hooks` package to construct the file so the on-disk shape stays canonical. `t.Logf` the resolved path to verify the seed lands under the isolated tree.
- Implement `hooksJSONBytes(t *testing.T) []byte` — read the resolved `hooks.json` file via `os.ReadFile` (no parsing). Return the raw bytes used for the byte-identical assertion. `t.Fatalf` on read errors.
- Implement `tailPortalLog(t *testing.T, stateDir string) string` — thin wrapper around `portaltest.ReadPortalLogSafe(stateDir)` returning the content as a string for substring-match assertions on Warn/Debug lines.
- Add a smoke-test subtest `TestTransientListPanesHelpers_Smoke` in the same file: build a `transientListPanesCommander{inner: &tmux.RealCommander{}, mode: failExitNonZero}`, call `Run("list-panes", "-a", "-F", "...")` — assert non-nil error and empty string; call `Run("list-windows", "-a")` — assert pass-through to real tmux. Use a real tmux fixture from `internal/tmuxtest` for the pass-through arm.
- Document the one-shot vs sticky decision inline: default to **sticky** — every `list-panes -a` invocation fails until the test resets the mode. Rationale: bootstrap step 11 calls `list-panes -a` exactly once via the adapter; the orphan-sweep at step 4 (Component B) also calls it. Sticky failure means both observe the transient consistently and the test does not depend on internal ordering. Tasks 3-2 / 3-3 can flip to one-shot if they need step-4 to succeed before step-11 observes the failure — the policy is per-test.
- Call `portaltest.IsolateStateForTest(t)` in any subtest that spawns subprocesses; apply `cmd.Env = env` on every spawned `exec.Cmd` per the project's daemon-test isolation rule (`CLAUDE.md` § "Test isolation for daemon-spawning tests").

**Acceptance Criteria**:
- [ ] File `cmd/bootstrap/transient_listpanes_helpers_integration_test.go` exists with `//go:build integration` header and `package bootstrap_test`
- [ ] `transientListPanesCommander` implements `tmux.Commander` (both `Run` and `RunRaw`)
- [ ] Interception applies only to invocations where `args[0] == "list-panes"` and `"-a"` is present in `args`; every other call is delegated to the inner Commander verbatim
- [ ] `seedHooksJSON`, `hooksJSONBytes`, and `tailPortalLog` helpers compile and have stable signatures consumable by Tasks 3-2 and 3-3
- [ ] Smoke-test subtest `TestTransientListPanesHelpers_Smoke` passes — verifies interception triggers on `list-panes -a` and pass-through works for unrelated commands
- [ ] `portaltest.IsolateStateForTest(t)` is invoked in the smoke test; the fingerprint backstop passes (no leakage into the developer's `~/.config/portal/state/`)
- [ ] No `t.Parallel()` calls per the cmd-package convention
- [ ] `go test -tags integration ./cmd/bootstrap/...` builds clean
- [ ] `go test ./...` (without the integration tag) remains green — the file is excluded by the build tag and does not break the default build

**Tests**:
- `"TestTransientListPanesHelpers_Smoke/intercepts_list_panes_dash_a_with_exit_nonzero"` — Commander stub returns non-nil error on `list-panes -a` call
- `"TestTransientListPanesHelpers_Smoke/intercepts_list_panes_dash_a_with_empty_stdout"` — Commander stub returns `("", nil)` for mode (b)
- `"TestTransientListPanesHelpers_Smoke/passes_through_unrelated_tmux_commands"` — `list-windows -a` reaches real tmux
- `"TestTransientListPanesHelpers_Smoke/seed_and_read_hooks_json_roundtrip"` — seed writes a valid `hooks.json`, `hooksJSONBytes` reads it back byte-for-byte
- `"TestTransientListPanesHelpers_Smoke/tail_portal_log_handles_missing_file"` — `tailPortalLog` does not fail when `portal.log` does not yet exist (relies on `ReadPortalLogSafe`'s ENOENT tolerance)
- `"TestTransientListPanesHelpers_Smoke/isolation_backstop_passes"` — the `IsolateStateForTest` fingerprint backstop registered in cleanup observes zero delta in the developer's state dir

**Edge Cases**:
- One-shot vs sticky policy: default sticky; expose a `OneShot bool` toggle on the Commander stub for Tasks 3-2 / 3-3 if they need step-4 (orphan sweep) to succeed before step-11 observes the transient
- Pass-through fidelity: the stub must delegate `RunRaw` as well as `Run`, otherwise scrollback-capturing paths in other parts of bootstrap break
- Hooks file path resolution: resolve from the `env` slice returned by `IsolateStateForTest(t)` rather than `os.Getenv`, so the helper is robust to future changes in which env vars `IsolateStateForTest` overrides
- Subprocess reap discipline: any helper that spawns a subprocess (none required here but possible in expansion) must use `portaltest.RegisterSubprocessCleanup` per the daemon-spawning-test rule
- tmux server teardown between subtests handled by `tmuxtest` fixtures; the helper file itself is server-agnostic and reusable across fresh-server and persisted-server scenarios

**Context**:
> Specification § "Deterministic Repro Mechanism" — Commander injection at the `tmux.Client` boundary is the canonical mechanism for reproducing failure mode (a). Stub `Commander` returning `exit 1` (or empty stdout for mode (b)) exercises the destructive path without spawning a real tmux server. Integration tests use the same mechanism at the integration boundary so they get the deterministic failure without coordinating a saver-respawn race.
>
> Specification § "Integration — Tmux Transient Simulation" — spawn a real tmux server, populate `hooks.json`, kill `_portal-saver` mid-bootstrap, and arrange for `list-panes -a` to return exit ≠ 0 via a `Commander` stub at the integration boundary. Assert `hooks.json` is unchanged at the end of the bootstrap.
>
> CLAUDE.md § "Test isolation for daemon-spawning tests" — every spawned subprocess must inherit `env` returned by `portaltest.IsolateStateForTest(t)`. The structural `*testing.T` parameter prevents the helper from being imported by non-`*_test.go` code, and the fingerprint backstop registered in `t.Cleanup` is defence-in-depth, not a substitute for the env override.
>
> Existing reference patterns in repo: `cmd/bootstrap/orphan_sweep_integration_test.go:1-67` (build tag header, isolation invocation), `internal/tmux/portal_saver_integration_test.go` (real-tmux fixture composition).

**Spec Reference**: `.workflows/bootstrap-cleanstale-wipes-hooks-on-tmux-transient/specification/bootstrap-cleanstale-wipes-hooks-on-tmux-transient/specification.md` § "Test Requirements" → "Deterministic Repro Mechanism", "Integration — Tmux Transient Simulation", "Integration — `portal clean` Analogue", "Coverage Matrix"

---

## bootstrap-cleanstale-wipes-hooks-on-tmux-transient-3-2 | approved

### Task 3-2: Bootstrap end-to-end integration test for tmux transient `list-panes` failure

**Problem**: Phases 1 and 2 close failure modes (a) and (b) at the unit level, but only an end-to-end integration test against a real tmux server can verify that the orchestrator's soft-warning wiring, the bootstrap step-ordering interactions (`EnsureSaver` → `Restore` → `EagerSignalHydrate` → `CleanStaleMarkers` → `SweepOrphanFIFOs` → `CleanStale`), and the new logging contract jointly preserve `hooks.json` under the original incident shape. Without this test, a regression in any step-11 wiring change could silently re-introduce the wipe, which is precisely the silent-data-destruction defect class this work unit exists to close. The spec's coverage matrix explicitly mandates this row.

**Solution**: Add an integration test in `cmd/bootstrap/` (under `//go:build integration`) that uses the Task 3-1 shared helpers to: (i) spin up a real tmux server via `tmuxtest`, (ii) seed `hooks.json` with ≥ 1 user-session entry, (iii) inject the `transientListPanesCommander` into the production `*tmux.Client` so `list-panes -a` returns exit ≠ 0 (mode (a)) or empty stdout (mode (b)), (iv) drive a bootstrap-triggering command end-to-end (via either direct orchestrator invocation through `buildProductionOrchestrator` or the `portal` subprocess binary built via `portalbintest.BuildPortalBinary`), (v) assert `hooks.json` is byte-identical before and after, (vi) assert `portal.log` contains the appropriate `Warn` line distinguishing mode (a) from mode (b).

**Outcome**: An integration test file (e.g., `cmd/bootstrap/cleanstale_transient_listpanes_integration_test.go`) ships with two subtests — one per failure mode — both passing under `-tags integration`. The test fails loudly if any future change reintroduces the destructive wipe at step 11 or removes the distinguishing log fingerprint introduced by Change 4.

**Do**:
- Create file `cmd/bootstrap/cleanstale_transient_listpanes_integration_test.go` with `//go:build integration` header and `package bootstrap_test`. Mirror the file-header convention from `cmd/bootstrap/orphan_sweep_integration_test.go:1-67`.
- Define `TestBootstrap_CleanStale_TmuxTransient_DoesNotWipeHooks(t *testing.T)` as the top-level test, with two subtests via `t.Run`:
  - `"mode_a_list_panes_exit_nonzero"`: Commander policy `failExitNonZero`.
  - `"mode_b_list_panes_empty_stdout"`: Commander policy `failEmptyStdout`.
- In each subtest:
  1. Call `env, stateDir := portaltest.IsolateStateForTest(t)` — registers the fingerprint backstop and scrubs the developer's state dir.
  2. Use `tmuxtest` to bring up a real tmux server fixture on an isolated socket; ensure teardown via `t.Cleanup`.
  3. Build a `*tmux.Client` using `tmux.NewClient(&transientListPanesCommander{inner: &tmux.RealCommander{...wrapping the test socket commander...}, mode: <policy>, sticky: true})`. The inner Commander needs to target the test tmux socket; if `RealCommander` does not accept a socket override, either (a) extend the helper from Task 3-1 to accept a base `Commander` constructed by `tmuxtest`, or (b) set the relevant `TMUX_TMPDIR` / `-L` env so the production `RealCommander` reaches the right socket. Use whichever path is idiomatic in existing real-tmux integration tests (audit `internal/tmux/portal_saver_integration_test.go` for the pattern).
  4. Seed `hooks.json` via `seedHooksJSON(t, stateDir, map[string]string{"user-proj:0.0": "claude --resume sess-1", "user-proj:0.1": "claude --resume sess-2", "other-proj:0.0": "echo resumed"})`. Capture `before := hooksJSONBytes(t)`.
  5. (Optional) Kill `_portal-saver` mid-bootstrap to compound the transient — exercises the original incident shape from spec § "Trigger Windows of Highest Empirical Risk" item 1. Skip if the orchestrator's first bootstrap call already triggers the same transient via the Commander stub.
  6. Drive a bootstrap-triggering command via the **commander-factory seam approach** (subprocess approach rejected — no env-var-driven Commander seam exists in production and adding one is out of scope). Add a small test-only seam in `cmd/bootstrap_production.go` following the `cleanDeps` / `bootstrapDeps` package-level-mutable-state pattern (CLAUDE.md § "DI / testing pattern"):
     ```go
     // commanderFactory is the indirection seam tests use to inject a
     // wrapping Commander. Production code leaves it at the default;
     // tests override and restore via t.Cleanup.
     var commanderFactory = func() tmux.Commander { return &tmux.RealCommander{} }
     ```
     Inside `buildProductionOrchestrator`, replace `client := tmux.DefaultClient()` with `client := tmux.NewClient(commanderFactory())`. One-line production change; the call path is unchanged because the default factory returns the same `*RealCommander` `DefaultClient` uses today. In the test, override the factory before invoking `buildProductionOrchestrator`:
     ```go
     base := commanderFactory()
     stub := &transientListPanesCommander{inner: base, mode: <policy>, sticky: true}
     prev := commanderFactory
     commanderFactory = func() tmux.Commander { return stub }
     t.Cleanup(func() { commanderFactory = prev })

     orch, _ := buildProductionOrchestrator()
     ctx := context.Background()
     _, warnings, err := orch.Run(ctx)
     ```
     Note the three-value destructure — `(*bootstrap.Orchestrator).Run` at `cmd/bootstrap/bootstrap.go:248` returns `(serverStarted bool, warnings []Warning, err error)`. The `serverStarted` bool is `_`-discarded here — it signals whether the tmux server was just started, consumed elsewhere for TUI-vs-bare-CLI warning drain ordering, but carries no assertion value for the hooks-preservation property under test. Assert against the returned `warnings` slice and `err`. Wiring caveat: this task introduces the seam in addition to consuming it — add the new package-level `var commanderFactory` and update the one call site inside `buildProductionOrchestrator` in the same PR so the test compiles. The production surface widens by one unexported variable — acceptable per the `cleanDeps` precedent.
  7. Capture `after := hooksJSONBytes(t)`. Assert `bytes.Equal(before, after)` — byte-identical, no entries removed.
  8. Read `portal.log` via `tailPortalLog(t, stateDir)`. Assert:
     - For mode (a): contains the substring `"stale-hook cleanup:"` + a propagated-error fragment (e.g., `"list-panes"` or `"exit 1"`) on a `WARN` line. Per spec § Change 4: mode (a) emits only the terminal Warn (no entry-point Debug because `ListAllPanes` itself failed).
     - For mode (b): contains the entry-point `"stale-hook cleanup: live=0 persisted=3"` Debug AND the hazard-guard `"stale-hook cleanup: zero live panes parsed with 3 hook(s) present; skipping to avoid mass-deletion hazard"` Warn.
  9. Assert the bootstrap call returned without a fatal abort — the orchestrator's warnings slice may contain a soft `Warning` but `Bootstrap()` must not return a fatal error. Per spec § "Bootstrap Posture Preserved".
  10. Apply `cmd.Env = env` to every spawned subprocess (any helper subprocess, daemon spawn, or `portal` binary invocation) per CLAUDE.md § "Test isolation for daemon-spawning tests".
  11. Register subprocess cleanup via `portaltest.RegisterSubprocessCleanup` if any `portal state daemon` is spawned indirectly (e.g., via the bootstrap's `EnsureSaver` step). Skip otherwise.
- Add a third subtest `"normal_path_legitimate_stale_removal_still_works"` as a regression guard: same setup but with `mode: passThrough`, seed `hooks.json` with one orphan entry (key not present in real tmux), assert the orphan is removed and the live entries are preserved, and assert `portal.log` contains the `"stale-hook cleanup: live=N persisted=M"` Debug followed by `"stale-hook cleanup: removed=1"` Debug. Validates the normal path is not broken by the hazard guard.

**Acceptance Criteria**:
- [ ] Test file `cmd/bootstrap/cleanstale_transient_listpanes_integration_test.go` exists with `//go:build integration` header
- [ ] Subtest `"mode_a_list_panes_exit_nonzero"` passes — `hooks.json` byte-identical, `portal.log` contains the propagated-error Warn (no entry-point Debug per Change 4 mutual exclusivity)
- [ ] Subtest `"mode_b_list_panes_empty_stdout"` passes — `hooks.json` byte-identical, `portal.log` contains the entry-point Debug AND the hazard-guard Warn with the correct counts
- [ ] Subtest `"normal_path_legitimate_stale_removal_still_works"` passes — orphan removed, live preserved, normal-path Debugs emitted
- [ ] `portaltest.IsolateStateForTest(t)` invoked in every subtest; fingerprint backstop passes (no developer-state-dir leakage)
- [ ] Bootstrap does not fatally abort in any subtest — the propagated error / hazard-guard skip surfaces as a soft warning, not a `PersistentPreRunE` failure (acceptance criterion 5 from spec)
- [ ] `cmd.Env = env` applied to any spawned subprocess
- [ ] No `t.Parallel()` calls
- [ ] `go test -tags integration -run TestBootstrap_CleanStale_TmuxTransient ./cmd/bootstrap/...` green
- [ ] `go test ./...` (default, no tag) remains green — build-tag-excluded
- [ ] No zombie `portal state daemon` subprocesses survive teardown (verify via `portaltest.PgrepPortalDaemons` in cleanup if relevant)

**Tests**:
- `"TestBootstrap_CleanStale_TmuxTransient_DoesNotWipeHooks/mode_a_list_panes_exit_nonzero"` — mode (a) end-to-end: hooks.json unchanged, propagated-error Warn in portal.log, no fatal abort
- `"TestBootstrap_CleanStale_TmuxTransient_DoesNotWipeHooks/mode_b_list_panes_empty_stdout"` — mode (b) end-to-end: hooks.json unchanged, entry-point Debug + hazard-guard Warn in portal.log, no fatal abort
- `"TestBootstrap_CleanStale_TmuxTransient_DoesNotWipeHooks/normal_path_legitimate_stale_removal_still_works"` — pass-through Commander: orphan removed, live preserved, removed=1 Debug emitted

**Edge Cases**:
- Commander stub must not interfere with bootstrap step 4 (orphan sweep, `pgrep`-based, not tmux-based) or step 9 (`CleanStaleMarkers`, uses `ListAllPanesWithFormat` directly — also goes through the same `list-panes -a` invocation). Step 9 may also observe the transient and surface its own hazard-guard Warn; that is correct behaviour and must not fail the test. Assert only on the step-11 hook-cleanup log lines, not on the step-9 marker-cleanup ones, to keep assertions stable.
- If step 9 observing the transient is also informative, add explicit assertions that step 9 also did **not** unset markers — confirms architectural consistency between the two callsites.
- Test must tolerate the orchestrator emitting an unrelated soft warning (e.g., `SaverDownWarning`) — assert by substring presence, not by exact warning-slice equality.
- The `_portal-saver` kill arm is optional — include it only if Option A allows it cleanly. The Commander stub alone is sufficient to trigger the destructive code path; the saver kill is realism, not necessity.
- Mode (a) must NOT emit the entry-point Debug line — verify explicitly that the substring `"live="` is absent on the WARN-only branch (mutual exclusivity per Change 4).
- Verify `hooks.json` byte-identity, not just key-set identity — a re-serialisation with reordered keys would also be a regression and the byte assertion catches it.

**Context**:
> Specification § "Coverage Matrix" — row "Tmux transient end-to-end" maps to "New integration test"; this task is that test.
>
> Specification § "Integration — Tmux Transient Simulation" — "Spawn a real tmux server, populate `hooks.json`, kill `_portal-saver` mid-bootstrap, and arrange for `list-panes -a` to return exit ≠ 0 via a `Commander` stub at the integration boundary. Assert `hooks.json` is unchanged at the end of the bootstrap."
>
> Specification § "Acceptance Criteria" — items 1 (hazard guard fires on `len(live)==0 && len(persisted)>0`), 2 (error propagation surfaces as soft warning), 4 (exact log-line contract: entry-point Debug + mutually-exclusive terminal line), 5 (bootstrap posture preserved — no fatal abort). All four are exercised by this test end-to-end.
>
> Specification § Change 4 — "Post-fix log distinguishability: failure modes (a) (exit ≠ 0) and (b) (exit 0 with empty stdout) become distinguishable in `portal.log` — mode (a) surfaces as the propagated-error `Warn` (no entry-point Debug line); mode (b) surfaces as the entry-point `Debug` followed by the hazard-guard `Warn`."
>
> CLAUDE.md § "Test isolation for daemon-spawning tests" — `portaltest.IsolateStateForTest(t)` MUST be called and `cmd.Env = env` applied to every spawned subprocess. The fingerprint backstop is defence-in-depth.
>
> CLAUDE.md § "Server bootstrap" — step-11 (CleanStale) runs after EnsureSaver, Restore, EagerSignalHydrate, CleanStaleMarkers, SweepOrphanFIFOs. The test exercises the full eleven-step orchestrator, not a bare step-11 invocation.

**Spec Reference**: `.workflows/bootstrap-cleanstale-wipes-hooks-on-tmux-transient/specification/bootstrap-cleanstale-wipes-hooks-on-tmux-transient/specification.md` § "Test Requirements" → "Integration — Tmux Transient Simulation", § "Acceptance Criteria" items 1, 2, 4, 5, § "Coverage Matrix" row "Tmux transient end-to-end"

---

## bootstrap-cleanstale-wipes-hooks-on-tmux-transient-3-3 | approved

### Task 3-3: `portal clean` end-to-end integration test for tmux transient `list-panes` failure

**Problem**: The `portal clean` subcommand at `cmd/clean.go:75-91` is the second destructive consumer of `ListAllPanes` and shares the hazard-guard fix in Phase 2. Without an end-to-end integration test against a real tmux server, a regression in the `RunE` closure could silently re-introduce the wipe at this callsite — bypassing the bootstrap-path coverage in Task 3-2. The defect-class scope confirms exactly two destructive callsites; both must have integration coverage per the spec's coverage matrix. This task closes the `portal clean` row.

**Solution**: Add an integration test that drives `portal clean` end-to-end via the `portal` binary (built with `portalbintest.BuildPortalBinary`), with the `transientListPanesCommander` from Task 3-1 injected into the binary's tmux client surface. Seed `hooks.json` with ≥ 1 user-session entry, invoke `portal clean`, assert `hooks.json` is byte-identical, assert `portal.log` contains the expected Warn line, assert `portal clean` exits 0 (no user-visible error per spec § Change 3 "soft-warning surfacing contract"), and assert stderr contains no `Removed stale hook:` lines for seeded entries.

**Outcome**: An integration test `cmd/cleanstale_transient_listpanes_clean_integration_test.go` (or a peer file under `cmd/`) ships with subtests for mode (a), mode (b), and the normal-path regression guard, all passing under `-tags integration`. The test fails loudly if the `portal clean` callsite reintroduces the destructive wipe under tmux transient.

**Do**:
- Create file `cmd/cleanstale_transient_listpanes_clean_integration_test.go` with `//go:build integration` header and `package cmd_test`. Mirror the file-header convention from existing integration tests in `cmd/` (e.g., `cmd/state_daemon_integration_test.go`, `cmd/reattach_integration_test.go`).
- Define `TestPortalClean_TmuxTransient_DoesNotWipeHooks(t *testing.T)` with subtests:
  - `"mode_a_list_panes_exit_nonzero"`: Commander policy `failExitNonZero`.
  - `"mode_b_list_panes_empty_stdout"`: Commander policy `failEmptyStdout`.
  - `"normal_path_legitimate_stale_removal_still_works"`: Commander pass-through; one orphan entry seeded; assert removal.
- Injection approach — `portal clean` runs as a Cobra subcommand inside the user's invocation of the `portal` binary. The `cleanDeps` package-level variable (`cmd/clean.go:24`) exists for unit-test injection but is not reachable from a subprocess. Two viable approaches:
  - **Option A (in-process)**: invoke the `cleanCmd.RunE` closure directly in the test process after seeding `cleanDeps = &CleanDeps{AllPaneLister: <stub>}` where the stub wraps the production `*tmux.Client` plus the transient-Commander interception. Lower fidelity (skips Cobra arg parsing) but full access to the assertions and seam injection. Use this if the existing `cmd/clean_test.go` tests follow this convention (audit `cmd/clean_test.go:327-368` for the pattern).
  - **Option B (subprocess)**: build the `portal` binary with `portalbintest.BuildPortalBinary`, run `portal clean` as `exec.Cmd` with `cmd.Env = env`, set `PORTAL_HOOKS_FILE` / `PORTAL_STATE_DIR` env to the isolated paths. Higher fidelity but cannot inject the Commander stub into the subprocess's tmux client without a production seam. Since no env-var-driven Commander injection exists in production code, Option B requires building a fake tmux binary on PATH — out of scope and high friction. Prefer Option A.
- For Option A, set `cleanDeps = &CleanDeps{AllPaneLister: <stub>}` in `t.Setenv`-style setup with `t.Cleanup(func() { cleanDeps = nil })`. The stub is a thin struct wrapping the real `*tmux.Client` (or a fake one) whose `ListAllPanes()` method routes through the `transientListPanesCommander`. Construct it via `tmux.NewClient(transientStub)`.
- In each subtest:
  1. `env, stateDir := portaltest.IsolateStateForTest(t)` — isolation + backstop.
  2. `tmuxtest` real-tmux server (only if needed for the pass-through subtest; mode (a) and mode (b) can skip the real server since `ListAllPanes` itself never reaches tmux).
  3. Seed `hooks.json` via `seedHooksJSON(t, stateDir, ...)`. Capture `before := hooksJSONBytes(t)`.
  4. Inject `cleanDeps` with the transient stub.
  5. Invoke `cleanCmd.RunE(cleanCmd, []string{})` (or equivalent — audit existing `cmd/clean_test.go` for the call convention).
  6. Assert `err` returned by `RunE` is `nil` — per spec § Change 3 "the propagated error is surfaced as a `Warn` log line at the callsite plus an early non-destructive return; the subcommand's `RunE` continues to return nil for the hook-cleanup tail's transient failures."
  7. Capture `after := hooksJSONBytes(t)`. Assert `bytes.Equal(before, after)`.
  8. Capture stdout/stderr (via `cleanCmd.SetOut` / `SetErr` to a `bytes.Buffer`). Assert neither stream contains the substring `"Removed stale hook:"` for any seeded entry — user-facing surface unchanged on the deferral branch (per spec § Change 3 "User-facing stderr output from `portal clean` is unchanged").
  9. Read `portal.log` via `tailPortalLog(t, stateDir)`. Assert:
     - Mode (a): contains the substring `"stale-hook cleanup:"` + propagated-error fragment on a `WARN` line; no entry-point Debug line (`"live="` substring absent on a mode-a-WARN-only branch).
     - Mode (b): contains entry-point `"stale-hook cleanup: live=0 persisted=3"` Debug AND hazard-guard `"stale-hook cleanup: zero live panes parsed with 3 hook(s) present; skipping to avoid mass-deletion hazard"` Warn.
- For the `"normal_path_legitimate_stale_removal_still_works"` subtest: pass-through Commander, seed hooks with one entry whose key is not present in the real tmux server, invoke `portal clean`, assert that entry is removed (`hooks.json` updated), stdout contains `"Removed stale hook: <key>"`, and `portal.log` contains the `"stale-hook cleanup: removed=1"` Debug.
- Add a `"persisted_empty_early_exit_emits_breadcrumb"` subtest: seed `hooks.json` as empty (or do not seed at all), invoke `portal clean`, assert `RunE` returns nil, `hooks.json` unchanged (still empty / absent), `portal.log` contains exactly `"stale-hook cleanup: persisted=0, skipping"` Debug per spec § Change 4 "portal clean early-exit special case", and no `live=` substring (early-exit fires before enumeration).
- Apply `cmd.Env = env` if any subprocess is spawned. Register `portaltest.RegisterSubprocessCleanup` for any daemon subprocess.

**Acceptance Criteria**:
- [ ] Test file `cmd/cleanstale_transient_listpanes_clean_integration_test.go` exists with `//go:build integration` header
- [ ] Subtest `"mode_a_list_panes_exit_nonzero"` passes — `hooks.json` byte-identical, `portal.log` contains propagated-error Warn, `RunE` returns nil, stderr/stdout free of `Removed stale hook:` lines for seeded entries
- [ ] Subtest `"mode_b_list_panes_empty_stdout"` passes — `hooks.json` byte-identical, entry-point Debug + hazard-guard Warn both present in `portal.log`, `RunE` returns nil
- [ ] Subtest `"normal_path_legitimate_stale_removal_still_works"` passes — orphan removed, live preserved, `removed=1` Debug emitted, stdout reports the removed hook
- [ ] Subtest `"persisted_empty_early_exit_emits_breadcrumb"` passes — early-exit Debug emitted, no enumeration occurs, no destructive action
- [ ] `portaltest.IsolateStateForTest(t)` invoked in every subtest; fingerprint backstop passes
- [ ] No `t.Parallel()` calls; `cleanDeps` mutation is wrapped in `t.Cleanup` reset per the cmd-package mutable-state convention
- [ ] `go test -tags integration -run TestPortalClean_TmuxTransient ./cmd/...` green
- [ ] `go test ./...` (default, no tag) remains green — build-tag-excluded
- [ ] No zombie subprocesses survive teardown

**Tests**:
- `"TestPortalClean_TmuxTransient_DoesNotWipeHooks/mode_a_list_panes_exit_nonzero"` — mode (a): hooks.json unchanged, propagated-error Warn in portal.log, RunE returns nil, stderr/stdout free of Removed-stale-hook lines
- `"TestPortalClean_TmuxTransient_DoesNotWipeHooks/mode_b_list_panes_empty_stdout"` — mode (b): hooks.json unchanged, entry-point Debug + hazard-guard Warn, RunE returns nil
- `"TestPortalClean_TmuxTransient_DoesNotWipeHooks/normal_path_legitimate_stale_removal_still_works"` — pass-through Commander: orphan removed, removed=1 Debug, stdout reports removal
- `"TestPortalClean_TmuxTransient_DoesNotWipeHooks/persisted_empty_early_exit_emits_breadcrumb"` — empty hooks.json: persisted=0 Debug breadcrumb, no enumeration, no destructive action

**Edge Cases**:
- The pre-enumeration early-exit at `cmd/clean.go:71-73` (persisted == 0) must NOT be conflated with the hazard-guard path — both emit a Debug but they are structurally distinct branches. Assert the early-exit subtest sees ONLY the `"persisted=0, skipping"` Debug and NO `"live="` substring.
- `cleanDeps` is package-level mutable state — test must reset it via `t.Cleanup(func() { cleanDeps = nil })` to avoid cross-test contamination. The cmd-package convention (no `t.Parallel`, mutable-state mocks cleaned by `t.Cleanup`) explicitly applies here per CLAUDE.md § "Build & Test".
- The `portal clean` callsite also runs the project-store cleanup at lines 41-56 before the hook cleanup tail — verify the project-store side is not affected by the Commander stub (it does not use `list-panes`). Either skip seeding projects entirely or seed projects such that no removals are reported, so stdout assertions on the absence of `Removed stale hook:` are not polluted by `Removed stale project:` lines (which are unrelated to this test).
- Stdout/stderr capture via `cleanCmd.SetOut(&buf)` / `SetErr(&buf)` is the canonical pattern in existing `cmd/clean_test.go`; audit and follow it. Resetting these in `t.Cleanup` is necessary because the command writers persist across subtests.
- Mode-a vs mode-b log distinguishability mirrors Task 3-2; assert by substring presence on the right log lines, not by exact equality of `portal.log` contents.
- `loadHookStore()` path resolution must respect the isolated `PORTAL_HOOKS_FILE` env override set by `IsolateStateForTest` — verify the seeded file is the one `portal clean` reads.

**Context**:
> Specification § "Coverage Matrix" — row "`portal clean` transient end-to-end" maps to "New integration test"; this task is that test.
>
> Specification § "Integration — `portal clean` Analogue" — "Same pattern against the `portal clean` callsite — assert it does not wipe entries on transient `ListAllPanes` failure or empty result."
>
> Specification § Change 3 "Soft-warning surfacing contract" — "For `portal clean`, the propagated error is surfaced as a `Warn` log line at the callsite plus an early non-destructive return; the subcommand's `RunE` continues to return nil for the hook-cleanup tail's transient failures (matching the existing pre-fix safety-net posture at lines 77-80, which already chose silence-and-continue over user-facing error). User-facing stderr output from `portal clean` is unchanged."
>
> Specification § Change 4 "portal clean early-exit special case" — "`portal clean` retains its existing pre-enumeration early-exit when `len(persisted) == 0`. On that branch, neither `ListAllPanes` nor the hazard guard runs. Emit a single `Debug` breadcrumb at the early-exit: `'stale-hook cleanup: persisted=0, skipping'`. The bootstrap adapter does NOT take this branch."
>
> CLAUDE.md § "Build & Test" — "Tests must not use `t.Parallel()` — the cmd package injects mocks via package-level mutable state (`bootstrapDeps`, `openDeps`, `attachDeps`, etc.) and cleans up with `t.Cleanup()`." This rule applies to `cleanDeps` in this task.
>
> CLAUDE.md § "Test isolation for daemon-spawning tests" — `portaltest.IsolateStateForTest(t)` and `cmd.Env = env` are mandatory. The fingerprint backstop is defence-in-depth.

**Spec Reference**: `.workflows/bootstrap-cleanstale-wipes-hooks-on-tmux-transient/specification/bootstrap-cleanstale-wipes-hooks-on-tmux-transient/specification.md` § "Test Requirements" → "Integration — `portal clean` Analogue", § "Acceptance Criteria" items 1, 2, 4, § "Coverage Matrix" row "`portal clean` transient end-to-end", § Change 3 "Soft-warning surfacing contract", § Change 4 "portal clean early-exit special case"
