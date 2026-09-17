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

The user drew an equivalence to killing the tmux session outright, which was read too literally — as a proposal that Escape retire the whole session, dropping it from the saved set so it never restores again. That reading was put back to the user as a fork, on the argument that it would turn each reboot into a cull and answer the other half of the seed's complaint: that the saved population only ever grows. It was a false path. The user's phrase meant only that a dismissed resume is irrecoverable, not that the session goes with it. Killing a session stays a separate, deliberate act.

The rejected option is worth keeping on the record because it is the tempting one: it solves the accumulation problem the seed names, and 43 of the 44 live sessions hold a single pane, so the multi-pane hazard that argues against it is rare. It is rejected on intent rather than on that hazard — declining a resume and disposing of a session are two different decisions, and binding them to one keystroke removes the user's ability to make only the first.

### Decision

**Escape removes the pane's resume registration, permanently, and nothing else.** The entry is cleaned out of the store rather than suppressed for the boot, so the prompt does not return on this boot, on the next attach, or after any future reboot. The pane falls through to a plain shell with its replayed scrollback still above it. The tmux session is untouched: it stays live, stays saved, and restores on the next reboot as an ordinary hookless pane — bare shell, scrollback intact, no prompt. Killing it is a separate act the user takes when they want it.

There is no "parent process" to fall back to, which the user was unsure about: the pane's only process during restore is Portal's hydrate helper, and it replaces itself with either the hook or the user's shell (`cmd/state_hydrate.go:153-196`). Declining means it takes the shell branch — exactly what a restored pane with no registered hook does today, so a declined pane is indistinguishable from one that never had a hook.

This makes the in-pane Escape a third removal route alongside the existing `portal hook rm` and a hand edit of the store — reached from where the user already is, instead of by remembering a CLI verb.

Sibling check: `resume-hooks-silently-lost` — its specification (2026-09-10) owns hook removal, defining the `hook rm` CLI, the rule that removing nothing always exits non-zero, and that a removal never unstamps the pane's durable token. This decision adds a route beside that CLI and contradicts none of those rules; because the key it removes is always a token baked from saved state, it also never touches the old-format entries that specification retains permanently.

Confidence: high.

---

## Waiting Pane Capture

### Context

The feature's premise is that a restored pane still reads as restored before its hook fires — skeleton, geometry and scrollback replay exactly as today, so the pane shows the work the user left there. That holds on the restore side. Nothing held it on the save side, and a background review found the hole.

Portal's saver re-reads every live pane on each tick and rewrites that pane's saved scrollback whenever the capture hashes differently from the last write (`cmd/state_daemon.go:263-290` — `CaptureAndHashPane` then `WriteScrollbackIfChanged`, which is a dedup, not a protection). A pane parked on a full-screen prompt therefore gets captured *as that prompt*, with no history behind it, within one tick of the prompt appearing.

The suppression mechanism already exists and is already used for precisely this hazard: restore stamps each skeleton-restored pane with a marker, and the daemon's capture loop skips any pane carrying one (`cmd/state_daemon.go:244`, `271-273`). But the hydrate helper clears that marker the moment scrollback replay finishes and before it hands off — replay, settle sleep, unset, exec (`cmd/state_hydrate.go:139-148`). Under lazy resume that unset lands at exactly the moment the prompt goes up.

Verified against the tree on 2026-09-17: the skip is scoped to the scrollback write alone. The pane is still enumerated into `sessions.json` by `CaptureStructure` and only its `.bin` rewrite is skipped, so freezing a pane does not drop it from the saved set.

### Journey

The compounding case is what makes this more than a lost file. The saved content becomes a picture of the prompt; the next reboot replays that picture and then draws a real prompt on top of it. The user comes back to a pane showing a dead screenshot of a question with a live copy of the same question over it — and the work they were returning for is gone with no copy anywhere.

No competing option was worth writing up. The user's response on seeing it: there is only one answer here.

### Decision

**The pane stays frozen for as long as its resume is unanswered.** The marker that already tells the saver to leave a pane alone is held through the waiting state instead of being cleared at the end of scrollback replay, and is cleared when the user answers — on Enter before the hook runs, on Escape before the pane falls through to a shell.

The cost is nil in practice: a pane waiting on a resume has no new content worth saving, so freezing it at its last live state is exactly the desired end state. A waiting pane keeps its place in the saved set throughout, so it restores normally on every subsequent reboot — with its original content and a fresh prompt, however many reboots it waits through.

**Revised in the same sitting.** The hazard this decision was written against was the prompt being *painted into the pane*, which is what would have overwritten the saved content. Under the surface the discussion then settled on — a tmux-drawn overlay that never enters the pane's buffer — the pane's content while waiting is byte-identical to what was already saved, so the saver's content-hash dedup would decline the write on its own and nothing would be corrupted even unfrozen. Measured on tmux 3.7c: with a menu displayed over a pane, `capture-pane -p` returns the pane's own content with no trace of the overlay.

The freeze is kept anyway, as the cheaper guarantee. It costs nothing, it does not depend on the dedup continuing to behave this way, and it holds regardless of what the pane turns out to hold while pending — which the prompt-surface section leaves open. The decision stands; only its justification changed.

Sibling check: `built-in-session-resurrection` — its specification records the marker lifecycle as "Helper unsets marker after dump + 100ms sleep", which is true of the code as it stands and is what this feature changes. No corrigendum is owed: that text is not a claim that has gone wrong, it is current behaviour this work supersedes, and the same reading applies to that specification's rejection of a Zellij-style confirmation prompt — a decision superseded by new product intent rather than a factual error.

(resolves review-001 F1)

---

## Prompt Surface

### Context

Discovery settled that the restored pane carries an in-pane prompt stating what is about to be run, Enter to approve and Escape to decline, and that it is sticky — ignoring it is legitimate and it must still be waiting after a detach and reattach. What renders it was open, and the answer turned out to decide the feature's memory cost as well as its looks.

### Options Considered

**A resident process per waiting pane** — something sits in each pane drawing the prompt and listening for the keypress.
- Pros: full control of the rendering; redraws itself on resize.
- Cons: reinstates the cost the feature exists to remove — measured at 17 MB resident for the Portal binary (`ps -o rss -p $(pgrep -f '^portal state daemon')` → `17744` KB on 2026-09-17), roughly 700 MB across a full waiting set of 41.

**A box painted into the pane, which then goes dead** — the helper draws the prompt as ordinary terminal output and exits; tmux holds the pane open showing it.
- Pros: zero processes; fully designed rendering; durable by construction — the picture is the pane's content, so it survives anything.
- Cons: it is a painting, not a widget. No process remains to re-centre it, so a resize leaves it off-centre or clipped unless a resize hook repaints it. And because it *is* pane content, the saver would capture it — the hazard the waiting-pane-capture section was written against.

**A tmux-drawn overlay, rendered on demand** — nothing is held in the pane at all; the prompt is drawn by tmux when the user lands on a pane that has an unfired resume, and is gone again when they leave.
- Pros: zero processes and nothing painted; tmux owns the drawing, so it is centred, styled from the theme, and re-centres on resize for free; it never enters the pane's buffer, so the saver cannot capture it; Enter and Escape are the overlay's own selections, so no key bindings are touched anywhere.
- Cons: the overlay belongs to the attached client, so it cannot itself be the durable thing — persistence has to come from state the overlay is rendered *from*, and something has to decide when to render it.

### Journey

The first two options were reached by asking what could hold a prompt in a pane, and both answered that question at a cost — one in memory, one in fidelity. The session went a long way down the second, including verifying that a fully styled box (rounded borders, colour, bold heading, dim command text, coloured key hints) survives in a dead pane with no process, and that a dead pane can be repainted in place.

The user then asked the question that dissolved it: does anything need to be displayed at all while the pane is not being looked at? A pane nobody is watching does not need to be holding a picture. What has to survive is the *fact* that a resume is pending — and that fact is already durable, because it is exactly the unfired hook entry. The prompt is a rendering of that fact, not a thing that has to persist.

That reframing turns stickiness from a constraint into a consequence. Ignoring the prompt changes no state, so the next time the pane is looked at, the same state renders the same prompt. Detaching, closing the window, rebooting — none of them are dismissals, so none of them clear anything. Only Enter and Escape change the state, which is precisely the rule the decline-semantics section already settled.

The tmux facility is `display-menu` — the same one behind the user's own Alt-M menu. Its sibling `display-popup` was rejected on sight: a popup runs a command in a floating mini-terminal, which puts a resident process back in the pane and undoes the whole gain.

Two properties were measured on tmux 3.7c before the call was made, both on a disposable `-S` socket:

- **The overlay is client-scoped, not pane-scoped.** Displayed with a client attached, it renders centred and styled over the pane; after a detach and reattach, the pane content is back and the menu is gone. Invoked with no client attached at all, tmux answers `no current client`. This is why the overlay cannot be the durable artifact.
- **The overlay never enters the pane's buffer.** With a menu displayed over a pane, the client's screen shows the menu over the content while `capture-pane -p` on that pane returns the content alone. The saver would therefore record the pane's real scrollback throughout.

### Decision

**The prompt is a tmux-drawn overlay rendered on demand, not an artifact held in the pane.** The durable thing is the pending resume itself; the prompt is what that state looks like when the user is in front of it, and it is drawn by tmux over the pane's existing content.

This is the only one of the three that carries every property the feature needs at once: nothing resident per waiting pane, a centred and designed box rather than a painting, correct rendering after a resize with no repaint machinery, no contamination of the saved scrollback, and stickiness for free. The alternatives each buy one of those by giving up another.

**The overlay is a near-full-pane bordered panel, not a small centred menu.** It is inset a little from the pane's edges, carries a border and a title, is styled from the Portal theme the user has chosen, and has room along its edges for metadata about what is being offered — the shape of Portal's own scrollback preview rather than a list of choices. The user's framing: "a floating overlay that's sort of slightly indented, but basically full screen… it has a border, a bit like the quick preview in Portal."

That rules out `display-menu`, which renders a list of items sized to its contents and admits no arbitrary body, and selects `display-popup`, which takes explicit dimensions, a border style, a title and a body of Portal's own drawing. Rendered on a disposable socket at 90%×80% with rounded borders and a Nord-ish palette, over a pane carrying unrelated content, it produces exactly the described panel — and `capture-pane -p` on the pane beneath returns that pane's own content untouched, as with the menu.

`display-popup` runs a command, so a process does exist — but only for as long as the panel is on screen, which is only while the user is standing in front of it deciding. That is categorically different from the resident-process option rejected above: the cost is per *decision*, not per waiting pane, and a waiting pane nobody is looking at still costs nothing. The on-demand reframing is what makes a process acceptable here, and it is what lets Portal draw the panel itself rather than accepting tmux's menu rendering.

**Panes with no pending resume are untouched.** In the user's worked example — one window, a left pane that held a resumable session and a right pane that held a bare shell — the right pane restores exactly as it does today, its scrollback in place, and goes on capturing normally. Work done in it during one attachment shows up in its scrollback on the next, unchanged by this feature.

Confidence: high on the surface itself. **Open below it**: what the pane holds behind the overlay, what triggers the render.

---

## Summary

### Key Insights

### Open Threads

### Current State
