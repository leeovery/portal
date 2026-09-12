# Review Tracking: Open With Forced Filter - Traceability

## Findings

### 1. The README never tells the reader what Tab after `x` now offers

**Type**: Incomplete coverage
**Spec Reference**: §9.2, documentation deliverables — "That the corrected completion offers live session names after the session-opening function, and no longer falls through to filenames for a path argument (§8.3)"
**Plan Reference**: Phase 5, task open-with-forced-filter-5-4 (Document the search form and the completion correction in the README)
**Move**: settled
**Change Type**: update-task

**Problem**:

The correction to `portal init` is the single largest behaviour change a long-time user will notice at their prompt: before it, Tab after `x` silently fell through to filenames; after it, Tab after `x` offers their live session names. The README task directs the writer to say only that the release "corrects `portal init` so Tab after `x` reaches Portal at all" — which tells the reader a plumbing fact and never tells them what they get. A user who restarts their shell, presses Tab after `x`, and sees session names where filenames used to be has no line in the documentation that predicted it; a user who was relying on filename completion for `x ~/Code/pro<TAB>` reads only that the fallback is gone, with no statement of what replaced it. The documentation is required to carry both halves of that trade, and as written it carries one.

**Proposal**:

Name the gain beside the loss in the README's Tab-completion paragraph, and put it on the task's acceptance criteria so the writer cannot ship the paragraph without it. The wording is the specification's own: the corrected completion offers live session names after the session-opening function. No new behaviour is implied — task 5-1 already offers exactly those names, and task 5-2 already routes `x` at them; this is the sentence that tells the user so. The Tests list is deliberately untouched: the README guard asserts literal tokens a user types rather than prose, which is the property that keeps accurate copy edits from churning the suite.

**Current**:

The task's fourth **Do** bullet:

```markdown
- Add a **Tab completion** paragraph at the end of the `x (open)` section (after the multi-window paragraph, before the git-root line at README:154): `x /po<TAB>` completes the term after the slash against live session names and leaves the slash in place; this release corrects `portal init` so Tab after `x` reaches Portal at all; an existing install picks the correction up only when the output of `portal init` is re-evaluated — a new shell, or re-running the eval in the profile — while the `/term` form itself works the moment the new binary is in place; and `x <path><TAB>` no longer falls through to filenames, which is `portal open`'s existing completion contract.
```

and, in **Acceptance Criteria**, the tenth and eleventh criteria:

```markdown
- [ ] The section documents the completion behaviour after the slash, the `portal init` correction, and that an existing install picks it up only on a new shell or a re-run of `portal init` while the form itself works with the new binary
- [ ] The section notes that a path argument after the function no longer completes filenames
```

**Proposed Text**:

The fourth **Do** bullet becomes:

```markdown
- Add a **Tab completion** paragraph at the end of the `x (open)` section (after the multi-window paragraph, before the git-root line at README:154): this release corrects `portal init` so Tab after `x` reaches Portal's completer at all, and `x <TAB>` now offers your live session names; `x /po<TAB>` completes the term after the slash against those same names and leaves the slash in place; an existing install picks the correction up only when the output of `portal init` is re-evaluated — a new shell, or re-running the eval in the profile — while the `/term` form itself works the moment the new binary is in place; and `x <path><TAB>` no longer falls through to filenames, which is `portal open`'s existing completion contract.
```

and the tenth and eleventh **Acceptance Criteria** become:

```markdown
- [ ] The section states that Tab after the session-opening function now offers live session names, and that `x /po<TAB>` completes the term after the slash against those same names
- [ ] The section documents the `portal init` correction and that an existing install picks it up only on a new shell or a re-run of `portal init` while the form itself works with the new binary
- [ ] The section notes that a path argument after the function no longer completes filenames — the deliberate loss beside that gain
```

**Resolution**: Pending
**Notes**:

---

### 2. Soft bootstrap warnings vanish when the session-list read fails

**Type**: Incomplete coverage
**Spec Reference**: §7.5, a search that resolves to a direct attach still delivers its warnings (the notice band never surfaces, so the warnings are written to the terminal instead); §3.7, a failed session-list read is reported in tmux's own terms and exits non-zero
**Plan Reference**: Phase 4, task open-with-forced-filter-4-3 (Deliver the accumulated soft bootstrap warnings on a single-match attach) — Edge Cases, fourth bullet; the same task's `SearchAttached()`-only gate in **Do** and **Acceptance Criteria**
**Move**: choice
**Change Type**: add-to-task

**Problem**:

Classifying the search form as a picker invocation takes soft bootstrap warnings — "the saver is down", "your saved state could not be restored" — off stderr and onto the in-TUI route. The plan then puts them back for the one path where the TUI never appears: the single-match attach. It does not put them back for the other such path: a failed session-list read, where the picker is likewise never painted and the process exits with tmux's error. On that path the user is told the session list could not be read and is told nothing about the saver being down — precisely the pairing where the second line would explain the first. The equivalent `-f` invocation on the same boot shows both, because its picker opens and its notice band surfaces them.

The record does not settle it. The warning-delivery rule is written for the attach ("on a single match the TUI tears down before the connector runs, so the warnings are written to the terminal instead"), and its reasoning — the band never surfaces, and the alternate screen is gone, so writing is safe — applies word for word to the failed read; but the failure section states just as plainly that the tmux error is the one failure path this form has and that it belongs to tmux rather than to the search. The plan itself flags this as the reading to review.

**Options**:
- Deliver them: write the accumulated warnings before the tmux error on both routes — drain the sink ahead of returning the enumeration error on the up-front count, and widen the teardown gate from `SearchAttached()` to also cover a recorded search error — so a torn-down picker always surrenders its warnings, whichever way it tore down (recommended)
- Leave them undelivered as the task now has it: the tmux error is the whole report on this path, the gate stays `SearchAttached()` alone, and the warnings die in the sink beside a `Ctrl-C`'d loading page's

**Resolution**: Pending
**Notes**: Whichever way this goes, the answer belongs in the task's Acceptance Criteria rather than only in its Edge Cases — the current "writes no warnings" reading is stated as a note and asserted by no criterion, so neither behaviour is pinned by a test as the task stands.

---
