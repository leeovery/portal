# Review Tracking: Open With Forced Filter - Gap Analysis

## Findings

### 1. The count is described as running against names only, while everything else has it matching directories too

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Important
**Affects**: §4.1 (The matched fields); collides with §4.4 (the entry-point table) and §4.1's own opening paragraph

**Problem**:
The number of live sessions a term matches is what decides whether the user is attached outright or lands in the picker. One passage says that number is taken against a session list that carries names and nothing else, while the same section's opening paragraph and the entry-point table both have it matching the session name *and* the recorded directory. Built on the first reading, a session found only by the place it lives is invisible to the count but present in the list the count opens — so `/port` reports nothing to match and opens an empty-looking search that then shows a row, or, worse, counts one name match and attaches outright where two sessions were actually eligible and the user should have been given the choice. The two readings disagree about which session the user ends up in.

**Proposal**:
Correct the clause. The recorded directory rides back with the session list at no extra cost, which is the whole reason it is in the match domain; what the shell form lacks before any picker exists is not directories but a *derived* directory — there is no pane read on that path and nowhere to cache one. Rewrite the clause to say that, which is what the paragraph's own argument needs.

**Current**:
**Never a derived directory.** The picker can derive a missing directory by asking a session's pane where it is, but only in the grouped views, and it caches the answer — so a session would be findable or not depending on which view the user last left the picker in. The shell form has it worse: K (§3.2) is taken before any picker exists, against a session list carrying names only.

**Proposed Text**:
**Never a derived directory.** The picker can derive a missing directory by asking a session's pane where it is, but only in the grouped views, and it caches the answer — so a session would be findable or not depending on which view the user last left the picker in. The shell form has it worse: K (§3.2) is taken before any picker exists, so a derived value has nowhere to come from and nowhere to be kept — the recorded directory that rides back with the session list is the only one there is.

**Resolution**: Approved
**Notes**: Applied to §4.1 verbatim.

---

### 2. A row can be returned on a directory the width has pushed off the screen, against a stated promise that what was matched is what is shown

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Minor
**Affects**: §6.2 (Placement and weight); relates to §4.1's "the searched form is the displayed form" and §6.1's accounting requirement

**Problem**:
The search promises that what it matched is what the row shows — that promise is why a directory under home is searched abbreviated rather than as tmux recorded it. But the row shortens the directory from the left when the width runs out, and drops it entirely below a floor. So a term that hit a leading segment (`/Code`) can return a row displaying `…/portal`, and one that hit a short-named session on a narrow terminal can return a row displaying no directory at all — the user is looking at a result with nothing on it accounting for why it is there, which is the exact failure the accounting column exists to prevent. A builder is also left unsure whether the promise binds the truncation ladder: whether the shortening must keep the matched run visible, or whether the tail always wins.

**Proposal**:
The tail always wins — the ladder is stated as always-from-the-left with the recognisable tail as its reason, and the match cannot depend on the rendering in any case, since the count is taken before a single row is laid out. Say so where the ladder is defined, and name the residual honestly: the column shows the recognisable tail of the matched value, not a guarantee that the matched run is on screen.

**Proposed Text**:
Add at the end of §6.2, after the paragraph beginning "**Below a floor the directory is dropped rather than truncated to noise.**":

The rendering never narrows the search. A term is tested against the whole recorded directory in its home-abbreviated form (§4.1), which is fixed before any row is laid out, so a row can survive on a segment the width pushed out of view or the floor dropped altogether. What the column carries is the recognisable tail of the value that was matched, not a promise that the matched run itself is on screen.

**Resolution**: Approved
**Notes**: Applied to §6.2 verbatim.

---

### 3. How a filtered picker lands is stated twice

**Source**: Specification analysis
**Category**: Duplication
**Move**: settled
**Priority**: Minor
**Affects**: §3.2 (Outcomes by match count) — the outcome table; the landing itself is §3.3

**Problem**:
Two places say how the picker lands when a term opens it — the outcome table's cells and the section written to fix that landing, which already states it covers a zero count and a two-or-more count alike. They agree today. A later change to the landing that reaches only one of them leaves the other describing a screen the product no longer produces, and there is nothing in either to tell a builder which one is authoritative.

**Proposal**:
Let the table carry what varies with the count and point at the landing rather than restating it. The cursor's row stays in the table cell: the no-reorder rule elsewhere is written against that phrase, and it is the count-specific half of the landing rather than a copy of the general one.

**Current**:
| K | Outcome |
|---|---|
| 0 | The picker opens on the Sessions page with the term applied as a committed filter, showing an empty list. |
| 1 | The matching session is attached directly. No picker. |
| >= 2 | The picker opens on the Sessions page with the term applied as a committed filter and the cursor on the first matching row. |

**Proposed Text**:
| K | Outcome |
|---|---|
| 0 | The picker opens on the Sessions page, landing as §3.3 sets it, with no session surviving the term. |
| 1 | The matching session is attached directly. No picker. |
| >= 2 | The picker opens on the Sessions page, landing as §3.3 sets it, with the cursor on the first matching row. |

**Resolution**: Approved
**Notes**: Applied to §3.2's outcome table verbatim.

---
