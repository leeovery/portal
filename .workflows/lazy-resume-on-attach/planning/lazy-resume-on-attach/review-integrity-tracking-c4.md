# Review Tracking: Lazy Resume On Attach - Integrity

## Findings

### 1. Phase 1 leaves CLAUDE.md wrong in four places and every later phase assumes it did not

**Severity**: Important
**Plan Reference**: Phase 1, tasks 1.1, 1.2, 1.5, 1.7
**Category**: Task Template Compliance / Task Self-Containment (the plan's own doc-edit convention, applied in Phases 2–6 and skipped in Phase 1)
**Move**: settled
**Change Type**: add-to-task

**Problem**:
Phase 1 changes four things CLAUDE.md states as fact and no Phase 1 task edits the file, so the repo's operating manual ships wrong the moment Phase 1 lands — and stays wrong, because Phases 2–6 each only correct the sentences *they* falsify. The four:

- `internal/resumemode` is a new production leaf package with no row in the package table. Every other production leaf there — `nanoid`, `shellquote`, `tmuxerr`, `tmuxout`, `storelog`, `warning`, `xdg`, `fileutil` — has one, and the table is the only place a later agent learns a package exists at all.
- The `prefs` row enumerates the file's literal key shape (`{"session_list_mode": …, "theme": …, "theme_light": …, "theme_dark": …, "theme_migrated": true}`) and states the package is "Deliberately a leaf (stdlib + `fileutil` only, no `internal/log`)". Task 1.5 adds `resume_mode` to the file and `internal/resumemode` to the allowlist, so both claims become false — and the second one actively misleads: a future contributor reads it as a rule the new import violates.
- The **Resume-hook command** paragraph says `hook list` renders the location "as a fourth tab-separated column after key/event/command". Task 1.7 makes it five columns. Anyone touching that command next reads the wrong arity — the same class of staleness Phase 2 treats as worth an edit for `captureFieldCount = 11 → 12`.
- The `hooks` row describes the store as "holding per-pane on-resume commands keyed by the **hook key**". After tasks 1.2–1.4 an event's stored value is a registration in either a string or an object shape, both permanently valid, with unmodelled attributes re-emitted verbatim. That is an on-disk contract the row documents at length and no longer describes.

Phase 2's own planner's note says the one-line edits "ride with the tasks that falsify them rather than a separate documentation task, **as Phase 1 did**" — an assumption the plan does not currently meet.

**Proposal**:
Give Phase 1 the same doc-edit discipline every later phase carries: one Do bullet per falsifying task, riding with the task rather than collected into a documentation task. The convention is the plan's own (tasks 2.1, 2.2, 2.3, 2.4, 3.3, 3.6, 4.1, 4.4, 4.5, 5.1, 6.1 each carry exactly this), and the wording follows what the edits must say to be true after the task lands. Do bullets only, no acceptance criteria — matching how every other single-sentence doc edit in the plan is carried.

**Current**:

Task 1.1 — final **Do** bullet:
```markdown
- Add `internal/resumemode/leaf_guard_test.go` modelled on `internal/nanoid/leaf_guard_test.go`: `sourceguardtest.AssertDepsWithin(t, resumeModePkg, nil, sourceguardtest.ForbiddingThirdParty(), lane)` over every lane in `sourceguardtest.Lanes()`.
```

Task 1.2 — final **Do** bullet:
```markdown
- Update the `hooks.Snapshot` literals in `internal/hooksweep/sweep_test.go` and `internal/hooksweep/decline_error_test.go` to the new value type, and confirm `StaleKeys`, `narrowToSnapshot`, `hooksweep` and `cmd/doctor.go`'s `checkStaleHooks` still compile and pass untouched — they read keys and lengths only.
```

Task 1.5 — final **Do** bullet:
```markdown
- Update the `prefs.json` row of the README's config table to name `resume_mode` alongside the grouping mode and the theme keys, stating that it holds `eager` or `lazy` and is the install-wide default a registration that names no mode of its own inherits.
```

Task 1.7 — final **Do** bullet:
```markdown
- Update the `hook list` line in the README's `hook` example block to describe the fifth column and what an empty cell means.
```

**Proposed Text**:

Task 1.1 — final **Do** bullet, with one bullet added after it:
```markdown
- Add `internal/resumemode/leaf_guard_test.go` modelled on `internal/nanoid/leaf_guard_test.go`: `sourceguardtest.AssertDepsWithin(t, resumeModePkg, nil, sourceguardtest.ForbiddingThirdParty(), lane)` over every lane in `sourceguardtest.Lanes()`.
- Add a `resumemode` row to CLAUDE.md's package table, beside the other leaves: the closed eager/lazy vocabulary — the `Mode` kind with its `Unset` zero meaning "names no mode", the two on-disk spellings, the single strict `Parse` that recognises those two words and nothing else, the shipped `Default` (lazy), and the `Resolve` that answers a registration's mode against the install's — stdlib-only, so `internal/hooks` and `internal/prefs` can each reach it without an edge between them, pinned by its own dependency guard across both lanes.
```

Task 1.2 — final **Do** bullet, with one bullet added after it:
```markdown
- Update the `hooks.Snapshot` literals in `internal/hooksweep/sweep_test.go` and `internal/hooksweep/decline_error_test.go` to the new value type, and confirm `StaleKeys`, `narrowToSnapshot`, `hooksweep` and `cmd/doctor.go`'s `checkStaleHooks` still compile and pass untouched — they read keys and lengths only.
- Edit the `hooks` row of CLAUDE.md's package table where it says the store holds per-pane on-resume commands: an event's stored value is a `Registration` decoded from either a JSON string (the command alone) or a JSON object (`command` plus `resume`), both shapes permanently valid with no migration between them; an entry a mutation did not name is re-emitted from the bytes it was decoded from, so an attribute the reader does not model and a `resume` value it cannot make sense of both survive a sibling's rewrite; and a value that is neither string nor object never fails the load for its neighbours.
```

Task 1.5 — final **Do** bullet, with one bullet added after it:
```markdown
- Update the `prefs.json` row of the README's config table to name `resume_mode` alongside the grouping mode and the theme keys, stating that it holds `eager` or `lazy` and is the install-wide default a registration that names no mode of its own inherits.
- Edit the `prefs` row of CLAUDE.md's package table: add `"resume_mode": "eager"\|"lazy"` to the literal key shape it enumerates, state that the field decodes tolerantly and independently like every other one there — missing, empty, corrupt, unrecognised or a file that cannot be read at all all give the shipped default, `lazy` — and that nothing in Portal writes it; and widen the leaf clause from "stdlib + `fileutil` only" to "stdlib + `fileutil` + `internal/resumemode` only", leaving the no-`internal/log` rule and its reasoning exactly as they are.
```

Task 1.7 — final **Do** bullet, with one bullet added after it:
```markdown
- Update the `hook list` line in the README's `hook` example block to describe the fifth column and what an empty cell means.
- Edit CLAUDE.md's **Resume-hook command** paragraph where it describes the location column as "a fourth tab-separated column after key/event/command": the location column is followed by a fifth holding the registration's mode — `eager`, `lazy`, or empty when it carries none — taken from the store read the listing already performs, so no second tmux read is added and the first four columns stay byte-identical for a positional parser.
```

**Resolution**: Pending
**Notes**:

---

### 2. Task 6.2's README instruction is keyed to a sentence the README does not contain

**Severity**: Minor
**Plan Reference**: Phase 6, task 6.2; Phase 6 "Planner's calls" in `planning.md`
**Category**: Acceptance Criteria Quality / Task Self-Containment
**Move**: settled
**Change Type**: update-task

**Problem**:
Task 6.2 tells the executor to edit "the sentence that names the host-terminal line as informational" so it "no longer claims it is the only one". No such sentence exists. The README's `xctl doctor` paragraph ends: "The host-terminal check (folding in the retired `spawn --detect`) prints the detected terminal and its bundle id so you can copy it into [`terminals.json`](#configuration)." — it neither calls that line informational nor claims it is the only one. The executor reaches the file, finds nothing matching the instruction, and has to invent both the correction and its placement; and task 6.3, which extends "the README's `xctl doctor` sentence about informational lines", depends on whatever they invent. Phase 6's planner's calls repeat the same misreading.

**Proposal**:
Restate the instruction as the addition it actually is, anchored to the sentence that does exist. The README's current text settles it — the edit is an added sentence after the host-terminal one, not a correction of it. Task 6.3's bullet then works unchanged, because 6.2 now demonstrably creates the sentence it extends.

**Current**:

Task 6.2 — fifth **Do** bullet:
```markdown
- Edit the `xctl doctor` paragraph of the README: the sentence that names the host-terminal line as informational no longer claims it is the only one — it names the pending-resume line beside it, stating that it reports how many panes are waiting and never affects the exit code.
```

`planning.md`, Phase 6 "Planner's calls", the doc-edits bullet:
```markdown
- Doc edits ride with the tasks that falsify them, as the earlier phases did: the README's doctor paragraph names the host-terminal line as the one informational line, and one sentence edit covers both new lines; CLAUDE.md's tmux row enumerates the client's pane-enumeration methods, and one line covers the new read.
```

**Proposed Text**:

Task 6.2 — fifth **Do** bullet:
```markdown
- Add a sentence to the `xctl doctor` paragraph of the README, after the one describing the host-terminal check: the report also carries informational lines that state rather than judge — the pending-resume line reporting how many panes are holding a resume decision — and, like the host-terminal line, they never affect the exit code. The paragraph makes no claim today about which lines are informational, so this is an addition rather than a correction of an existing sentence.
```

`planning.md`, Phase 6 "Planner's calls", the doc-edits bullet:
```markdown
- Doc edits ride with the tasks that falsify them, as the earlier phases did: the README's doctor paragraph describes the host-terminal check but says nothing today about informational lines, so one added sentence introduces the category and the pending-resume line, and the resume-mode task extends that same sentence; CLAUDE.md's tmux row enumerates the client's pane-enumeration methods, and one line covers the new read.
```

**Resolution**: Pending
**Notes**: Task 6.3's own bullet ("Extend the README's `xctl doctor` sentence about informational lines to name the resume-mode line beside the pending-resume one") needs no change once 6.2 creates that sentence.

---
