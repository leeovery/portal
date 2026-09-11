# Review Tracking: Open With Forced Filter - Input Review

## Findings

### 1. The slash typed with nothing after it ships undocumented

**Source**: `.workflows/open-with-forced-filter/discussion/open-with-forced-filter.md` — `filter-shortcut-form` → Decision → "#### 2026-09-11 — revised" ("**`x /` — the sigil with no term — opens the picker with the filter open and empty, the cursor in it, ready to type.** It is `x` followed by `/`, not plain `x` and not a usage error.")

**Category**: Enhancement to existing topic
**Move**: settled
**Affects**: §9.2 (What the documentation must carry)

**Problem**:
`x /` — the slash with nothing after it — opens the picker with its filter already open, empty and focused, ready to type into. It is a deliberate gesture: an earlier ruling made it a usage error and the user reversed that, so a bare slash never errors. Nothing that ships to the user says it exists. The help text and README are told to carry the three outcomes that follow from a match count, and the term-less form takes no count, so it falls through every documented case. A one-keystroke shortcut nobody can discover is a shortcut nobody uses, and the feature's whole deliverable is a gesture becoming muscle memory.

**Proposal**:
Add the term-less form to what the help text and README must carry, beside the three match-count outcomes: `x /` opens the picker with the filter open and empty. What determines it is the source's own reversal — the bare slash was promoted from a refusal to a supported form, and every other user-visible shape of the sigil is already on the documentation list.

**Current**:
```
- The sigil form and its recognition rule, including the single-segment absolute-directory cost and the `-p` escape (§2.2, §2.4).
- The three outcomes by match count (§3.2).
- What the search matches, and that it matches by containment while the picker's own filter stays fuzzy (§4).
```

**Proposed Text**:
```
- The sigil form and its recognition rule, including the single-segment absolute-directory cost and the `-p` escape (§2.2, §2.4).
- The three outcomes by match count (§3.2).
- The term-less form — a slash on its own opens the picker with its filter open and empty, ready to type into, and never errors (§2.5).
- What the search matches, and that it matches by containment while the picker's own filter stays fuzzy (§4).
```

**Resolution**: Approved
**Notes**: Applied to §9.2 verbatim.
