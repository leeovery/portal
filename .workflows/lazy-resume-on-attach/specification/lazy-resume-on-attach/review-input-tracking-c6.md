# Review Tracking: Lazy Resume On Attach - Input Review

## Findings

### 1. A waiter that dies and cannot lift the freeze leaves a frozen pane and a lying indicator

**Source**: No source covers this. `When The Waiter Itself Goes Away` (discussion) settles that a dead waiter's chain clears the pending marker and then execs the user's shell, and addresses no failure of that clear; `When The Marker Cannot Be Written Or Cleared` covers a failed clear only on the two answer paths, where a live waiter is there to hold the answer. The intersection — the waiter is gone, so nothing can hold anything — is a blind spot in both.
**Category**: Gap/Ambiguity
**Move**: decide
**Affects**: §4.3 (a waiter that exits drops the pane to a plain shell), §7.3 (the inverse failure and its enumeration of routes), §8.2 (what the marker's failures record)

**Problem**:
A waiting pane whose program is killed by name, crashes, or is reclaimed comes back as a usable shell — but lifting the capture freeze on the way out is a tmux write, and the specification says nothing about it failing. When it does, the user keeps working in that pane for weeks while its saved transcript stands still at the moment it paused: every reboot restores the stale content, and the real work is never saved. Two surfaces actively mislead them meanwhile — the picker row goes on showing a pending-resume dot and `portal doctor` goes on counting the pane, both promising a decision the pane no longer holds; walking into it finds an ordinary shell. Nothing anywhere names the state, and there is no route back to a saving pane short of destroying it.

The specification currently tells an implementer this cannot happen: it states outright that a marker wrongly left set has no route left open, and enumerates the routes to a live wrongly-frozen pane as two, both closed. A `pkill portal` across a full waiting set is one command, so the case is reachable in bulk on the exact install the feature is built for.

**Proposal**:
The pane must still get its shell — a pane left closed takes its session and transcript with it, which is the whole reason the drop-to-a-shell rule exists — so the handover runs whether or not the clear succeeded. What the record leaves open is whether the failure is recorded: the call is to record it exactly as the marker that could not be *written* is recorded (one WARN on the helper's existing hydrate catalog naming the pane and the error), because the specification's own reason for that line is that a degradation the user cannot read off the pane in front of them must leave a trace, and this one is worse — the pane it leaves behind reads as healthy and its two indicators read as wrong. The enumeration of routes to a live wrongly-frozen pane then names three rather than two, the third reachable but recorded.

Alternatives that also fit the record: say nothing, since no source asks for a line here and the surviving dot is arguably its own signal — rejected because the dot says the opposite of what is true; or retry the clear before handing over, which changes the odds and not the end state, and still owes the same answer when the retries run out.

**Proposed Text**:

In §4.3, after the paragraph beginning "**A waiter that exits without having handed the pane over drops the pane to a plain shell.**":

> **The chain hands the pane over whether or not the marker came off.** A pane left closed is the outcome this rule exists to prevent, so a clear that fails does not hold the shell back. What the pane is left holding is stated rather than hidden: a working shell whose saved scrollback the saver will not rewrite again, still carrying a pending dot in the picker and a place in doctor's count for a decision it no longer holds. The failure is recorded the way a marker that could not be written is (§8.2) — one WARN on the helper's existing hydrate catalog, naming the pane and the error that refused the clear — because it is the one state the pane in front of the user reads as healthy.

In §7.3, replacing the paragraph beginning "**The inverse failure — a marker wrongly left set, freezing a pane's saved content forever — has no route this feature leaves open.**":

> **The inverse failure — a marker wrongly left set, freezing a pane's saved content forever — is reachable on one route only, and that route is recorded.** The marker lives on the pane and the pane's only process is the waiter, so the marker goes when the pane goes. Two routes to a live pane whose marker is wrong are closed outright: an answer whose clear failed is held rather than carried out, so the pane goes on waiting and the marker is still the truth (§7.2), and a restore never respawns a waiting pane, because it skips any session that is already live (§9.1). The third is a waiter that died and could not lift the freeze on its way out — it leaves a working shell on a frozen pane and a WARN naming it (§4.3). What is left is a pane respawned out from under its waiter by hand — the user destroying the process that held that pane's state, in the same class as a hand edit of the store.

**Resolution**: Routed
**Notes**: The call landed in the discussion, extending When The Waiter Itself Goes Away; §4.3 aligned to it. The retry question was left to the builder explicitly; the record was not.

---

## Observations

- The `prefs.json` key `resume_mode` (§3.1) is named by the specification alone — the sources put the setting in `prefs.json` and never name the key. It is the only way to change an install-wide default that has no UI, so the string is user-facing, but any name works once the specification fixes one.
- The sources state that a pane which fell through to eager because its marker could not be written "reads zero in the pending count"; §8.2 keeps the indistinguishable-from-configured-eager consequence and drops that detail.
- §1 carries the pane-count command behind its 43×1-pane figure but not `portal hook list | wc -l` behind the registration counts; the figures themselves are all present and dated.
- Neither the sources nor the specification names a documentation surface for `--resume-mode`, the new `hook list` column, or `resume_mode` — the feature ships on by default and its off-switch is a hand-edited key.
- The store read the waiting program takes *before* it draws (§6.1, "to know whether to draw at all") has no stated failure behaviour of its own; the general rule that a lookup failure gives a bare shell covers it by implication.
