# Review Tracking: Open With Forced Filter - Gap Analysis

## Findings

### 1. The text the fuzzy routes match against is never fixed

**Source**: Specification analysis
**Category**: Enhancement to existing topic
**Move**: settled
**Priority**: Important
**Affects**: §4.3 (The matching rule), §4.4 (The rule diverges by entry point)

**Problem**:
`-f` and a hand-typed picker filter are required to match the session name and the recorded directory as one joined run of text, so that a term can be found across the two. Nothing says how the two values are joined — whether the directory follows the name or precedes it, and what sits between them. The choice is not cosmetic: it decides which cross-field matches exist at all, and the stock matcher ranks matches partly on what follows a separator, so two builders making different choices produce two different orderings of the same filtered list from the same typed text. A builder also has nothing to go on for a session that carries no recorded directory, where a naive join leaves a dangling separator on the end of the matched text.

**Proposal**:
Fix the joined form as the name, one space, then the recorded directory in its home-abbreviated form — the same text the row itself puts on screen (§6.2 places the directory one space after the name). That derivation is the specification's own principle that what was matched is what is shown (§4.1), and it settles the empty case: a session with no recorded directory has nothing to append, so its matched text is its name alone.

**Current**:
**On the two fuzzy routes the fields are joined**, as the picker's stock matcher expects, so a cross-field match is possible there — the first letters of the term found in the name and the rest in the path. That is accepted: those routes always show their rows, their rule was already the loose one (§4.4), and separating the fields would cost the list its ranking through the stock matcher.

**Proposed Text**:
**On the two fuzzy routes the fields are joined**, as the picker's stock matcher expects, so a cross-field match is possible there — the first letters of the term found in the name and the rest in the path. **The joined text is the session name, one space, then the recorded directory in its home-abbreviated form** — character-for-character the row the user is shown (§6.2), so what the matcher scores and what the eye reads are one string; a session carrying no recorded directory joins to its name alone, with no trailing separator. That is accepted: those routes always show their rows, their rule was already the loose one (§4.4), and separating the fields would cost the list its ranking through the stock matcher.

**Resolution**: Approved
**Notes**: Applied to §4.3 verbatim.

---

### 2. Nothing keeps the derived directory out of the slot the match and the column read

**Source**: Specification analysis
**Category**: Enhancement to existing topic
**Move**: settled
**Priority**: Important
**Affects**: §4.1 (The matched fields), §6.1 (The requirement)

**Problem**:
Two directory values can exist for one session while the search list is on screen: the one recorded at creation, and the one the grouped views work out by asking a pane where it is and then keep. The search and the directory column are required to read only the recorded one, so that a regroup cannot make a session findable by a path it was not findable by a moment earlier, or put a path beside a name that showed none. But the requirement is stated only as an outcome — nothing says the two values are held apart. A builder who lets the derived value settle into the session's one directory slot satisfies grouping and silently breaks both guarantees: the column starts showing paths that were never searched, and the answer to the same term changes depending on which view the user last regrouped into.

**Proposal**:
State the separation that makes the outcome reachable — a session carries the recorded directory and any derived directory as two distinct values, the derived one readable by grouping and by nothing else. That is what §4.1 already demands in effect ("neither the match nor the directory column ever reads it") and the only means by which it can hold once a derived value exists.

**Current**:
This holds *within* a single picker as well as across launches. The grouped views derive a missing directory while the sigil's own narrowed list is on screen, and retain what they derive; that value belongs to grouping and to nothing else. Neither the match (§4.4) nor the directory column (§6.1) ever reads it, so a regroup can never make a session findable by a path it was not findable by a moment earlier, and can never put a path beside a name that showed none.

**Proposed Text**:
This holds *within* a single picker as well as across launches. The grouped views derive a missing directory while the sigil's own narrowed list is on screen, and retain what they derive; that value belongs to grouping and to nothing else. **A session therefore carries the two as separate values — the recorded directory, which may be absent, and the derived one, which grouping alone reads and writes. A derived value never lands in the recorded one.** Neither the match (§4.4) nor the directory column (§6.1) ever reads it, so a regroup can never make a session findable by a path it was not findable by a moment earlier, and can never put a path beside a name that showed none.

**Resolution**: Approved
**Notes**: Applied to §4.1 verbatim.

---

### 3. The completion fix silently ends filename completion after `x`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §8.3 (The correction), §8.5 (Rollout consequence)

**Problem**:
Today Tab after the session-opening function falls through to the shell's own filename completion, because Portal is never asked and the shell has nothing else to offer — which is how `x ~/Code/pro<TAB>` completes a directory to mint in. Once the request is corrected to reach Portal, Portal answers, and Portal's answer switches the filename fallback off. Every path argument to `x` therefore stops completing on the day the corrected `portal init` output is evaluated. A builder is given no verdict on that: they will either treat the loss as a regression and reintroduce a filename fallback — which would let `x /tm<TAB>` complete to `/tmp/` and turn a search into a mint, the exact outcome the recognition rule is built to prevent — or ship the loss with no line in the documentation the section already obliges.

**Proposal**:
Take the loss deliberately and say so: after the correction the session-opening function completes exactly as `portal open` already does through the control function, live session names and no filenames. The specification already fixes that contract — Portal switches the shell's filename fallback off, which is what guarantees `x /tm<TAB>` never becomes `/tmp/` — so preserving a filename fallback for the same word is not available, and the correction is aligning the two functions rather than choosing a new behaviour.

**Proposed Text**:
New paragraph, appended to §8.3:

The correction brings the session-opening function onto `open`'s existing completion contract, which the control function already gets: live session names, and no filename fallback. Path arguments — `x ~/Code/pro<TAB>` — complete to filenames today only because Portal is never asked and the shell has nothing else to offer; once Portal answers, it switches that fallback off (§2.2), and it must, since a filename fallback on the same word is what would turn `x /tm<TAB>` into `/tmp/` and a search into a mint. The loss is taken deliberately and is the same behaviour the control function has always had.

New bullet, in §9.2's list:

- That the corrected completion offers live session names after the session-opening function, and no longer falls through to filenames for a path argument (§8.3).

**Resolution**: Approved
**Notes**: Applied to §8.3 and §9.2 verbatim.

---

### 4. The count is said to precede any picker, which a cold boot contradicts

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Minor
**Affects**: §4.1 (The matched fields)

**Problem**:
The reason given for matching only the recorded directory is that the match count is taken before any picker exists, so no derived value could be had or kept. On a cold machine that is not what happens: the form is classified as a picker invocation, the loading page is painted immediately, and the count is only taken once the whole bootstrap has run — with the picker process already on screen (§3.4, §7.1). A builder reading the reason as a statement of sequence can conclude the count must be computed before the interface starts, which is the one arrangement the cold path forbids. The argument itself survives intact — no session row has been rendered at that point, so there is still nowhere for a derived value to come from.

**Proposal**:
Anchor the sentence to the fact that carries the argument — no session row has been rendered when the count is taken — rather than to the absence of a picker, which the cold path disproves.

**Current**:
The shell form has it worse: K (§3.2) is taken before any picker exists, so a derived value has nowhere to come from and nowhere to be kept — the recorded directory that rides back with the session list is the only one there is.

**Proposed Text**:
The shell form has it worse: K (§3.2) is taken before a single session row has been rendered — on a cold machine while the loading page still stands (§3.4) — so a derived value has nowhere to come from and nowhere to be kept; the recorded directory that rides back with the session list is the only one there is.

**Resolution**: Approved
**Notes**: Applied to §4.1 verbatim.

---

### 5. The directory's only separation from the name is a colour the colourless render does not have

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Minor
**Affects**: §6.2 (Placement and weight)

**Problem**:
The directory is packed one space after the session name and told apart from it by weight alone — a muted colour against the name's. Under the colourless render there is no weight, and the row becomes one undifferentiated run of text with a space in it. A builder has no verdict for that case and will invent one: brackets, a bullet, a dash, or an alignment column, each of which then diverges from the coloured row and from whatever the next builder invents.

**Proposal**:
The row is unchanged: one space, no added glyph, no bracket, no column. The established rule is that *state* stays glyph-backed and never colour-only; a directory is an annotation on a name rather than state, nothing is lost when its weight is, and the path's own separators already read as a path.

**Proposed Text**:
New paragraph, in §6.2, after the paragraph beginning "Balance comes from colour rather than alignment":

Where colour is off entirely the row is unchanged — the same single space, no bracket, glyph or separator introduced to stand in for the weight. The established carve-out asks that *state* never be carried by colour alone; a directory annotates the name rather than reporting state, and a path is legible as a path by its own separators.

**Resolution**: Approved
**Notes**: Applied to §6.2 verbatim.

---

### 6. The bare slash's landing is restated where the subject is the count

**Source**: Specification analysis
**Category**: Duplication
**Move**: settled
**Priority**: Minor
**Affects**: §3.2 (Outcomes by match count)

**Problem**:
What a slash on its own puts on screen — the picker with its filter open, empty and focused — is fixed in §2.5, the section that owns the term-less form. The passage that establishes no count is taken repeats that landing beside the reference to its home. Two statements of one fact drift apart under later editing, and the copy here is the one nobody would think to update: a change to how the term-less form lands would be made where the form is defined, leaving this passage quietly wrong about what the user sees.

**Proposal**:
Keep the fact this passage owns — that no count is evaluated for a term-less sigil, on a machine with one live session as on a machine with twenty — and let the reference carry the landing.

**Current**:
**A term-less sigil takes no count.** `x /` carries nothing to match, so no count is evaluated and none of the rows below apply: it opens the picker on the whole live session list, filter focused and empty (§2.5), on a machine holding one live session as on a machine holding twenty.

**Proposed Text**:
**A term-less sigil takes no count.** `x /` carries nothing to match, so no count is evaluated and none of the rows below apply: it opens the picker on the whole live session list as §2.5 sets it, on a machine holding one live session as on a machine holding twenty.

**Resolution**: Approved
**Notes**: Applied to §3.2 verbatim.

---
