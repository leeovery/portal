---
phase: 5
phase_name: "Retire attach & spawn"
total: 3
---

## cli-verb-surface-redesign-5-1 | approved

### Task 5.1: Retire `attach` — delete the command and migrate its behaviours to `open --session`

**Problem**: `portal attach` is fully absorbed by `open` — its two former jobs are `open --session <name>` (exact/no-guess attach for scripts, Phase 2) and `portal open --session <name> --ack <batch>:<token>` (the spawned-window exec target, Phase 3). The command form only ever existed for cross-process callers; both `open` and the former `attach` already call the identical in-process connect functions (`switch-client` inside tmux / exec `attach-session` outside). Keeping `attach` is now pure dead surface, and the redesign's headline is a single public session verb — so `attach` must be deleted outright, not aliased or deprecated.

**Solution**: Delete `cmd/attach.go` and `cmd/attach_test.go` in one atomic green commit. Relocate the two symbols they house that survivors still need (`SessionValidator` → `kill`; the shared `mockSessionConnector`/`mockSessionValidator` test doubles → a surviving `_test.go`). Migrate every remaining `attach` reference — the abridged-fast-path regression, the version-guard non-exempt case, the tmux-missing rows, the reattach integration cases — to `open --session`. Drop the stale `attach` argv→role case in `internal/log/ResolveProcessRole` and its test. No back-compat alias, no deprecation warning (the `hooks`→`hook` carve-out is Phase 6, NOT applicable here).

**Outcome**: `portal attach` no longer exists (unregistered on `rootCmd`, gone from `--help`); `open --session <name>` preserves attach's exact behaviour (inside-tmux switch-client / outside exec-attach; `No session found: <name>` hard-fail that never mints and never pops the picker); the abridged latch-satisfied fast-path is proven command-agnostic via `open --session`; `internal/spawn` (`AckWriter`/`NewServerOptionAckChannel`/`ParseSpawnAckFlag`) is untouched; the whole module builds and both test lanes are green.

**Do**:
- **Relocate `SessionValidator` before deleting `attach.go`.** Move the interface (`cmd/attach.go:14-17`, `type SessionValidator interface { HasSession(name string) bool }`) verbatim into `cmd/kill.go` (kill's `KillDeps.Validator` is its live consumer; the reattach integration test's `var _ SessionValidator = (*mockSessionValidator)(nil)` compile-assertion also depends on it). Do not change its shape.
- **Relocate the shared test doubles before deleting `attach_test.go`.** Move `mockSessionConnector` (`cmd/attach_test.go:19-31`) and `mockSessionValidator` (`cmd/attach_test.go:34-40`) into a new surviving file `cmd/session_mocks_test.go` (package `cmd`). They are consumed across `cmd/kill_test.go`, `cmd/version_guard_test.go`, `cmd/abridged_route_test.go`, `cmd/reattach_integration_test.go`, `cmd/open_test.go`, and `cmd/open_fatal_test.go`, so they MUST survive the file deletion. The attach-only doubles `mockAckWriter` and `ackWrite` (`cmd/attach_test.go:42-63`) are deleted with the file (no other consumer — `open`'s `--ack` tests use `spawntest.FakeAckChannel` per Phase 3).
- **Delete `cmd/attach.go` entirely**: `attachCmd`, `AttachDeps`, `attachDeps`, `buildAttachDeps`, and the `init()` that registers `attachCmd` and its `--spawn-ack` flag. `SessionValidator` is the only thing that leaves rather than dies (relocated above). Note `cmd/attach.go:64` is the last cmd-package consumer of the package-level `spawnLogger` var (declared in `cmd/spawn.go`); after this deletion that var is declared-but-unused at package scope, which the Go compiler tolerates — Task 5-2 removes the declaration. Do not remove it here (that would break `cmd/spawn.go` until 5-2).
- **Delete `cmd/attach_test.go` entirely** (`TestAttachCommand`, `TestAttachSpawnAck`, plus the relocated/deleted doubles above).
- **Migrate the abridged-fast-path regression (AC #5)** — `cmd/abridged_route_test.go`, `TestPersistentPreRunE_Abridged_AttachTakesAbridgedPath`. Rename to `TestPersistentPreRunE_Abridged_OpenSessionTakesAbridgedPath`; set args `[]string{"open", "--session", "proj-abc123"}`; replace the `attachDeps`/`AttachDeps` injection with the Phase-2 `open --session` wiring (`openDeps`/`OpenDeps`'s session connector + session-existence seams, session `proj-abc123` present, a `mockSessionConnector` capturing the connect). Keep the assertions verbatim in spirit: `runner.calls == 0` (the full orchestrator never ran — the fast-path is command-agnostic) AND the connector fired for `proj-abc123` (the command proceeded normally). This is the load-bearing proof that `open` takes the same abridged latch-satisfied path `attach` did and no bootstrap behaviour was lost. Update the file's package doc comment (line 12) that lists `attachDeps` to name `openDeps`.
- **Migrate the version-guard non-exempt case** — `cmd/version_guard_test.go`, the `"portal attach"` row in `TestVersionGuard_InvokedForOtherNonExemptCommands` (lines 76-86). Change to `name: "portal open --session"`, `args: []string{"open", "--session", "my-session"}`, and inject `openDeps` (session `my-session` present, `mockSessionConnector`) instead of `attachDeps`/`AttachDeps`. This preserves the "the version guard runs for a non-exempt session-pinned open" assertion (proving the spawned-window exec target is version-guarded like any other command). Update the package doc comment (line 2) listing `attachDeps`.
- **Migrate/remove the tmux-missing rows** — `cmd/root_test.go`: remove the `attachCmd.Flags().Lookup("spawn-ack")` reset block in `resetRootCmd` (lines 59-62) IN THIS COMMIT (it references the deleted `attachCmd` var and would break compilation otherwise); remove the `"portal attach fails without tmux"` row in `TestTmuxDependentCommandsFailWithoutTmux` (lines 72) — the `"portal open fails without tmux"` row (line 70) already covers the tmux precheck, so the attach row is redundant, not migrated. `cmd/root_integration_test.go` (`//go:build integration`): remove the `"attach prints error to stderr and exits 1"` row (lines 52-58) — the `"open prints error to stderr and exits 1"` row (39-44) already covers it.
- **Migrate the reattach integration cases** — `cmd/reattach_integration_test.go` (`//go:build integration`). The five attach-based cases (`TestReattachIntegration_SteadyStateReattachZeroStructuralRewrites`, `_HasSessionPostBootstrapForSavedNames`, `_AttachInsideTmuxSwitchClientPath`, `_AttachOutsideTmuxAttachSessionPath`, `_UnknownNameNotFoundError`) exercise `portal attach NAME` + `attachDeps`/`AttachDeps` (connector + validator). Migrate each to `rootCmd.SetArgs([]string{"open", "--session", NAME})` with the Phase-2 `openDeps`/`OpenDeps` session wiring (connector = `mockSessionConnector`, validator = the real socket-backed `client`). Preserve every assertion: the inside-tmux switch-client vs outside exec-attach dispatch (both now via `open --session`), the steady-state saved_at invariance, the has-session-post-bootstrap contract, and the `UnknownNameNotFoundError` `No session found: nope-not-here` hard-fail with no connector dispatch (Phase 2's `--session` preserves the exact message and the never-mint/never-picker behaviour). Update the many `cmd/attach.go` doc-comment references in this file to `cmd/open.go`; update the package doc comment (lines 93-94) listing `attachDeps`.
- **Drop the stale attach argv→role case** — `internal/log/process_role.go`: change `case "open", "x", "attach":` (line 67) to `case "open", "x":`; update the doc-comment mapping (line 35, `open … / x … / attach … / bare -> tui`) to drop `attach`. `internal/log/process_role_test.go`: remove the `{"attach foo", []string{"attach", "foo"}, "tui"}` row (line 30) and the `{"attach", "foo"}` closed-space input (line 167); update the section comment (line 27) and the `cmd/attach.go` reference in the doc comment (line 72).
- **Retain `internal/spawn` untouched**: `spawn.AckWriter`, `spawn.NewServerOptionAckChannel`, `spawn.ParseSpawnAckFlag` remain (Phase 3's `open --ack` receiver consumes them). Only the `attach --spawn-ack` flag + its parse in the deleted `attach.go` go away.
- **No compat alias, no deprecation warning** for `attach` — deliberate no-back-compat posture.

**Acceptance Criteria**:
- [ ] `portal attach` is not registered on `rootCmd`; `attachCmd`, `AttachDeps`, `attachDeps`, `buildAttachDeps`, and the `--spawn-ack` flag no longer exist; no code references the deleted command.
- [ ] `SessionValidator` is relocated to `cmd/kill.go` (still consumed by `kill` and the reattach compile-assertion), not deleted; `mockSessionConnector`/`mockSessionValidator` are relocated to a surviving `_test.go` and every consumer still compiles.
- [ ] `open --session <name>` preserves attach's behaviour: session-not-found hard-fails with `No session found: <name>` (never mints, never pops the picker), inside-tmux dispatches switch-client and outside dispatches exec attach-session.
- [ ] The abridged latch-satisfied fast-path is proven command-agnostic: `open --session <name>` on a satisfied latch runs the full orchestrator zero times and still connects — the migrated regression (AC #5) demonstrates no bootstrap behaviour was lost with `attach`'s removal.
- [ ] `internal/spawn` `AckWriter`/`NewServerOptionAckChannel`/`ParseSpawnAckFlag` are retained and untouched.
- [ ] `ResolveProcessRole` no longer maps `attach` to `tui` (the `attach` case is dropped); `open`/`x`/bare still resolve to `tui`; its test is updated and green.
- [ ] No cobra alias and no deprecation warning exist for `attach`.
- [ ] `go build ./...` and `go test ./...` (unit lane) are green; the integration lane (`go test -tags integration -p 1 ./...`) is green.

**Tests**:
- (unit, `cmd/open_test.go` — migrated from the deleted attach tests) `"open --session attaches an existing session inside tmux via switch-client"` and `"...outside tmux via exec attach-session"` — inject `openDeps` session wiring; assert the connector captured the name.
- (unit) `"open --session <missing> hard-fails No session found and never connects, mints, or pops the picker"` — validator reports absent; assert the exact message and no connector/mint/picker call.
- (unit, `cmd/abridged_route_test.go`) `"open --session on a satisfied latch takes the abridged path"` — assert `runner.calls == 0` and the connector fired (command-agnostic fast-path; AC #5 regression).
- (unit, `cmd/version_guard_test.go`) `"the version guard runs for a non-exempt open --session"` — assert `versionChecker` called once.
- (unit, `internal/log/process_role_test.go`) `"ResolveProcessRole no longer maps attach; open and x still resolve to tui"`.
- (unit, `cmd/root_test.go`) the `resetRootCmd` helper compiles with the `attachCmd` flag-reset removed; the tmux-missing table no longer has an attach row.
- (integration, `//go:build integration`, `cmd/reattach_integration_test.go`, `tmuxtest` sockets — these already run under isolated per-test tmux servers) the five migrated reattach cases pass via `open --session`: steady-state zero-rewrite, has-session-post-bootstrap, inside-tmux switch-client, outside exec-attach, unknown-name not-found.
- (integration, `cmd/root_integration_test.go`, `portalbintest`-built binary) `open` errors without tmux (existing row); the attach row is removed.

**Edge Cases**:
- `SessionValidator` relocated (still consumed by `kill` + the reattach compile-assertion), not deleted.
- `mockSessionValidator`/`mockSessionConnector` relocated to a surviving `_test.go`; `mockAckWriter`/`ackWrite` deleted (attach-only).
- Abridged latch-satisfied fast-path stays command-agnostic — the migrated abridged test proves no bootstrap behaviour lost (AC #5).
- Inside-tmux switch-client vs outside exec-attach reattach cases migrate to `open --session`.
- Session-not-found hard-fail (never mints, never picker) preserved through `open --session`.
- `--spawn-ack` removed; its receipt role now lives on `open --ack` (Phase 3).
- `internal/spawn` `AckWriter`/`NewServerOptionAckChannel`/`ParseSpawnAckFlag` retained.
- Drop the stale `attach` argv→role case in `internal/log/ResolveProcessRole` + its test.
- No compat alias / no deprecation warning.
- `version_guard`/`root`/`root_integration` attach cases migrated or removed; `attachCmd` flag-reset removed from `resetRootCmd` in the same commit.
- The picker keymap "attach" action label is UI copy (a verb the user sees in the footer), not a command reference — untouched.

**Context**:
> Spec § `attach` — Retired: "`portal attach` is **deleted outright** — not aliased, not deprecated-with-warning. Every current `attach` invocation has an `open` equivalent (`open` accepts session names; the exact/no-guessing path is `open --session`). … `attach`'s two former jobs are absorbed: (1) exact/no-guessing attach for scripts → `open --session <name>`; (2) the exec target of every spawned host window → `portal open --session <name> --ack <batch>:<token>`. Both `open` and the former `attach` already call the same internal Go functions in-process … the command form existed only for cross-process callers. Nothing is lost by deleting the public command. The bootstrap fast-path is command-agnostic — `BootstrappedLatchSatisfied` is consulted once in `PersistentPreRunE` for any bootstrap-needing command (`open` included) … So `open` takes the same abridged fast-path `attach` did; there is no bootstrap reason to keep `attach`." Spec § Back-Compat & Deprecation Story: "`attach` and `spawn` are **removed** — not aliased, not deprecated-with-warning. … The `x` / `xctl` shell functions re-emit from `portal init` and keep working untouched (`x` already maps to `portal open`)."
>
> Grounding notes from the code: `SessionValidator` is declared in `cmd/attach.go` and consumed by `cmd/kill.go`'s `KillDeps` and the reattach compile-assertion, so it relocates rather than dies. The cmd package-level `spawnLogger` var lives in `cmd/spawn.go` and its only cmd-package consumer is `attach.go:64` — after this task it is a declared-but-unused package var (compiler-tolerated); Task 5-2 removes the declaration when `spawn.go` is deleted. Phase 3's `open --ack` DEBUG breadcrumb uses the `resolve` logger, not the cmd `spawnLogger`. The Phase-2 `open --session` and Phase-3 `open --ack` seams (`OpenDeps`' session connector + session-existence + `AckWriter`) are the wiring the migrated tests inject.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `attach` — Retired (incl. Spawned-window contract); § Back-Compat & Deprecation Story; § Command Surface Summary — Removed public commands.

---

## cli-verb-surface-redesign-5-2 | approved

### Task 5.2: Retire `spawn` — delete the command, `--detect`, and burst body; relocate the shared host-terminal seams

**Problem**: `portal spawn` is fully absorbed by `open`'s multi-target burst (Phase 3, which made the picker/CLI bursts exec `open --session --ack`) and `spawn --detect` is replaced by `doctor`'s host-terminal line (Phase 4). The `spawn` command is now dead surface. But `cmd/spawn.go` also houses the shared host-terminal seams (`TerminalDetector`, `productionSpawnSeams`, `buildProductionSpawnSeams`, `buildResolver`) that survivors depend on — `openTUI` and the `open` burst (`runOpenBurst`, Phase 3) and `doctor` (Phase 4) all consume them — so the file cannot simply be deleted. The shared `internal/spawn` service, the `spawn` log component, and the `@portal-spawn-*` markers are explicitly out of scope for this redesign and must stay untouched.

**Solution**: In one atomic green commit, relocate the four still-consumed host-terminal seams from `cmd/spawn.go` into a surviving non-command file (`cmd/spawn_seams.go`), delete every spawn-command-only symbol (`spawnCmd`, `SpawnDeps`, `spawnDeps`, `buildSpawnDeps`, `runSpawn`, `spawnDetector`, `unsupportedSpawnMessage`, the `logSpawn*` wrappers, the cmd `spawnLogger` var, and the `--detect` flag), then delete the now-empty `cmd/spawn.go`. Delete `cmd/spawn_test.go` outright (its burst/pre-flight/permission/logging coverage is now proven via the Phase-3 `open` burst tests and the Phase-4 `doctor` tests), relocating only `fakeTerminalDetector` (still used by `open_spawn_detect` + `doctor` tests). Split `cmd/spawn_seams_test.go` (keep `TestBuildProductionSpawnSeams`, delete `TestBuildSpawnDeps_*`). No back-compat alias, no deprecation warning.

**Outcome**: `portal spawn` and `spawn --detect` no longer exist; the host-terminal seams live in `cmd/spawn_seams.go` and still wire `openTUI`, the `open` burst, and `doctor`; `internal/spawn` (incl. `SplitNetN`, still consumed by the picker) and the `spawn` log component and `@portal-spawn-*` markers are byte-for-byte untouched; `productionSpawnSeams.Logger = log.For("spawn")` is retained; the module builds and both lanes are green.

**Do**:
- **Relocate the four shared host-terminal seams** into a new `cmd/spawn_seams.go` (package `cmd`), verbatim except doc-comment updates:
  - `TerminalDetector` interface (`cmd/spawn.go:24-26`, `Detect() spawn.Identity`) — its surviving consumer is Phase 4's `DoctorDeps.Detector`. Update its doc comment to stop referencing "the spawn command's --detect dry-run"; it is now the seam the `doctor` host-terminal line and the `open` burst detect through.
  - `productionSpawnSeams` struct (`cmd/spawn.go:275-283`).
  - `buildProductionSpawnSeams(client *tmux.Client)` (`cmd/spawn.go:292-302`) — keep `Logger: log.For("spawn")` (retained). Update its doc comment: it is now the single construction site read by `openTUI` (tuiConfig population, `cmd/open.go:569`), the `open` burst (`runOpenBurst`, Phase 3), and `doctor` (Phase 4) — no longer "the spawn CLI (buildSpawnDeps)".
  - `buildResolver()` (`cmd/spawn.go:382-388`). Update its doc comment to reference the `open` burst + `doctor` rather than "the spawn command".
  - These are all package `cmd`, so relocating them within the package keeps `openTUI`, `runOpenBurst`, and `doctor` compiling with no call-site edits.
- **Delete every spawn-command-only symbol** from `cmd/spawn.go`, then delete the emptied file: `spawnCmd` and its `init()` (the `--detect` flag registration, `SetFlagErrorFunc`, and `rootCmd.AddCommand(spawnCmd)`), `SpawnDeps`, `spawnDeps`, `buildSpawnDeps`, `runSpawn`, `spawnDetector`, `unsupportedSpawnMessage`, `logSpawnGone`/`logSpawnUnsupported`/`logSpawnPermission`/`logSpawnSummary`, and the cmd package-level `var spawnLogger = log.For("spawn")` (line 19 — its last consumer, `attach.go`, was removed in Task 5-1, so it is now safely deletable).
- **Delete `cmd/spawn_test.go` entirely** — `TestSpawnCommand`, `TestSpawnPipeline`, `TestSpawnPreflight`, `TestSpawnPartialFailure`, and all their local helpers (`spawnPipelineDeps`, `withBurster`, `manualClock`, `seqIDGen`, `wantAttachArgv`, `goneExists`, `spyDetector`, `cleanOrderConnector`, `fakeSessionConnector`, `ghosttyIdentity`, `appleTerminalIdentity`, the `spawnPipelineExe`/`spawnPipelinePATH` consts, and `isSwitchConnector`/`isAttachConnector` at lines 1326-1333 which are used only within this file). This coverage is superseded: the burst/pre-flight/permission/leave-what-opened/`portal.log`-outcome behaviour is now the `open` burst's (Phase 3, `cmd/open_test.go` + `internal/spawn` tests), and the `--detect` host-terminal identity is now `doctor`'s (Phase 4, `cmd/doctor_test.go`).
- **Relocate `fakeTerminalDetector`** (`cmd/spawn_test.go:23-29`) into a surviving test file — `cmd/spawn_seams_test.go` is the natural home (it is the host-terminal-seam test file). It still satisfies both `cmd.TerminalDetector` and `tui.TerminalDetector` (both are `Detect() spawn.Identity`), so `cmd/open_spawn_detect_test.go` (sets `cfg.detector`) and the Phase-4 `doctor` tests (set `DoctorDeps.Detector`) keep compiling.
- **Split `cmd/spawn_seams_test.go`**: keep `TestBuildProductionSpawnSeams` and its helper `isolateTerminalsFile`; delete `TestBuildSpawnDeps_PartialInjectionKeepsInjectedFillsRest` AND the now-orphaned `cmdWithClient` helper (lines 37-41 — used only by that deleted test). Prune the imports that only the deleted test needed (`context`, `io`, `log/slog`, `github.com/spf13/cobra`, `github.com/leeovery/portal/internal/spawntest`); keep those the surviving `TestBuildProductionSpawnSeams` uses (`os`, `path/filepath`, `slices`, `testing`, the `log`/`logtest`/`spawn`/`tmux` packages). Update the file's package doc comment (lines 3-6) to drop the `buildSpawnDeps` mention.
- **Remove the `spawnCmd` flag-reset** from `resetRootCmd` (`cmd/root_test.go:55-58`, the `spawnCmd.Flags().Lookup("detect")` block) IN THIS COMMIT — it references the deleted `spawnCmd` var. There are no `spawn` rows in `root_test.go`'s or `root_integration_test.go`'s tmux-missing tables, so nothing else there needs migrating.
- **Retain, untouched**: all of `internal/spawn` (the service — `Detector`, `Resolver`, `Burster`, `composeOpenArgv`, `AckChannel`, `SplitNetN` in `internal/spawn/split.go` which the picker's `dispatchBurst`/`burst_progress.go` still consume, the `spawn`-component `spawnLogger` in `internal/spawn/detect.go`, the log emitters, etc.), the `spawn` log component, and the `@portal-spawn-*` markers. The picker burst already execs `open --session --ack` (Phase 3) — it never referenced `spawnCmd`, so it needs no change; confirm its tests stay green.

**Acceptance Criteria**:
- [ ] `portal spawn` (including `--detect` and the burst body) is deleted outright: `spawnCmd`, `SpawnDeps`, `spawnDeps`, `buildSpawnDeps`, `runSpawn`, `spawnDetector`, `unsupportedSpawnMessage`, the `logSpawn*` wrappers, and the cmd `spawnLogger` var no longer exist; no code references the deleted command.
- [ ] `TerminalDetector`, `productionSpawnSeams`, `buildProductionSpawnSeams`, and `buildResolver` are relocated to `cmd/spawn_seams.go` and still consumed by `openTUI`, the `open` burst (`runOpenBurst`), and `doctor` — the build is green with no call-site edits.
- [ ] `internal/spawn` (incl. `SplitNetN`), the `spawn` log component, and the `@portal-spawn-*` markers are byte-for-byte untouched; `productionSpawnSeams.Logger = log.For("spawn")` is retained.
- [ ] `cmd/spawn_test.go` is deleted; `fakeTerminalDetector` is relocated to a surviving `_test.go`; `cmd/spawn_seams_test.go` keeps `TestBuildProductionSpawnSeams` and drops `TestBuildSpawnDeps_*` (and the orphaned `cmdWithClient`), with imports pruned.
- [ ] `spawn --detect`'s replacement (`doctor`'s host-terminal line, Phase 4) is present, so no user-facing capability is absent; the picker burst still opens windows via `open --session --ack` (Phase 3) with its tests green.
- [ ] No cobra alias and no deprecation warning exist for `spawn`.
- [ ] `go build ./...` and `go test ./...` (unit lane) are green (golangci-lint clean of unused-symbol warnings — `cmdWithClient` and the spawn-only helpers removed); the integration lane is green.

**Tests**:
- (unit, `cmd/spawn_seams_test.go`) `TestBuildProductionSpawnSeams` remains green in its relocated production home — asserts `Exists` is the client's `HasSession` probe, `Ack` is a `*spawn.ServerOptionAckChannel`, `Logger` is the `spawn`-component logger, and `Detector`/`Resolve`/`Exe`/`Getenv` are wired.
- (unit, `cmd/open_spawn_detect_test.go`) `TestBuildTUIModel_ThreadsDetectionSeams` still green — proves `openTUI`'s detection-seam wiring survives the seam relocation (uses the relocated `fakeTerminalDetector`).
- (unit, `cmd/doctor_test.go`, Phase 4) the host-terminal-line tests still green against the relocated `TerminalDetector` + `buildResolver` seams and the relocated `fakeTerminalDetector`.
- (unit) `"portal spawn is not a registered command"` — a light assertion here; the full retired-surface guard is Task 5-3.
- Build/vet gate: `go build ./...` + `go test ./...` (unit) green; `golangci-lint run` clean (no unused `cmdWithClient` / spawn-only symbols).
- (integration) the existing Phase-3 `open` burst integration + Phase-4 `doctor` integration cover the behaviour formerly proven by the deleted `spawn` integration tests; no new integration test is required by this task (deletion + relocation only).

**Edge Cases**:
- `internal/spawn` service + the `spawn` log component + `@portal-spawn-*` markers retained (out of scope), reached only via `open`'s burst.
- Relocate `TerminalDetector`/`productionSpawnSeams`/`buildProductionSpawnSeams`/`buildResolver` to a surviving non-command home (consumed by `openTUI` + `runOpenBurst` + `doctor`).
- Delete spawn-only `spawnDetector`/`SpawnDeps`/`buildSpawnDeps`/`runSpawn`/`unsupportedSpawnMessage`/`logSpawn*`/cmd `spawnLogger` var.
- `--detect`'s replacement is `doctor`'s host-terminal line (Phase 4); picker burst already execs `open --session --ack` (Phase 3), never `spawn`.
- `spawn_seams_test` split (keep `TestBuildProductionSpawnSeams`, delete `TestBuildSpawnDeps_*` + the orphaned `cmdWithClient`).
- Relocate `fakeTerminalDetector` out of the deleted `spawn_test.go` (still used by `open_spawn_detect` + `doctor` tests).
- Delete `spawn_test.go` (burst/pre-flight/permission/logging coverage now proven via the Phase-3 `open` burst + Phase-4 `doctor`).
- `productionSpawnSeams.Logger = log.For("spawn")` retained; `SplitNetN` retained (picker consumes it).
- `spawnCmd` flag-reset removed from `resetRootCmd` in the same commit; `internal/spawn` otherwise untouched; no compat alias / no deprecation warning.

**Context**:
> Spec § Host-terminal detection folded in (`--detect` retired): "`spawn --detect` (a dry-run that printed the detected host terminal's identity …) is retired with `spawn`. Its job folds into `doctor`: the picker keeps calling `Detect()` in-process; `doctor` calls the same function and prints a line …". Spec § Command Surface Summary — Removed public commands: `portal spawn [sessions…]` → `portal open <t1> <t2> …`; `portal spawn --detect` → `portal doctor`. Spec § Scope of the redesign: "Out of scope: internal package/component/marker names (`internal/spawn`, the `spawn` log component, `@portal-spawn-*` markers) — these are unaffected by the redesign." Spec § Back-Compat & Deprecation Story: "`attach` and `spawn` are **removed** — not aliased, not deprecated-with-warning."
>
> Grounding notes: Phase 3 (Task 3-5/3-6) did NOT relocate the host-terminal seams — it left them in `cmd/spawn.go` and consumed `buildProductionSpawnSeams(client)` from there in `runOpenBurst`; `openTUI` (`cmd/open.go:569`) already consumes them today, so `cmd/spawn.go` cannot be deleted without relocating them (this task does the single relocation — no double-move). Phase 4 (Task 4-4) wires `DoctorDeps.Detector TerminalDetector`, `spawn.NewDetector(client)`, and `buildResolver().Resolve` — the same seams, so they must survive for `doctor`. `internal/tui` defines its OWN `tui.TerminalDetector` (`internal/tui/spawn_detect.go`) and its OWN `spawnLogger` (`internal/tui/model.go` field / `internal/spawn/detect.go` var) — those are separate from the deleted cmd `TerminalDetector`/`spawnLogger` and are untouched. `internal/tui/burst_progress.go:448` mentions `unsupportedSpawnMessage` only in a comment (its code path uses `spawn.UnsupportedNoopMessage`), so deleting the cmd function breaks nothing.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `doctor` — Host-terminal detection folded in (`--detect` retired); § Command Surface Summary — Removed public commands; § Scope of the redesign; § Back-Compat & Deprecation Story.

---

## cli-verb-surface-redesign-5-3 | approved

### Task 5.3: Retired-surface & reachability guard

**Problem**: With `attach` (Task 5-1) and `spawn` (Task 5-2) deleted, nothing yet pins the retired surface against silent regression — a future refactor could re-register either command, add a back-compat alias, or leak them into `--help`/completion, and no test would catch it. The phase's headline acceptance ("surface shrinks to a single public session verb, behaviours preserved, no aliases") needs a standalone guard proving both the *absence* of the two verbs and the *reachability* of every behaviour `open` absorbed.

**Solution**: Add a focused regression guard (`cmd/retired_surface_test.go`, unit lane) that asserts `rootCmd` exposes no `attach`/`spawn` child and no cobra alias resolving to either, that neither appears in `--help`, bare-`portal` help, or generated completion, that no deprecation warning / back-compat alias exists for either, and that every former behaviour is reachable via `open` (the `--session` pin, the hidden `--ack` receipt flag, and multi-target positionals). The guard asserts ONLY the two verbs' absence — the `state`-hide, `hook` rename, and finalized tab-completion surface are Phase 6 and are explicitly out of scope for this assertion.

**Outcome**: A green guard test that fails loudly if `attach`/`spawn` (or an alias to either, or a deprecation shim) ever re-appears, and that documents the reachability contract — `open --session <name>`, `portal open --session <name> --ack <batch>:<token>`, and multi-target `open` — as the single-verb replacement for both retired commands. The `x`/`xctl` shell functions (mapping to `portal open`) are confirmed untouched.

**Do**:
- Add `cmd/retired_surface_test.go` (package `cmd`, unit lane, no `t.Parallel` per the cmd-package convention). Group the assertions:
  - **No child command named `attach` or `spawn`**: iterate `rootCmd.Commands()` and assert none has `Name() == "attach"` or `"spawn"`.
  - **No alias resolving to either**: iterate every command's `Aliases` slice and assert none equals `"attach"` or `"spawn"`; additionally assert `rootCmd.Find([]string{"attach"})` and `rootCmd.Find([]string{"spawn"})` do NOT resolve to a real subcommand (cobra returns the root with the token left as an unrecognised arg — assert the returned `*cobra.Command` is `rootCmd`, i.e. the verb was not matched to any child or alias). This distinguishes "deleted" from "kept behind a silent alias".
  - **Absent from help**: render the root help/usage text (execute `portal --help` or `rootCmd.UsageString()` into a buffer) and assert the Available Commands list contains neither `attach` nor `spawn` as a command entry. (Match on the command column, not a loose substring, so unrelated help prose can't produce a false pass/fail.)
  - **Absent from generated completion**: generate a completion script (e.g. `rootCmd.GenBashCompletion(buf)` / the `__complete` machinery) and assert neither `attach` nor `spawn` is offered as a root subcommand. Keep this assertion narrow — only that the two verbs are absent — because the *finalized* completion surface (session-name/alias-key candidates) is Phase 6 and must not be asserted here.
  - **No deprecation warning / no back-compat shim**: assert there is no command (hidden or visible) that would run for `attach`/`spawn` and print a deprecation notice — i.e. the `Find` result above returns `rootCmd` with the arg unmatched, and no hidden command carries `attach`/`spawn` in `Name()`/`Aliases`. Contrast (in a comment) the deliberate `hooks`→`hook` silent-alias carve-out, which is Phase 6 and does NOT apply to `attach`/`spawn`.
  - **Reachability of the absorbed behaviours**: assert `openCmd.Flags().Lookup("session")` exists (the exact/no-guess attach pin, Phase 2); assert `openCmd.Flags().Lookup("ack")` exists and is `Hidden` (the spawned-window receipt, Phase 3 — `portal open --session <name> --ack <batch>:<token>` is the spawned-window exec target); assert `openCmd` accepts multiple positional targets (the multi-window burst — assert its `Args` validator admits ≥2 positionals, e.g. it is not `cobra.MaximumNArgs(1)`). These prove the two retired verbs' jobs are reachable through `open` without re-invoking the full burst (that end-to-end behaviour is Phase 1-3's coverage).
  - **`x`/`xctl` untouched**: execute `portal init zsh` (bootstrap-exempt, no tmux) into a buffer and assert the emitted `x` shell function maps to `portal open` (references `open`, not `attach`/`spawn`) — confirming the launcher shell integration is unchanged. If an existing `init` test already asserts the `x`→`open` mapping, reference it here rather than duplicating; otherwise add the focused assertion.
- No production code change is required (Tasks 5-1/5-2 already removed the verbs); if any assertion fails, the fix belongs to 5-1/5-2, not here — this task's deliverable is the guard.

**Acceptance Criteria**:
- [ ] `rootCmd` exposes no `attach` or `spawn` child command and no cobra alias resolving to either; `Find(["attach"])`/`Find(["spawn"])` resolve to `rootCmd` with the token unmatched (deleted, not aliased).
- [ ] Neither `attach` nor `spawn` appears in `--help`, bare-`portal` help, or generated completion (asserted narrowly on the command entries, not the finalized completion candidate surface).
- [ ] No deprecation warning and no back-compat alias exist for either verb (the deliberate no-back-compat posture; the `hooks`→`hook` carve-out is Phase 6 and does not apply).
- [ ] Every former behaviour is reachable via `open`: `open --session <name>` (exact/no-guess attach), `portal open --session <name> --ack <batch>:<token>` (spawned-window exec target — `--session` present, `--ack` present + hidden), and multi-window opening via multi-target `open` (the positional args validator admits ≥2 targets).
- [ ] The `x`/`xctl` shell functions map to `portal open` and are untouched.
- [ ] The guard asserts only the two verbs' absence — `state`-hide, `hook` rename, and the finalized tab-completion surface are Phase 6 and are not asserted here.

**Tests** (all in `cmd/retired_surface_test.go`, unit lane):
- `"rootCmd has no attach or spawn child command"`.
- `"no cobra alias resolves to attach or spawn, and Find leaves the token unmatched"`.
- `"attach and spawn are absent from --help and bare portal help"`.
- `"attach and spawn are absent from generated completion"` (narrow: only the two verbs' absence).
- `"no deprecation warning or back-compat alias exists for attach or spawn"`.
- `"open exposes --session and the hidden --ack (the former attach jobs are reachable)"`.
- `"open accepts multiple positional targets (multi-window reachability)"`.
- `"portal init emits an x function mapping to portal open (unchanged)"`.

**Edge Cases**:
- `rootCmd` exposes no attach/spawn child and no cobra alias resolving to either.
- Neither appears in `--help`, generated completion, or bare `portal` help.
- No deprecation warning and no back-compat alias for either (contrast the `hooks`→`hook` carve-out, which is Phase 6, not here).
- Reachability: exact/no-guess attach via `open --session <name>`; spawned-window exec target `portal open --session <name> --ack <batch>:<token>`; multi-window via multi-target `open`.
- State-hide / hook rename / tab-completion additions are Phase 6 — this guard asserts only the two verbs' absence, not the finalized completion surface.
- `x`/`xctl` shell functions (map to `portal open`) untouched and still work.

**Context**:
> Spec § `attach` — Retired and § Command Surface Summary — Removed public commands: `portal attach <session>` → `portal open --session <name>` (or bare `open <name>`); `portal spawn [sessions…]` → `portal open <t1> <t2> …`; `portal spawn --detect` → `portal doctor`. Spec § Back-Compat & Deprecation Story: "There is no back-compat story — deliberately. … `attach` and `spawn` are **removed** — not aliased, not deprecated-with-warning. … The `x` / `xctl` shell functions re-emit from `portal init` and keep working untouched (`x` already maps to `portal open`). No alias lifecycle exists because no compat aliases exist. One deliberate exception: `hooks` → `hook` keeps `hooks` as a permanent, silent cobra alias" — a Phase-6 carve-out that does NOT apply to `attach`/`spawn`. Spec § Command Surface Summary — Hidden: `portal open --ack <batch>:<token>` invoked by spawned host windows.
>
> This guard is the phase's acceptance proof in isolation (per the phase rationale: "the deletion's acceptance — surface shrinks, behaviours preserved, no aliases — is verified in isolation"). The `state`-namespace hiding, the `hook` rename (+ silent `hooks` alias), and the tab-completion additions land in Phase 6; this guard deliberately scopes itself to the two retired verbs so it does not couple to the not-yet-final completion surface.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `attach` — Retired; § Back-Compat & Deprecation Story; § Command Surface Summary (Removed public commands; Hidden); § Bare `portal` (no subcommand).
