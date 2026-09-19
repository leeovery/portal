# Review Tracking: lazy-resume-on-attach - Gap Analysis

## Findings

### 1. What the pane shows when the freeze will not lift

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Critical
**Affects**: §7.2 (when the freeze is released, and a freeze that cannot be lifted), §6.1 (Enter), §6.2 (confirmed discard)

**Problem**:
Two rules govern the moment the user answers, and they cannot both be built as written. One says the pane leaves the panel's screen and is showing its own transcript again *before* the pane's protection from the saver is dropped. The other says that if the protection cannot be dropped, the pane "keeps the panel and says so in place" — which is only possible if the panel is still on the screen when the drop is attempted. An implementer who resolves the collision by attempting the drop while the card is still up reinstates exactly the window that was closed: a single saver tick landing in it rewrites the pane's saved transcript as history-minus-its-last-screenful with a picture of the card over the end, on the path every single resume takes. An implementer who follows the ordering and does not think about what comes next leaves the user looking at their own transcript with no panel, no message, a process that swallows every key, and a freeze that never lifts — the pane reads as resumed, refuses to type, and its saved history stops at the moment it paused for the rest of the pane's life.

**Proposal**:
The pane leaves the panel's screen first on every answer, the freeze is lifted from there, and a lift that fails is answered by drawing the panel again with the reason on its report row — not by holding the card on screen through the attempt. Determined by the two rules the record already carries: the protection may be dropped only once the pane is off the card, or a tick captures the card over a truncated transcript; and nothing may be handed over while the freeze stands, or the pane is frozen for life. Both hold together only if the failure is answered by a redraw, and nothing is at risk in the interval — the freeze is still in force and the pane is showing its true content, so a tick landing there captures what is really in the pane.

**Current**:
**A freeze that cannot be lifted holds the answer.** If the marker cannot be cleared, the pane keeps the panel and says so in place (§5.3); neither the hook nor the fall-through to a shell runs while the marker stands, and the key can be pressed again. Handing the pane over with it still set would freeze that pane's saved scrollback for the rest of the pane's life — the pane goes on being used and every reboot restores the transcript it held when it paused — and nothing reports that state or reclaims it, since no sweep reaches a pane option (§8.2) and the waiter that would have died with the pane has just exec'd away. This is the shape a discard that cannot be written already takes (§6.2): what the screen claims and what the pane holds never disagree.

**Proposed Text**:
**A freeze that cannot be lifted holds the answer.** The pane leaves the panel's screen first, as every answer does; if the marker cannot then be cleared, the panel is drawn again carrying the reason on its report row (§5.3), and the key can be pressed again from it. Neither the hook nor the fall-through to a shell runs while the marker stands. Nothing is at risk in between: the freeze is still in force and the pane is showing its own transcript, so a tick landing there captures what is really in the pane. Handing the pane over with the marker still set would freeze that pane's saved scrollback for the rest of the pane's life — the pane goes on being used and every reboot restores the transcript it held when it paused — and nothing reports that state or reclaims it, since no sweep reaches a pane option (§8.2) and the waiter that would have died with the pane has just exec'd away. This is the shape a discard that cannot be written already takes (§6.2): what the screen claims and what the pane holds never disagree.

**Resolution**: Approved
**Notes**: Applied verbatim. Real contradiction between the two rules cycle 2 landed; the redraw resolution is the only reading that satisfies both.

---

### 2. A pane that outlives its waiter, beside the claim that none can

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Important
**Affects**: §7.3 (the inverse failure), §7.2 (a freeze that cannot be lifted), §8.2 (no staleness case and no sweep)

**Problem**:
The record states flatly that no state exists in which a pane survives while its freeze is wrongly in force, and the decision to build no backstop and no reporting for that state rests on it. The same record then spends a paragraph on what to do when precisely that happens — naming the moment the waiting program hands the pane over and is gone — and records, twice, that this kind of marker survives a pane being respawned, which is a pane outliving the process that held its state. A builder who takes the absolute claim at face value writes nothing that notices a pane frozen by mistake and nothing that says so anywhere. The user then goes on working in a pane whose saved transcript silently stopped at the moment it paused: every reboot brings that pane back to that moment with everything since missing, and no screen, listing or diagnosis points at it.

**Proposal**:
The claim is narrowed to what the record supports. The marker travels with the pane and the pane's only process is the waiter, so it goes when the pane goes; the two routes that reach a live pane with a wrong marker are both already closed — an answer whose clear failed is held rather than carried out, so the pane keeps waiting and the marker is still the truth, and a restore never respawns a waiting pane because it skips a session that is live. What remains is a pane respawned out from under its waiter by hand, which is the user destroying the process holding that pane's state, in the same class as hand-editing the store. Determined by the freeze-cannot-be-lifted rule, by restore's skip of live sessions, and by the pane-option survival across a respawn that the same section records.

**Current**:
**The inverse failure — a marker wrongly left set, freezing a pane's saved content forever — is structurally hard to reach.** The marker lives on the pane, the waiting program is the pane's only process, and it dies with the pane. There is no state in which the pane survives while the marker is wrong.

**Proposed Text**:
**The inverse failure — a marker wrongly left set, freezing a pane's saved content forever — has no route this feature leaves open.** The marker lives on the pane and the pane's only process is the waiter, so the marker goes when the pane goes. Two routes reach a live pane whose marker is wrong, and both are closed: an answer whose clear failed is held rather than carried out, so the pane goes on waiting and the marker is still the truth (§7.2), and a restore never respawns a waiting pane, because it skips any session that is already live (§9.1). What is left is a pane respawned out from under its waiter by hand — the user destroying the process that held that pane's state, in the same class as a hand edit of the store.

**Resolution**: Approved
**Notes**: Applied verbatim. The absolute claim predates cycle 2's failed-clear rule; narrowed to the routes the record closes.

---

### 3. What a waiting pane costs after the user has looked at it

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §4.2 (the waiter hands off before it waits), §6.2 (the confirmation), §5.3 (the card's report row)

**Problem**:
A waiting pane draws more than once. It draws the card at restore, the confirmation when the discard key is pressed, the card again when Escape backs out of that, and the card with a line of text when an answer cannot be carried out. The record works out the cost of a draw for exactly one of those — the resize — and leaves the rest to interpretation. The obvious way to make a keypress produce a second screen is to keep the drawing machinery inside the waiting process, and that decision is invisible until somebody measures it: every pane the user has touched, and under the natural generalisation every waiting pane, then carries the whole rendering path instead of the floor for as long as it waits. A full waiting set costs hundreds of megabytes at rest rather than tens — the memory curve this feature exists to flatten, restored by the path a user walks every time they stop to consider a pane.

**Proposal**:
Every screen a waiting pane shows takes the same handover the resize takes: the process that holds a pane between screens carries the wait and nothing else, and the confirmation, the return to the card, and the card carrying a report are each a fresh draw that hands back to a fresh wait. Determined by the rule the record already states — drawing touches the theme and the rendering path, those pages stay resident for that process's life, and drawing then blocking in the same process carries all of it for as long as the pane waits — of which the resize is one instance rather than the only one.

**Proposed Text**:
Append to §4.2, after the paragraph beginning "The redraw is taken once the size has settled":

**Every screen the pane shows takes that same handover.** A wait is not one draw: `d` puts up the confirmation, Escape brings the card back, and an answer that cannot be carried out redraws the card with its report row (§5.3, §6.2). Each is a fresh draw that hands back to a fresh wait, exactly as a resize does, so the process holding a pane between screens carries the wait and nothing else — and the resting cost of a waiting set does not depend on how many of its panes the user has stopped to look at.

**Resolution**: Approved
**Notes**: Applied verbatim. Determined by §4.2's own stated rule — the resize was one instance of it, not the only one.

---

### 4. Where a discard the store refuses says so

**Source**: Specification analysis
**Category**: Contradiction
**Move**: decide
**Priority**: Important
**Affects**: §6.2 (a discard that cannot be written), §5.3 (the card's report row), §5.4 (the discard confirmation)

**Problem**:
A discard the store will not accept is reported to the user in place, so that what the screen claims and what the disk holds never disagree — and the row that carries such a report is placed on the waiting panel's card, between the command and the key hints. But the moment a discard fails is the moment after the user pressed the confirm key, with the confirmation screen in front of them and the panel gone. The two readings put the message in different places. Built one way, the confirmation stays up with the reason on it and the same key retries. Built the other, the confirmation vanishes and the panel comes back carrying a line of text — which is exactly what backing out of the confirmation looks like, so the user reads their discard as cancelled, presses the discard key again, confirms again, and never learns that nothing was removed either time. That is the outcome the report was added to rule out.

**Proposal**:
The report lands on the screen the key was pressed on: a discard the store refuses leaves the confirmation up with the reason on it, and the confirm key retries from there. What leaned it: the rule exists so the screen and the store never disagree, and a confirmation that disappears on a failed write is indistinguishable from one that was backed out of — the single reading that leaves the user believing they cancelled when they did not. Alternative that also fits the record: the confirmation closes and the waiting panel carries the report, treating the failure as a return to the panel with its reason stated, with the discard key re-opening the confirmation.

**Proposed Text**:
Append to §5.4, after the paragraph beginning "The discard confirmation is the kill modal, retitled":

**The confirmation carries a report row too.** A discard the store will not accept (§6.2) is reported on the confirmation itself, on the same single line the waiting panel's card gives a report (§5.3), and `y` retries from there. The screen the user answered on stays in front of them: a confirmation that closed on a failed write would be indistinguishable from one that was backed out of, and the user would read a discard that did nothing as a discard they cancelled.

**Resolution**: Routed
**Notes**: The call landed in the discussion, extending The Panel's Report Row And Its Redraw; §5.4 aligned to it.

---

## Observations

- The case for deciding in the pane rests on the transcript being there for the decision, while the canvas covers the pane opaquely until the decision is made and there is no way to look under it (§8, §5.2, §9.1).
- What the automatic stale sweep writes as its recoverable `value` for an object-form entry is unstated, while the discard route's line is specified as carrying the command (§6.4, §3.2).
- "The panel is drawn once, at restore" reads against the redraws a resize and a keypress each produce (§4.4, §4.2).
- What `hook list` renders in the command column for an object carrying no command is unstated, while what restore does with one is decided (§3.4, §3.2).
- Whether a swallowed key clears the card's report row, or only a key that acts, is unstated (§5.3, §4.3).
- The structural capture is said to merge a frozen pane's *previous* record back into the fresh index, without saying what a freshly started daemon's first tick after a reboot has to merge (§7.2).
