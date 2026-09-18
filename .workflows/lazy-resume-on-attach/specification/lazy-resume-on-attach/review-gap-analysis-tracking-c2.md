# Review Tracking: lazy-resume-on-attach - Gap Analysis

## Findings

### 1. Two answers to what keeps the saver off a waiting pane

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Critical
**Affects**: §7.2 (the freeze), §7.3 (the freeze is held by the pane), §8.2 (the pending marker)

**Problem**:
The document says two incompatible things about what protects a waiting pane's saved transcript. Read one way, the mark restore already uses to keep the saver off a pane during replay is simply not cleared when replay finishes and carries the protection through the whole wait. Read the other way — the way the marker section and the picker-visibility section both state — that mark is cleared at the end of replay exactly as it is today, and a second mark, written onto the pane itself, is what carries the protection. The first reading builds the protection as an address: a session name plus a window and a pane index. All three move. A pane waited on for a week whose session gets renamed, or beside which a window is closed, loses its protection without anything saying so, and the next `portal open` sweeps the mark away as stale on top of that. The saver then rewrites that pane's saved transcript as history-minus-its-last-screenful with a picture of the panel over the end. The user finds out at the next reboot: the work they were last reading is gone and a dead card sits where it was.

**Proposal**:
The freeze section states the guarantee — the saver is kept off a waiting pane for the whole of the wait — and leaves which mark carries it to the section that decides that, where the pane-scoped mark is chosen and its survival across every rearrangement tmux can perform is recorded. Determined by the specification's own decisions: the mid-restore mark keeps its existing lifecycle and its other jobs unchanged, the pane-scoped mark is set before it is cleared, and the helper's clear of the mid-restore mark at the end of replay is the ordering the marker section states outright.

**Current**:
**The marker that already tells the saver to leave a pane alone is held through the waiting state** instead of being cleared at the end of scrollback replay. Today the hydrate helper clears it the moment replay finishes and before it hands off — replay, settle sleep, unset, exec (`cmd/state_hydrate.go:139-148`) — which under lazy resume would land at exactly the moment the panel goes up.

**Proposed Text**:
**The saver is kept off a waiting pane for the whole of the wait.** Today the only thing that keeps it off is the mid-restore marker, which the hydrate helper clears the moment replay finishes and before it hands off — replay, settle sleep, unset, exec (`cmd/state_hydrate.go:139-148`) — which under lazy resume would land at exactly the moment the panel goes up. That marker keeps its lifecycle and its other jobs unchanged; what holds the freeze through the wait is a second marker carried by the pane itself, set before the mid-restore one is cleared (§7.3).

**Resolution**: Pending
**Notes**:

---

### 2. The moment the protection is dropped is written twice, one step apart

**Source**: Specification analysis
**Category**: Duplication
**Move**: settled
**Priority**: Important
**Affects**: §8.2 (the pending marker), §7.2 (the freeze and when it is released)

**Problem**:
When the user answers the panel, two things happen: the pane's protection from the saver is dropped, and the pane leaves the panel's screen to reveal the transcript underneath. The order between them is decided in the freeze section — the protection goes only once the pane is back on its own transcript — and restated one step earlier in the marker section, as happening at the moment the user answers. An implementer working from the second reading drops the protection while the card is still up, and a saver tick landing in that gap rewrites the pane's saved transcript as history-minus-its-last-screenful with the card over the end: the exact loss the freeze exists to prevent, now reachable on the path every single resume takes. Two statements of one ordering also drift apart with nothing catching it, and the half that ends up wrong is the half the implementer happens to read.

**Proposal**:
The ordering keeps its single home in the freeze section, where it is decided and where its consequence is stated, and the marker section names the two actions that clear the mark without restating when. Determined by the one-home rule and by the freeze section already carrying the ordering in full with the loss it prevents.

**Current**:
Resume and a confirmed discard each clear it, both in the waiting program, at the moment the user answers.

**Proposed Text**:
Resume and a confirmed discard each clear it, both in the waiting program, on the ordering §7.2 sets.

**Resolution**: Pending
**Notes**:

---

### 3. A pane that cannot be marked, and waits anyway

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: decide
**Priority**: Important
**Affects**: §8.2 (only a pane that is going to wait is marked), §7.3 (the freeze is held by the pane), §2 (resume modes)

**Problem**:
A waiting pane is protected by a mark written onto it in the moment before the panel goes up, and the whole design takes as given that no moment exists in which a pane is waiting without one. What the pane does when that write does not succeed is unstated. Carried on with regardless, the pane waits unprotected for as long as the user takes to answer — which can be days — and one saver tick in that window rewrites its saved transcript as history-minus-its-last-screenful with a picture of the panel over the end. There is no copy of that screenful anywhere, nothing reports it, and the user meets it at the next reboot as missing work with a dead card where it was. Three defensible ways to go exist and nothing on the page chooses between them, so the behaviour an install gets in this case is whatever the implementer happened to write.

**Proposal**:
A pane that cannot be marked does not wait: the helper fires the hook, and the pane comes back exactly as an eager registration leaves it. What leaned it: the freeze is decided as protection against lost work rather than clutter, so an unprotected wait is the one state the design refuses, and the feature's own account of the worst acceptable outcome is the user landing where eager would have put them — which makes eager the sanctioned place to degrade to. Alternatives that also fit the record: painting the panel anyway and accepting that a single tick may cost a screenful, or dropping the pane to a plain shell with its registration untouched so the next boot tries again.

**Proposed Text**:
Append to §8.2, after the paragraph beginning "Only a pane that is going to wait is marked":

**A pane that cannot be marked does not wait.** If the pending marker cannot be written, the helper does not paint: it fires the hook as an eager registration does, and the pane comes back as today's restore leaves it. A wait with no marker on the pane is the one state the design refuses — the saver rewrites that pane's saved transcript as history-minus-its-last-screenful plus the card (§7.1) at the first tick that lands, for as long as the user takes to answer — and landing the user where eager would have put them is the degradation this feature already accepts (§2.1).

**Resolution**: Pending
**Notes**:

---

### 4. A freeze that will not lift, on the path every resume takes

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: decide
**Priority**: Important
**Affects**: §7.2 (when the freeze is released), §6.1 (Enter), §6.2 (confirmed discard), §8.2 (no staleness case)

**Problem**:
Answering the panel drops the pane's protection and then hands the pane to its command or to a plain shell. What happens when the protection cannot be dropped is unstated, and the damage outlives the answer: the mark stays on the pane, and the saver refuses that pane's scrollback write for the rest of the pane's life. The user resumes, works in that pane for weeks, and every reboot restores it to the transcript it held at the moment it paused — everything since is silently absent. Nothing reports it and nothing reclaims it: the record's own reason for believing a mark can never be wrongly left set is that the waiting program dies with the pane, which stops being true the instant it hands the pane over to the hook, and no sweep reaches a mark carried by a pane.

**Proposal**:
The answer is not carried out while the mark still stands: the pane keeps the panel, says so in place, and the key can be pressed again — neither the hook nor the fall-through to a shell runs. What leaned it: the record already rules out a panel that disappears while the state it named survives, which is the rule a discard the store will not accept already takes, and the alternative leaves a permanently frozen pane with no surface reporting it and no route back. Alternative that also fits the record: handing the pane over anyway, on the grounds that a usable pane matters more than a saved transcript, and accepting a pane whose saved history silently stops at the pause.

**Proposed Text**:
Append to §7.2, after the paragraph beginning "It is cleared when the user answers":

**A freeze that cannot be lifted holds the answer.** If the marker cannot be cleared, the pane keeps the panel and says so in place; neither the hook nor the fall-through to a shell runs while the marker stands, and the key can be pressed again. Handing the pane over with it still set would freeze that pane's saved scrollback for the rest of the pane's life — the pane goes on being used and every reboot restores the transcript it held when it paused — and nothing reports that state or reclaims it, since no sweep reaches a pane option (§8.2). This is the shape a discard that cannot be written already takes (§6.2): what the screen claims and what the pane holds never disagree.

**Resolution**: Pending
**Notes**:

---

### 5. The panel is required to report things it has nowhere to put

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: decide
**Priority**: Important
**Affects**: §5.3 (the waiting panel's content), §6.2 (a discard that cannot be written)

**Problem**:
The waiting panel's contents are enumerated exhaustively — a header, the command under its label, two key hints, and nothing else — while the discard rules require the panel to report in place when the store will not accept a removal, so the user knows the registration they asked to destroy is still there. There is no slot in the enumeration for that report. Each implementer invents one: a line replacing the key hints, a flash that clears after a second, the command swapped out for the message. If it is built as a flash, a user who looked away at the wrong moment sees the panel unchanged and reads it as the key not registering, presses `d` again, and never learns that nothing was removed — the one outcome the discard rules set out to rule out.

**Proposal**:
The card gains a message row between the command and the key hints, present only when there is something to report, and what it says stays there until the next key is pressed rather than timing out. What leaned it: the report exists so the screen and the store never disagree, and a report that can expire unseen fails exactly that; a row inside the card also keeps the panel one object rather than adding a second surface to a pane that has room for nothing. Alternatives that also fit the record: a timed flash in the footer's place, or replacing the command in the body with the message until a key is pressed.

**Proposed Text**:
Append to §5.3, after the paragraph beginning "A command longer than the card wraps rather than being cut":

**A card with something to report carries one more row.** When an answer cannot be carried out — a discard the store will not accept (§6.2) among them — the reason is stated on a single line between the command and the key hints, and it stays there until the next key is pressed rather than timing out: a report the user can miss leaves them believing the thing they asked for happened. The row is present only when there is something to say, and a panel with nothing to report carries exactly the three parts above.

**Resolution**: Pending
**Notes**:

---

### 6. Resizing the terminal restarts every waiting pane, continuously

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: decide
**Priority**: Important
**Affects**: §4.2 (the waiter hands off before it waits)

**Problem**:
A waiting pane redraws by replacing itself with a fresh Portal process that paints the panel and hands back to a fresh wait, and the cost is described as paid at the moment of the resize. A terminal being dragged to a new size does not deliver one size change — it delivers a stream of them, to every pane, for as long as the drag lasts. With a full waiting set of forty-odd panes that is hundreds of Portal launches a second, each reading the theme and rendering, from an action the user takes without thinking about it. What they notice is the machine stalling and the panels stuttering while they drag a window edge. Whether a redraw is owed per size change or once the size settles is unstated, and the literal reading of the handover is the one that storms.

**Proposal**:
A redraw is taken once the pane's size has settled, not once per size change — a pane being resized draws once, at the end. What leaned it: the panel holds nothing that moves, so a redraw mid-drag shows the user nothing they will still be looking at a moment later, and the resting-cost argument the design rests on assumes a redraw is an occasional event rather than a stream. Alternatives that also fit the record: redrawing on every size change and accepting the storm as the cost of the simplest waiter, or leaving the card as drawn until the next keypress or attach, which costs nothing and leaves a briefly mis-sized card on screen.

**Proposed Text**:
Append to §4.2, after the paragraph beginning "A resize is the same handover run backwards":

**The redraw is taken once the size has settled, not once per size change.** A terminal dragged to a new size delivers a stream of size changes to every pane in the window, and a full waiting set answering each of them with a fresh draw would be hundreds of process launches a second for the length of the drag. The panel holds nothing that moves, so one draw at the end of the stream is the whole of what it owes, and the cost of a resize stays a single handover per pane.

**Resolution**: Pending
**Notes**:

---

## Observations

- A theme committed while panes are waiting does not reach panels already drawn, so an install can hold panels in the previous theme until each pane is resized or answered (§5.2, §4.2).
- The discard confirmation's consequence line is called plain-language but its wording is not given, while every other string on both screens is specified word for word (§5.4, §5.2).
- Where doctor's two new lines — the install's resume mode and the pending count — sit in its ordered catalog is unstated (§3.4, §8.1).
- A hand-edited `resume_mode` value Portal cannot recognise is dropped from `prefs.json` the next time any other preference is written, since every writer re-encodes the whole file (§3.1).
- Whether each waiting pane's process brackets itself in the log, and whether it marks its exit when tmux tears the pane down, is unstated — a full set leaves dozens of unpaired starts per reboot if it does not (§4.2, §4.3).
- The panel's title reads `Resume session` while the decision is per pane, and one session can hold more than one waiting pane (§5.3, §8.3).
