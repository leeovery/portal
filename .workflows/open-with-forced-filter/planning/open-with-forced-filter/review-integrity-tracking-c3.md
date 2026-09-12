# Review Tracking: Open With Forced Filter - Integrity

## Findings

### 1. The classification flip silently empties a probe-free guard of its subject

**Severity**: Minor
**Plan Reference**: Phase 4, task 4-4 (`open-with-forced-filter-4-4`) — Do
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Move**: settled
**Change Type**: add-to-task

**Problem**:
The guard that keeps a plain `x ~/Code/app` from paying a tmux round-trip before anything is decided stops covering that case the moment this task lands, and stays green while it happens. `TestShouldRunConcurrentBootstrap_IssuesNoProbe` drives three rows through the decider — a non-`open` command, a direct-path `open`, and a bare picker `open` — and asserts none of them issues a single tmux call. Its direct-path row is the argument `/dir`, which the widened predicate now reads as a search form, so that row turns into a second copy of the picker row and the CLI-classified case it names is no longer exercised by anything. The row still passes, so nothing surfaces: a later change that made the decider probe tmux on the CLI path would add a startup round-trip to every ordinary `x <path>` invocation with no test standing in its way. The plan already treats this exact shape-change as something to re-point by hand — task 1-4 moves `cmd/bootstrap_warnings_test.go`'s `/nonexistent-path-for-test` fixture to a multi-segment path for the same reason — so only this second fixture is missing its counterpart.

**Proposal**:
Add a Do step to task 4-4 re-pointing that row's argument to a multi-segment path, beside the two fixture re-points the task already carries. The call is determined rather than chosen: the row's stated subject is a direct-path open, `/dir` stops being one under this task's own predicate, and the plan's established remedy for a fixture the recognition rule reclassifies is to lengthen the path rather than weaken the rule. No Tests change is owed — the row's assertion is unchanged and it is not this task's headline behaviour.

**Current**:
```
**Do**:
- Change `isTUIPath` (`cmd/root.go:168`) to `cmd.Name() == "open" && (len(args) == 0 || len(searchFormPositionals(cmd, args)) > 0) && !anyOpenDomainPin(cmd)`, taking the scan from `cmd/open_search.go` (task 1-5) rather than restating the shape test.
- Leave `anyOpenDomainPin` (`:176`), `shouldRunConcurrentBootstrap` (`:180`), the orchestrator's step set, `cmd/bootstrap/progress_emitter.go` and `internal/tui/loading_progress.go` untouched — this decides which invocations take the existing route, never how that route behaves.
- Extend `TestIsTUIPath` (`cmd/concurrent_bootstrap_gate_test.go:28`) with the search-form rows, reusing its `openProbeCmd` / `openProbeCmdWithFlags` helpers, including a parity row asserting the same verdict for `-f port` and `/port`.
- Build the `--` separator row's command by parsing the whole line rather than handing a helper a hand-written args slice — `c := openProbeCmd()`, `_ = c.ParseFlags([]string{"~/Code/api", "--", "ls", "/tmp"})`, then `isTUIPath(c, c.Flags().Args())`. Both existing helpers return a command whose flag set has never been parsed, where `ArgsLenAtDash()` is pflag's `-1` default and the scan therefore reads every word it is given, the command's own `/tmp` included; parsing is what records the separator, and `Args()` is the same post-parse slice cobra hands `PersistentPreRunE` (the `--` itself is not in it). The rows carrying no separator are unaffected and keep the plain helper call.
- Add a cold-route test beside `TestPersistentPreRunE_ColdTUI_DefersBootstrap` (`cmd/concurrent_bootstrap_route_test.go:17`) driving `open /port` through `rootCmd.Execute()`: the orchestrator must record zero synchronous `Run` calls and `openTUIFunc` must observe a deferred bootstrap on the context.
- Leave `TestPersistentPreRunE_EmitsWarningsForOpenWithPositionalArg` (`cmd/bootstrap_warnings_test.go:255`) green on its multi-segment path fixture — a path positional is still a CLI line — and add its search-form counterpart asserting an empty stderr with the warning still in the sink.
```

**Proposed Text**:
```
**Do**:
- Change `isTUIPath` (`cmd/root.go:168`) to `cmd.Name() == "open" && (len(args) == 0 || len(searchFormPositionals(cmd, args)) > 0) && !anyOpenDomainPin(cmd)`, taking the scan from `cmd/open_search.go` (task 1-5) rather than restating the shape test.
- Leave `anyOpenDomainPin` (`:176`), `shouldRunConcurrentBootstrap` (`:180`), the orchestrator's step set, `cmd/bootstrap/progress_emitter.go` and `internal/tui/loading_progress.go` untouched — this decides which invocations take the existing route, never how that route behaves.
- Extend `TestIsTUIPath` (`cmd/concurrent_bootstrap_gate_test.go:28`) with the search-form rows, reusing its `openProbeCmd` / `openProbeCmdWithFlags` helpers, including a parity row asserting the same verdict for `-f port` and `/port`.
- Build the `--` separator row's command by parsing the whole line rather than handing a helper a hand-written args slice — `c := openProbeCmd()`, `_ = c.ParseFlags([]string{"~/Code/api", "--", "ls", "/tmp"})`, then `isTUIPath(c, c.Flags().Args())`. Both existing helpers return a command whose flag set has never been parsed, where `ArgsLenAtDash()` is pflag's `-1` default and the scan therefore reads every word it is given, the command's own `/tmp` included; parsing is what records the separator, and `Args()` is the same post-parse slice cobra hands `PersistentPreRunE` (the `--` itself is not in it). The rows carrying no separator are unaffected and keep the plain helper call.
- Re-point `TestShouldRunConcurrentBootstrap_IssuesNoProbe`'s `"direct-path open"` row (`cmd/concurrent_bootstrap_gate_test.go:175`), whose `/dir` argument is a single-segment absolute path: give it a multi-segment path (e.g. `/dir/sub`) so the row keeps covering the CLI-classified line it is named for. It asserts only that the decider issues no tmux round-trip, so it stays green either way — but under the widened predicate `/dir` is a search form, which would leave that case uncovered and make the row a duplicate of the bare-picker row beside it.
- Add a cold-route test beside `TestPersistentPreRunE_ColdTUI_DefersBootstrap` (`cmd/concurrent_bootstrap_route_test.go:17`) driving `open /port` through `rootCmd.Execute()`: the orchestrator must record zero synchronous `Run` calls and `openTUIFunc` must observe a deferred bootstrap on the context.
- Leave `TestPersistentPreRunE_EmitsWarningsForOpenWithPositionalArg` (`cmd/bootstrap_warnings_test.go:255`) green on its multi-segment path fixture — a path positional is still a CLI line — and add its search-form counterpart asserting an empty stderr with the warning still in the sink.
```

**Resolution**: Pending
**Notes**:
