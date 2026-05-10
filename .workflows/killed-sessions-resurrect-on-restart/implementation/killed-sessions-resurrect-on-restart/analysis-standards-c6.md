# Standards Findings — killed-sessions-resurrect-on-restart (cycle 6)

```
AGENT: standards
FINDINGS: none

SUMMARY: Cycle 5's two cleanup tasks both landed cleanly:

- T8-1 — shellQuoteSingle deleted; buildHydrateCommand emits the bare form per cycle 5's architecture recommendation (Option 1):
  - internal/restore/session.go:430-435 returns fmt.Sprintf("portal state hydrate --fifo %s --file %s --hook-key %s", fifo, file, hookKey) with no escape helper.
  - The `strings` import was dropped from session.go since the helper was its only consumer.
  - The docstring at session.go:423-429 explicitly documents the contract: literal `'` in inputs would break parsing, but Portal's sanitization (sanitizeSessionName at internal/state/panekey.go:41-58 filters `/`, `\`, `\0`) does not currently produce such inputs.
  - The white-box snapshot test internal/restore/session_build_hydrate_test.go was rewritten to assert the bare form and added a defect-D regression guard (no `sh -c`, no `exec $SHELL`).
  - The integration-style snapshot at internal/restore/session_test.go:568-594 (TestSessionRestorer_HydrateCommandFormat) tracks the same shape.

- T8-2 — buildReattachOrchestrator migrated to NewRestoreAdapter and shared OpenTestLogger:
  - cmd/reattach_integration_test.go:164-174 now uses restoretest.OpenTestLogger(t, stateDir) and bootstrapadapter.NewRestoreAdapter(client, stateDir, logger), eliminating the open-coded restore.Orchestrator two-step preamble and the OpenLogger + Cleanup boilerplate.
  - The "github.com/leeovery/portal/internal/restore" import was dropped from cmd/reattach_integration_test.go.
  - cmd/bootstrap/orchestrator_builder_test.go:118-120 now delegates to restoretest.OpenTestLogger so the 12 cmd/bootstrap call sites continue to work via the in-package one-line wrapper.
  - internal/restoretest/logger.go was added (untagged, exported helper) with companion logger_test.go pinning the <stateDir>/portal.log path convention.

Spec conformance verification (re-confirmed against cycle-5 baseline):
- Fix 1 (Bootstrap eager-signaling step): cmd/bootstrap/bootstrap.go:289-300 EagerSignalHydrate runs between Restore (step 5) and Clear @portal-restoring (step 7). Soft Warn-and-swallow error posture per spec § Failure Posture.
- Fix 2 (Timeout-path corrections): cmd/state_hydrate.go:260-277 handleHydrateTimeout calls unsetSkeletonMarkerOrLog (spec § Fix 2 → Specific Changes → 1); runHydrate's timeout branch (lines 102-117) routes the fall-through through execShellOrHookAndExit symmetrically with file-missing (spec § Fix 2 → Specific Changes → 2). The 100ms settle-sleep is preserved at line 111.
- Fix 3 (Wrapper drop): internal/restore/session.go:430-435 buildHydrateCommand emits the bare `portal state hydrate --fifo X --file Y --hook-key Z` form with no sh -c envelope per spec § Fix 3 → Behaviour. The spec's "shell-escaped using the existing internal/tmux quoting helper" phrasing presumed a helper that never existed; cycle 5's architecture analysis correctly classified this as a deliberate plan supersession (Option 1: delete; pathological-input scope explicitly documented in the docstring).
- CLAUDE.md "Server bootstrap" at lines 69-83 accurately enumerates the ten-step list with EagerSignalHydrate at step 6 per spec § Bootstrap Step Numbering Update.

Project-convention compliance:
- golang-pro MUST DO checks pass: errors are wrapped/handled explicitly at the changed sites, no naked returns introduced, exported new helper (OpenTestLogger) is documented, table-driven and subtest patterns preserved.
- code-quality DRY / Compose-Don't-Duplicate principles applied correctly — cycle 6 collapsed two concrete duplications (the shellQuoteSingle latent-bug helper and the OpenLogger + Cleanup preamble) rather than introducing premature abstraction.

No new or remaining spec or project-convention drift introduced after cycle 5's cleanup landed.
```

STATUS: clean
