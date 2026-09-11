# Review Tracking: Open With Forced Filter - Input Review

## Findings

### 1. The tab-completion section says running a bare slash is an error

**Source**: `discussion/open-with-forced-filter.md` → `## filter-shortcut-form` → Decision → `#### 2026-09-11 — revised`: "**`x /` — the sigil with no term — opens the picker with the filter open and empty, the cursor in it, ready to type.** It is `x` followed by `/`, not plain `x` and not a usage error. … Nothing about using `/` errors."
**Category**: Enhancement to existing topic
**Move**: settled
**Affects**: §8.1 (Completion looks past the sigil)

**Problem**:
The document tells the builder two opposite things about typing `x /` and pressing Enter: that it opens the picker with an empty filter and the cursor in it, and — in the paragraph on what Tab offers after a bare slash — that it is a usage error. Built from the second, the simplest gesture the feature offers refuses the user at the prompt instead of opening the filter they asked for, and the reason given for completing a bare slash at all rests on an error that does not exist.

**Proposal**:
Correct the completion paragraph to the one outcome the record holds for a bare slash — it opens the picker on an empty filter — and rest the case for completing it on what completion actually buys there: a full session name, which narrows to one and attaches. The source reversed the error reading explicitly, and nothing else in the document carries it.

**Current**:
`/<TAB>` — the sigil with nothing after it — offers every live session name, since every name is prefixed by the empty term. The bare slash is a usage error only when it is run (§2.5), which is precisely what makes completing it worth doing.

**Proposed Text**:
`/<TAB>` — the sigil with nothing after it — offers every live session name, since every name is prefixed by the empty term. Left as typed it opens the picker on an empty filter (§2.5); accepting a completion instead narrows the term to one session and attaches it, which is what makes completing the bare slash worth doing.

**Resolution**: Approved
**Notes**: Applied to §8.1 verbatim.

---

### 2. The sibling-correction section says a search that matches nothing hard-fails

**Source**: `discussion/open-with-forced-filter.md` → `## unambiguous-direct-attach` → Decision → `#### 2026-09-11 — revised`: "**every other count opens the picker pre-filtered**, zero included. A term matching nothing lands in the picker showing an empty list under its filter … Nothing is written to stderr and the exit status is not a failure."
**Category**: Enhancement to existing topic
**Move**: settled
**Affects**: §11 (Correction Owed to `cli-verb-surface-redesign`), final paragraph

**Problem**:
The closing statement about how the search form sits beside the domain pins says that a term matching no session hard-fails the way a pin does. The feature's actual answer is the opposite: a term matching nothing opens the picker showing an empty list, writes nothing to stderr, and exits successfully. A builder working from that closing statement ships `x /zzz` as an error at the prompt — stranding the user having done nothing, where an empty list is one keystroke from recovery — and the two halves of the document disagree about the single most common miss.

**Proposal**:
Correct the clause to the outcome the record holds: the form has no unresolvable case to fall back from, because a zero count is a filter result that opens the picker. The paragraph's own point — that the pinned-domain contract is adjacent rather than breached — stands without it, since the sigil is not a domain pin.

**Current**:
**The pinned-domain contract is adjacent rather than breached.** That contract holds that every domain pin hard-fails on an unresolvable target and never falls back to the TUI picker. The sigil is not a domain pin: reaching the picker is its purpose (§3.2) rather than a fallback from a failure, and on its own unresolvable case — K = 0 — it hard-fails exactly as a pin does.

**Proposed Text**:
**The pinned-domain contract is adjacent rather than breached.** That contract holds that every domain pin hard-fails on an unresolvable target and never falls back to the TUI picker. The sigil is not a domain pin: reaching the picker is its purpose (§3.2) rather than a fallback from a failure, and it has no unresolvable case to fall back from — a term matching nothing is a filter result that opens the picker on an empty list (§3.2).

**Resolution**: Approved
**Notes**: Applied to §11 verbatim.

---

### 3. The picker's hand-typed filter is missing from the inventory of what this feature changes

**Source**: `discussion/open-with-forced-filter.md` → `## search-match-domain` → Journey: "**There is one filter, not three.** `-f/--filter`, the new `/` form, and typing `/` by hand inside the picker all narrow the same list through the same `FilterValue`. Widening the match domain therefore widens it for all three — this is not a change scoped to the new form." Retained through the Decision's 2026-09-11 amendment, which confines the divergence to the matching rule and keeps the fields shared.
**Category**: Enhancement to existing topic
**Move**: settled
**Affects**: §10.5 (The rest of the argument surface)

**Problem**:
The document's account of what this feature changes about the surface that already ships names one thing — the flag. It omits the filter the user types by hand inside the picker, which after this feature also finds sessions by the directory they were opened in. That is a behaviour change every picker user meets on a screen this feature otherwise leaves alone, and anyone working from that account — release notes, README, the test plan, the reviewer asking what moved — treats it as no change at all and ships it unannounced and untested.

**Proposal**:
Name the widened match domain once, as reaching the flag and the hand-typed filter alike, and leave the fields themselves where they are settled. The source ruled the fields shared across all three entry points and recorded in the same breath that the widening is not scoped to the new form.

**Current**:
**Nothing is retired, renamed or deprecated.** `-f`, all four domain pins, and the bare positional chain keep their current prominence, and the domain pins and the bare positional chain keep their current behaviour exactly. Two changes are owed to the existing surface: `-f`'s widened match domain (§4.2), and how the `-f` / `/term` pair is described (§9.1).

**Proposed Text**:
**Nothing is retired, renamed or deprecated.** `-f`, all four domain pins, and the bare positional chain keep their current prominence, and the domain pins and the bare positional chain keep their current behaviour exactly. Two changes are owed to the existing surface: the widened match domain, which reaches `-f` and the filter typed by hand in the picker alike (§4.2), and how the `-f` / `/term` pair is described (§9.1).

**Resolution**: Approved
**Notes**: Applied to §10.5 verbatim.
