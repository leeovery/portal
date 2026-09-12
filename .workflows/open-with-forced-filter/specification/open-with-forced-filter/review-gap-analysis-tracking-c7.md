# Review Tracking: Open With Forced Filter - Gap Analysis

## Findings

### 1. The text the fuzzy routes match is defined two incompatible ways

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Important
**Affects**: §4.3 (the matching rule, joined-fields paragraph); collides with §6.2 (placement, truncation and the drop floor)

**Problem**:
On the two fuzzy routes — `-f` and a filter typed by hand — the text a session is matched against is defined once as "the session name, one space, then the recorded directory", and once as the rendered row the user is shown. Those are not the same string: the rendered row also carries the window count and the attached marker, and its directory is left-truncated or dropped altogether when the terminal is narrow. A builder taking the second reading would match against whatever the row happened to render, so a session would become findable or unfindable as the user resizes their terminal, and a term like `2` would start hitting window counts. The display rule elsewhere is explicit that width never narrows the search, so only the first reading can stand.

**Proposal**:
Keep the precise definition — name plus one space plus the home-abbreviated recorded directory — and drop the claim that the joined text is character-for-character the rendered row. State instead that the join is taken from those two values before the row is laid out, so truncation, the drop floor and the row's fixed slots have no part in what is matched. This is what the display section already fixes when it says the rendering never narrows the search.

**Current**:
**On the two fuzzy routes the fields are joined**, as the picker's stock matcher expects, so a cross-field match is possible there — the first letters of the term found in the name and the rest in the path. **The joined text is the session name, one space, then the recorded directory in its home-abbreviated form** — character-for-character the row the user is shown (§6.2), so what the matcher scores and what the eye reads are one string; a session carrying no recorded directory joins to its name alone, with no trailing separator. That is accepted: those routes always show their rows, their rule was already the loose one (§4.4), and separating the fields would cost the list its ranking through the stock matcher.

**Proposed Text**:
**On the two fuzzy routes the fields are joined**, as the picker's stock matcher expects, so a cross-field match is possible there — the first letters of the term found in the name and the rest in the path. **The joined text is the session name, one space, then the recorded directory in its home-abbreviated form** — the two values the row is built from (§6.2), so what the matcher scores and what the eye reads are the same text; a session carrying no recorded directory joins to its name alone, with no trailing separator. The join is over those two values and nothing else, and it is taken before the row is laid out: the width-driven left-truncation and the floor that drops the directory altogether (§6.2) narrow what is displayed and never what is matched, and the row's count and attached slots are no part of the matched text. That is accepted: those routes always show their rows, their rule was already the loose one (§4.4), and separating the fields would cost the list its ranking through the stock matcher.

**Resolution**: Approved
**Notes**: Applied to §4.3 verbatim.

---

### 2. Tab completion can hand back a session name that stops being a search

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §8.1 (what completion offers); consequence of §2.2 (the no-second-slash recognition rule)

**Problem**:
Completion offers live session names with the slash kept on the front, and directories are excluded from the offer precisely because a completed path would carry a second slash and read as a path rather than a search. A session name can itself carry a slash — tmux accepts one, sessions created outside Portal appear in the list, and the rename modal refuses only a colon and a leading `$` or `-`. Tab would then complete `/foo` to `/foo/bar`, which the recognition rule reads as a path: the user's search silently becomes a directory argument that mints, or falls through to alias and zoxide, instead of attaching the session they were completing. Nothing in the completion rule keeps that candidate out.

**Proposal**:
Exclude any live session name containing a slash from what is offered, on the same ground the directories are excluded — a candidate that completes to a word holding a second slash leaves search territory. Such a session stays matchable by a term that stops short of the slash; it is only never offered.

**Proposed Text**:
Add to §8.1, immediately after the paragraph beginning "Directories are deliberately excluded from what is *offered*":

A live session name carrying a `/` is held back for the same reason. Completing one would produce a word with a second slash in it, which §2.2 reads as a path — so the completion would take the user out of the search form they typed. Such a session is still reachable: a term stopping short of the slash matches it by containment like any other. It is only never offered.

**Resolution**: Approved
**Notes**: Applied to §8.1 verbatim.

---

### 3. On a fast cold boot it is undecided when the single-match attach fires

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §3.4 (when the count is taken), §7.4 (accepted cost), §7.6 (the concurrent path is unchanged)

**Problem**:
On a cold start the search form shows the loading page, and when exactly one session matches the loading page is replaced by the attach. The loading page Portal already shows on this path is held for a minimum span so it cannot flash up and vanish. Two readings follow and they behave differently: either the attach fires the instant the count can be taken, cutting that minimum span short, or the page stands out its minimum and the attach follows. On a machine with nothing saved to restore, bootstrap can finish inside that span, so the user either sees a page torn away mid-appearance or waits about a second longer than they need to. A builder has no way to tell which was intended, and this is the first impression the feature makes after a reboot.

**Proposal**:
The concurrent path is stated as unchanged by this work — the form is added to the set of invocations that take it, and nothing about how it behaves is altered. The minimum the loading page already holds therefore stands, and the search form changes only what replaces the page when it lifts. Say so where the timing is described, so the shorter-cut reading is closed.

**Proposed Text**:
Add to §3.4, at the end of the paragraph beginning "K is evaluated against the live session set":

The loading page's own minimum display span is untouched by this (§7.6): where the count can be taken before that span has elapsed, the page stands for the remainder of it, and the attach or the picker follows when it lifts.

**Resolution**: Approved
**Notes**: Applied to §3.4 verbatim.

---

### 4. The promise that completion can never turn a search into a path holds only after the shell is restarted

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Minor
**Affects**: §2.2 (the recognition rule's completion safety claim); collides with §8.2, §8.3 and §8.5 (the session-opening function never reaches Portal's completer, and the fix arrives only when the init output is re-evaluated)

**Problem**:
The recognition rule is defended by a flat promise that completion can never add the trailing slash that would turn a search back into a path, because Portal switches the shell's filename fallback off. That promise does not hold for the session-opening function on any install that has not re-evaluated its shell startup file: the completion section establishes that Tab after that function never reaches Portal at all, which is exactly why the shell completes filenames there today, and that the correction reaches a user only on a new shell. So during that window `x /tm<TAB>` does complete to `/tmp/` and mints, while `x /tmp` typed out searches. A reader is told the shape can never be converted and is separately told the conversion is live until they restart their shell.

**Proposal**:
Qualify the promise rather than the rule: it holds unconditionally where Portal is asked — the full `portal open` invocation — and holds for the session-opening function once the corrected init output is live in the user's shell, which the rollout section already fixes as the moment of a new shell.

**Current**:
Completion cannot turn one shape into the other (§8.1), and Portal switches the shell's filename fallback off (`portal completion bash | grep -n 'compopt +o default'`) — so `x /tm<TAB>` never becomes `/tmp/`. The trailing slash that keeps an argument a path is therefore always one the user types, and a single-segment absolute directory typed without it is a sigil however it was reached.

**Proposed Text**:
Completion cannot turn one shape into the other (§8.1). Portal switches the shell's filename fallback off wherever it is asked for completions (`portal completion bash | grep -n 'compopt +o default'`), so `portal open /tm<TAB>` never becomes `/tmp/`, and neither does `x /tm<TAB>` once the corrected `portal init` output is live in the user's shell — until then Tab after that function is answered by the shell rather than by Portal and still falls through to filenames (§8.2, §8.5). The trailing slash that keeps an argument a path is therefore always one the user types, and a single-segment absolute directory typed without it is a sigil however it was reached.

**Resolution**: Approved
**Notes**: Applied to §2.2 verbatim.

---

### 5. What the user types into the bare slash's empty filter is matched by an unstated rule

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Minor
**Affects**: §4.4 (which rule applies while the search form's filter text stands), §2.5 (the bare slash)

**Problem**:
A slash on its own opens the picker with the filter open, empty and waiting to be typed into — and typing into it is the whole point of the form. Which rule those keystrokes are matched by is left to be worked out: the search form's own containment rule is described as holding while the filter text is the one the form supplied, and the bare slash supplied nothing, so the first character typed is already a different value. Two builders will read this differently, and the difference is visible — under the picker's own scattered-letter rule a term like `port` returns a session at `~/Projects/rust-tools`, which containment would never return. The answer needs saying once rather than deriving.

**Proposal**:
The bare slash is fixed as the plain picker-filter gesture — the user's own `/` keypress, reached from the shell — so what they type into it is the picker's own filter and is matched by the picker's own rule from the first keystroke. Nothing about the containment rule reaches text the form never supplied.

**Proposed Text**:
Add to §4.4, after the paragraph beginning "**The containment set holds for as long as the sigil's filter text stands untouched.**":

**The term-less form supplies nothing for that test to hold.** `x /` lands with an empty, focused filter (§2.5) — the picker's own filter gesture, reached from the shell — so the first character typed there is already a value the sigil did not supply, and the picker's own rule applies from that keystroke on.

**Resolution**: Approved
**Notes**: Applied to §4.4 verbatim.

---

### 6. Widening the match to directories leaves unexplained rows in every picker but the one this feature opens

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Minor
**Affects**: §4.5 (accepted cost and confidence), consequence of §4.2 (the fields widen for all three entry points) and §6.3 (the directory column is scoped to a search-opened picker)

**Problem**:
Sessions become findable by the directory they were opened in for every filter entry point, including `-f` and the filter a user types by hand in an ordinary picker. The column that accounts for such a hit — the directory shown beside the name — is given only to a picker the search form opened. So in an ordinary picker a session now appears under a filter with nothing on its row explaining why, which is the exact confusion the column exists to remove, and it is a change to a screen users already have. Every other cost this work takes is named and owned; this one is only implied, so a reader weighing the feature does not see it and nobody can tell whether it was accepted or overlooked.

**Proposal**:
Name it as an accepted cost beside the false-positive cost it belongs with. The behaviour itself stands as decided — the fields are deliberately uniform across entry points, and the column is deliberately scoped to the search-opened picker — and the mitigation is already inherent: rows appear, none the user expected goes missing.

**Proposed Text**:
Add to §4.5, after the existing paragraph:

**Accepted with it:** the widening reaches the pickers this feature does not open. There, a session can surface on the strength of a recorded directory no row displays, because the directory column is scoped to a search-opened picker (§6.3). The visible effect is rows appearing, never rows the user expected going missing, and narrowing the fields by entry point is the alternative §4.2 rejects.

**Resolution**: Approved
**Notes**: Applied to §4.5 verbatim.

---

### 7. The same nine-point documentation payload is assigned to both the command help and the README

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: choice
**Priority**: Minor
**Affects**: §9.2 (what the documentation must carry, and which surfaces carry it)

**Problem**:
Nine points are listed as owed to the documentation, and both `portal open --help` — including the one-line description of the `-f` flag — and the README's open section are named as carrying them. Two of the nine are about the shell function's completion and the fact that an existing install only picks it up on a new shell, which is the init command's business rather than `open`'s, and none of the nine will fit a flag description. A writer therefore has to invent the split, and the two plausible splits produce visibly different help output: one wall of text covering shell-startup rollout, or a short command-level summary.

**Options**:
- Both surfaces carry all nine points, with `--help` gaining the completion and rollout notes as well.
- `portal open --help` carries the points about `open`'s own surface — the form and its recognition rule, the three outcomes, the term-less form, that it composes with nothing, and the `-f` versus `/term` distinction — and the README additionally carries the completion correction and its rollout consequence (recommended).

**Resolution**: Approved
**Notes**: Option 1 chosen — `--help` carries `open`'s own surface; the README adds the completion correction and its rollout. Applied to §9.2.

---

### 8. The documentation list restates the bare slash's landing instead of pointing at it

**Source**: Specification analysis
**Category**: Duplication
**Move**: settled
**Priority**: Minor
**Affects**: §9.2 (documentation payload bullet for the term-less form); the fact's home is §2.5, with the never-an-error property owned by §3.2

**Problem**:
What a slash on its own opens — the picker with its filter open, empty and ready to type into — is written out in full in two places: where the form is defined and again in the list of what the documentation must carry. The neighbouring items in that list name their topic and point at where it is settled. A later change to the landing behaviour will be made in one place and leave the other stating the old behaviour, at which point the documentation is written from the stale copy.

**Proposal**:
Reduce the item to what it is for — naming the topic the documentation owes — and let the form's own definition carry the behaviour, as the surrounding items do.

**Current**:
- The term-less form — a slash on its own opens the picker with its filter open and empty, ready to type into, and never errors (§2.5).

**Proposed Text**:
- The term-less form as §2.5 sets it, and that it is not an error.

**Resolution**: Approved
**Notes**: Applied to §9.2 verbatim.

---
