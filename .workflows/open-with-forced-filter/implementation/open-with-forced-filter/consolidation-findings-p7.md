# Consolidation Findings: open-with-forced-filter (Phase 7)

## Findings

### F1: The command-pending picker can mint and exec away before the bootstrap's terminal event arrives, defeating the delivery this phase built

- **Class**: behaviour
- **Failure**: On a cold or version-unlatched server, `portal open -- <command>` (and its pre-existing `-e <command>` spelling) takes the concurrent-bootstrap route and builds a picker that paints the Projects page from frame one — `WithCommand` forces `PageProjects`, so none of the loading-page serialisation every other concurrent route relies on applies to it. The projects list is a JSON read that lands in milliseconds; the orchestrator's restore and scrollback replay take seconds. Enter on a project (or `n` in the cwd) inside that window mints the session, `SessionCreatedMsg` quits Bubble Tea, `finishTUI` connects, and outside tmux the connector `syscall.Exec`s — the orchestrator goroutine dies mid-flight. Three losses follow at once. The saved sessions restore had not yet reached never come back. `@portal-restoring`, set at step 3 and cleared at step 8, leaks: the `_portal-saver` daemon suppresses `captureAndCommit` for the remaining life of the tmux server, so `sessions.json` and every pane's scrollback stop being written and the *next* reboot loses everything, not just this one. And the two events this phase just taught the command-pending model to consume — the `BootstrapCompleteMsg` warnings the teardown now writes, and the `BootstrapFatalMsg` that now ends the run non-zero — arrive at a process that has already gone, so the run still exits 0 in silence. Noticed as: after a reboot only some sessions came back, portal said nothing, and from then on nothing new is ever saved until the next cold boot. The phase's own `createSession` guard names exactly this race in its comment ("the quit a fatal issues is asynchronous, so a keypress already queued can still reach this") and closes the millisecond-wide half of it while the seconds-wide half stays open.
- **Evidence**:
  - `internal/tui/model.go:1513-1523` — the phase's `Init` change: a command-pending model now issues `m.progressReceiver`, so the terminal event has an arm to reach. Nothing bounds when it reaches it.
  - `internal/tui/model.go:508-514` — `WithCommand` sets `activePage = PageProjects`, and `internal/tui/build.go` applies it after `New(...opts)`, overriding the `PageLoading` a started server would otherwise set. This is the only concurrent-bootstrap route with no loading gate.
  - `internal/tui/model.go:1785-1791` — the phase's `createSession` guard: refuses a mint once `fatalActive` is set, and nothing else. `m.bootstrapComplete` is not consulted.
  - `internal/tui/model.go:1877-1885` (`handleProjectEnter`) and `internal/tui/model.go:2793-2802` (`handleNewInCWD` → `createSessionInCWD`) — the two mint entry points, both already routed through that one chokepoint.
  - `internal/tui/model.go:1682-1684` — `SessionCreatedMsg` sets `m.selected` and returns `tea.Quit`, with no bootstrap state consulted.
  - `cmd/root.go:123-133` — the concurrent branch: the runner goes in the context, `serverStartedKey` is deliberately unset, nothing joins.
  - `cmd/open.go:629-635` — `pipe.start(cmd.Context(), deferred.runner)` launches the orchestrator goroutine; `cmd/open.go:736-749` — `p.Run()` returns and `finishTUI` connects with no join on it. `cmd/bootstrap_progress.go:142-146` — `ServerStarted()` / `Warnings()` / `Err()` have no production consumer, so there is nothing to read post-`Run` even if the read were synchronised.
  - `cmd/bootstrap_progress.go:88-96` — `send` abandons an event once the context is cancelled, which is what makes the abandonment silent rather than a hang.
- **Proposed shape**: Extend the chokepoint the phase already chose. In `createSession`, a model whose `progressReceiver` is non-nil and whose `bootstrapComplete` is false must not mint *now* — it should stage the chosen directory on the model and issue the mint from the `BootstrapCompleteMsg` arm, never from the `BootstrapFatalMsg` arm (which already quits). The Projects page already composes a band slot (`renderProjectBandSlot`), so the wait has somewhere to be reported rather than reading as a dead keypress. A warm command-pending model has a nil receiver and must be left byte-identical: its bootstrap already ran before `Init`. Dropping the keypress outright is the smaller change but makes the user press Enter twice for a reason they cannot see, and still leaves `n` and Enter diverging from the other routes' "the picker cannot act on a half-bootstrapped server" contract this phase wrote into its own comments.
- **Bank**: reviewer entry for `open-with-forced-filter-7-1` — "the command-pending picker stays fully interactive for the whole concurrent bootstrap, so the terminal event can still be outrun" — **confirmed** against the final state, and folded in whole. One correction to its detail: the route is pre-existing for the `-e <command>` spelling only; this implementation's phase 4 consolidation task widened it to `portal open -- <command>` by taking `isTUIPath`'s first disjunct through `preDashPositionals` (see S1).

## Comment Corrections

- `internal/tui/model.go:1638` — claims the page has no notice band. The Projects page composes one (`renderProjectBandSlot` → `activeProjectNoticeBand`), and a warning flash claims it ahead of the pending-command banner — which is precisely how the loading route surfaces these on the concurrent path (`surfaceBufferedWarnings` → `setFlash`). The teardown route is a design choice about whose band this is, not an absence of one.

  OLD:
  ```go
			// No loading page and no notice band to reach, so the teardown write is
			// the only route left. Appended rather than assigned: what was staged
			// before the run is a disjoint set the terminal is owed just as much.
  ```
  NEW:
  ```go
			// No loading gate to hand these to, and this page's band is the
			// pending-command banner's, so the teardown write is the route — the
			// same one a warm picker takes. Appended rather than assigned: what was
			// staged before the run is a disjoint set the terminal is owed just as
			// much.
  ```

- `internal/tui/model.go:233-234` — falsified by this phase: the field is no longer written only before `Init`. The `BootstrapCompleteMsg` arm now appends to it on the command-pending route.

  OLD:
  ```go
	// pending is staged before Init; a loading page moves it into
	// bufferedWarnings, and with no loading page it stays for the teardown.
  ```
  NEW:
  ```go
	// pending is staged before Init and, with no loading page, added to by the
	// terminal bootstrap event; a loading page moves it into bufferedWarnings
	// instead. What is still pending at the end is written at teardown.
  ```

## Spec Defects

### S1: §7 enumerates one addition to the concurrent-bootstrap set; the tree made two, and the second is the ungated one

- **Claim**: §7.6 — "Nothing about how the concurrent bootstrap behaves changes. This section adds the sigil to the set of invocations that take it." Supported by §7.2 — "anything positional is classified as not-heading-for-the-picker (`sed -n '168,170p' cmd/root.go` → `return cmd.Name() == "open" && len(args) == 0 && !anyOpenDomainPin(cmd)`)".
- **Observed**: `cmd/root.go:172-177` now reads `return len(preDashPositionals(cmd, args)) == 0 || len(searchFormPositionals(cmd, args)) > 0`, so the quoted line no longer exists in the tree and post-dash words no longer disqualify an invocation. The set of invocations taking the concurrent bootstrap therefore gained two members in this implementation, not one: the sigil, and `portal open -- <command>` — the latter from phase 4's consolidation task (commit `eae96c47c`, `.../consolidation-tasks-p4.md`), whose own reasoning asserted that the equivalent `portal open -e claude` "takes the loading page", which it does not: `WithCommand` (`internal/tui/model.go:508-514`) forces `PageProjects` for both spellings. The admitted member is thus the one member of that set with no gate between the user and the tmux server, which is what phase 7 spent a whole task partly closing and what F1 reports as still open.
- **Read**: spec stale on the enumeration — the `open -- <command>` reclassification is deliberate and was reasoned through in this implementation's own record, so §7.6's sentence and §7.2's code citation are behind the tree rather than the tree being wrong, and the specification should name both additions. The behaviour §7.6 asserts is not stale but untrue of the second member and genuinely open: "nothing about how the concurrent bootstrap behaves changes" holds for the sigil, which the loading page gates, and does not hold for the command-pending picker the same change admitted.
