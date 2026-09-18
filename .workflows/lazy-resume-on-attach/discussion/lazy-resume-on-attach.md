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

**The key binding was then revisited and reversed.** A background review put the destruction's cost against its ceremony: one unmodified keystroke, no confirmation, over a user-authored command with no other copy anywhere. The scenario it was argued from — a user walking dozens of panes pressing the discard key on the ones they are finished with — turned out not to be how the user would ever work; culling a batch of finished sessions happens in the picker, by marking them and killing them, not pane by pane. But the underlying objection survived the scenario that carried it.

The decisive argument was the user's, and it is about consistency rather than danger. Everywhere else in Portal, Escape means *back out* — it reverses, it never acts. Binding it here to an irreversible deletion would make it the one place in the product where the reflex key destroys something. Assuming the positive instead — Enter resumes, a named key discards, a confirmation in front of the discard — restores that meaning everywhere, and leaves Escape on the resume panel with nothing to do, because there is nowhere to back out to. An inert Escape reads as cleaner than a dangerous one.

### Decision

**Discarding removes the pane's resume registration, permanently, and nothing else.** The entry is cleaned out of the store rather than suppressed for the boot, so the prompt does not return on this boot, on the next attach, or after any future reboot. The pane falls through to a plain shell with its replayed scrollback still above it. The tmux session is untouched: it stays live, stays saved, and restores on the next reboot as an ordinary hookless pane — bare shell, scrollback intact, no prompt. Killing it is a separate act the user takes when they want it.

**Escape is not the key that does it.** The discard is bound to its own deliberate key, and confirmed before it lands.

- **Enter resumes.** The positive outcome is the assumed one and takes the most reflexive key.
- **A named key discards**, opening a second confirmation over the first — *this is permanent* — where **`y`** agrees and Escape backs out to the resume panel. *(Amended 2026-09-18 — this had Enter agreeing. Enter resumes on the panel one keystroke earlier, so confirming the discard with it makes the same key mean "bring it back" and "delete it forever" on consecutive screens; a user who presses `d` and then confirms the way they just did has thrown the registration away. Portal's two existing destructive confirmations — killing a session, deleting a project — both take `y` with `esc` to cancel, through one shared builder (`internal/tui/kill_modal.go:13`, `delete_modal.go:12`, `destructive_confirm.go:16`). The panel is modelled on those modals and the discard key `d` was itself derived from the picker's grammar; the confirm key was the one place that grammar was not carried across.)*
- **Escape on the resume panel does nothing at all.** There is nowhere to back out to, so it is inert.

That last property is the point rather than a side effect: it makes Escape mean exactly one thing everywhere in Portal — *back out, change nothing* — with no site where it also destroys something. A key the user's hands press without consulting them can then never be the key that loses work.

The discard key is **`d`**, derived from the picker's existing split rather than chosen fresh: `k` kills a live thing (a session), `d` deletes a persisted record (a project). Nothing is killed here — the session and the pane both survive — and what goes is a stored registration, which is `d`'s side of that line.

**What the discard destroys is recorded.** The removed command is written to the log as it goes. Portal already destroys these entries two ways and treats them differently: the typed removal command records only which entry went, while the automatic stale sweep deliberately records the command itself, its own source comment calling that the recoverable form an operator copies back out of the log (`internal/hooks/store.go:371-373` against `:221`). This path is the sweep's situation rather than the typed command's — it is reached by a keystroke on a panel the user is walking through, not by naming a key on a command line — so it takes the sweep's treatment. The confirmation makes the act deliberate; the log line makes it recoverable anyway, and costs nothing.

There is no "parent process" to fall back to, which the user was unsure about: the pane's only process during restore is Portal's hydrate helper, and it replaces itself with either the hook or the user's shell (`cmd/state_hydrate.go:153-196`). Declining means it takes the shell branch — exactly what a restored pane with no registered hook does today, so a declined pane is indistinguishable from one that never had a hook.

This makes the in-pane `d`, once confirmed, a third removal route alongside the existing `portal hook rm` and a hand edit of the store — reached from where the user already is, instead of by remembering a CLI verb. *(Amended 2026-09-18 — this named Escape as the route; the reversal below rebound the discard to `d` behind a confirmation and left Escape inert.)*

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

**The pane stays frozen for as long as its resume is unanswered.** The marker that already tells the saver to leave a pane alone is held through the waiting state instead of being cleared at the end of scrollback replay, and is cleared when the user answers — on Enter before the hook runs, and on a confirmed discard before the pane falls through to a shell. *(Amended 2026-09-18 — this said "on Escape"; the decline decision rebound the discard to `d` behind a confirmation.)*

The cost is nil in practice: a pane waiting on a resume has no new content worth saving, so freezing it at its last live state is exactly the desired end state. A waiting pane keeps its place in the saved set throughout, so it restores normally on every subsequent reboot — with its original content and a fresh prompt, however many reboots it waits through.

**The freeze is load-bearing, not insurance.** *(Amended 2026-09-18 — this paragraph argued the freeze was optional: that under a tmux-drawn overlay, which never enters a pane's buffer, a waiting pane's content would be byte-identical to what was already saved and the saver's content-hash dedup would decline the write on its own. That surface was then rejected — a tmux overlay captures the whole client's keyboard, and tmux has no pane-scoped one.)*

The settled panel is painted into the pane's **alternate screen**, and the saver reads a pane with `capture-pane -e -p -S -` (`internal/tmux/tmux.go:752`). Measured against that exact invocation on tmux 3.7c, with a live process holding the alternate screen open over a pane carrying 40 lines of prior output: the capture returns the **primary buffer's history with the alternate screen appended at the tail** — the real transcript, then the card's lines. (An earlier reading here used a bare `capture-pane -p`, which returns the visible screen alone; that is why the card looked like a wholesale replacement rather than an addition.)

So an unfrozen waiting pane hashes differently from its last write and the saver rewrites its saved file as *transcript plus a picture of the card*. Once, not per tick — identical captures dedup. The transcript is not lost, which is milder than a replacement, but it is not recoverable either: the next reboot replays that file, so the dead card image returns as part of the pane's history and a live card is drawn over it. A pane waited on across three reboots accretes three dead cards into a transcript that can never shed them.

The decision stands unchanged, and the freeze is load-bearing rather than insurance: without it a waiting pane's saved history silently accretes junk for as long as it waits. That raises how durably the freeze is held.

### The freeze is held by the pane, not by the pane's position

The marker that suppresses capture today is addressed positionally — session name plus window and pane index. The saver recomputes that address for every live pane each tick and skips only on an exact match (`cmd/state_daemon.go:263-273`), and bootstrap's stale-marker sweep unsets any marker no live positional address answers to, enumerating through the same positional format (`cmd/bootstrap/stale_marker_cleanup.go`). All three components of that address move: closing an earlier window renumbers, `break-pane` and `move-pane` relocate, and a rename changes the session half.

Today that exposure is the few seconds between skeleton restore and handover, which is why it has never mattered. This feature stretches it to the whole waiting life. A pane waited on for a week, whose session is renamed or whose sibling window is closed, loses its protection twice over: the saver's skip stops matching and the capture lands, and the next `portal open` — which runs bootstrap, and which the user runs constantly — sweeps the marker away as stale. This is the failure `resume-hooks-silently-lost` already fixed once for hook keys, reappearing on a different marker.

**The saver's skip gains a second condition rather than changing its first.** A pane is left alone when it is mid-restore — the existing positional marker, whose other jobs are unchanged — **or** when it is waiting, read from the pane-scoped pending marker decided in Pending Visibility. That marker travels with the pane through every rearrangement tmux can perform, measured in `resume-hooks-silently-lost` against `break-pane`, `move-pane`, a window close under `renumber-windows`, `respawn-pane -k` and a session rename.

Two consequences follow and are part of the decision:

- **The pending marker is set before the mid-restore marker is cleared.** A gap where neither is set is a one-tick window in which the saver writes the card into the pane's saved history — the whole failure, in the space between two steps.
- **The saver reads it for free.** It already enumerates every pane on the server each tick with a per-pane format to build its structural index, and that format already carries the pane's durable token as a column. The pending marker joins it as another column rather than costing a second tmux call. The arity of that read changes, which is the same contained move the pane token made.

The inverse failure — a marker wrongly left set, freezing a pane's saved content forever — is structurally hard to reach: the marker lives on the pane, the waiting program is the pane's only process, and it dies with the pane. There is no state in which the pane survives while the marker is wrong.

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

That rules out `display-menu`, which renders a list of items sized to its contents and admits no arbitrary body. It initially selected `display-popup`, which takes explicit dimensions, a border style, a title and a body of Portal's own drawing, and which produced exactly the described panel when rendered on a disposable socket over a pane carrying unrelated content.

**The popup was then rejected outright: a tmux overlay captures the whole client's keyboard.** The user set a hard constraint — a waiting pane must never block the panes beside it — and the popup violates it. Measured on tmux 3.7c: with a two-pane window, the *right* pane selected as the active pane, and a popup opened over the *left* pane, keystrokes sent from the attached client were delivered to the popup and never reached the focused right pane at all. The intuition that panes are independent is correct and does not apply, because a popup is not a pane; tmux's own description is "a rectangular box drawn over the top of any panes", and it intercepts the client's input ahead of pane routing. `display-menu` is the same kind of object.

**There is no pane-scoped overlay facility in tmux.** Both of its overlay primitives are client-scoped by construction, and nothing else in the command set draws over a pane — the remaining candidates put text in the pane's own content (`remain-on-exit-format`, pane borders) or in the status line. An overlay that floats above one pane while the others stay live is not something tmux offers.

The user's own observation is what resolves it: Portal's rename modal — the reference for this panel — was never a tmux overlay either. It is Portal drawing with Lipgloss into a surface it owns. The painted panel is closer to how Portal already works than the popup ever was.

**Panes with no pending resume are untouched.** In the user's worked example — one window, a left pane that held a resumable session and a right pane that held a bare shell — the right pane restores exactly as it does today, its scrollback in place, and goes on capturing normally. Work done in it during one attachment shows up in its scrollback on the next, unchanged by this feature.

**The panel is a full-pane canvas with a small card centred on it, not a large box with text adrift in it.** The overlay fills the pane, painted in the active Portal theme so nothing behind it shows through, and the decision itself sits in a compact bordered card in the middle — the shape of Portal's existing rename modal: a header row carrying the title and a state badge, a body, and a footer row of key hints. A box stretched to near-full size with a few lines in the middle "might look quite lost on a big terminal window"; the canvas-plus-card shape is what Portal already uses everywhere else for exactly this reason.

The reuse is at the presentation layer, not the code path: the picker's modals are Bubble Tea components rendering into its own model, while this panel is drawn by a short-lived program hosted in a popup. What carries across is the theme tokens and the modal's visual grammar, which is what makes it read as Portal rather than as a tmux dialog.

**The panel is painted into the pane's alternate screen, so it never enters the scrollback.** The user's one hesitation about painting was that the panel is not user content — it is a hold placed on the session, never asked for — and once it is gone it should leave no trace in the history. The alternate screen is exactly that facility: the buffer `vim` and `less` draw on, which is not added to a pane's scrollback ring.

Measured on tmux 3.7c: a pane printed two lines of real content, entered the alternate screen, painted a card, and exited without leaving it. The pane goes dead with the card visible and `alternate_on` set; `capture-pane -p` returns the card, while `capture-pane -a -p` returns the two original lines, intact underneath. The panel is therefore visible without ever being part of the pane's history, and the genuine scrollback is preserved beneath it the whole time it waits.

Nothing blocks: the pane is dead, holds no process, and captures no input beyond its own key table. Panes beside it are fully live throughout.

**Answering hands the pane over in place; nothing is wiped and nothing is re-laid.** *(Amended 2026-09-18 — this paragraph described answering as clearing the pane and re-dumping the scrollback, and attributed the capture freeze's purpose to keeping the saved file available for that re-dump. That was the dead-pane design, which the waiting-pane-mechanism decision rejected: a pane revived by `respawn-pane -k` does lose its history, but the settled design never respawns.)* The replayed transcript is already in the pane's primary buffer, underneath the alternate screen the panel is drawn on, so leaving the alternate screen reveals it and the resume command starts over it. The freeze's purpose is what the waiting-pane-capture section states, not this.

Confidence: high on the surface itself. **Open below it**: the repaint on resize.

---

## Pending Pane State

### Context

A pane whose resume has not fired still restores its saved scrollback — which, for the case that motivated this work, is a photograph of a session that is no longer running. Whether that content should be replayed at all behind the waiting prompt was put as a fork: replay it as today, or suppress it for panes with a pending resume so the pane is clean underneath.

### Journey

The case for suppressing rested on a claim that the replayed content is *deceptive*: it looks live and is not, and for a tool that redraws its own history on resume it is redundant as well.

The user rejected the framing and the argument moved the call. A scrollback ending in an exited session is not a lie — it is an accurate record of what happened, and it is exactly what the pane would show if the session had been quit by hand. The visible end of the transcript says plainly that the thing is gone. There is nothing to be protected from.

The second argument was decisive on scope: restoring a pane's content and resuming its process are two separate concerns, and making the first conditional on the second buys a small presentational gain at the cost of a conditional in the middle of the restore path. The user also noted that the resume is not the only route back — the tool's own resume picker is always available — so a pane that keeps its history is strictly more useful than one that does not.

The presentational worry that prompted the fork is answered by the surface rather than by the restore path: the panel's canvas covers the pane while the decision is pending, so the old transcript is hidden until the user has chosen, and revealed only once they have declined and know what they are looking at.

### Decision

**Scrollback replays exactly as it does today, for every restored pane, whether or not a resume is pending.** Nothing in the restore path branches on whether a pane has an unfired hook. A declined pane is left showing its genuine history above a bare shell — the same end state as quitting the session by hand.

---

## Waiting Pane Mechanism

### Context

Two designs for what a pane *is* while its resume waits, indistinguishable to the user and close on merit:

- **A waiting program.** The helper restore already puts in each pane does not finish. It lays down the scrollback, paints the card, and blocks on a keypress. Enter drops the card and hands the pane to the resume command in place — no restart, nothing re-laid.
- **A dead pane.** The helper paints the card and exits. tmux holds the pane open with no process in it. Enter and Escape are tmux key bindings that restart the pane with the right payload.

### Options Considered

**A waiting program**
- Pros: an insert at a branch that already exists — the helper's hook lookup already forks on hook-present, and the wait goes between that fork and the existing handover. Restore stays ignorant of hooks. No tmux state to unwind. Live rendering, so a resize repaints itself. Failure modes are a process's: killable, interruptible, and it leaves the pane in a defined state. Testable in the fast lane, beside the helper's existing nine unit-test files and its handover injection seam.
- Cons: a resident process per waiting pane, whose cost scales with the saved set — the very quantity the seed says only grows. Measured floor: a minimal Go binary blocked on a read is 1.7 MB resident, so the real helper is several MB, a few hundred across a population of 41.

**A dead pane**
- Pros: no cost curve at all — the pending state is tmux's, not a process's. Nothing to OOM or kill. Inspectable: `portal doctor` could ask tmux which panes are pending. A better substrate for the agent-aware direction than "a process is blocked in there".
- Cons: eight or nine distinct pieces against one, three of them tmux state with a lifecycle to unwind on every exit path — including a pane closed or a session killed mid-wait. The helper's flow must reorder (it dumps before it looks up today). The resize repaint becomes required machinery, because a dead pane cannot redraw itself. And a dead pane is a state the user has no intuition for: if its bindings fail, the prompt is visible and no key does anything.

### Journey

From first principles the dead pane looked like the better design, and the reasoning stood on the feature's own thesis: a cost proportional to a set that only grows is the shape of problem this work exists to remove. At 41 sessions a waiting program is a few hundred MB; at 200 it is a gigabyte, and 200 is where the seed says this is heading. The user reached the same place independently — "the right engineering design here is zero load + memory."

Its containment was worked through and looked tractable. One module owning the pending state as a single concept, arming and disarming three tmux settings together and never independently; an idempotent disarm, so every plausible cleanup site can call it without first checking, which removes the need to enumerate exit paths; and a bootstrap sweep as the backstop, which is the idiom the codebase already uses for stale skeleton markers and orphan FIFOs. The leak window is further bounded by tmux itself: `remain-on-exit` and key bindings are server state, so a reboot clears them and the exposure is one server lifetime.

**Then the key interception was measured, and it does not scope to a pane.** `key-table` is a session option, not a pane option. tmux accepts `set-option -p -t <pane> key-table <name>` without complaint and resolves it upward: set on one pane of a two-pane session, it reads back set on the session *and on the sibling pane that was never named*. Verified on tmux 3.7c — before the write, all three scopes report the option unset; after it, all three report `portal-wait`. And a custom key table swallows every key, not only its bound ones: with the table active, `Enter` fired the binding and the word typed after it never reached the pane's process at all.

So arming one waiting pane's keys arms every pane in its session, and locks them. In the worked example — left pane waiting, right pane a shell the user wants to work in — the right pane's keyboard would be dead exactly as it was under the rejected popup, and for the same reason: the interception is not pane-scoped.

That is the hard constraint the user set, and it is the constraint that already eliminated the tmux overlay.

One route survives for the dead pane, and it is worse. Bind `Enter` and `Escape` in tmux's **root** table globally, each wrapped in a conditional that fires Portal's command when the focused pane is waiting and passes the key through otherwise. It satisfies the letter of the constraint, at the price of Portal permanently rebinding two keys across the user's entire tmux server, with a conditional evaluated on every press — `Escape` most of all, which every modal program on the machine depends on. That is not a trade worth making for a memory curve.

### Decision

**A waiting program in the pane.** The dead pane cannot intercept its own keys without locking its sibling panes or globally rebinding two keys for the whole server, and neither is acceptable. A process in a pane reads the keys sent to it and nothing else — the scoping the design needs is a property of processes, not something tmux has to provide.

The choice was forced rather than preferred, and the reasoning that favoured the dead pane still stands on its own terms; it simply rests on a capability tmux does not have at pane granularity.

**The resident cost is the runtime floor, not the binary.** Go pages in lazily, so linking a library costs disk rather than memory. Measured on this machine: a Go binary linking the rendering library and never touching it, blocked on a read, is **1680 KB** resident — indistinguishable from one importing nothing at all at 1696 KB, and 4.8 MB on disk. The 22 MB the daemon carries is the cost of doing work, not of existing.

So no separate binary is warranted: the waiter is the same Portal binary entered on a path that does almost nothing. Roughly 2 MB per waiting pane, some 80 MB across a population of 41, against the 13.1 GB the seed measured.

**The process that draws must hand off to a fresh one before waiting.** Drawing touches the theme and the rendering path, and those pages stay resident for that process's life. Drawing and then blocking in the same process would carry all of it for as long as the pane waits. Drawing and then replacing the process image with a minimal wait puts the resting state back at the floor — the helper already ends in exactly that kind of handover, so this is the shape the code is already built around, not a new one.

A resize is the same handover run backwards: the waiter replaces itself with a fresh draw, which draws at the new width and hands back to a fresh wait. The resting state stays at the floor and the cost is paid only at the moment of the resize.

**Enter and `d` act; every other key is swallowed, signals included.** *(Amended 2026-09-18 — this read "Enter and Escape act"; the decline decision rebound the discard to `d` and made Escape inert on this panel. Escape is live only inside the confirmation `d` opens, where it backs out.)* The waiter is the pane's only process, so anything that kills it takes the pane with it — Ctrl-C, Ctrl-D, Ctrl-Z. It must refuse to die rather than exit. The rule that falls out is a safety property as much as a mechanism: a stray paste, an errant `send-keys`, or a key pressed in the wrong window cannot answer the prompt, because nothing but those keys means anything to it.

**The command is read at the moment the user answers, not carried from when the panel was drawn.** The waiting program has to look the registration up before it draws, to know whether to draw at all — but it reads it again when Enter is pressed, and runs what the store holds then.

**Settled by derivation** (2026-09-18) — not discussed. Determined by how long the gap now is: a pane can wait for days, and in that time the entry can be removed by `portal hook rm --pane-key`, rewritten by a re-registration, or hand-edited. Acting on a value read days earlier would resume something the user had already deregistered — the one case where the two readings differ, and the one where the stale reading is plainly wrong. Re-reading costs a single file read at a moment that is already doing far more, and an entry that has gone drops the pane through to a shell exactly as an unregistered pane does, which is a path the helper already has.

**That refusal covers what a person at the keyboard can send, and stops there.** When tmux tears the pane down — the user kills the session, closes the window, or the server shuts down — the waiter exits. It does not decline the hangup, and a closed terminal ends it.

**Settled by derivation** (2026-09-18) — not discussed. Determined by the feature's own purpose: the rule as first written was unbounded, and a waiter that declines every signal outlives the destruction of its own pane, so culling fifteen finished sessions from the picker would leave fifteen Portal processes running with nothing to attach to — the resident cost this work exists to remove, reinstated on the cleanup path. Neither the bootstrap marker sweep nor the tmux server's own exit reaps a process that has refused the hangup. The swallow rule's stated purpose is that nothing accidental can *answer* the prompt, and pane teardown is not an answer. (review-002 F7)

---

## Render Trigger

### Context

Held open from the moment the prompt became something rendered on demand rather than something painted and left: if the panel is drawn when the user arrives at the pane, something has to notice them arriving. "Landing on a pane" is several different events — taking focus, switching to the session, attaching to a window where the pane is one of three — and picking the wrong one either flashes prompts at a user cycling past on their way somewhere else, or never offers a pane split into an existing session.

### Journey

The question dissolved rather than being answered. Once the waiting program was settled, there is nothing to trigger: the panel is drawn by the helper at hydration, which bootstrap already drives for every restored pane, and it simply stays on the pane's alternate screen until the user answers. Arriving at the pane is not an event Portal needs to observe, because the panel is already there.

What made the trigger necessary was the assumption that a waiting pane holds nothing — and that assumption was what the key-scoping measurement retired.

### Decision

**No trigger.** The panel is drawn once, at restore, by the machinery that already runs for every pane. No focus hooks, no attach hooks, no per-event rendering. `client-attached` and `client-session-changed`, named in the seed as likely surfaces, are not touched by this feature.

*Folded in:* the seeded **trigger-and-timing** and **prompt-persistence** subtopics are both answered here and by the mechanism above. Timing has no remaining question — the panel exists from restore, so there is no moment at which it must be produced. Persistence likewise needs no machinery of its own: the waiting program holds the panel for as long as it waits, so ignoring the prompt, detaching, closing the window and reattaching all leave it exactly as it was, and a reboot restores the pane and draws it again from the still-unfired registration. Stickiness is the absence of any dismissal path rather than a feature — only Enter and the discard key change anything.

---

## Pending Visibility

### Context

A reboot leaves roughly forty-one panes each holding a pending decision, and the panel exists only inside its own pane. A background review asked whether the feature owes any surface outside the pane at all — there is otherwise no way to ask what is waiting, short of walking into every pane to find out.

### Journey

The scenario the review argued from — walking dozens of panes pressing discard on the finished ones — is not how the user works. A batch of finished sessions gets culled from the picker: mark them, kill them. That kills the session outright, which takes its pending resume with it, so the bulk path already exists for the case that matters.

It exists but it is not bulk. Measured against the tree: multi-select deliberately ignores the kill key, with the source stating why — none of the row actions compose with a marked set (`internal/tui/model.go:2588-2594`). So culling fifteen finished sessions today is fifteen rounds of select, confirm, repeat. That is the picker's gap rather than this feature's, and this feature neither widens nor narrows it.

What the feature does owe is an answer to "what is waiting", because it creates a state that previously did not exist and puts it somewhere invisible.

A dashboard was rejected: a pending-resume list is a second feature wearing this one's clothes, and the pane is the right place to decide, because the pane is where the context is. The decision needs the transcript above it, which no list can carry.

### Decision

**The panel in the pane is the whole interaction surface.** No list, no bulk answer path, no way to resume or discard from outside the pane it belongs to.

**Pending panes are visible as a passing count in `portal doctor`**, which already reports on this machinery, and as a single glyph on the picker's session row — the state is new, and the picker is where the user would notice it. The row already carries an attached indicator; attached and pending-resume are independent, so it is a second glyph rather than a second meaning for the first. The wider row rework that would give those glyphs a proper home is parked on the roadmap (see Open Threads).

**A pending pane is marked explicitly, with a pane option.** The waiting program sets the marker when it starts waiting; resume and discard each clear it.

Deriving the state instead was argued for and rejected. tmux reports what is actually running in every pane in one read, so a pane running the waiter is a waiting pane by definition — nothing to set, nothing to clear, nothing that can go stale. It was rejected on two counts. It does not compose: every consumer — the picker, doctor, and whatever an agent-aware Portal wants later — has to re-derive it and re-handle its ambiguity, since tmux reports a process's name without its arguments, so any pane briefly running another Portal command reads as pending. And it carries nothing: a marker set at the moment a pane starts waiting can hold metadata about the pause, which a process name cannot. What that metadata should be is open — the point is only that the facility exists, and that a derivation forecloses it.

The lifecycle objection that favoured deriving was overstated. The clears are the two explicit actions the feature already has; the only leak is a pane closed mid-wait, which is what bootstrap's existing stale-marker sweep is for; and the whole thing dies with the tmux server regardless. Portal models every other pane and session condition as a tmux option — the restore markers, the restoring flag, the spawn acks, the directory stamp, the pane token — and this is that vocabulary rather than an addition to it.

**A non-zero pending count never fails the check and never changes doctor's exit code.** The number is detail on a line that passes.

**Settled by derivation** (2026-09-18) — not discussed. Determined by what doctor's exit code means against what this feature produces: a check passes or fails, the exit code is zero only if all pass, and the catalog's nearest neighbours — the stale-hook and stale-project counts — fail the moment their count is non-zero. Pending resumes are not a fault. A correctly functioning install presents roughly forty-one of them after a reboot, which is precisely the state this feature is built to produce, so wiring the count like its neighbours would make `portal doctor` report failure on success and break the scriptable exit code its whole design rests on. (review-002 F8)

**A pane option rather than a server option keyed by position.** `resume-hooks-silently-lost` established this directly: positional keys go stale when tmux renumbers panes, and its measurements confirmed a pane user-option survives `break-pane`, `move-pane`, a window close under `renumber-windows`, `respawn-pane -k` and a session rename. The whole-server enumeration for pane options already exists, so the picker and doctor each cost one read.

---

## Eager Lazy Preference

### Context

Discovery settled that an install can choose eager resumption — today's behaviour, everything fires at boot — or lazy prompting, and framed the scope as "at the server level, at the user level" without settling it. A background review asked whether one setting is enough: the store holds arbitrary user-authored commands, and a thing the user walks up to behaves very differently under a prompt that waits forever than a thing that needs to be *up* whether or not anyone looks at it — a dev server, a tunnel, a watcher. The mechanism assumes every registration is the first kind; nothing requires it.

### Journey

The measured install does not settle it either way: every registration on it is the same kind of thing. So the question was put as whether the case exists at all, and the answer was to cover it rather than bet on it — the cost of the finer model turned out to be small enough that betting was the worse trade.

### Decision

**An install-wide default, with a three-state per-hook override.** A registration carries eager, lazy, or nothing; nothing means inherit, so an entry with no setting follows the install and changes with it. An entry that sets either one holds that choice regardless of what the install says. A user who wants a particular resume to always come back automatically sets it eager; one who wants a particular resume to always ask sets it lazy; everything else is governed centrally.

**Two equally primary shapes for a stored registration.** *(Amended 2026-09-18 — this said the override was "additive on the stored entry, exactly as the pane token was added to the saved pane schema without a migration". That was wrong: the pane token went onto a Go struct with tolerant decode, while `hooks.json` is `map[hook_key]map[event]command` (`internal/hooks/store.go:28-31`) — strings all the way down, with no slot for an attribute that is not a command. The cost was weighed as free and is not.)*

An event's value is **either** the command as a string **or** an object carrying the command alongside its settings:

```json
{
  "0gT5gC": { "on-resume": "cd \"/Users/leeovery/Code/flowx\" && claude --resume 45604077-…" },
  "2zyjmp": { "on-resume": { "command": "cd \"/Users/leeovery/Code/nod\" && claude --resume d89f89ab-…",
                             "resume": "eager" } }
}
```

**Neither shape is legacy and neither deprecates the other.** The string form is the primary shape for a registration that is only a command; the object form is the primary shape for one that carries configuration. Both are permanently valid, there is no migration, and there is no future pass that converts one into the other. The writer picks by whether there is anything to carry.

That last rule is load-bearing rather than cosmetic. The external Claude Code `SessionStart` hook fires `portal hook set` on every session start, and each call rewrites the whole file — so a writer that always emitted the object form would convert every entry on the install to the verbose shape on the next Claude start. Emitting the string form when there is nothing to carry keeps a hand-edited file looking as it does today, with the occasional expanded entry where something has been pinned. The object is open-ended, so it is room for whatever comes later rather than a slot cut for this one setting.

**The out-of-repo consumers were checked before the shape was chosen.** `~/.claude/hooks/portal-resume-hook.sh:125` reads `portal hook list` and filters on the event column rather than parsing the file, and that output does not change shape — the command lands in the same column out of either form. `~/.claude/hooks/portal-resume-backfill.sh:71` does parse the file with `jq`, and would read an object where it expects a string — but it looks entries up by the pre-token `session:window.pane` key and so already matches nothing on the current install; the user confirms it is no longer used, having been a backstop for the hook-loss defect that has since been fixed. Measured on 2026-09-18: all 41 keys in the live file are token-shaped, none are old-format, and `portal doctor` reports no stale hooks across 7 passing checks.

The alternative — a second entry beside `on-resume` in the same inner map — was rejected on that first consumer. It keeps `jq` working, but it puts a non-event in the event namespace and adds a row to `hook list`, which is a machine interface the live script parses. Nesting keeps that output honest and disturbs only the script that is already inert.

**The override is set where the registration is made: `portal hook set`.** A flag on the command that writes the entry, alongside `--on-resume`. This follows from who writes registrations — the external `SessionStart` hook is a shell script, not a person at a screen, so the route has to be something a script can pass. The waiting panel is not the place for it: a panel offering "always resume this one without asking" would be setting a durable preference from a surface whose whole job is answering one instance of a question.

**The install-wide setting has a home already; the way to change it does not.** `prefs.json` holds the install's UI preferences — the theme and the session-list grouping mode — and is the natural place for this. What does not exist is any surface for setting it: the theme picker is the only preference with a UI, so until a settings screen exists this one is changed by hand-editing a file. That raises the stakes on the default rather than changing where it lives.

**The install-wide default is lazy.** The feature ships on rather than waiting to be discovered, and an install that upgrades and reboots meets prompts rather than processes.

Defaulting eager was weighed and rejected. It is the conservative choice — an upgrade changes nothing, and the new behaviour is opted into — but it leaves the feature switched off behind a setting with no UI, which is a poor place to leave the thing the work exists to deliver. What makes lazy safe is that it is not a silent change: a pane holding a prompt says what it is and what key answers it, so the worst first-boot outcome is a few extra keypresses landing the user exactly where eager would have put them. The risk that would have changed this is an install belonging to someone who did not choose the upgrade and reboots expecting their processes back; against one extra keypress on a self-explaining panel, it was not enough.

A settings screen to change it without hand-editing `prefs.json` is parked on the roadmap (see Open Threads).

---

## Restore Pipeline Integration

### Context

The feature inserts an indefinite pause into the middle of a pipeline built on the assumption that hydration completes in seconds. The surfaces named in the seed — the hydrate helper's exec chain, the restore engine's phase split, the eager signal pass at bootstrap, and the global attach hooks — were carried forward to be checked rather than assumed safe.

### Journey

The edges were measured against the tree rather than reasoned about, and most of them turned out to need nothing.

**A second bootstrap does not disturb a waiting pane.** Every `portal open` runs the orchestrator, and restore runs inside it — but it skips any saved session whose name is already live (`internal/restore/restore.go:118`). A session holding a waiting pane is live, so it is never re-restored and its pane is never respawned out from under the user.

**A frozen pane is not dropped from the saved set.** The freeze suppresses that pane's scrollback write, and the structural capture merges the pane's *previous* record back into the fresh index rather than omitting it — guarded so that a stale marker cannot resurrect a pane whose session, window or pane is gone (`internal/state/capture.go:96-127`). So a pane can wait indefinitely and still be restored on the next boot, with the transcript it had when it paused.

**Bootstrap's two sweeps leave it alone.** The stale-marker sweep only unsets markers whose pane is no longer live, and a waiting pane is live. The orphan-FIFO sweep has nothing to reclaim: the helper unlinks its FIFO as soon as the hydrate signal arrives, long before the panel is drawn.

**The eager signal pass is unchanged.** It still writes the hydrate byte to every freshly-armed pane, and the helper still replays on receiving it. What changes is only what the helper does afterwards.

**The pending state is never persisted.** It is recomputed on each boot from a registration that has not fired, so nothing about it needs to reach `sessions.json` and no schema moves.

### Decision

**Nothing in the restore pipeline changes except the helper's own tail.** No new bootstrap step, no change to step ordering, no change to the eager signal pass, and no change to the global hooks the seed flagged. The insertion is contained to the one process that was already the last thing to run in a restored pane.

The one open question this section carried — whether the marker the freeze rides on survives a pane rearrangement mid-wait — is answered in Waiting Pane Capture: it does not, and the freeze gains a second condition read from the pane-scoped pending marker instead.

---

## Summary

### Key Insights

### Open Threads

- **Preferences UI** — parked on the product roadmap (`preferences-ui`, horizon `next`). A settings screen in the picker for install preferences, so `prefs.json` is not hand-edited. The theme picker is currently the only preference with any UI, and this feature adds a second setting that needs one — and the agent-aware direction will add more.
- **Picker row redesign** — parked on the product roadmap (`picker-row-redesign`, horizon `next`). Dropping the window count, which reads "1 window" on every row of the measured install (42 of 42 live sessions hold a single window, `tmux list-windows -a -F '#{session_name}' | sort | uniq -c`), dropping the "attached" word in favour of its glyph alone, right-aligning a status strip that grows leftward as indicators appear, and showing each session's directory path beside its name. It changes every row for every session and is driven by its own rationale rather than by this feature; the pending-resume indicator this feature needs is a single glyph that strip would host.

### Current State
