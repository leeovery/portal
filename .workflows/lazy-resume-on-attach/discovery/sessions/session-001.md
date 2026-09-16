# Discovery Session 001

Date: 2026-09-16
Work unit: lazy-resume-on-attach

## Description (as of session)

Restored panes replay as they do today, but the registered resume hook is held behind an in-pane confirmation prompt instead of firing automatically at boot — with an install-level preference deciding whether an install prompts at all or keeps today's eager behaviour.

## Seed

- seeds/2026-08-21-lazy-resume-on-attach.md (inbox:idea)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

The work arrived from a system-performance conversation rather than from Portal itself. The user's machine — a 64 GB M1 MacBook Pro — has been performing poorly, and the diagnosis pointed at Portal's restore behaviour: every reboot brings back the full saved session population, and each restored pane fires its registered resume hook whether or not the user ever looks at that pane. With a large historical session set, that means tens of long-lived processes resident permanently, most of them behind sessions the user is not working in. The framing the user brought was explicit: the eager behaviour is working exactly as designed, and the question is whether the design itself was wrong.

The 21 Aug inbox capture (`lazy-resume-on-attach`) records the same conclusion with measurements taken on the same machine: 42 live Claude processes, 13.1 GB resident between them, averaging 318 MB each and peaking at 702 MB — roughly a fifth of the machine committed to sessions the user was mostly not looking at, with the pageout counter over a million. That note also observes that the pressure scales with the saved-session count, which only ever grows: every new piece of work adds a session that will be resumed on every subsequent reboot, indefinitely, regardless of whether it is still active work. The note was read in as this work unit's seed.

The shape the note left deliberately open was the trigger — what "actually attaching" means, given a pane can be switched to briefly, be part of an attached session without being the active pane, or be visible in a split without having focus — and whether resumption should be automatic on first view or something the user triggers per pane. That was the first thread pulled in this session. The fork was put as a product question: automatic-on-view means a session is already coming back when you reach it, at the cost of resuming panes you only glanced at; explicit means nothing ever starts by accident, at the cost of one keystroke per session you do want.

The user settled it decisively and went further than the fork offered, citing Zellij as the reference: the restored pane should carry an in-pane prompt — a modal or popover inside the pane — stating what is about to be run, with Enter to approve and Escape to decline. The prompt states the command generically (the mechanism is not Claude-specific; Portal's resume machinery is generic and its messaging must stay so), not naming any particular tool.

The decisive property the user added is **stickiness**: ignoring the prompt is a legitimate outcome. A pane whose prompt is left unanswered can be detached from and returned to later, and the prompt is still waiting. The worked example the user gave: a window with three panes, one holding a resumable session and two holding other work — open the window, use the two panes that are needed, detach, and come back later to resume the third if and when it is wanted. The user named this as their ideal scenario, which makes "the prompt survives detach/reattach" a first-class requirement rather than an edge case.

The user also asked for the behaviour to be configurable — described as "at the server level, at the user level" — so an install can choose eager resumption (today's behaviour) or lazy prompting. The precise configuration scope was not settled beyond that framing.

On shape: this was read as a single coherent feature rather than an epic. One deliverable, one user-facing behaviour change, and one genuinely new surface — an interactive prompt rendered inside a restored tmux pane, which Portal has never had (today a restored pane either runs its hook or is a plain shell, with no third state in which the pane is holding something pending). The trigger semantics, the decline path, and the eager/lazy preference are decisions inside that one feature rather than independently shippable work. The three-pane example is the same feature seen from another angle, not a second one. The user confirmed the read.

One question was explicitly flagged as undecided and carried forward to discussion: what a **decline** means, as distinct from ignoring. Ignoring keeps the prompt — the user was clear on that. But Escape is a different act, and whether the pane is then finished for this boot or merely quiet until the next attach changes how the feature behaves in daily use. This was deliberately not settled during shaping.

Threads named in the seed as relevant surfaces, carried forward rather than explored here: the hydrate helper's exec chain, the restore engine's skeleton/geometry phase split, the eager signal-hydrate pass at bootstrap, and the client-attached and client-session-changed global hooks.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

(none)
