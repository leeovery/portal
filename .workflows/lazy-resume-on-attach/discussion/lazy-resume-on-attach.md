# Discussion: Lazy Resume On Attach

## Context

Portal restores every saved session on each reboot, and every restored pane fires its registered resume hook as part of coming back — whether or not the user ever looks at that pane. The firing is unconditional in the hydrate helper's exec chain: once scrollback replay finishes, `execShellOrHookAndExit` looks the pane's baked hook key up in `hooks.json` and, on a hit, execs `sh -c '<HOOK>; exec $SHELL'` (`cmd/state_hydrate.go:172-196`). There is no third state — a restored pane either runs its hook immediately or is a plain shell.

The cost is memory, and it scales with the saved-session count rather than with what the user is working on. Measured on the user's 64 GB M1 MacBook Pro on 2026-08-21 and recorded in the seed: 42 live Claude processes, 13.1 GB resident between them, averaging 318 MB each and peaking at 702 MB, with the pageout counter over a million — roughly a fifth of the machine committed to sessions the user was mostly not looking at. Every new piece of work adds a session that will be resumed on every subsequent reboot, indefinitely, regardless of whether it is still active work.

The feature holds the hook behind an in-pane confirmation prompt instead of firing it at boot. Everything else about restore is unchanged — skeleton, geometry and scrollback all replay as today, so the pane still reads as restored; what changes is that the live process behind it starts when the user says so.

### Inherited position

Discovery settled these with the user. They are this discussion's working ground, not questions to re-open unless something surfaced here contradicts them.

- **Resumption is explicit, not automatic-on-view.** The fork was put as automatic-on-first-view (a session is already coming back when you reach it, at the cost of resuming panes you only glanced at) against explicit-per-pane (nothing starts by accident, at the cost of a keystroke). The user chose explicit and went further than the fork offered, citing Zellij as the reference.
- **The mechanism is an in-pane prompt.** A modal or popover rendered inside the restored pane, stating what is about to be run, with Enter to approve and Escape to decline. This is a surface Portal has never had.
- **The prompt's copy is generic.** Portal's resume machinery is tool-agnostic and its messaging must stay so — the prompt states the command without naming Claude or any particular tool.
- **The prompt is sticky.** Ignoring it is a legitimate outcome. A pane whose prompt is left unanswered can be detached from and returned to later, with the prompt still waiting. The user's worked example, named as their ideal scenario: a window with three panes, one holding a resumable session and two holding other work — open the window, use the two panes needed, detach, come back later to resume the third if and when it is wanted. This makes surviving detach/reattach a first-class requirement rather than an edge case.
- **The behaviour is configurable.** An install can choose eager resumption (today's behaviour) or lazy prompting. The user framed the scope as "at the server level, at the user level"; the precise scope was not settled.
- **This is one feature, not an epic.** One deliverable, one user-facing behaviour change, one new surface. The trigger semantics, the decline path and the eager/lazy preference are decisions inside it.

### Carried forward undecided

- **What a decline means, as distinct from ignoring.** Ignoring keeps the prompt — the user was clear. Escape is a different act, and whether the pane is then finished for this boot or merely quiet until the next attach changes how the feature behaves in daily use. Deliberately not settled during shaping.

### Surfaces named as relevant

Named in the seed and carried forward rather than explored: the hydrate helper's exec chain (`portal state hydrate`), bootstrap step 6 and the restore engine's phase A/B split in `internal/restore/`, the `client-attached` and `client-session-changed` global hooks, and the eager signal-hydrate pass at bootstrap step 7.

### References

- [Seed: Fire resume hooks on attach, not on reboot](../seeds/2026-08-21-lazy-resume-on-attach.md) (inbox:idea, 2026-08-21)
- [Discovery session 001](../discovery/sessions/session-001.md)

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture. The Discussion Map lives in the manifest.*

---

## Summary

### Key Insights

### Open Threads

### Current State
