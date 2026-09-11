# Review Tracking: Open With Forced Filter - Gap Analysis

## Findings

### 1. The slash typed on its own can attach a session instead of opening the picker

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Critical
**Affects**: §2.5 The bare slash; §3.2 Outcomes by match count; §3.3 The committed-filter landing

**Problem**:
A user with exactly one live session types `x /` — the slash alone, the gesture for "open the picker and let me type" — and the specification supports two opposite outcomes. The term-less form is described as opening the picker with the filter empty and focused, given without condition. The match-count rule counts the live sessions matching the term and attaches outright when the count is one, and an empty term is contained in every name a session has, so the count is every live session. A builder who reads the count rule first ships an `x /` that drops a one-session user straight into that session, showing them nothing and giving them no filter to type into. The same reading also makes `x /` on a fresh machine with no sessions land on a *committed* empty filter rather than a focused one, which is the opposite of the stated gesture.

**Proposal**:
The term-less form carries nothing to match, and the landing rule already names it as the exception that commits no filter. Make the same carve-out explicit for the count: no term, no count — the slash alone opens the picker whatever the live session list holds.

**Current**:
Let K be the number of live sessions matching the term under §4.

**Proposed Text**:
Let K be the number of live sessions matching the term under §4.

**A term-less sigil takes no count.** `x /` carries nothing to match, so no count is evaluated and none of the rows below apply: it opens the picker on the whole live session list, filter focused and empty (§2.5), on a machine holding one live session as on a machine holding twenty.

**Resolution**: Pending
**Notes**:

---

### 2. Where the directory sits in a session row is unsettled

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: choice
**Priority**: Important
**Affects**: §6.2 Placement and weight

**Problem**:
Two builders will lay out the same row differently and both will be following the document. One puts the directory immediately after the session name, so every row's directory starts wherever that row's name happened to end. The other anchors it against the fixed window-count slot, so the directories form an aligned block down the right of the list. The document says the directory sits alongside the name, and also calls it a column that takes its width from what remains — which of those governs is never stated. The difference is not cosmetic: the directory is shortened from the left so its tail survives, and under the anchored reading those tails line up into a scannable column, while under the packed reading both edges are ragged on every row.

**Options**:
- Left-packed — the directory begins one space after the session name, so its start column varies row to row and both of its edges are ragged.
- Right-anchored — the directory ends flush against the fixed window-count slot with a minimum gap kept after the name, so the surviving path tails align down the list and the variable gap absorbs the difference in name lengths. (recommended)

**Resolution**: Pending
**Notes**:

---

### 3. A refused command line has no message written for it

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Minor
**Affects**: §5.1 The rule

**Problem**:
Every refused line — a second target, a trailing command, a domain pin, a filter flag, the internal handshake flag — has to print something to the user, and no wording is fixed anywhere. A builder can emit one generic "invalid arguments" across all of them, leaving the user to work out which word on their line caused it, or invent a different sentence for each case so the surface reads as if several people built it. This is user-visible copy on the one path this form can refuse a user at all.

**Proposal**:
The shape is already fixed — the refusal a filter flag gives when it is combined with something it excludes. Fix the content to match it: one line that names the search form and the element it collided with, so the user can see which word to drop.

**Proposed Text**:
**A refusal names what collided.** Each line above is refused with a single message naming the search form and the element beside it that may not be there — the shape `-f`'s own mutual-exclusion refusal already takes — rather than a generic complaint about the arguments that leaves the user to find the offending word themselves.

**Resolution**: Pending
**Notes**:

---

### 4. The directory column for the slash typed on its own

**Source**: Specification analysis
**Category**: Enhancement to existing topic
**Move**: settled
**Priority**: Minor
**Affects**: §6.3 Scope; §2.5 The bare slash

**Problem**:
`x /` and plain `x` followed by `/` put the user in the same place, one keystroke apart, and only one of them is specified to show a directory beside every session name. Which way `x /` goes is not stated: a builder can read the scoping rule literally and paint a directory column across the user's whole session list, or read the term-less form as ordinary picker plus filter and leave it off — in which case the rows the user filters to by their directory, which the picker's own filter now matches, arrive with nothing on screen accounting for why they are there.

**Proposal**:
The column is scoped to the picker a search opened and is deliberately kept after the user edits the filter text, because rows can still be present on the strength of their directory. The term-less form is that same search and its filter is edited by definition, so it carries the column.

**Current**:
Scoped to the picker session a sigil opened — across every grouping mode that list can be in (§6.1), and for as long as that picker is open.

**Proposed Text**:
Scoped to the picker session a sigil opened — the term-less form of §2.5 included — across every grouping mode that list can be in (§6.1), and for as long as that picker is open.

**Resolution**: Pending
**Notes**:

---

### 5. What completion offers is stated in two places

**Source**: Specification analysis
**Category**: Duplication
**Move**: settled
**Priority**: Minor
**Affects**: §2.2 The recognition rule; §8.1 Completion looks past the sigil

**Problem**:
What tab completion offers for a slash-leading word — live session names carrying the slash, never directories — is written out twice: once inside the recognition rule as support for the claim that completion cannot turn a search into a path, and once in the completion section that owns the decision and gives its reason. The two agree today. A later change to what completion offers has to find both, and the copy that gets missed comes back as a contradiction about whether Tab can produce a path.

**Current**:
Completion cannot turn one shape into the other. The words it offers for a slash-leading argument are live session names carrying the sigil, never directories (§8.1), and Portal switches the shell's filename fallback off (`portal completion bash | grep -n 'compopt +o default'`) — so `x /tm<TAB>` never becomes `/tmp/`.

**Proposed Text**:
Completion cannot turn one shape into the other (§8.1), and Portal switches the shell's filename fallback off (`portal completion bash | grep -n 'compopt +o default'`) — so `x /tm<TAB>` never becomes `/tmp/`.

**Resolution**: Pending
**Notes**:

---

### 6. Tab-then-Enter is promised as an attach it cannot guarantee

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Minor
**Affects**: §8.1 Completion looks past the sigil; §4.3 The matching rule

**Problem**:
The payoff the completion work is justified by — press Tab, press Enter, you are in the session — is stated as a certainty, and the matching rule does not deliver one. Completing to a full session name still leaves a second match when another session's recorded directory contains that name, or when a renamed session's name contains it, and the user lands in the picker instead. An acceptance criterion written from this sentence ("completing a session name and pressing Enter attaches it") fails on a machine where one session sits in a checkout named after another, and the builder is left arguing with a test rather than with the rule.

**Current**:
`/po` completing to `/portal-a1b2` leaves exactly one match, which under §3.2 attaches outright, so `/po<TAB><Enter>` becomes the whole interaction.

**Proposed Text**:
`/po` completing to `/portal-a1b2` normally leaves that session as the only match, which under §3.2 attaches outright, so `/po<TAB><Enter>` becomes the whole interaction; where a second session's name or recorded directory also contains the completed name, the same keystrokes land in the picker on those two.

**Resolution**: Pending
**Notes**:

---
