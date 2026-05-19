---
topic: saver-kill-respawn-loop-leaks-daemons
cycle: 4
total_proposed: 1
---
# Analysis Tasks: saver-kill-respawn-loop-leaks-daemons (Cycle 4)

## Task 1: Migrate five inline daemonRunFunc holders to the existing withImmediateRun helper
status: approved
severity: low
sources: duplication

**Problem**: The helper `withImmediateRun(t) **daemonDeps` already exists at `/Users/leeovery/Code/portal/cmd/state_daemon_test.go:35-45` and is used at ~14 sites. Five other call sites bypass it and inline the same 6-line block verbatim:
- `/Users/leeovery/Code/portal/cmd/state_daemon_run_test.go:747-753` (TestDaemonStartup_SeedsHashMapFromDisk)
- `/Users/leeovery/Code/portal/cmd/state_daemon_run_test.go:795-801` (TestDaemonStartup_LoadsPrevIndexFromSessionsJSON)
- `/Users/leeovery/Code/portal/cmd/state_daemon_run_test.go:823-829` (TestDaemonStartup_HandlesMissingSessionsJSONAsNilPrev)
- `/Users/leeovery/Code/portal/cmd/state_daemon_run_test.go:862-868` (TestDaemonStartup_LogsWarningOnUndecodableSessionsJSON)
- `/Users/leeovery/Code/portal/cmd/state_daemon_test.go:207-213` (TestStateDaemon_PassesPreparedDepsToRunFunc)

Aggregate ~30 LOC of pure duplication; the helper and the call sites share the `cmd` package.

**Solution**: Replace each inline block with `holder := withImmediateRun(t)`.

**Outcome**: Five tests collapse from 6 setup lines each to 1. No behaviour change.

**Do**:
1. At each of the five listed sites, delete the 6-line `holder := new(*daemonDeps); prev := daemonRunFunc; daemonRunFunc = func(_ context.Context, deps *daemonDeps) error { *holder = deps; return nil }; t.Cleanup(func() { daemonRunFunc = prev })` block and replace with `holder := withImmediateRun(t)`.
2. Run `go test ./cmd/...` and confirm same number of tests pass.

**Acceptance Criteria**:
- Zero remaining inline `daemonRunFunc = func(_ context.Context, deps *daemonDeps) error { *holder = deps; ...` blocks at the five listed sites.
- `withImmediateRun(t)` is called at each of those sites.
- `go test ./cmd/...` passes with the same case count.
- go vet/gofmt/golangci-lint clean on touched files.

**Tests**:
- No new tests. Existing daemon-startup and pass-deps tests are the regression net.
