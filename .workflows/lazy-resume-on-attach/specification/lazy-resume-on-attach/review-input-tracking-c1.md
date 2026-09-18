# Review Tracking: Lazy Resume On Attach - Input Review

## Findings

### 1. The panel's copy is never required to be tool-agnostic

**Source**: `discussion/lazy-resume-on-attach.md` — Inherited position: "**The prompt's copy is generic.** Portal's resume machinery is tool-agnostic and its messaging must stay so — the prompt states the command without naming Claude or any particular tool."
**Category**: Enhancement to existing topic
**Move**: settled
**Affects**: §5.2 (the panel's canvas and card), with consequences for §5.4's confirmation copy and §8.3's help-modal legend

**Problem**:
Portal runs whatever command a registration holds — a dev server, a tunnel, an editor, an agent — and its messaging has to stay neutral about which. The new surface leaves two user-visible strings unwritten: the discard confirmation's plain-language consequence line, and the help modal's indicator legend. Nothing tells whoever writes them that they must name no particular tool, and the panel is being built from an install where every registration happens to launch the same one. A user whose resume command is a dev server would read a confirmation telling them a coding agent will not come back — copy that is wrong for them and that has to be reverted once someone notices.

**Proposal**:
Carry the constraint into the panel's design rules, beside the no-raw-hex rule that already sits there: every string the panel and its confirmation render is tool-agnostic, stating the command and nothing about what the command is, and the same holds for the legend that ships with the indicator.

**Current**:
Every colour is a theme token, as everywhere else in Portal — the panel holds no raw hex.

**Proposed Text**:
Every colour is a theme token, as everywhere else in Portal — the panel holds no raw hex.

**Every string this surface renders is tool-agnostic.** Portal's resume machinery runs whatever command a registration holds, so nothing the panel shows names a particular tool — it states the command and says nothing about what the command is. That covers the discard confirmation's consequence line (§5.4) and the indicator legend that ships with the picker's pending dot (§8.3) as much as the panel itself.

**Resolution**: Pending
**Notes**:

---

### 2. Nothing says only a waiting pane is marked pending

**Source**: `discussion/lazy-resume-on-attach.md` — Pending Visibility ("The helper sets the marker before it clears the mid-restore marker") read against Eager Lazy Preference ("A registration carries eager, lazy, or nothing … an entry that sets either one holds that choice regardless of what the install says")
**Category**: Enhancement to existing topic
**Move**: settled
**Affects**: §8.2 (the explicit pending marker), §7.3 (the freeze held by the pane)

**Problem**:
The rule for setting the pending marker is written flat — the helper sets it before it clears the mid-restore marker, before it paints — with no condition attached. A pane pinned to resume automatically never paints and never holds a waiting process, so nothing on that pane ever clears a marker set on it. Built to the rule as written, every automatically-resuming pane comes back from a reboot carrying a marker for the rest of its life, and the saver then refuses to save that pane's scrollback for the rest of its life. The user works in that pane for weeks; the next reboot restores it to the content it had at the moment it was first restored, and every hour of work in between is gone with no copy anywhere. Nothing surfaces it while it is happening — the pane looks normal, and the loss only appears at the next boot.

It also quietly undoes the argument that a marker can never be wrongly left set, which holds only because the marked pane's sole process is the waiter and dies with the pane.

**Proposal**:
State the condition: the helper resolves the pane's mode before it clears the mid-restore marker, and marks only a pane that is going to wait. A pane with no registration, and one resolving to automatic resume, is never marked and is captured exactly as it is today.

**Current**:
The marker is the pane user-option `@portal-resume-pending` (§7.3). **The helper sets it before it clears the mid-restore marker, and therefore before it paints** — so the pane is never unprotected. Resume and a confirmed discard each clear it, both in the waiting program, at the moment the user answers.

**Proposed Text**:
The marker is the pane user-option `@portal-resume-pending` (§7.3). **The helper sets it before it clears the mid-restore marker, and therefore before it paints** — so the pane is never unprotected. Resume and a confirmed discard each clear it, both in the waiting program, at the moment the user answers.

**Only a pane that is going to wait is marked.** The helper resolves the pane's mode (§2) before it clears the mid-restore marker, so a pane with no registration — and one whose registration resolves eager — is never marked and goes on being captured exactly as it is today. That condition is also what holds the unreachability above: the marker only ever lands on a pane whose sole process is the waiter, which dies with the pane. A marker set on a pane that then execs its hook would have nothing left to clear it, and the saver would refuse that pane's scrollback write for the rest of the pane's life.

**Resolution**: Pending
**Notes**:

---

### 3. A session row's pending dot has no stated predicate

**Source**: `discussion/lazy-resume-on-attach.md` — Pending Visibility ("as a single glyph on the picker's session row") and Panel Visual Design ("a pending resume shows as a second dot"); the marker itself is decided as a **pane** option in the same section
**Category**: Enhancement to existing topic
**Move**: settled
**Affects**: §8.3 (the picker's session row), read against §8.1 (doctor's per-pane count)

**Problem**:
Waiting is a property of a pane; the picker's row is a session. The row's new indicator is specified down to its colour, its packing order and its colourless letter, but nothing says what makes a row show it. A session holding two panes, one of them waiting, either shows the dot or does not depending on which way it was built — and built the wrong way, the user scanning the picker for what is waiting sees a clean row over a pane holding an unanswered decision, which is precisely the question this indicator exists to answer. The two surfaces also count in different units, so a user comparing the diagnostic count against the dots down the list gets two different answers with nothing explaining why.

**Proposal**:
Say that a row shows the pending dot when any pane in that session is waiting, and note that the diagnostic count is per pane, so a session holding two waiting panes contributes two to the count and one dot to the list.

**Current**:
**The session row drops the word `attached` and gains a second dot.** The green attached indicator loses its label and stands alone; a pending resume shows as a second dot in `accent.attention`.

**Proposed Text**:
**The session row drops the word `attached` and gains a second dot.** The green attached indicator loses its label and stands alone; a pending resume shows as a second dot in `accent.attention`. **A row carries that dot when any pane in the session is waiting** — the row answers whether the session holds a decision, not how many it holds. Doctor counts panes (§8.1), so a session holding two waiting panes contributes two to that count and one dot to the list.

**Resolution**: Pending
**Notes**:

---

### 4. The panel's theme is unresolved when the install runs a light/dark pair

**Source**: `discussion/lazy-resume-on-attach.md` — Prompt Surface: "painted in the active Portal theme so nothing behind it shows through", "styled from the Portal theme the user has chosen"; Waiting Pane Mechanism: "Drawing touches the theme and the rendering path". The source settles that the panel is themed and never says how the theme resolves outside the picker.
**Category**: Gap/Ambiguity
**Move**: decide
**Affects**: §5.2 (the panel's canvas and card), §4.2 (the draw-then-hand-off split)

**Problem**:
Portal's theme setting is either one named theme or a light/dark pair, and a pair is only half an answer until something decides which half the terminal is. The picker settles that by asking the terminal and falling back to dark, before it paints anything. The panel is painted by a different process, in a pane, at boot, and nothing says what that process does with a pair. Built the cheap way, a user who runs the adaptive pair on a light terminal reboots and every restored pane comes up carrying a dark card on a dark canvas — forty panels at once, all in the half of their own theme they do not use, and no setting in the product that looks like the cause.

**Proposal**:
Resolve it the way the rest of Portal does: a named theme paints from the first frame, and a light/dark pair runs the same detect-or-timeout appearance gate the picker runs — a query to the terminal against a short timeout, dark when there is no answer — in the process that draws. That process hands off before it waits, so the gate is paid once per draw and nothing of it is held while the pane waits. What leaned: the drawing process is already paying the theme and rendering cost this gate rides on, and the failure it prevents lands on every restored pane at the same moment. Two alternatives also fit the record — resolving a pair straight to its dark half with no query, which costs nothing at boot and is wrong on a light terminal; or having the panel ignore the pair and paint from the shipped dark default, which is the same outcome stated as a rule rather than a fallback.

**Proposed Text**:
**The theme resolves as it does everywhere else in Portal.** A named theme paints from the first frame. A light/dark pair runs the same detect-or-timeout appearance gate the picker runs — a query to the terminal raced against a short timeout, resolving dark when there is no answer — in the process that draws. That process hands off before it waits (§4.2), so the gate is paid once per draw and nothing of it stays resident while the pane waits.

**Resolution**: Pending
**Notes**:

---

## Observations

- The names the specification gives the new identifiers are its own — the `resume_mode` key in `prefs.json`, the `--resume-mode` flag, the `@portal-resume-pending` pane option and its `1` value, and the `discard` / `panel` log-vocabulary members. The source decides that each exists and never names any of them.
- The source's framing for why declining is not an escape hatch — "a pane holding a resumable process is *for* that process, and if they wanted a plain shell they would open a new session" — is dropped; §6 carries the conclusion without it.
- The source notes that the resume is not the only route back, since the tool's own resume picker is always available, as part of why a pane keeping its history is strictly more useful. §9.1 carries the scope argument without this one.
- Behaviour of the card in a pane too narrow to hold it is unspecified; Portal has existing precedent for a render floor.
- Validation of an unrecognised `--resume-mode` value, and of an unrecognised `resume` value in a hand-edited store, is unspecified.
- What doctor's pending count reports when no tmux server is running is unspecified; the line passes either way.
