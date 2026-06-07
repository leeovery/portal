---
status: in-progress
created: 2026-06-07
cycle: 1
phase: Gap Analysis
topic: session-tagging-and-grouping
---

# Review Tracking: session-tagging-and-grouping - Gap Analysis

## Findings

### 1. Tag value normalisation / validation rules are undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Tag Data Model & Persistence; Assigning & Managing Tags; Grouping Semantics (By Tag)
**Priority**: Critical

**Details**:
The spec says the Tags field behaves "exactly like the existing alias field" (type + Enter to add, `x` to remove), but never defines validation/normalisation rules for tag *values*. Because tags are also the grouping key, value handling has direct user-visible consequences the alias field does not have:

- **Case sensitivity:** Is `Work` the same tag as `work`? In By Tag mode this decides whether they collapse into one group heading or render as two separate groups. Unstated.
- **Whitespace:** Are leading/trailing spaces trimmed? Is `"  work"` distinct from `"work"`? Tags drive group headings, so untrimmed values produce confusing/duplicate headings.
- **Empty / whitespace-only tag:** Can the user press Enter on a blank input and add an empty tag? What renders as its heading?
- **Duplicate within a project:** "A directory carries multiple tags" — is adding the same tag twice to one project deduped, rejected, or silently allowed (producing a project that appears once-per-duplicate under the same heading)?
- **Allowed characters / max length:** Any constraints, or anything goes?

An implementer cannot build the add-tag path or the group-key derivation without these rules, and getting them wrong silently fractures groups. The "implicit tags = union across projects" model makes the canonical comparison form load-bearing (the union dedup depends on it).

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 2. Lazy stamp-on-render mutates tmux state during a render pass — failure/ordering behaviour unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Session → Directory Resolution (The lazy stamp-on-render fallback)
**Priority**: Important

**Details**:
The fallback "resolves that session's directory from the active pane's current path → git-root and stamps `@portal-dir` then and there (lazy)." This introduces a write (set-session-option) and a `git rev-parse`-equivalent during what is otherwise a read/render. Several behaviours are left to the implementer:

- **What if the stamp write fails** (tmux error / session killed mid-render)? Does the session still render this pass via the in-memory derived value, or drop out?
- **What if git-root derivation succeeds but the set-option fails** — is the session stamped next render, every render (perf), or treated as Unknown?
- **Per-render cost when many un-stamped sessions exist** (first ship with 15-20 live sessions): the spec frames this as "the un-stamped minority," but on first ship *all* live sessions are un-stamped, so the very first grouped render does N git-root derivations + N writes. Is that acceptable inline, or does it need a one-time amortisation note? The spec asserts "no perf objection" only for the steady-state minority case.
- **Is the derived-and-stamped value also used for *this* render**, or only cached for the next one? (Spec implies this render — "They appear in By Project immediately" — but doesn't state it explicitly.)

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 3. Prefs file format, filename, and schema undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Mode Persistence & Empty States (Remember last mode)
**Priority**: Important

**Details**:
The spec introduces "a small prefs file under `~/.config/portal/`, using the existing `configFilePath` + `AtomicWrite` pattern" to persist the last-used grouping mode, but leaves the concrete shape undefined:

- **Filename** (e.g. `prefs.json`?) — needed so it slots into `configFilePath` per-file env-var resolution like the other config files.
- **Format & schema** — JSON object with what key? (e.g. `{"session_list_mode": "by-tag"}`). The three modes need a stable on-disk encoding (string enum? int?).
- **Corrupt / unparseable prefs file behaviour** — fall back to Flat? Warn? The other stores have defined decode-failure behaviour; this new one needs the same.
- **Migration / env-var override** — does it participate in the `migrateConfigFile` one-shot move and the per-file env-var override convention, or is it exempt?
- **Write timing** — is the mode persisted on every toggle press, or on exit? (Every `s` press writing to disk vs. flush-on-quit is a real implementation choice.)

Without these an implementer must invent the file layout and failure semantics.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 4. By Project heading label text is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Grouping Semantics (Modes — By Project); TUI Rendering (Group headers)
**Priority**: Important

**Details**:
By Project renders "a heading per directory," and the header example is `Portal ··· 2`. But the spec never states what string the heading actually shows. The `Project` record has both `name` and `path`. Candidates: project `name`, the full `path`, or the directory basename. This matters because:

- Two different paths can share a basename (`~/code/portal` and `~/archive/portal`) — basename headings would collide/merge visually.
- The example `Portal` is capitalised, suggesting the project `name`, but that is never stated.

An implementer needs the heading source field and any collision handling defined.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 5. Toggle behaviour from the "No tags yet" signposted state is undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: TUI Rendering (Toggle key); Mode Persistence & Empty States (By Tag with zero tags)
**Priority**: Important

**Details**:
Two interacting rules:
1. The persisted mode reopens Portal in the user's last view — including **By Tag**.
2. By Tag with zero tags renders the flat list + "No tags yet" signpost.

If the user's persisted mode is By Tag and they have (or later delete to) zero tags, Portal opens showing the signposted flat list. Pressing `s` from there — the spec says the cycle is Flat → By Project → By Tag → Flat. From the signposted By Tag state, one `s` press advances to Flat. That is probably fine, but the spec never confirms which logical mode the signposted state *is* for cycling purposes, nor whether the user can land back on a meaningless By Tag while still having zero tags (the cycle will keep returning them to a signpost). Confirm the cycle still includes By Tag when zero tags exist, or whether By Tag is skipped — this is a concrete branch an implementer must code.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 6. Header count semantics for multi-membership sessions are unstated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: TUI Rendering (Group headers — Counted); Grouping Semantics (Pattern B)
**Priority**: Minor

**Details**:
Headers are "Counted" (`Portal ··· 2`). In By Tag mode a session appears under each tag it has (Pattern B). It is reasonable to infer each tag header counts the sessions rendered under *that* heading (so one session contributes to multiple counts), but this is never stated. Confirm: per-group count = number of rows under that heading (so totals across headings exceed the live session count in By Tag mode). Cheap to state, removes an implementer guess.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 7. "Cursor never lands on a header" — first/empty-cursor and boundary behaviour underspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: TUI Rendering (Group headers — Non-selectable)
**Priority**: Minor

**Details**:
Headers are non-selectable; "the cursor jumps session-to-session and never lands on a header." `bubbles/list` does not natively support non-selectable rows, so this is custom skip logic the implementer must build. Edge cases left open:

- **Initial cursor position** when the list opens in grouped mode — first session row (skipping the leading header), presumably.
- **GoToStart/GoToEnd (`g`/`G` are bound by bubbles/list)** — do these land on the first/last *session* or could they land on a header?
- **Filtering interaction:** the spec says grouping must be a render-layer concern so the built-in filter "only ever sees session items." If headers are injected at render only (not list items), the non-selectable skip is automatic — but if headers are list items, skip logic and the filter-only-sees-sessions requirement may conflict. Clarify which implementation the build-note mandates, since it determines whether skip logic is even needed.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 8. Behaviour when a project is deleted (or its tags removed) while sessions are live is not stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Tag Data Model (Lifecycle); Grouping Semantics
**Priority**: Minor

**Details**:
The lifecycle section says tags "are removed when the project is deleted (projects-page `d`)." But a session stamped with `@portal-dir = <that dir>` can still be live after its project record is deleted. On the next grouped render:

- **By Project:** the `@portal-dir` lookup finds no `Project` record. Does that session fall to the **Unknown** bucket (which is defined only for the unresolvable-git-root case), or get a heading from the bare path, or drop out?
- **By Tag:** no project record → no tags → **Untagged** bucket, presumably. Worth confirming.

The Unknown bucket is currently scoped to "no `@portal-dir` AND no derivable git-root." A stamped session whose project record was deleted is a *different* case (stamp present, lookup misses) that the spec does not route. An implementer needs the missing-project-record-but-stamped path defined.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 9. Acceptance criteria are implicit, not explicit

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Whole specification (planning readiness)
**Priority**: Minor

**Details**:
The spec is decision-rich and rationale-heavy but states no explicit acceptance criteria / verifiable behaviours per section (e.g. "Given a project with tags [work, personal] and one live session in it, By Tag renders that session under both `work` and `personal` headings and under no Untagged group"). Most behaviours are inferable, but a planner breaking this into tasks would benefit from a short Given/When/Then list to anchor test cases — especially for the three modes, the Untagged/Unknown buckets, flatten-on-filter restore, and the empty-state signpost. This is a planning-readiness nicety, not a blocker.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---
