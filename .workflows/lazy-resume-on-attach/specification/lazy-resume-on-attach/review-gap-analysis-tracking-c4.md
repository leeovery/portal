# Review Tracking: lazy-resume-on-attach - Gap Analysis

## Findings

### 1. Two places a refused discard reports itself, and one of them is the screen that means "cancelled"

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Critical
**Affects**: §5.3 (the waiting panel's report row), §5.4 (the discard confirmation's report row), §6.2 (a discard that cannot be written)

**Problem**:
The document says two incompatible things about where a user learns that the discard they just confirmed did not happen. One rule puts that report on the confirmation screen itself, with the confirm key retrying from there, and states outright why: a confirmation that vanishes on a failed write looks exactly like one the user backed out of. The other — the rule that defines the report line and enumerates the failures it carries — names the refused discard among them and places the line on the waiting panel's card, between the command and the key hints. Built that way, confirming a discard the store refuses closes the confirmation and brings the panel back, which is pixel-for-pixel what pressing Escape does. The user reads their discard as cancelled, presses the discard key again, confirms again, and the same nothing happens. They never learn that the registration they meant to destroy is still there, and the panel is back after every reboot with nothing having explained why. That is precisely the outcome the report line was introduced to rule out.

**Proposal**:
A refused discard reports on the confirmation, and only there; the waiting panel's card carries the report for the failure reachable from the waiting panel — a freeze that will not lift. Determined by the specification's own decisions: the only route to a discard is through the confirmation, so the moment a discard is refused the confirmation is the screen in front of the user, and the rule that the screen the user answered on must stay in front of them already settles where the line goes.

**Current**:
**A card with something to report carries one more row.** When an answer cannot be carried out — a discard the store will not accept (§6.2), a freeze that will not lift (§7.2) — the reason is stated on a single line between the command and the key hints, and it stays there until the next key is pressed rather than timing out: a report the user can miss leaves them believing the thing they asked for happened. The row is present only when there is something to say, and a panel with nothing to report carries exactly the three parts above.

**Proposed Text**:
**A card with something to report carries one more row.** When an answer cannot be carried out, the reason is stated on a single line between the command and the key hints, and it stays there until the next key is pressed rather than timing out: a report the user can miss leaves them believing the thing they asked for happened. The row is present only when there is something to say, and a panel with nothing to report carries exactly the three parts above. The card carries the report for a freeze that will not lift (§7.2), on either answer. A discard the store will not accept never reaches the card: it is answered from the confirmation and reported there, on that screen's own row (§5.4), because a confirmation that closed on a failed write would be indistinguishable from one the user backed out of.

**Resolution**: Approved
**Notes**: Applied verbatim. Real contradiction with the rule cycle 3 landed in §5.4; §5.4 is the home and §5.3 now defers to it.

---

### 2. The panel that comes back after a discard names a registration that is already gone

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §7.2 (a freeze that cannot be lifted), §6.2 (confirmed discard), §6.1 (Enter reads the store again), §5.3 (the card's parts)

**Problem**:
A confirmed discard removes the registration while the confirmation is still on screen — it has to, so a store that refuses the write can be reported there — and only then is the pane's protection from the saver dropped. If that drop fails, the rule is that the panel comes back with the reason on it. But the store is empty by then, and what the returning panel shows is left open. The obvious reading of the rule that the waiting program looks its registration up "to know whether to draw at all" is that a redraw does the same — and a redraw that re-reads finds nothing and paints nothing. The pane is then a pane that looks ordinarily restored, with its transcript showing, that swallows every key the user types and whose saved history is frozen at the moment it paused for the rest of its life. The user has a dead keyboard in a pane that looks fine, no message anywhere, and every future reboot brings that pane back to the same moment with everything since missing. The alternative guess — draw the card but treat the vanished entry as a reason to hand the pane over — is the one thing the freeze rules already forbid.

**Proposal**:
The redraw after a discard is the card as it was drawn, carrying the command that was removed and both key hints, with the reason on its report row; it is not a fresh decision about whether to draw. Enter then reads the store again, finds nothing, and drops the pane through to a plain shell once the marker clears — which is where the discard was going anyway. Determined by the record: the freeze rule requires the panel to be drawn again with its reason, the card's parts put the command above that line, and the rule for Enter against an entry that has gone already lands the pane on a plain shell with the marker cleared. The lookup that decides whether to draw at all governs the first draw at restore, not a redraw the freeze rule has already ordered.

**Proposed Text**:
Append to §7.2, after the paragraph beginning "A freeze that cannot be lifted holds the answer":

**The panel that comes back after a discard names a registration that is already gone.** On the discard path the entry is removed while the confirmation is still up, so a store that refuses the write can be reported there (§5.4); a marker that then refuses to clear brings the card back with nothing in the store behind it. It comes back as it was drawn — the removed command under its label, both key hints live — with the reason on its report row (§5.3). Enter reads the store again, finds nothing, and drops the pane through to a plain shell once the marker clears (§6.1), which is where the discard was going. The redraw is not a fresh decision about whether to draw at all: one that re-read the store would find no registration, paint nothing, and leave a pane that looks restored, swallows every key and has its saved transcript frozen for the rest of the pane's life.

**Resolution**: Approved
**Notes**: Applied verbatim. Determined by the discard ordering and §6.1's re-read rule — the redraw shows what was drawn, not a fresh decision to draw.

---

## Observations

- A discard that cannot be written is described as saying so "on the panel" with "the key" pressable again, while the confirmation is the screen it is actually answered from and `y` is the key that retries (§6.2, §5.4).
- An object carrying no command, or an empty one, is decided as not a registration, while a string-form entry holding an empty command is not addressed (§3.2).
- The WARN that records an eager fall-through adds an event to the hydrate helper's closed catalog without naming it, while the discard line's two vocabulary additions are named and justified (§8.2, §6.4).
- What doctor's resume-mode line and pending count report when the read itself fails — no server running — is unstated (§3.4, §8.1).
- The pending-marker section still leans on "the unreachability of the inverse failure" after that claim was narrowed to the routes the record closes (§8.2, §7.3).
- A session's pending dot says a decision is held somewhere in the session; attaching gives no route to the pane holding it, which matters only for a multi-pane session (§8.3).
