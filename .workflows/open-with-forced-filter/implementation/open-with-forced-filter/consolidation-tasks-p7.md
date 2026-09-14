# Consolidation Tasks: open-with-forced-filter (Phase 7)

## Task 1: Hold the command-pending picker's mint until the bootstrap's terminal event arrives
placement: phase 7
severity: behaviour

**Problem**: `portal open -- <command>` and `portal open -e <command>` on a cold or version-unlatched server take the concurrent-bootstrap route and build a picker that paints the Projects page from frame one — `WithCommand` (`internal/tui/model.go:509-516`) forces `PageProjects`, so none of the loading-page serialisation every other concurrent route relies on applies. The projects list is a JSON read that lands in milliseconds; the orchestrator's restore and scrollback replay take seconds. Enter on a project (or `n` in the cwd) inside that window mints the session, `SessionCreatedMsg` quits Bubble Tea (`internal/tui/model.go:1685-1687`), `finishTUI` connects, and outside tmux the connector `syscall.Exec`s — the orchestrator goroutine dies mid-flight.

Three losses land at once:

- The saved sessions restore had not yet reached never come back.
- `@portal-restoring` — set at bootstrap step 3, cleared at step 8 — leaks. The `_portal-saver` daemon suppresses `captureAndCommit` for the remaining life of the tmux server, so `sessions.json` and every pane's scrollback stop being written and the *next* reboot loses everything, not just this one.
- The two events phase 7 just taught this model to consume — the `BootstrapCompleteMsg` warnings the teardown now writes, and the `BootstrapFatalMsg` that now ends the run non-zero — arrive at a process that has already gone. The run exits 0 in silence.

Noticed as: after a reboot only some sessions came back, portal said nothing, and from then on nothing new is ever saved until the next cold boot.

Phase 7's own `createSession` guard (`internal/tui/model.go:1788-1794`) names exactly this race in its comment — "the quit a fatal issues is asynchronous, so a keypress already queued can still reach this" — and closes the millisecond-wide half of it while the seconds-wide half stays open. It consults `fatalActive` and nothing else; `m.bootstrapComplete` is never read.

This is the one member of the concurrent-bootstrap set with no gate between the user and a half-bootstrapped server. The specification now says so (Corrigendum 2026-09-14, §7.2 and §7.6), which is what makes this a defect against a correct spec rather than a difference of reading.

**Solution**: Extend the chokepoint phase 7 already chose rather than adding a second one. In `createSession`, a model whose `progressReceiver` is non-nil and whose `bootstrapComplete` is false must not mint now: stage the chosen directory on the model and issue the mint from the `BootstrapCompleteMsg` arm — never from the `BootstrapFatalMsg` arm, which already quits, and which must continue to refuse the mint outright.

Report the wait rather than swallowing the keypress: the Projects page already composes a band slot (`renderProjectBandSlot` → `activeProjectNoticeBand`), so the deferred mint has somewhere to announce itself. A user who presses Enter must see that their choice was taken and is waiting, not a dead keypress.

A **warm** command-pending model has a nil `progressReceiver` — its bootstrap ran before `Init` — and must come out byte-identical: it mints immediately as it does today.

Dropping the keypress outright is the smaller change and is rejected: it makes the user press Enter twice for a reason they cannot see, and still leaves `n` and Enter diverging from the "the picker cannot act on a half-bootstrapped server" contract that phase 7 wrote into its own comments and that every other concurrent route enforces with the loading page.

**Outcome**: A cold `portal open -- <command>` can no longer exec away mid-restore. The bootstrap either completes and the staged mint proceeds, or it fails and the run ends non-zero with its message — and in both cases `@portal-restoring` is cleared by the step that owns it, so the daemon keeps capturing.

**Do**:

- Add the staged-mint state to `Model` (`internal/tui/model.go`), beside the bootstrap fields: the chosen directory plus an explicit "a mint is staged" flag, so a staged empty directory (a model whose `cwd` never resolved) stays distinguishable from nothing staged. Add the in-flight condition the chokepoint reads — `progressReceiver != nil && !bootstrapComplete` — as one named predicate method rather than restating the pair at each site.
- Turn `createSession` into a pointer-receiver method. It keeps its `fatalActive` refusal unchanged; when the bootstrap is in flight it stages the directory, calls `resyncPageLayouts` (the band is about to swap, and the Projects list budget is measured off the rendered slot), and returns nil in place of the mint closure. **First stage wins**: a second Enter or `n` while one is staged must change neither the staged directory nor the band, since the band has already announced the pick and a silent re-target is worse than a keypress that visibly changes nothing. Every model that is not in flight mints exactly as today. At both call sites (`handleProjectEnter`, and `handleNewInCWD` → `createSessionInCWD`) assign the returned command to a local **before** returning — `return m, m.createSession(dir)` leaves the order of the model read and the mutating call unspecified, so the staging would be lost on the copy that is returned.
- Issue the staged mint from the `BootstrapCompleteMsg` arm, keyed on the staged flag rather than on `commandPending`: clear the staging and `resyncPageLayouts` (the wait is over, so the band drops back to the pending-command banner), then return the mint command `createSession` would have returned — sequenced after the existing `pendingBootstrapWarnings` append, so the warnings that same event carried are still staged for the teardown to write before the connect. The arm's existing `fatalActive` early return is what denies a complete arriving after a fatal; the `BootstrapFatalMsg` arm issues no mint and keeps its `tea.Quit`.
- Report the wait in the Projects band: add a copy constant beside `commandBandText` in `internal/tui/notice_band.go` — `Finishing startup — your pick will run when it's done` — and an arm in `activeProjectNoticeBand` returning it as a `bandInfo` claim while a mint is staged. It sits **below** the shared `flashSlotClaim` prologue, which both arbiters must keep opening with, and **above** the `commandPending` arm, so the banner asking for a pick is displaced once the pick is made. No new render path: `renderActiveProjectNoticeBand`'s generic branch already renders any non-`bandCommand` role.

**Acceptance Criteria**:

- [ ] On a command-pending model with a non-nil `progressReceiver` and `bootstrapComplete` false, Enter on a project and `n` in the cwd both reach `sessionCreator` with nothing: no `CreateFromDir` call, no `SessionCreatedMsg`, no quit.
- [ ] The staged directory is minted from the `BootstrapCompleteMsg` arm, forwarding the same `command` an immediate mint would forward, and the mint is issued for exactly the directory the keypress chose.
- [ ] No staged mint is ever issued from the `BootstrapFatalMsg` arm, nor by a `BootstrapCompleteMsg` arriving after a fatal; the fatal still returns `tea.Quit` and the run ends non-zero having minted nothing.
- [ ] A warm command-pending model (nil `progressReceiver`) mints on the keypress exactly as it does today — no staging, no band change, no deferred issue.
- [ ] While a mint is staged, `activeProjectNoticeBand` returns the wait claim in place of the `bandCommand` banner, and returns the banner again once the mint is issued.
- [ ] A live flash still claims the Projects slot ahead of the wait band: `activeProjectNoticeBand` keeps opening with `flashSlotClaim`.
- [ ] `projectBandHeight` matches the rendered slot while the wait band is live, and the list budget is re-measured at both the staging and the issuing.
- [ ] Esc and Ctrl-C still quit while a mint is staged — the picker is never a dead end.
- [ ] The loading-gated routes are untouched: the existing `internal/tui` and `cmd` suites pass unchanged, with no new call reaching a real tmux server.

**Tests**:

- `"it stages the pick instead of minting while the concurrent bootstrap is still running"`
- `"it mints the staged pick when the bootstrap's complete event arrives"`
- `"it stages n's cwd mint on the same route"`
- `"it keeps the first pick when a second is made while the mint is staged"`
- `"it mints immediately on the warm command-pending route"`
- `"it mints nothing when the bootstrap ends in a fatal, on the fatal itself and on a complete after it"`
- `"it reports the wait in the projects band while a mint is staged, and shows the pending-command banner again once it is issued"`
- `"it keeps a live flash ahead of the wait band"`
- `"it reserves the wait band's rows in the projects list budget"`
- `"it still cancels on Esc while a mint is staged"`
