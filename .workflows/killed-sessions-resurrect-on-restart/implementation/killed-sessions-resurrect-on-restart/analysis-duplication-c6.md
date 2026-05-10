# Duplication Findings — killed-sessions-resurrect-on-restart (cycle 6)

```
AGENT: duplication
FINDINGS:
- FINDING: Five integration-test sites in phase5_integration_test.go and reboot_roundtrip_test.go still open-code the `restoreInner := &restore.Orchestrator{Client, StateDir, Logger}` + `&bootstrapadapter.RestoreAdapter{Inner: restoreInner}` two-step preamble that `bootstrapadapter.NewRestoreAdapter` exists to collapse
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/cmd/bootstrap/phase5_integration_test.go:167-176, /Users/leeovery/Code/portal/cmd/bootstrap/phase5_integration_test.go:261-270, /Users/leeovery/Code/portal/cmd/bootstrap/reboot_roundtrip_test.go:326-334, /Users/leeovery/Code/portal/cmd/bootstrap/reboot_roundtrip_test.go:937-945, /Users/leeovery/Code/portal/cmd/bootstrap/reboot_roundtrip_test.go:1189-1203, /Users/leeovery/Code/portal/internal/bootstrapadapter/adapters.go:107-115
  DESCRIPTION: T6-2 (cycle 3 cleanup) introduced `bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)` precisely to collapse this two-step preamble into a single call, and adopted it at the four cycle-3-scope integration sites. T8-2 (cycle 5 cleanup) extended adoption to cmd/reattach_integration_test.go on the rationale "this site is *not* a pre-existing site untouched by the work unit — T4-3 actively rewrote the surrounding orchestrator-construction code." Phase5_integration_test.go and reboot_roundtrip_test.go meet the same "actively rewritten by this work unit" criterion: T1-4 (Orchestrator.Run gained the EagerSignalHydrate step), T4-2 (integration builder flipped EagerSignaler default to real), T4-4 (SeedSessionsJSON helper consumed), and T7-1 (poll-helper migration to restoretest) all touched both files. The constructor's docstring at adapters.go:97-102 names "cmd/bootstrap_production.go" as the only deliberate non-migrated open-coded site; phase5/reboot are not on that exemption list and their surrounding orchestratorOpts literal already mixes constructor-style adapters (e.g. line 945 wires `&bootstrapadapter.FIFOSweeper{...}` inline alongside `&bootstrapadapter.RestoreAdapter{Inner: restoreInner}`).
    Example collapse for phase5_integration_test.go:167-176 (10 lines → 4 lines):
        Before:
            logger := openTestLogger(t, stateDir)
            restoreInner := &restore.Orchestrator{
                Client:   client,
                StateDir: stateDir,
                Logger:   logger,
            }
            o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
                Restore: &bootstrapadapter.RestoreAdapter{Inner: restoreInner},
                ...
            })
        After:
            logger := openTestLogger(t, stateDir)
            o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
                Restore: bootstrapadapter.NewRestoreAdapter(client, stateDir, logger),
                ...
            })
  Net deletion: ~6 lines per site × 5 sites = ~30 lines, plus the `"github.com/leeovery/portal/internal/restore"` import drops from both files. orchestrator_builder_eager_default_test.go also references `&restore.Orchestrator{}` (lines 50, 88) but as deliberately empty zero-value sentinels for type-assertion-only tests — not duplication candidates.
  RECOMMENDATION: At each of the five sites listed above, replace the `restoreInner := &restore.Orchestrator{...}` + `Restore: &bootstrapadapter.RestoreAdapter{Inner: restoreInner}` pair with `Restore: bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)`. Drop the `"github.com/leeovery/portal/internal/restore"` import from both files. Production wiring at cmd/bootstrap_production.go remains exempt per the constructor's docstring rationale (inline-struct-adapter parity at that site).

- FINDING: `TestSignalHydrate_OpenFIFOForSignalUsesNonBlockingFlags` (cmd-package) and `TestOpenFIFOForSignal_NonBlockingFlags` (state-package) test the same function with byte-equivalent assertions
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/cmd/state_signal_hydrate_test.go:316-350, /Users/leeovery/Code/portal/internal/state/signal_hydrate_test.go:244-274
  DESCRIPTION: Both tests target the same production function `state.OpenFIFOForSignal` and run byte-equivalent assertions: (a) skip on Windows via `runtime.GOOS == "windows"`, (b) `syscall.Mkfifo(path, 0o600)` at a fresh temp path, (c) call `state.OpenFIFOForSignal(path)` against a no-reader FIFO, (d) record elapsed via `time.Since(start)`, (e) assert non-nil file is unexpected and `errors.Is(err, syscall.ENXIO)`, (f) assert `elapsed < 100*time.Millisecond` to prove `O_NONBLOCK` is set. The bodies differ only in (i) the failure-message prefix and (ii) `path` construction (`dir + "/no-reader.fifo"` vs `filepath.Join(dir, "no-reader.fifo")`). Both files were heavily rewritten by this work unit: `internal/state/signal_hydrate_test.go` is the canonical home for the retry-ladder + open-flag contract (T1-1 relocated `writeFIFOSignal` here; T4-1 added the `FIFOSignaler` seam coverage), and `cmd/state_signal_hydrate_test.go` was rewritten by T1-2 to repoint at the shared writer. The cmd-package test's docstring claims it "validates that state.OpenFIFOForSignal — the production opener bundled inside state.SendHydrateSignal / state.DefaultFIFOSignaler — opens a real FIFO with no reader and returns ENXIO immediately" — i.e. it explicitly tests the same primitive the state-package test owns, not a cmd-layer concern. The cmd-package file already has a separate end-to-end signal-hydrate-via-cobra-Execute test (`TestStateSignalHydrate_AcceptsLeadingDashSessionViaCobraExecute`, line 405) which is the legitimate cmd-layer integration; the OpenFIFOForSignal test is a redundant copy of internal/state's primitive coverage. Same kind of cross-package duplicate-coverage cycle 2 addressed for `RecordingFIFOSignaler` (where the same fake had been declared in both consumer test packages); here it is the test itself rather than the fake.
  RECOMMENDATION: Delete `TestSignalHydrate_OpenFIFOForSignalUsesNonBlockingFlags` (lines 316-350 of cmd/state_signal_hydrate_test.go). The canonical `TestOpenFIFOForSignal_NonBlockingFlags` in internal/state/signal_hydrate_test.go is byte-equivalent and lives next to the function under test. Net deletion: ~35 lines plus the now-unused `"runtime"` and `"syscall"` imports from cmd/state_signal_hydrate_test.go (verify no other test in the same file consumes them).

SUMMARY: Cycle 5's two cleanup tasks landed correctly: shellQuoteSingle is gone with the simplified bare-form buildHydrateCommand, and buildReattachOrchestrator now uses NewRestoreAdapter + restoretest.OpenTestLogger. Two new low-severity duplication candidates remain in cycle 6 scope: (a) five sites in phase5_integration_test.go (2) and reboot_roundtrip_test.go (3) still open-code the restore.Orchestrator + RestoreAdapter two-step preamble that NewRestoreAdapter exists to collapse — same rationale that motivated cycle 5's reattach migration applies (files actively rewritten by T1-4 / T4-2 / T4-4 / T7-1, in scope for cycle 6); (b) TestSignalHydrate_OpenFIFOForSignalUsesNonBlockingFlags (cmd-package) is a byte-equivalent copy of TestOpenFIFOForSignal_NonBlockingFlags (state-package) — deleting the cmd-package copy preserves coverage at the state-package home for the function under test. Both are mechanical extract/delete-and-reuse cleanups; do not block correctness.
```
