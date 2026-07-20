TASK: cli-verb-surface-redesign-5-2 — Retire `spawn` — delete the command, `--detect`, and burst body; relocate the shared host-terminal seams

ACCEPTANCE CRITERIA (from Phase 5 row 5-2 + Phase 5 phase-level ACs):
- `portal spawn` (incl. `--detect` + burst body) deleted outright; no back-compat alias, no deprecation warning.
- `internal/spawn` service + `spawn` log component + `@portal-spawn-*` markers retained (out of scope), reached only via `open`'s burst.
- Relocate `TerminalDetector`/`productionSpawnSeams`/`buildProductionSpawnSeams`/`buildResolver` to a surviving non-command home (consumed by `openTUI` + `doctor`).
- Delete spawn-only `spawnDetector`/`SpawnDeps`/`buildSpawnDeps`/`runSpawn`/`unsupportedSpawnMessage`/`logSpawn*`/cmd `spawnLogger` var.
- `--detect`'s replacement is `doctor`'s host-terminal line (Phase 4).
- `spawn_seams_test` split (keep `TestBuildProductionSpawnSeams`, delete `TestBuildSpawnDeps_*`); relocate `fakeTerminalDetector` out of the deleted `spawn_test.go`; delete `spawn_test.go`.
- `productionSpawnSeams.Logger = log.For("spawn")` retained; `SplitNetN` retained (picker consumes it).
- root/root_integration spawn cases migrated or removed + `spawnCmd` flag-reset removed.

STATUS: Complete

SPEC CONTEXT: Spec §"Command Surface Summary" (Removed public commands) maps `portal spawn` → `portal open <t1> <t2> …` (multi-target) and `portal spawn --detect` → `portal doctor` host-terminal line. §"Back-Compat & Deprecation Story": `attach`/`spawn` removed — not aliased, not deprecated. §"Scope of the redesign" explicitly holds internal package/component/marker names (`internal/spawn`, `spawn` log component, `@portal-spawn-*`) OUT of scope — they survive untouched. §"Host-terminal detection folded in (`--detect` retired)": the picker keeps calling `Detect()`; `doctor` calls the same function.

IMPLEMENTATION:
- Status: Implemented (correct end-state; one benign, justified deviation from the plan-row's literal deletion list — see Notes).
- Location:
  - cmd/spawn.go — deleted (absent from tree).
  - cmd/spawn_test.go — deleted (absent from tree).
  - cmd/spawn_seams.go — surviving home for the shared seams: `TerminalDetector` (iface), `productionSpawnSeams` (struct), `buildProductionSpawnSeams` (line 51), `buildResolver` (line 80), `spawnDetector` (line 66), `spawnLogger` var (line 16).
  - Consumers: picker via open.go:859-901 (`spawnSeams.Detector/Resolve/Exists/Ack/Exe/Getenv/Logger`); open burst via cmd/open_burst_run.go:70-122 (`buildOpenBurstDeps` defaults from the same bundle; Detector default routes through `spawnDetector(cmd)`); doctor via cmd/doctor.go:145-156 (`resolveDoctorDeps` independently reconstructs `spawn.NewDetector(client)` + `buildResolver().Resolve` closure per Task 11-1 — deliberate, documented).
  - `--ack` marker write (open.go:372-387 `writeAckMarker`) consumes `spawnLogger` for its best-effort DEBUG line; `spawn.UnsupportedNoopMessage` replaces the deleted `unsupportedSpawnMessage` (open_burst_run.go:168).
  - internal/spawn/ — fully retained (ack/ackid/burst/detect/logemit/split/… all present); `spawn` component bound at internal/spawn/detect.go:21; `@portal-spawn-*` markers in internal/spawn/ack*.go; `SplitNetN` retained at internal/spawn/split.go, sole consumer internal/tui/burst_progress.go:491.
  - Deleted spawn-only symbols confirmed gone: `runSpawn`, `SpawnDeps`, `buildSpawnDeps`, `unsupportedSpawnMessage` — no definitions or uses remain (grep clean). No cmd-layer `logSpawn*` remain.
  - No `spawnCmd` reference anywhere (grep clean); cmd/root_test.go resetRootCmd (line 22-79) resets only open's flags (exec/filter/session/path/alias/zoxide/ack) — no spawn/attach flag-reset. cmd/root_integration_test.go has zero spawn/attach references.
  - No compat alias: cmd/retired_surface_test.go proves no child, no cobra alias, absent from help + generated completion; the sole sanctioned alias is `hooks`→`hook` (Phase 6, hookCmd.Aliases).
- Notes: The plan-row lists "delete spawn-only `spawnDetector`" and "delete cmd `spawnLogger` var", but both are RETAINED — correctly. They are no longer spawn-only: `spawnDetector` is the open burst's Detector default (open_burst_run.go:89) and `spawnLogger` backs the `--ack` DEBUG line (open.go:382). Deleting them would break surviving functionality; both retained symbols are genuinely consumed (not orphaned). This is a plan-text-vs-implementation drift where the implementation made the right call, not a defect.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/spawn_seams_test.go — `TestBuildProductionSpawnSeams` kept (verifies Exists→HasSession, Ack type, Logger component=="spawn", Detector/Resolve/Exe/Getenv wired); `TestBuildSpawnDeps_*` deleted as specified. Houses relocated `fakeTerminalDetector` + `cmdWithClient`.
  - cmd/open_burst_seams_test.go — `TestBuildOpenBurstDeps_*` verifies the burst DI defaulting, incl. Detector default via `spawnDetector` and shared-bundle fills.
  - cmd/open_spawn_detect_test.go — picker detection-seam wiring, consumes relocated `fakeTerminalDetector`.
  - cmd/doctor_test.go — `TestDoctorHostTerminalLine` + `TestDoctorHostTerminalNeverDrivesExit` cover the `--detect` replacement (supported/unsupported/remote, exit-code independence), consuming `fakeTerminalDetector`.
  - cmd/retired_surface_test.go (Task 5-3 guard) + cmd/root_test.go `TestSpawnCommandIsRetired` — spawn/attach absence across registration, aliases, help, completion; behaviours reachable via `open`.
  - The former spawn_test.go burst/pre-flight/permission/logging coverage now lives in cmd/open_burst_run_test.go, cmd/open_multitarget_test.go, and internal/spawn/{burst,logemit,preflight,classify,split}_test.go.
  - `fakeTerminalDetector` has exactly one definition (spawn_seams_test.go:29), consumed by open_spawn_detect / open_burst_run / doctor tests — no duplicate.
- Notes: No under- or over-testing observed. Tests assert behaviour (component attr, seam wiring, reachability), not implementation internals.

CODE QUALITY:
- Project conventions: Followed — package-level `*Deps` DI seams with production defaulting (buildOpenBurstDeps), small interfaces (`TerminalDetector`), no t.Parallel in cmd tests, spawn-component logger bound once. Consistent with .claude/skills golang-dependency-injection / naming / testing.
- SOLID principles: Good — single shared construction site (`buildProductionSpawnSeams`) prevents picker/burst wiring drift; `TerminalDetector` is a 1-method seam.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good — the relocated seams and deliberate doctor-independence are heavily and accurately commented (post Task 11-1).
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/spawn_seams.go:59 — `buildProductionSpawnSeams` calls `log.For("spawn")` a second time for the bundle's `Logger` while the package-level `var spawnLogger = log.For("spawn")` (line 16) already binds it; reference `spawnLogger` for the field to single-source the cmd-layer spawn-component binding (same cached logger, zero behaviour change).
