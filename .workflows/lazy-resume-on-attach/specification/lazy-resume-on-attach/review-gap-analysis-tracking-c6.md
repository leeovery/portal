# Review Tracking: lazy-resume-on-attach - Gap Analysis

## Findings

### 1. A killed waiter can drop the pane's protection while the card is still on its screen

**Source**: Specification analysis
**Category**: Enhancement to existing topic
**Move**: settled
**Priority**: Critical
**Affects**: §4.3 (a waiter that exits without having handed the pane over), §7.2 (the protection goes only once the pane is off the panel's screen), §7.1 (what the saver's own capture returns while the panel is up)

**Problem**:
When the process holding a waiting pane is killed — a stray signal, a crash, or the `pkill portal` anyone chasing a stuck daemon types — the pane is handed to a plain shell and its protection from the saver is dropped so that capture can resume. Nothing says the pane must be back on its own transcript before that protection goes, and the order is not free: a capture taken while the panel is still on the pane's screen returns the pane's history minus its last screenful with a picture of the card over the end, and that is what gets written as the pane's saved transcript. A single tick landing in the space between the two steps costs the user the screenful they were last reading, permanently — there is no copy of it anywhere — and leaves a dead card in the saved history where their work was. One `pkill portal` puts every waiting pane on the install through that gap at the same moment, so on the measured install that is forty-odd panes each losing the work they were last looking at, discovered at the next reboot with a dead card sitting where it was.

**Proposal**:
The pane comes off the panel's screen before its protection is dropped on this path too, exactly as it does on both answers — the chain takes the pane back to its own transcript, then clears the marker, then execs the shell. Determined by the record's own measurement of what the saver's invocation returns over a pane holding the panel, and by the decision already taken that no moment may exist in which a pane is unprotected while the card is on its screen. The rule that the shell runs whether or not the clear succeeded is untouched: what is fixed is only the order of the two steps that precede it.

**Current**:
**A waiter that exits without having handed the pane over drops the pane to a plain shell.** It runs as the tail of a chain that clears the pending marker and then execs the user's shell — the shape the hydrate helper already uses for a hook (`sh -c '<HOOK>; exec $SHELL'`). A killed, crashed or reclaimed waiter therefore leaves the pane alive with its transcript above it, the session intact, the marker cleared so capture resumes (§7.2), and the registration untouched, so the next reboot offers the panel afresh. Without it the pane closes — and on an install where 43 of 44 sessions hold a single pane (§1), the session closes with it and the next capture drops it from the saved set with its whole transcript. The cost is a resident shell parent per waiting pane, a megabyte or so on top of the floor (§4.2).

**Proposed Text**:
**A waiter that exits without having handed the pane over drops the pane to a plain shell.** It runs as the tail of a chain that takes the pane off the panel's screen, clears the pending marker, and then execs the user's shell — the shape the hydrate helper already uses for a hook (`sh -c '<HOOK>; exec $SHELL'`). Those two steps keep the order every answer takes (§7.2): the pane is showing its own transcript again before its protection is dropped, because a tick landing while the card is still up writes that pane's saved transcript as history-minus-its-last-screenful plus the card (§7.1), and a `pkill portal` puts every waiting pane on the install through that window at once. A killed, crashed or reclaimed waiter therefore leaves the pane alive with its transcript above it, the session intact, the marker cleared so capture resumes (§7.2), and the registration untouched, so the next reboot offers the panel afresh. Without it the pane closes — and on an install where 43 of 44 sessions hold a single pane (§1), the session closes with it and the next capture drops it from the saved set with its whole transcript. The cost is a resident shell parent per waiting pane, a megabyte or so on top of the floor (§4.2).

**Resolution**: Approved
**Notes**: Applied verbatim. Determined by the answer-path ordering §7.2 already sets and the §7.1 measurement behind it.

---

### 2. A pane frozen for life, beside the claim that no such pane can exist

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Important
**Affects**: §7.3 (the inverse failure has no route this feature leaves open), §4.3 (the chain hands the pane over whether or not the clear succeeded), §8.2 (no staleness case and no sweep)

**Problem**:
The document says two incompatible things about whether a live pane can be left wrongly frozen. One passage states that the feature leaves no route to one, names the two routes that reach it, closes both, and rules that what remains is the user destroying the process by hand — and the decision to build no sweep, nothing that reclaims such a pane and no surface that reports it rests on exactly that. The waiter-death rule then opens a third route and says so outright: when the process holding a waiting pane dies, the pane is handed to a shell whether or not its protection was successfully dropped, and a drop that did not land is called the worse of the two ways a pane can be wrongly frozen. A builder who takes the first passage at face value treats the state as unreachable and writes nothing that notices it. What the user gets is a pane that looks entirely ordinary and works normally while its saved transcript stands still for the rest of that pane's life — every reboot brings it back to the moment it paused with every line of work since missing — and, at the same time, a dot in the picker and a count in doctor claiming a decision is waiting at a pane that holds none and that no key can answer.

**Proposal**:
The claim is narrowed to what the record supports: two routes are closed, one is left open deliberately because a closed pane is the worse failure, and the log line the chain emits is what makes that pane findable, since nothing sweeps it. Determined by the record's own waiter-death rule, which states both the route and its record; this corrects an exhaustiveness claim the document has since outgrown rather than reopening the decision behind it.

**Current**:
**The inverse failure — a marker wrongly left set, freezing a pane's saved content forever — has no route this feature leaves open.** The marker lives on the pane and the pane's only process is the waiter, so the marker goes when the pane goes. Two routes reach a live pane whose marker is wrong, and both are closed: an answer whose clear failed is held rather than carried out, so the pane goes on waiting and the marker is still the truth (§7.2), and a restore never respawns a waiting pane, because it skips any session that is already live (§9.1). What is left is a pane respawned out from under its waiter by hand — the user destroying the process that held that pane's state, in the same class as a hand edit of the store.

**Proposed Text**:
**The inverse failure — a marker wrongly left set, freezing a pane's saved content forever — is reachable one way, and that way is recorded.** The marker lives on the pane and the pane's only process is the waiter, so the marker goes when the pane goes. Two of the routes that reach a live pane whose marker is wrong are closed: an answer whose clear failed is held rather than carried out, so the pane goes on waiting and the marker is still the truth (§7.2), and a restore never respawns a waiting pane, because it skips any session that is already live (§9.1). One is open by design — a waiter that dies without handing the pane over hands it to a shell whether or not the clear landed, because a closed pane is the worse failure (§4.3). Nothing sweeps the marker it leaves behind (§8.2), so the WARN that chain emits is the whole of what makes that pane findable. Beyond those there is a pane respawned out from under its waiter by hand — the user destroying the process that held that pane's state, in the same class as a hand edit of the store.

**Resolution**: Approved
**Notes**: Applied verbatim. Real contradiction with the route cycle 5's fallback opened by design; the claim is narrowed to name it and the WARN that makes it findable.

---

### 3. A permanent deletion confirmed on a screen too small to draw

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: decide
**Priority**: Important
**Affects**: §5.4 (the discard confirmation), §6.2 (`y` completes the discard), §5.2 (the size below which the surface degrades)

**Problem**:
Restored panes come back at whatever size the user left them, and a narrow split can be smaller than the bordered card this surface draws. The waiting panel has an answer for that — its parts stack plainly without a frame and its keys keep working at every size — but the confirmation standing between the discard key and a permanent deletion has none: what that pane shows after the discard key is pressed is unstated, and whether the key that agrees still acts is unstated with it. Built the obvious way, as the picker's kill confirm with no small-pane story, the user in a narrow pane presses the discard key, sees nothing change or sees a broken frame, presses the key the footer offered a moment earlier, and destroys the only copy of a user-authored resume command with the question never having appeared on screen. There is no route back, nothing reports it, and they find out at the next reboot when the pane comes back as a bare shell and the work it was holding has no way to start.

**Proposal**:
The confirmation degrades with the pane exactly as the waiting panel does, and its two keys act at every size: below the size the card needs, the destructive title, the command, the consequence line and the key hints stack plainly on the canvas. What leaned it: the surface's rule is that a pane swallowing every key must always say what it is waiting for, and that holds hardest on the one screen whose key destroys something a user cannot get back. Alternatives that also fit the record: refusing the discard key below that size and saying so on the report row, so a discard can only be made where the question can be read — which keeps the pane honest but takes the discard route away in a small pane; or drawing the confirmation at whatever the frame allows and letting it clip, which keeps one rendering path at the risk of a question the user cannot read.

**Proposed Text**:
Append to §5.4, after the paragraph beginning "The confirmation carries a report row too":

**The confirmation degrades with the pane, as the waiting panel does.** Below the size the card needs (§5.2) the frame goes and the parts stack plainly on the canvas — the `▲ Discard resume?` title, the command, the consequence line and `y discard   esc cancel` — and `y` and Escape act at every size, as Enter and `d` do. A confirmation that drew nothing there would leave the user pressing the key the footer offered a moment earlier against a question they never saw, and what goes is the only copy of a user-authored command.

**Resolution**: Routed
**Notes**: The call landed in the discussion, extending Panel Rendering Limits; §5.4 aligned to it.

---

## Observations

- The memory headline — a full waiting set of 41 at some 80 MB — does not fold in the resident shell parent the fallback chain adds per waiting pane (§4.2, §4.3).
- Whether the discard's log line is emitted at all when the discard found nothing to remove, and what it carries as the removed command, is unstated (§6.4, §6.2).
- A waiter killed from outside turns a waiting pane into a plain shell with nothing on any user-facing surface recording it — the picker dot and the pending count simply stop, and only a failed clear leaves a line (§4.3, §8).
- The picker's multi-select marker is also a dot on the session row, while the fixed packing order names only attached and pending (§8.3).
- What the helper resolves when `prefs.json` cannot be read at all, as opposed to being absent, empty or corrupt, is unstated (§3.1, §8.2).
