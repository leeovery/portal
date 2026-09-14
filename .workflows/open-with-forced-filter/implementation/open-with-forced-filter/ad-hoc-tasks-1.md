# Ad Hoc Tasks: open-with-forced-filter

## Task 1: Deliver the concurrent bootstrap's terminal event to the command-pending picker
placement: new phase "Ad Hoc"

**Problem**: `portal open -- <command>` on a cold or version-unlatched server takes the concurrent-bootstrap route — `isTUIPath` (`cmd/root.go:172-176`) is true for a line with no pre-dash positionals, so `shouldRunConcurrentBootstrap` returns true and `cmd/open.go` starts the orchestrator in a goroutine streaming over the progress channel. The model it starts alongside never drains that channel. `Model.Init` (`internal/tui/model.go:1511-1514`) returns `tea.Batch(requestBg, detectTimeout, m.loadProjects())` on the `m.commandPending` branch and returns *before* the `PageLoading` branch that issues `m.progressReceiver`, so no message from the orchestrator ever reaches `Update`.

Two consequences, of different severity:

1. **A soft warning is dropped.** A `SaverDownWarning` staged by `stageBootstrapWarningsOnModel` is never buffered and never surfaced — the same loss task 6-4 just closed on the warm route, still open on this one. The user learns their state daemon is down at the next reboot, when `sessions.json` and every pane's scrollback are gone.
2. **A bootstrap fatal is swallowed, and the process exits 0.** `BootstrapFatalMsg` never arrives, so `m.fatalActive` is never set, `FatalError()` (`internal/tui/model.go:455`) returns nil, and `finishTUI`'s fatal branch (`cmd/open.go:592`) does not fire. `pipe.Err()` is never consulted either. Portal proceeds to mint a session against a half-bootstrapped server and reports success. On the picker's own concurrent route the same fatal quits Bubble Tea with a `state.destructive` error frame and a non-zero exit; here it is silent.

A second, compounding defect sits behind it: `WithCommand` (`internal/tui/model.go:507-513`) sets `activePage = PageProjects`, and `tui.Build` (`internal/tui/build.go:173-176`) applies it *after* `New(...opts)`, so it overrides the `PageLoading` that `WithServerStarted(true)` set. Issuing the receiver alone would therefore still drop the warnings, because the `BootstrapCompleteMsg` arm gates its buffering on `m.activePage == PageLoading` (`internal/tui/model.go:1621-1629`). Both halves have to move together.

The pipe itself is sound — buffered 64 and context-guarded (`cmd/bootstrap_progress.go:94-99`) — so nothing leaks or hangs. This is silent loss, not a deadlock.

**Solution**: Issue the progress receiver on the command-pending branch of `Init`, and make the two terminal arms reachable for a model that is not on `PageLoading`.

The design call the task must settle and state: a command-pending picker has no loading page, so the warnings have no notice band to reach and must take the same route task 6-4 established for the warm picker — stay pending, and get written by `finishTUI` after teardown. That means the `BootstrapCompleteMsg` arm must *not* buffer them for this model, which the existing `PageLoading` gate already achieves; what it must do is stop that gate from also being the thing that decides whether the receiver was issued at all. The fatal arm has no such subtlety: `fatalActive` must be set whatever page the model is on, so `FatalError()` is non-nil at teardown and the existing non-zero exit path runs. Prefer the smallest change that makes the fatal reachable and leaves the loading page's behaviour byte-identical.

**Outcome**: A cold `portal open -- <command>` reports a down daemon like every other picker invocation, and a bootstrap fatal on that line ends the process non-zero with its user message rather than minting a session against a half-bootstrapped server at exit 0.

**Do**:
- Reproduce both losses as failing tests first, against the model rather than a live program: build a command-pending model with a progress receiver (`tui.Build` with `Command` set and `ServerStarted` true), drive `Init`, and assert the returned batch issues the receiver. Then drive a `BootstrapFatalMsg` through `Update` on a command-pending model and assert `FatalError()` is non-nil.
- Issue `m.progressReceiver` on the `commandPending` branch of `Init` (`internal/tui/model.go:1511-1514`) when it is non-nil, alongside what that branch already batches. Do not synthesize a `BootstrapCompleteMsg` on this branch when the receiver is nil — the warm command-pending route has no gate to satisfy and task 6-4's teardown write already covers it.
- Make the `BootstrapFatalMsg` arm (`internal/tui/model.go:1633-1640`) set `fatalActive` for a command-pending model. Leave its "stay on PageLoading" comment true of the loading page, and state in the comment what a command-pending model does instead.
- Leave the `BootstrapCompleteMsg` arm's `PageLoading` buffering gate exactly as it is, so the command-pending model's warnings stay pending and reach `finishTUI` — the route task 6-4 built. Confirm by test rather than by reading.
- Check whether the command-pending model must also refuse to mint its session when a fatal is active, the way `finishTUI`'s fatal branch precedes `processTUIResult` — and if `processTUIResult` can still run the command on a fatal model, stop it.
- Do not touch `isTUIPath`, `shouldRunConcurrentBootstrap`, the notice band, or the picker's own concurrent route.

**Acceptance Criteria**:
- [ ] A cold-route command-pending model issues its progress receiver from `Init`; a warm one (nil receiver) issues nothing extra and its batch is otherwise unchanged
- [ ] A `BootstrapFatalMsg` delivered to a command-pending model sets `fatalActive`, so `FatalError()` is non-nil at teardown and the process exits non-zero with the fatal's user message
- [ ] A bootstrap fatal on `portal open -- <command>` does not mint a session
- [ ] A `SaverDownWarning` accumulated on the cold command-pending route reaches stderr exactly once, after teardown, via the pending route task 6-4 established — not the notice band, and not twice
- [ ] The loading page's behaviour is byte-identical: the picker's concurrent route still dismisses on `BootstrapCompleteMsg`, still surfaces warnings in its notice band, and still shows the destructive error frame on a fatal
- [ ] `isTUIPath`, `shouldRunConcurrentBootstrap` and the notice band are unchanged
- [ ] `go test ./...` passes; `go vet ./...`, `gofmt -l .` and `golangci-lint run ./...` are clean

**Tests**:
- "it issues the progress receiver for a command-pending model on the cold route"
- "it issues no bootstrap message for a command-pending model on the warm route"
- "it records a bootstrap fatal delivered to a command-pending model"
- "it reports a non-nil FatalError at teardown for a command-pending fatal"
- "it does not run the pending command when a bootstrap fatal is active"
- "it writes a down-daemon warning once at teardown on the cold command-pending route"
- "it leaves the loading page's complete-message handling unchanged"
