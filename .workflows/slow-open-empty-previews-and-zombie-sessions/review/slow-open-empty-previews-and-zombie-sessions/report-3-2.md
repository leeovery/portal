TASK: 3-2 — Reorder BootstrapPortalSaver to create-placeholder, set-option, respawn-daemon

STATUS: Complete

SPEC CONTEXT: Component F decouples saver-session creation from daemon launch. Three-step ordering: create with placeholder; set option; respawn-pane -k with real daemon.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/portal_saver.go:553-592` — `BootstrapPortalSaver` create branch executes create → set-option → respawn-pane gated by `createdSession`
  - `internal/tmux/portal_saver.go:708-728` — `createPortalSaverWithRetry` passes `portalSaverPlaceholderCommand`
  - Line 572-574: SetSessionOption error wrapped `"bootstrap _portal-saver: set destroy-unattached: %w"`
  - Line 577-579: RespawnPane error wrapped `"bootstrap _portal-saver: respawn daemon: %w"`
  - Line 588: readiness barrier seam (3-3 extension point)
  - `RespawnPane` signature `(target, command string) error` unchanged

TESTS:
- Status: Adequate
- Location: `internal/tmux/portal_saver_test.go`
- `TestBootstrapPortalSaver_CreateOrderingIsCreateThenSetOptionThenRespawn` (line 281), argv-recorder ordering
- `TestCreatePortalSaverWithRetry_UsesPlaceholderCommand` (367)
- `TestBootstrapPortalSaver_PropagatesRespawnPaneFailureWithRespawnDaemonContext` (332)
- `TestBootstrapPortalSaver_PropagatesSetOptionFailureWithSessionAndOptionName` (788)
- `TestBootstrapPortalSaver_NoOpWhenSessionExistsAndDaemonAlive` (466) — happy path 0 new/kill, 1 set-option
- `TestBootstrapPortalSaver_ConcurrentRaceTreatsExistingSessionAsSuccess_AndStillRespawns` (415)
- `TestBootstrapPortalSaver_RecoversFromFlockLoserEmptySession/DeadPaneSession` (532, 572)
- Not over-tested

CODE QUALITY:
- Project conventions: Followed; package-level seam pattern; swapSeam + t.Cleanup
- SOLID: Good; single responsibility; kill-and-wait and readiness extracted behind seams
- Complexity: Low; linear with two early exits + createdSession gate
- Modern idioms: `fmt.Errorf("...: %w", err)`; tolerant `_ =` on best-effort
- Readability: Good

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] `TestBootstrapPortalSaver_NoOpWhenSessionExistsAndDaemonAlive` relies on nil `respawnPane` handler to fail if respawn fires; an explicit `countCalls("respawn-pane") == 0` would be tighter
- [idea] `createdSession` gate comment explains *that* respawn is create-branch-only but not *why* — fold spec rationale into inline comment to deter future "simplification"
