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

## Decline Semantics

### Context

Discovery settled that ignoring the prompt keeps it waiting in perpetuity, and deliberately left open what Escape does as a distinct act. The answer changes what the feature is in daily use: whether declining is a "not now" the user can walk back, or a disposal.

Scale, measured on the user's install on 2026-09-17: 44 live tmux sessions, 43 of them holding a single pane and one holding two (`tmux list-panes -a -F '#{session_name}' | sort | uniq -c | awk '{print $1}' | sort -n | uniq -c` → `43 × 1 pane, 1 × 2 panes`), against 41 registered resume hooks (`portal hook list | wc -l` → `41`). So the working shape is one session, one pane, one resumable process — a reboot would present roughly 41 waiting prompts, almost all of them alone in their session.

### Options Considered

**Per-boot decline** — Escape gives a shell and the pane stops offering until the next reboot, when the prompt returns.
- Pros: "no" means no without destroying anything; the reboot is a natural re-decision point.
- Cons: the offer keeps coming back for work the user has already finished with, so the population never shrinks.

**Re-offer on next attach** — Escape gives a shell, but the prompt returns the next time the pane is attached.
- Pros: the offer is never lost.
- Cons: a pane you have declined keeps asking every time you visit it — a pane you cannot put down.

**Destructive decline** — Escape retires the resume permanently; the registration is removed and never comes back, on this boot or any future one.
- Pros: matches the intent — Escape is only ever pressed by someone who has decided this work is finished; gives the growing saved population a cull point.
- Cons: irreversible, behind one keystroke, over a user-authored command with no other copy.

### Journey

The session opened on a false premise: that decline's job is an escape hatch to a usable shell, for the case where you reach a waiting pane and want a terminal in that directory rather than the session. The user rejected the framing outright — a pane holding a resumable process is *for* that process, and if they wanted a plain shell they would open a new session rather than dismiss a prompt to borrow the pane. The pane is occupied and paused, waiting for a yes or a no, and nothing else.

That reframes decline entirely. It is not an escape hatch; it is a retirement. The user's words: "I've decided, actually, I don't need that session." Pressing Escape is an act of disposal, not deferral — which is exactly why it has to be distinguishable from ignoring, and why the re-offer options are wrong. An offer that comes back after you have declined it is treating a decision as a hesitation.

The user drew the equivalence to killing the tmux session outright: that would remove the resume too, and Escape is meant to be the same thing reached from inside the pane. Whether the equivalence extends to the session itself — whether "gone" reaches only the registration or the whole session — is the open half below.

### Decision

**Escape is destructive and permanent.** The pane's resume registration is removed. The prompt does not return on this boot, on the next attach, or after a future reboot. The pane falls through to a plain shell, with its replayed scrollback still above it, and the user can use it or close it.

This makes the in-pane Escape a third removal route alongside the existing `portal hook rm` and a hand edit of the store — reached from where the user already is, instead of by remembering a CLI verb.

Sibling check: `resume-hooks-silently-lost` — its specification (2026-09-10) owns hook removal, defining the `hook rm` CLI, the rule that removing nothing always exits non-zero, and that a removal never unstamps the pane's durable token. This decision adds a route beside that CLI and contradicts none of those rules; because the key it removes is always a token baked from saved state, it also never touches the old-format entries that specification retains permanently.

Confidence: high on the semantics. **Open within this subtopic**: how far "gone" reaches — the registration alone, or the tmux session with it.

---

## Summary

### Key Insights

### Open Threads

### Current State
