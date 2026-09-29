# Analysis Tasks: lazy-resume-on-attach (Cycle 8)

## Task 1: A Hydrate Tail Parks Only the Pane Its Mark Step Returned
severity: medium
sources: architecture

**Problem**: The helper resolves the pane's resume decision once, at the top of `runHydrate` (`cmd/state_hydrate.go:112`), and stores it in `hydrateConfig.Decision`, a `*resumeDecision` (`:61-64`). Waiting is made safe by a step that runs later, `markPendingThenUnsetSkeletonMarker` (`:348-356`). That step writes the pending marker, fills the decision's `Pane` and `Exe` (`markResumePending`, `:374`), sets `Wait` back to false on a refusal, and then clears the skeleton marker. It runs in three places:
- inline on the replay tail (`:177`);
- inside `handleHydrateTimeout` (`:320`);
- inside `handleHydrateFileMissing` (`:340`).

The two handlers are injectable seams that take `hydrateConfig` by value. The pointer exists only so that a change made inside a handler's copy reaches the exec running on `runHydrate`'s own copy. So `Wait` means "resolved lazy" until the mark step runs and "marked" after it. `execResumeChainAndExit` (`:262-274`) cannot tell the two apart. It parks on `Decision.Exe` and `Decision.Pane`, and only the mark step fills those.

A tail that reaches the exec without the mark step therefore parks the pane on an empty executable (`'' state resume-draw …`). That exits 127, and the backstop drops the pane to a bare shell. The user gets no panel, no hook, no pending marker and no `set resume pending marker failed` WARN. The only log record is an exec breadcrumb reading `hook_present=true`. The registration is skipped for that boot, and `portal.log` does not say why.

Every tail marks today, so this cannot happen on the current tree. But the specification's rule that "the mode is resolved and the marker written once, ahead of whichever clear runs" is held only because each tail remembers one call, and two of those calls sit inside replaceable seams. This shape has gone wrong before: the 2026-09-19 corrigendum records that both degraded tails used to restore eagerly whatever mode was resolved.

There is also a second route to the exec. `execShellOrHookAndExit`'s nil-Decision arm (`:243-244`) does its own lookup and fires a lazy registration eagerly. Production never reaches it, because `runHydrate` always sets the decision, but `TestHydrateLazy_NilDecisionFallsBackToItsOwnLookup` (`cmd/state_hydrate_lazy_test.go:454`) keeps it alive.

**Solution**: Make the marked pane a value the exec needs, rather than a field one step fills in on a shared pointer. The direction follows from the specification's rule above.
- `handleHydrateTimeout` and `handleHydrateFileMissing` keep only their tail-specific reporting. The timeout handler keeps the reset preamble, the FIFO removal, and its WARN and INFO lines. The file-missing handler keeps its three-way WARN and its INFO line. Neither calls the mark step, and both seams keep their signatures and their nil-handler behaviour.
- `runHydrate` runs mark-then-unset itself on each of its three tails, in today's order:
  - replay: settle sleep, then mark;
  - signal timeout: handler, then mark, then settle sleep, then exec;
  - file missing: handler, then mark, then exec.
- The mark step returns the parked pane (pane id and executable) as a value. It returns nil when the decision is not to wait, and nil when the mark is refused; a refusal still emits its `set resume pending marker failed` WARN.
- The exec takes the resolved registration and that value. A non-nil parked pane parks on the chain (`execHandOff`, `hook_present=true`). A nil one goes through `handOffToHookOrShell` with the registration's command, as cycle 2 settled.
- `hydrateConfig` loses its `Decision` field. `resumeDecision` keeps only what is resolved at the top (whether the pane waits, and the registration). It never carries `Exe` or `Pane`, and nothing mutates it after it is resolved.
- The nil-Decision arm of `execShellOrHookAndExit` is removed, along with the test that pins it. Tests that drive the exec directly build the decision they need.

Nothing else changes. That covers the settled hand-off helpers (`execHandOff`, `resumeHandOff`, `handOffToHookOrShell`) and the exec-seam guard's permitted set. It also covers the parked chain's trap and backstop, the order in which markers are set and cleared, and every tail's log lines, reset bytes and argv. The lazy suite runs all three tails through the production handlers (`lazyTails`, `cmd/state_hydrate_lazy_test.go:37-66`), and that suite, the hydrate suites and the exec-log suites form the regression net.

**Outcome**: A pane can park on its resume chain only through the pane id and executable that the mark step returned, after it wrote the pending marker. A tail that did not mark has no such value, so parking on an empty executable becomes impossible to write. No route to the exec ignores the resolved mode. On the tree as it stands, every tail marks, logs, clears and hands off exactly as it does today.
