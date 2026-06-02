TASK: Align restoretest.OpenTestLogger with the production sink's portal.log file contract (portal-observability-layer-11-2)

ACCEPTANCE CRITERIA:
- OpenTestLogger no longer writes a bare regular-file portal.log the production migration guard would unlink; it produces the production portal.log.<date> + symlink shape or routes through internal/log's handler.
- *testing.T-first signature and *slog.Logger return preserved; existing call sites compile unchanged.
- close-on-cleanup via t.Cleanup preserved.
- Doc comment accurately describes the portal.log contract.
- No production runtime behavior changed; only the test-infra logger modified.

STATUS: Complete

SPEC CONTEXT:
internal/log rotating sink owns portal.log as a SYMLINK → dated portal.log.<date>. On first write reopen runs migrationGuard which os.Remove()s any regular-file portal.log (frees the name for a symlink); no-ops once portal.log is a symlink. Day file 0o600 O_APPEND, symlink target the bare relative dated basename. The old OpenTestLogger wrote a bare regular file a co-resident binary's sink would delete — a latent trap.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/restoretest/logger.go:50-89 (OpenTestLogger + swingPortalLogSymlink); doc 27-49; consts 12-25. Call sites: restore/exit_closes_pane_integration_test.go:375, restore/integration_test.go:121/223/320.
- Notes: Opens dated day file <stateDir>/portal.log.<date> (O_APPEND|O_CREATE|O_WRONLY, 0o600) then atomically establishes portal.log as a SYMLINK to the bare relative basename via swingPortalLogSymlink (os.Symlink to temp + os.Rename) — byte-identical to production swingSymlink. Documented fallback path (mirror the shape) correctly chosen over route-through-internal/log: the preferred path is not cleanly achievable — Init installs a PROCESS-WIDE handler + emits process: start/exit markers; newRotatingSink/newTextHandler unexported; a standalone stateDir-scoped logger would need new exported API (scope creep, violates AC5) or hijacking the global handler (breaks log.SetTestHandler). Signature/return preserved; four call sites compile unchanged. t.Cleanup close preserved (:60). No production code touched; no import cycle (restoretest doesn't import internal/log; local portalLogName/portalLogDateLayout constants mirror production to avoid the import per established convention). dayName computed once and reused for both file and target — can't diverge; layout "2006-01-02" matches production dateLayout.

TESTS:
- Status: Adequate
- Location: internal/restoretest/logger_test.go
- Coverage: TestOpenTestLogger_WritesToPortalLog (logs through the symlink, asserts message + key=value attr + INFO level on disk); TestOpenTestLogger_ProducesProductionSinkShape (direct AC test — portal.log is a SYMLINK via Lstat/ModeSymlink not a regular file, target == bare relative portal.log.<date> basename, dated day file exists as regular file).
- Notes: Would fail if broken (revert to regular file → ModeSymlink assertion; wrong/absolute target → readlink equality; discarded writer → content). Two distinct properties (content-on-disk vs on-disk-shape). The destructive-contention scenario is exercised end-to-end by exit_closes_pane_integration_test.go (real binary + OpenTestLogger sharing stateDir) but that asserts tmux state, not log content.

CODE QUALITY:
- Project conventions: Followed (local-constant mirroring w/ no-import-cycle rationale; t.Helper/t.Fatalf/t.Cleanup; test-only in restoretest).
- SOLID: Good — swingPortalLogSymlink single-responsibility atomic-swing helper mirroring production swingSymlink.
- Complexity: Low.
- Modern idioms: Yes (%w wrapping, os.IsNotExist stale-temp reclaim, slog.NewTextHandler).
- Readability: Good — doc explains the load-bearing why (migration guard, O_APPEND co-existence).
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/restoretest/logger.go duplicates internal/log's portal.log naming contract (portalLogName, "2006-01-02", bare-relative target, temp-then-rename swing); comments require lockstep but nothing enforces it — if production changes dateLayout or target shape, this test infra drifts silently. Consider a small shared leaf package (analogous to tmuxout/tmuxerr) exposing the day-file/symlink-target name builders, consumed by both internal/log and restoretest, for a single source of truth. Requires a design decision (new package vs export from internal/log).
- [idea] The destructive-contention regression is only structurally prevented, not asserted by a dedicated test. A focused test opening OpenTestLogger against a stateDir, triggering a production reopen/migrationGuard against the same dir, and asserting both writers' records survive would lock in the exact property. Optional given the shape unit test.
