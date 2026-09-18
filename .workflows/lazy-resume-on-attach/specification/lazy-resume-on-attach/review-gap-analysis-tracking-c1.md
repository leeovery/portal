# Review Tracking: lazy-resume-on-attach - Gap Analysis

## Findings

### 1. Pinning a mode on its own destroys the command it was meant to pin

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Critical
**Affects**: §3.3 (setting the override), §2.2 (a registration is written whole), §3.2 (the stored shapes)

**Problem**:
Someone pinning an existing registration will reach for `portal hook set --resume-mode lazy` on its own — it reads like changing a setting on something that already exists. A registration is written whole and nothing is carried forward from the entry it replaces, so honouring that invocation replaces a working registration with one that has a mode and no command. The user-authored command is gone, there is no other copy of it anywhere, and there is no route back. Nothing says so at the time: the user sees a successful command, and finds out at the next reboot when the pane comes back as a bare shell with the session it was holding unreachable.

**Proposal**:
`portal hook set` refuses `--resume-mode` unless `--on-resume` is passed with it — non-zero exit, nothing written. Determined by the two stored shapes, both of which carry a command, and by the whole-write rule that forbids carrying an attribute forward from the entry being replaced: an invocation with no command has no valid entry it could write, so pinning a mode means re-passing the command it belongs to.

**Proposed Text**:
Append to §3.3, after the paragraph introducing the flag:

**A mode is always passed with the command it belongs to.** `--resume-mode` on its own, with no `--on-resume`, is refused — the command exits non-zero and writes nothing. A registration is written whole (§2.2) and both stored shapes carry a command (§3.2), so there is no entry a mode could attach to by itself; pinning an existing registration means re-passing its command alongside the flag.

**Resolution**: Pending
**Notes**:

---

### 2. A stored registration the reader cannot make sense of

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: decide
**Priority**: Important
**Affects**: §3.2 (the stored registration), §6.1 (degradation to a plain shell), §3.4 (the mode column)

**Problem**:
The store is hand-editable by design, and the object form invites exactly the mistakes a hand edit makes: `"resume": "Lazy"`, `"resume": true`, a stray key, or an object that carries settings and no command at all. What the user gets in each case is unstated, and the readings pull in opposite directions. Read an unrecognised mode as eager and one typo turns the install's whole restored set back into processes launching at boot — the cost the feature exists to remove. Read it as a hard error and a single bad character can fail a pane's restore, or the whole file's. Read a commandless object as a registration and the panel offers to run nothing.

**Proposal**:
An unreadable attribute never fails a pane. A `resume` attribute that is absent, empty, or holds anything other than `eager` or `lazy` carries no mode: the registration inherits the install-wide default exactly as a string-form entry does, and the mode column reads empty for it. An object carrying no command, or an empty one, is not a registration at all: the pane falls through to a plain shell, which is the degradation an absent entry already takes. What leaned it: the same setting in `prefs.json` already resolves anything it cannot read to the shipped default, and the resume path's existing answer to a lookup it cannot satisfy is a usable pane rather than a failed one. Alternatives that also fit the record: treating an unrecognised mode as eager (matching today's unconditional firing), or refusing the entry loudly at restore so the typo is noticed rather than absorbed.

**Proposed Text**:
Append to §3.2, after the paragraph beginning "Neither shape is legacy":

**A stored value the reader cannot make sense of never fails a pane.** An object whose `resume` attribute is absent, empty, or holds anything other than `eager` or `lazy` carries no mode — the registration inherits the install-wide default exactly as a string-form entry does, and the mode column (§3.4) reads empty for it. An object carrying no command, or an empty one, is not a registration: the pane falls through to a plain shell as an unregistered pane does (§6.1). Nothing is rewritten to correct either case; the file stays as the user left it.

**Resolution**: Pending
**Notes**:

---

### 3. Answering the panel can cost the user the screenful they were last reading

**Source**: Specification analysis
**Category**: Enhancement to existing topic
**Move**: settled
**Priority**: Important
**Affects**: §7.2 (the freeze and when it is released), §6.1 (Enter), §6.2 (discard)

**Problem**:
When the user answers, two things happen: the pane's protection from the saver is dropped, and the pane leaves the panel's screen to reveal the transcript underneath. The order between them is unstated. Dropped first, the pane sits for a moment unprotected while the card is still up, and a saver tick landing in that moment rewrites the pane's saved transcript as history-minus-its-last-screenful with a picture of the card over the end — the exact loss the freeze was built to prevent, now reachable on the path every single resume takes. The user finds out at the next reboot, when the work they were last reading is missing from the pane and a dead card sits where it was.

**Proposal**:
The pane leaves the panel's screen first, and the protection is dropped only once the pane is back on its own transcript — on both answers. Determined by the measurement that a capture taken while the panel's screen is up returns the transcript minus its last screenful plus the card, and by the decision that no moment may exist in which a pane is both unprotected and showing the card; the mirror of the same rule already governs the start of the wait.

**Current**:
**It is cleared when the user answers** — on Enter before the hook runs, and on a confirmed discard before the pane falls through to a shell (§6).

**Proposed Text**:
**It is cleared when the user answers** — on Enter before the hook runs, and on a confirmed discard before the pane falls through to a shell (§6) — and on both paths only once the pane has left the panel's screen and is showing its own transcript again. Clearing while the card is still up leaves a window in which a single saver tick rewrites the pane's saved transcript as history-minus-its-last-screenful plus the card (§7.1) — the whole failure, in the space between two steps, on the path every resume takes. This is the mirror of the rule that sets the pending marker before the mid-restore one is cleared (§7.3).

**Resolution**: Pending
**Notes**:

---

### 4. A discard that removes nothing, and a discard that cannot be written

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: decide
**Priority**: Important
**Affects**: §6.2 (confirmed discard), §6.4 (what the discard records)

**Problem**:
A confirmed discard is a write to a file another process can be holding, and the entry it means to remove can already be gone — days can pass between the panel being drawn and the key being pressed, and in that time the entry can be removed by the CLI, replaced by a re-registration, or hand-edited away. What the user is told, and where the pane lands, is unstated for both. If a failed write silently drops the pane to a shell, the user believes the registration is gone, and the panel is back after the next reboot with nothing having explained why — which reads as the feature ignoring them, and invites them to discard the same pane again.

**Proposal**:
A discard that removes nothing because the entry has already gone is treated as done: the marker clears and the pane falls through to a shell, because the state the user asked for is the state on disk. A discard that cannot be written — the store unreadable, or the lock unavailable — leaves the pane waiting and says so on the panel, so what the screen claims and what the disk holds never disagree, and the user can press the key again. What leaned it: a discard is irreversible and the entry is the only copy of a user-authored command, so the one outcome worth refusing is the user believing something was removed that was not. Alternative that also fits the record: falling through to a shell on a failed write as well, treating the panel's disappearance as the answer and the panel's return next boot as the correction.

**Proposed Text**:
Append to §6.2, after the paragraph beginning "A confirmed discard removes the pane's resume registration":

**A discard that finds nothing to remove is still a discard.** If the entry has already gone — removed by `portal hook rm`, replaced by a re-registration, or hand-edited away while the pane waited — the marker clears and the pane falls through to a shell as it would after a removal, because the end state the user asked for is the state the store is already in. **A discard that cannot be written leaves the pane waiting and says so on the panel**: an unreadable store or an unavailable lock is reported in place, the registration and the marker both stand, and the key can be pressed again. The one outcome ruled out is a pane that drops its panel while the registration it named survives.

**Resolution**: Pending
**Notes**:

---

### 5. The command is the only thing identifying the pane, and it does not fit

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: decide
**Priority**: Important
**Affects**: §5.3 (the waiting panel's body), §5.4 (the discard confirmation), §5.2 (card geometry)

**Problem**:
The command is the only content on the panel that says which piece of work this pane is holding, and a real one is long — the measured install's entries run to a quoted directory path plus a `--resume <id>` tail. The card is the existing rename modal's geometry, sized for a session name. What the user sees when the command runs past the card's inner width is unstated, and each way of handling it loses something different: cut at the tail and the id that distinguishes one waiting pane from the next is gone, cut at the head and the directory that says which project it is goes instead, left alone and it breaks the frame it is drawn in. The same string is rendered again on the discard confirmation, where the user is being asked to destroy it permanently and has nothing else to go on.

**Proposal**:
The command wraps within the card's inner width over at most three lines, with anything beyond that marked with an ellipsis, and the discard confirmation renders it the same way. What leaned it: the command is the panel's only identifying content and the discard is irreversible, so the whole of it should be readable before either key is pressed, and three lines holds a realistic registration whole without the card growing to the size the canvas-plus-card shape exists to avoid. Alternatives that also fit the record: one line cut at the tail (compact, loses the id), or one line cut at the head (keeps the id, loses the path).

**Proposed Text**:
Append to §5.3, after the bullet list:

**A command longer than the card wraps rather than being cut.** The registered command is the only thing on the panel that says which piece of work the pane is holding, and a realistic one carries a directory and an identifier. It wraps within the card's inner width over at most three lines, with anything beyond marked `…`; the card's width is unchanged. The discard confirmation renders the command the same way (§5.4).

**Resolution**: Pending
**Notes**:

---

### 6. A pane smaller than the card

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: decide
**Priority**: Important
**Affects**: §5.2 (the canvas and the card), §5.3 (the waiting panel)

**Problem**:
Restored panes come back at whatever size the user left them, and a narrow split or a short one can be smaller than the card the panel centres on it. What that pane shows is unstated, and the failure is quiet in the worst way: a pane that declines to draw looks like an ordinary restored pane with its transcript in place, while silently swallowing everything the user types into it. They have no sign that the pane is holding a decision, no clue that Enter answers it, and a keyboard that appears to be broken.

**Proposal**:
Below the size the card needs, the panel degrades rather than disappearing: the canvas is still painted and the title, the command and the key hints stack plainly without the card frame, down to the smallest pane — and Enter and `d` act at every size. What leaned it: a pane that swallows keys must always say what it is waiting for, and the canvas alone is what distinguishes a waiting pane from a restored one. Alternative that also fits the record: painting the canvas with the key hints alone below the floor, dropping the command, which keeps the smallest pane legible at the cost of not saying what would run.

**Proposed Text**:
Append to §5.2, after the paragraph beginning "The overlay fills the pane":

**A pane too small for the card still says what it is.** Below the size the card needs, the panel degrades instead of disappearing: the canvas is painted as always, and the title, the command and the key hints stack plainly without the card frame, down to the smallest pane a restore can produce. Enter and `d` act at every size. A waiting pane swallows every other key (§4.3), so one that drew nothing would read as an ordinary restored pane with a dead keyboard.

**Resolution**: Pending
**Notes**:

---

### 7. Which keys act while the discard confirmation is up

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: decide
**Priority**: Important
**Affects**: §6.2 (the confirmation), §6.3 (key choices), §4.3 (the swallow rule)

**Problem**:
Once the confirmation is up, whether the keys from the screen underneath still act is unstated. Enter resumes one keystroke earlier, so a user who presses `d` and then reflexively Enter — the reflex every confirmation dialog in the world trains — either gets the session resumed, which is the opposite of what they just asked for and leaves the registration in place, or gets nothing and has to work out which key the screen wants. The reasoning behind choosing `y` over Enter says the same key must not mean "bring it back" and "delete it forever" on consecutive screens; what Enter means on the second screen is the part still open.

**Proposal**:
While the confirmation is up, `y` and Escape are the only keys that act. Enter, `d` and everything else are swallowed exactly as they are on the waiting panel, so the pane never acts on a key the screen does not offer. What leaned it: the swallow rule exists so nothing accidental can answer the panel, and a live Enter on the confirmation is precisely the accidental answer the two screens' key choices were arranged to avoid. Alternative that also fits the record: Enter stays live throughout and resumes from the confirmation too, on the grounds that resuming is the non-destructive answer and backing out to it is what Escape does anyway.

**Proposed Text**:
Append to §6.2, after the paragraph beginning "`d` opens a second confirmation":

**While the confirmation is up, `y` and Escape are the only keys that act.** Enter, `d` and everything else are swallowed there exactly as they are on the waiting panel (§4.3) — the pane never acts on a key the screen in front of the user does not offer, and the reflex of confirming with Enter costs nothing but a second press of the key the footer names.

**Resolution**: Pending
**Notes**:

---

### 8. The rule that a waiting pane survives in the saved set is written twice

**Source**: Specification analysis
**Category**: Duplication
**Move**: settled
**Priority**: Important
**Affects**: §9.1 (what was measured and needs nothing), §7.2 (the freeze)

**Problem**:
Whether a pane that is frozen for weeks still gets restored is the property the whole waiting state depends on — get it wrong and a user who leaves a pane paused loses the session entirely at the next reboot. The rule is stated in full twice, in the freeze section that decides it and again in the restore-pipeline survey, each with its own wording and the same citation. Two full statements of one guarantee can be revised apart without anything catching it, and the half that ends up wrong is the half the implementer happens to read.

**Proposal**:
The rule keeps its home in §7.2, where the freeze is decided and its consequences belong, and the restore-pipeline survey carries a one-line statement with a reference rather than a second full account.

**Current**:
**A frozen pane is not dropped from the saved set.** The freeze suppresses that pane's scrollback write; the structural capture merges the pane's *previous* record back into the fresh index rather than omitting it, guarded so that a stale marker cannot resurrect a pane whose session, window or pane is gone (`internal/state/capture.go:96-127`). A pane can wait indefinitely and still be restored on the next boot, with the transcript it had when it paused.

**Proposed Text**:
**A frozen pane is not dropped from the saved set.** The freeze suppresses that pane's scrollback write and nothing else, so a pane can wait indefinitely and still be restored on the next boot with the transcript it had when it paused (§7.2).

**Resolution**: Pending
**Notes**:

---

## Observations

- A resize while the discard confirmation is up: whether the redraw comes back on the confirmation or on the waiting panel is unstated (§4.2, §5.4).
- The help modal gains the indicator legend in the same change, but its wording is not given, while the panel's own copy is specified string by string (§8.3, §5.2).
- The picker's pending read is costed at one whole-server enumeration, but when it is taken relative to a sessions-list refresh is unstated — a dot answered in another window could linger for the life of an open picker (§8.2, §8.3).
- What doctor's pending line reads when the count is zero is unstated (§8.1).
- The ordering rule "the pending marker is set before the mid-restore marker is cleared" is stated in full, with its justification, in both §7.3 and §8.2.
- Under an adaptive light/dark pair, restore draws with no client attached to almost every pane, so the dark fallback is the common case at boot rather than the edge §5.2 presents it as; nothing re-resolves it when a client later attaches (§5.2, §4.4).
- What `portal hook set` does with a `--resume-mode` value that is neither `eager` nor `lazy` is unstated (§3.3).
