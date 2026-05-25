TASK: 5-5 — Integration test self-eject when _portal-saver absent

STATUS: Complete

SPEC CONTEXT: Component D bullet 1 "Self-eject on absent saver". `osExit(0)` directly bypasses `daemonShutdownFunc`/`defaultShutdownFlush`. Stale-stays-stale pidfile, zero scrollback mutation.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/state_daemon_self_supervision_integration_test.go:109-370` `TestSelfEject_PortalSaverAbsent_ExitsCleanly`
- File also colocates 5-6/5-7/5-8

TESTS:
- Status: Adequate
- All four sub-assertions explicit (exit 0, no panic, log marker, stale daemon.pid)
- Load-bearing 2s lower floor on exit latency (364-369) guards against counter incrementing faster than ticker
- Diagnostic dump (portal.log + stderr) on every fatal path
- `PORTAL_LOG_LEVEL=INFO` set explicitly (172-179)
- Pre-state assertions on both daemon.pid AND daemon.lock absence
- `daemon.Wait` on goroutine + deadline-bound select
- `t.Cleanup` SIGKILL guard

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel`; `IsolateStateForTest` per CLAUDE.md
- SOLID: Good; constants named (`selfEjectExitBudget`, `selfEjectLogMarker`, `selfEjectExitPollTick`)
- Complexity: Low
- Modern idioms: `errors.Is(err, os.ErrNotExist)`
- Readability: Excellent; file-header explains choreography and assertion taxonomy

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [quickfix] `selfEjectExitPollTick` constant comment (95-97) says "Not used directly" but sibling 5-6 consumes it at line 517; update or drop `var _` guard at 639
- [idea] Exit budget arithmetic hard-codes N=3 and TickerPeriod=1s; if either moves, budget silently desyncs; 5-8 handles via `legitimateColdStartHysteresisMirror`
- [idea] One sentence on constant comment noting "not consumed by legitimate-cold-start test" would aid cross-referencing
