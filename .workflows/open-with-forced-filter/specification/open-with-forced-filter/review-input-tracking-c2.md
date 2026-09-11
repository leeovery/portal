# Review Tracking: Open With Forced Filter - Input Review

## Findings

### 1. On the flag and hand-typed routes, the two searched fields may be matched as one string

**Source**: `discussion/open-with-forced-filter.md` — `search-match-domain` Journey ("There is one filter, not three. `-f/--filter`, the new `/` form, and typing `/` by hand inside the picker all narrow the same list through the same `FilterValue`") and its Decision amendment ("The divergence is confined to the matching rule; the matched fields remain shared"), read against `search-result-display` Decision ("Scoped to the sigil's own list. How Sessions rows render generally is untouched")
**Category**: Gap/Ambiguity
**Move**: choice
**Affects**: §4.2 (the matched fields are shared across all three filter entry points), §4.3 (the matching rule), §4.4 (the rule diverges by entry point)

**Problem**:
After this feature `-f port` and a `/` typed by hand inside the picker both search a session's directory as well as its name, and both keep the picker's subsequence matcher. If those two fields reach that matcher as a single joined string, a session can come back because the first letters of the term were found in its name and the rest in its path — a row whose name contains nothing resembling what was typed and whose directory contains nothing resembling it either. On both of those routes the row shows a name, a window count and an attached marker and nothing more, so there is no way to work out why it is on screen; the sigil's own list at least carries the directory that explains it. Whether a cross-field match can be returned at all is unstated, and it is decided by how the widening is built, so one build shows those rows and another does not.

**Options**:
- The two fields reach the picker's matcher as one combined value, as its stock matcher expects, and a cross-field match is accepted on those routes — the picker shows its rows and its rule was always the loose one (recommended)
- The two fields are tested separately on the fuzzy routes as well, best match winning, so no row can be returned that neither field explains — at the cost of the list no longer ranking through the stock matcher

**Proposed Text**:

**Resolution**: Approved
**Notes**: Option 1 chosen — the fuzzy routes join the two fields as the stock matcher expects and accept a cross-field match; the sigil keeps its separate tests. Applied to §4.3.

---

### 2. A search term can match the part of a path the row never shows

**Source**: `discussion/open-with-forced-filter.md` — `search-match-domain` Decision, "Which directory value is matched" ("Only the directory tmux records against the session (`@portal-dir`) … Never a derived one") read against `search-result-display` Decision and Summary Key Insight 5 ("A match domain the row does not display is a match the user cannot audit")
**Category**: Gap/Ambiguity
**Move**: choice
**Affects**: §4.1 (the matched fields), §4.3 (the matching rule), §6.2 (placement and weight — the home abbreviation)

**Problem**:
Every session opened anywhere under the user's home is recorded at an absolute path beginning `/Users/<name>/`, while its row displays that path abbreviated to `~/…`. If the term is tested against the path as recorded, `/user` or `/lee` matches every session the user has, and each returned row displays a directory containing no such text — the list that exists to explain why a row is in front of the user explains nothing, and the search is loudest exactly where it is least useful. Where such a term happens to leave one survivor it is worse than noise: a term that hit only the machine's username attaches a session outright, with nothing shown that accounts for the choice.

**Options**:
- The directory is searched in the same home-abbreviated form the row displays, so what was matched is what is shown and the home prefix stops generating matches (recommended)
- The directory is searched exactly as tmux recorded it, accepting that a term hitting the home prefix returns rows that display no such text

**Proposed Text**:

**Resolution**: Pending
**Notes**:

---
