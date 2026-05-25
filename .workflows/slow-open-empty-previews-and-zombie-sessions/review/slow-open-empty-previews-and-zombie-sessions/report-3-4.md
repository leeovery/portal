TASK: 3-4 — Compose unhealthy-saver recreate path with new ordering

STATUS: Complete

SPEC CONTEXT: Component F mandates the four-step recreate ordering (kill → new-session(placeholder) → set destroy-unattached=off → respawn-pane(daemon)). Unhealthy-saver branch falls through to new ordering. No persistent placeholder leak across single recovery bootstrap.

IMPLEMENTATION:
- Status: Implemented (verification + coverage; no new production code required — composition was already correct after 3-2/3-3)
- Location:
  - `internal/tmux/portal_saver.go:553-592` — unhealthy branch (lines 556-562) calls `saver.Ops.KillAndWait` then sets `sessionPresent = false`, falling cleanly into create branch (565-570) → SetSessionOption (572) → RespawnPane daemon (577) → readiness (588)
  - `internal/tmux/portal_saver.go:632-651` — `EnsurePortalSaverVersion` signature unchanged; delegates via `saver.Ops.KillAndWait` then `BootstrapPortalSaver(c, stateDir)` (650), automatically inheriting new ordering

TESTS:
- Status: Adequate
- Coverage at `internal/tmux/portal_saver_test.go`:
  - Line 3154 `TestBootstrapPortalSaver_RecyclesPlaceholderOnlySaverViaNewOrdering` — pins kill < new < set < respawn ordering via `assertKillNewSetRespawnOrdering` (3105); argv literal pins for both new-session placeholder and respawn-pane -k daemon; counts each call exactly once
  - Line 3229 `TestEnsurePortalSaverVersion_AliveMismatch_FlowsThroughNewBootstrapOrdering`
  - Line 3287 `TestEnsurePortalSaverVersion_NotAlive_SkipsKillAndStillUsesNewOrdering`
  - Line 3371 `TestBootstrapPortalSaver_NoPersistentPlaceholderLeakAcrossSingleRecovery` — scans full argv stream and asserts LAST pane-mutating call is respawn-pane with daemon command, not new-session with placeholder
- `assertKillNewSetRespawnOrdering` (3105) is reusable source-order index scan
- Package-level `init()` (line 54) shrinks readiness defaults to 1ms/5ms so create-branch tests don't pay production 2s timeout

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel`; DI via package-level seam vars restored via `t.Cleanup`
- SOLID: Good; single-purpose helper; each test isolates one invariant
- Complexity: Low
- Modern idioms: `t.Helper()`, `t.Cleanup`, `t.Fatalf` for setup vs `t.Errorf` for assertions
- Readability: Good; doc-comments cite spec + plan task cross-references

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] `TestBootstrapPortalSaver_RecyclesPlaceholderOnlySaverViaNewOrdering` does not explicitly pin kill-session argv target `-t _portal-saver` (counts call, asserts ordering). Tighten to literal argv pin
- [idea] Four-test cluster repeats `stubAliveCheck`/`shrinkRetryDelay`/`stubReadinessReady` setup; could DRY with `setupTask34(t, alive bool)`
