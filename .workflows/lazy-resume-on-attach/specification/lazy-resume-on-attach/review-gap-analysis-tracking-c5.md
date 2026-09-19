# Review Tracking: lazy-resume-on-attach - Gap Analysis

## Findings

### 1. A waiting pane that loses its process loses the whole session with it

**Source**: Specification analysis
**Category**: Enhancement to existing topic
**Move**: decide
**Priority**: Critical
**Affects**: §4.3 (the waiter is the pane's only process), §7.2 (the freeze and what a stranded marker costs), §9.1 (a waiting pane keeps its place in the saved set)

**Problem**:
A pane that is waiting on a resume has one process in it, and that process is now the pane's life support. The record decides what the pane does when the user presses a key at it and when tmux tears it down, and leaves open the third case: the process going away on its own. Every other way a restored pane holds itself open survives this — an eager pane that finishes its command falls back to a shell and the pane stays — but a waiting pane's process is the whole of it, so the pane closes, and on an install where 43 of 44 live sessions hold exactly one pane, the session closes with it. The next capture drops that session out of the saved set with its whole transcript, and nothing reports it. A single `pkill portal` — the sort of thing anyone chasing a stuck daemon types — would take every waiting pane on the machine in one go: forty-odd sessions and every line of their history, unrecoverable, and the user finds out at the next reboot when the picker comes back nearly empty. Today the same command costs a daemon that respawns itself.

**Proposal**:
The pane outlives its waiter. If the waiter goes for any reason other than tmux tearing the pane down, the pane stays open and comes back to the panel, with the pending marker it is still carrying left in place and still true. What leaned it: the waiting state is decided as a property of the pane throughout — it survives detach, it survives reboot, it keeps its place in the saved set, and the marker that protects it is deliberately carried by the pane rather than by anything that can move — so the one thing that should not be able to end it is the loss of a process that merely holds it. Alternatives that also fit the record: the pane closes as any pane whose process exits does, accepting that a single-pane session and its transcript go with it, on the grounds that the waiter already refuses every signal a keyboard can send; or the pane comes back as a plain shell, which keeps the session but strands the pending marker with nothing left to clear it, freezing that pane's saved transcript for the rest of its life.

**Current**:
**The waiter is the pane's only process, so anything that kills it takes the pane with it** — Ctrl-C, Ctrl-D, Ctrl-Z. It must refuse to die rather than exit.

**Proposed Text**:
**The waiter is the pane's only process, so anything that kills it would take the pane with it** — Ctrl-C, Ctrl-D, Ctrl-Z. It must refuse to die rather than exit.

**A pane does not lose its session because its waiter died.** If the waiter goes for any reason other than tmux tearing the pane down — killed from outside, swept up by a `pkill portal`, or gone of its own accord — the pane stays open and comes back to the panel, carrying the pending marker it never lost. The wait is the pane's state and the process only holds it, which is the same reason the marker is carried by the pane rather than by anything that can move (§7.3). Letting the pane close instead would take a single-pane session with it, and 43 of the 44 live sessions on the measured install hold exactly one pane (§1): the session is destroyed, the next capture drops it from the saved set with its whole transcript, and nothing anywhere says it happened. Coming back as a plain shell instead would leave the pending marker standing with nothing left to clear it, which freezes that pane's saved transcript for the rest of its life (§7.2).

**Resolution**: Routed
**Notes**: Landed in the discussion as the subtopic When The Waiter Itself Goes Away, carrying the derivation marker; §4.3 aligned to it. The call diverges from the staged proposal: rather than the pane outliving its waiter and returning to the panel (nothing is watching to respawn it, and a held-open dead pane is the design the key-scoping measurement ruled out), the waiter runs as the tail of a marker-clearing chain that execs the shell — the hydrate helper's existing shape.

---

### 2. A line of typed or pasted text can destroy a registration outright

**Source**: Specification analysis
**Category**: Contradiction
**Move**: decide
**Priority**: Important
**Affects**: §4.3 (the swallow rule and the safety property claimed for it), §6.2 (the confirmation), §6.3 (the key choices)

**Problem**:
The document says two incompatible things about whether input the user did not deliberately aim at the panel can destroy anything. The swallow rule states outright, as the safety property it exists to deliver, that a stray paste, an errant `send-keys` or a key pressed in the wrong window cannot answer the panel — nothing but two keys means anything to it. The keys themselves say otherwise. `d` opens the destructive confirmation and `y` completes it, and both letters are ordinary characters in ordinary text: a line sent to the wrong pane, or pasted into one, that happens to carry a `d` and then a `y` — `cd ~/dev && yarn`, among countless others — walks the whole discard through without a human ever seeing either screen. What goes is the only copy of a user-authored resume command, permanently, with no route back; the user learns of it at the next reboot, when the pane comes back as a bare shell and the work it was holding has no way to start. A builder reading the safety property believes this is already impossible and writes nothing that makes it so.

**Proposal**:
The confirm key answers a confirmation the user has actually seen: input that was already in flight when the confirmation opened is dropped rather than treated as agreement, so completing a discard takes a key pressed after the screen was in front of the user. Keys remain exactly as decided — Enter and `d` on the panel, `y` and Escape on the confirmation — and the safety property is restated as what it really delivers. What leaned it: the confirmation exists so that destroying a registration is a deliberate act, and a `y` that arrived before the question did was never an answer to it; an accidental Enter costs the user a process starting early, which the next reboot undoes, while an accidental discard costs work that cannot be recovered. Alternatives that also fit the record: leave the keys untouched and narrow the safety claim to what it delivers, accepting that a run of text can discard; or hold the confirmation inert for a short moment after it is drawn, so anything arriving in a burst is ignored by timing rather than by origin.

**Current**:
The rule that falls out is a safety property as much as a mechanism: a stray paste, an errant `send-keys`, or a key pressed in the wrong window cannot answer the panel, because nothing but Enter and `d` means anything to it.

**Proposed Text**:
Replacing that sentence in §4.3:

The rule that falls out is a safety property as much as a mechanism: nothing but Enter and `d` means anything to the panel, so a stray paste, an errant `send-keys`, or a key pressed in the wrong window has nothing to reach beyond those two. The worst it can reach is a resume — a process started sooner than the user meant, which the next reboot offers again. It cannot reach the discard, which is completed only from the confirmation and only by a key pressed after that screen was drawn (§6.2).

Appended to §6.2, after the paragraph beginning "While the confirmation is up, `y` and Escape are the only keys that act":

**`y` answers a confirmation the user has seen.** Input already in flight when `d` opened the confirmation is dropped rather than read as agreement, so a discard is completed only by a key pressed after that screen was in front of the user. Both letters are ordinary characters in ordinary text, and a line sent or pasted into the wrong pane that carries a `d` and then a `y` would otherwise walk the whole discard through with nobody having seen either screen — destroying the only copy of a user-authored command, permanently. The confirmation is there to make the act deliberate (§6.4); a key that arrived before the question did was not an answer to it.

**Resolution**: Routed
**Notes**: Landed in the discussion as the subtopic Input Already In Flight, carrying the derivation marker; §4.3 aligned to it.

---

## Observations

- The choice of a pane-scoped marker, and the five-operation measurement behind it, is stated in full in both §7.3 and §8.2 with the same citation.
- The card's report-row wording is unstated, while every other string the panel renders is given word for word (§5.3, §5.2).
- The report row's colour is unstated, while the surface is required to hold no raw hex and take every colour from a token (§5.3, §5.2).
- Whether `D` and `Y` act as `d` and `y` do is unstated (§6.2, §6.3).
- A resize redraw days into a wait paints the command as it was drawn, while Enter runs what the store holds at that moment, so the card can name one command and start another (§4.2, §6.1).
- The pending state is described as recomputed each boot "from a registration that has not fired", while a registration the user resumed on the previous boot is equally unfired on the next one (§9.2, §4.4).
