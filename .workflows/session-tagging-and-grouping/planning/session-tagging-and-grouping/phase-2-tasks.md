---
phase: 2
phase_name: Grouped render — By Project & By Tag
total: 6
---

## session-tagging-and-grouping-2-1 | approved

### Task session-tagging-and-grouping-2-1: Extend SessionItem with group metadata (group key, heading label, optional tag)

**Problem**: The render-layer item model requires every `bubbles/list` item to remain a session instance while carrying enough information for the delegate to inject a heading at a group-key boundary and for By-Tag mode to materialise a multi-tag session as several instances. Today `SessionItem` (`internal/tui/session_item.go`) wraps only a `tmux.Session` — it has no group key, no heading label, and no per-instance tag, so the render layer cannot decide where group boundaries fall or which session attaches.

**Solution**: Add group-metadata fields to `SessionItem` — a `GroupKey` (the sort/boundary key), a `GroupHeading` (the human label the delegate dims), an optional `Tag` (the canonical tag for a By-Tag instance), and a `CatchAll` flag (marks an Unknown/Untagged-bucket instance so the pinned-last ordering and label are unambiguous). All new fields are zero-valued for Flat items, so a flat item is byte-for-byte today's item; `FilterValue()` and `Title()` continue to return the session name; the underlying `Session` is unchanged so multiple instances of one session share the same attach target.

**Outcome**: `SessionItem` carries `GroupKey string`, `GroupHeading string`, `Tag string`, and `CatchAll bool` alongside the existing `Session tmux.Session`; a zero-value/Flat `SessionItem` has empty group metadata and renders and filters exactly as today; two `SessionItem`s built from the same `tmux.Session` but different `Tag`/`GroupKey` both report the same `FilterValue()`/`Title()` (the session name) and the same underlying `Session.Name`, establishing that duplicate instances are views of one session.

**Do**:
- In `/Users/leeovery/Code/portal/internal/tui/session_item.go`, extend the `SessionItem` struct:
  - `GroupKey string` — the boundary/sort key (canonical dir path for By Project, canonical tag for By Tag; empty for Flat).
  - `GroupHeading string` — the label the delegate renders dimmed (project `name` for By Project, tag value for By Tag, `"Unknown"`/`"Untagged"` for catch-alls; empty for Flat).
  - `Tag string` — the canonical tag this instance sits under in By-Tag mode (empty for Flat, By Project, and Untagged).
  - `CatchAll bool` — true for an Unknown (By Project) or Untagged (By Tag) instance; lets the grouping builders and ordering pin these last without string-matching the heading.
- Leave `FilterValue()` and `Title()` returning `i.Session.Name` unchanged — the built-in filter must keep matching on session name only, and headers must never become filterable items.
- Keep `Description()` unchanged (window count + attached badge).
- Do NOT change the existing `ToListItems` signature/behaviour — it still produces flat items with empty group metadata (it is the Flat-mode builder). The By-Project / By-Tag builders are added in tasks 2-2 / 2-3 as separate constructors that return `[]list.Item` of these enriched `SessionItem`s.
- Add a small construction note (doc comment) clarifying that two `SessionItem`s sharing a `Session` but differing in `Tag`/`GroupKey` are independently selectable views of one underlying session, and that selection/attach keys on `Session.Name` (see task 2-6).

**Acceptance Criteria**:
- [ ] `SessionItem` has `GroupKey string`, `GroupHeading string`, `Tag string`, and `CatchAll bool` fields in addition to `Session tmux.Session`.
- [ ] A zero-value `SessionItem` (Flat) has empty `GroupKey`/`GroupHeading`/`Tag` and `CatchAll == false`.
- [ ] `FilterValue()` and `Title()` return `Session.Name` regardless of group metadata.
- [ ] Two `SessionItem`s built from the same `tmux.Session` with different `Tag`/`GroupKey` both report the same `Session.Name` (shared underlying session).
- [ ] `ToListItems` still produces flat items with empty group metadata (unchanged behaviour).
- [ ] Existing `TestSessionItem` / `TestToListItems` assertions still pass unmodified.

**Tests**:
- `"it leaves Flat items with empty group metadata"`
- `"it returns the session name from FilterValue regardless of group fields"`
- `"it returns the session name from Title regardless of group fields"`
- `"it builds two instances of one session sharing the same underlying Session.Name"`
- `"it keeps ToListItems producing flat items with no group metadata"`

**Edge Cases**:
- Zero-value/Flat item carries no group metadata — proves the additive, no-regression guarantee at the item level.
- `FilterValue` still returns the session name (the built-in filter never sees a heading).
- Multiple By-Tag instances share one underlying `Session` (duplicate instances are views, not distinct targets).

**Context**:
> Build note: grouping must be a render-layer concern: every `bubbles/list` item is a session instance, and group headings are injected at render time as visual separators — never as list items. "Session instance" — not "exactly one item per session": By Tag mode legitimately materialises a multi-tag session as several instances. The key invariant is that no list item is a header.
>
> Item model: the flat item slice fed to `bubbles/list` is pre-sorted into grouped order, and headers are injected at each group-key boundary (when the current item's group key differs from the previous item's). Selecting any instance of a session attaches the same underlying session (the duplicate instances are views of one session, not distinct targets).

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Filter Composition → Build note, § Item model (the central rendering mechanism))

## session-tagging-and-grouping-2-2 | approved

### Task session-tagging-and-grouping-2-2: By Project grouping builder (session → dir key → project name heading, pre-sorted)

**Problem**: By Project mode must render each live session exactly once under its project `name` heading, with the slice pre-sorted into grouped order (heading alphabetical, then session name) so the delegate can inject a heading at each group-key boundary. Phase 1 produced the resolution primitives (`tmux.Session.Dir`, `project.CanonicalDirKey`, `project.MatchProjectByDir`) but nothing assembles a session list into By-Project grouped order.

**Solution**: Add a pure builder in `internal/tui` that takes the live sessions and the loaded project records and returns a pre-sorted `[]list.Item` of `SessionItem`s. Each session resolves to a canonical directory key (its `Session.Dir`, already canonicalised upstream by Phase 1; the lazy fallback/restamp from Phase 1 has already populated `Dir` for un-stamped sessions before this builder runs), looks up the matching `Project` via `MatchProjectByDir` to get the heading `name`, and is assigned `GroupKey = canonical dir path`, `GroupHeading = project name`. Items are sorted by `(GroupKey, Session.Name)` — keying the group on the canonical path, not the name, so two distinct directories sharing a name form two separate groups. Sessions that do not resolve to a known project are handed to the catch-all routing built in task 2-4 (this builder marks them, task 2-4 owns the Unknown bucket and pinning); this task focuses on the known-project happy path and the sort/boundary contract.

**Outcome**: Given N live sessions and a set of project records, `buildByProject` returns one `SessionItem` per session, each with `GroupKey` = its canonical directory path and `GroupHeading` = the matched project's `name`, the slice ordered group-by-group alphabetically by heading then by session name within a group; two distinct directories that share a `name` produce two separate `GroupKey` groups (each renders its own heading at its own boundary); a session whose directory resolves to no known project is flagged for the Unknown bucket (handed to task 2-4's routing) rather than silently dropped.

**Do**:
- Add `func buildByProject(sessions []tmux.Session, projects []project.Project) []list.Item` (e.g. a new `internal/tui/grouping.go` in package `tui`).
- For each session: compute the canonical dir key. `Session.Dir` is already the canonical stamped/derived value from Phase 1 — but defensively run it through `project.CanonicalDirKey(s.Dir)` so the lookup key matches stored `Project.Path` exactly (the spec's lookup-key-matches-stored-path invariant). An empty `Session.Dir` (no stamp, fallback yielded nothing) means unresolvable → route to the Unknown bucket (task 2-4).
- Look up the project via `project.MatchProjectByDir(projects, s.Dir)`. On hit, `GroupKey = CanonicalDirKey(s.Dir)`, `GroupHeading = matched.Name`. On miss (stamped path with no matching `Project` record, e.g. project deleted while session live) → route to Unknown (task 2-4) — do NOT synthesise a heading from the bare path.
- Sort the resolved (known-project) items by `GroupKey` ascending, then `Session.Name` ascending (`slices.SortFunc` with a stable tuple compare). The group key is the canonical path, NOT the heading name — so `~/code/portal` and `~/archive/portal` (both named `Portal`) sort/group as two distinct keys even though their headings read identically.
- Return one item per session (By Project is Pattern A — single membership). Leave `Tag` empty and `CatchAll` false on known-project items.
- Append the catch-all (Unknown) items LAST via the task 2-4 helper (this task may stub the catch-all call behind task 2-4's signature; the two tasks compose — keep this builder's known-project sorting independently testable by passing only resolvable sessions).
- This is a pure function (no tmux calls, no I/O) so it is directly unit-testable with seeded `tmux.Session` slices and `project.Project` slices — exercisable without the `s` toggle existing.

**Acceptance Criteria**:
- [ ] `buildByProject` returns exactly one item per resolvable session (Pattern A — single membership).
- [ ] Each resolvable item has `GroupKey` = canonical dir path and `GroupHeading` = the matched project `name`.
- [ ] Items are ordered by `GroupKey` (alphabetical), then `Session.Name` (alphabetical) within a group.
- [ ] Two distinct directories sharing a project `name` produce two distinct `GroupKey` groups (key is path, not name).
- [ ] A session with empty `Session.Dir`, or a stamped dir matching no `Project` record, is routed to the Unknown bucket (task 2-4), never dropped.
- [ ] Zero live sessions returns an empty slice (no headers, no panic).

**Tests**:
- `"it builds one item per session under its project name heading"`
- `"it orders items by group key then session name"`
- `"it forms two separate groups for two distinct dirs sharing a project name"`
- `"it routes a session with empty Dir to the Unknown bucket"`
- `"it routes a stamped dir with no matching project record to the Unknown bucket"`
- `"it returns an empty slice for zero live sessions"`

**Edge Cases**:
- Two distinct dirs sharing a project name form two groups (key is canonical path, not name).
- `Session.Dir` empty (no stamp, fallback yielded nothing) → Unknown bucket.
- Stamped dir with no matching project record (deleted project, live session) → Unknown bucket.
- Zero live sessions → empty slice.

**Context**:
> By Project — single-valued → Pattern A. A session has exactly one directory, so it appears once. By Project items are pre-sorted by project heading, then session name.
> By Project headings show the project `name` (the friendly name on the `Project` record). The grouping key is the canonical directory path, not the name — so two distinct directories that happen to share a `name` form two separate groups that may display the same heading text. This visual repeat is accepted in v1.
> Stamped, but no matching project record: the path lookup misses → the session falls to the Unknown bucket. No attempt is made to synthesise a heading from the bare path.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Grouping Semantics → Modes, Heading label text, Ordering; § Item model)

## session-tagging-and-grouping-2-3 | approved

### Task session-tagging-and-grouping-2-3: By Tag grouping builder (one item per (session, tag), pre-sorted)

**Problem**: By Tag mode is multi-valued (Pattern B): a session whose directory carries tags `[work, personal]` must render once under each tag heading, materialised as one list item per `(session, tag)` pair, with `work`/`Work`/`WORK` collapsing into a single heading via the canonical form. A zero-tag session contributes exactly one item destined for the Untagged bucket. No builder produces this `(session, tag)` expansion in pre-sorted grouped order.

**Solution**: Add a pure `buildByTag` builder in `internal/tui` that, for each session, resolves its directory's project (via `MatchProjectByDir`), reads that project's `Tags` (already stored in canonical lower-cased form by Phase 1), and emits one `SessionItem` per tag with `GroupKey = Tag = canonical tag`, `GroupHeading = canonical tag`. A session whose project has no tags (or whose directory resolves to no project) emits exactly one item flagged for the Untagged catch-all (handed to task 2-4). The full item slice is sorted by `(GroupKey, Session.Name)` so the delegate injects a heading at each tag boundary, with the catch-all pinned last by task 2-4.

**Outcome**: Given a project tagged `[work, personal]` with one live session, `buildByTag` emits two items — one with `Tag == "work"` and one with `Tag == "personal"` — each `GroupHeading` equal to its canonical tag; two projects tagged `work` and `Work` respectively yield items all under a single `work` `GroupKey`/`GroupHeading` (canonical collapse); a zero-tag session (or a session whose directory has no project record) emits exactly one item flagged for Untagged; the resolvable items are ordered by tag (alphabetical) then session name.

**Do**:
- Add `func buildByTag(sessions []tmux.Session, projects []project.Project) []list.Item` to `internal/tui/grouping.go`.
- For each session: resolve its project via `project.MatchProjectByDir(projects, s.Dir)` (run `s.Dir` through the same canonical keying as task 2-2). On a project hit with a non-empty `Tags` slice, emit one `SessionItem` per tag: `Tag = tag`, `GroupKey = tag`, `GroupHeading = tag` (tags are already canonical from Phase 1; do not re-normalise destructively, but treat them as canonical comparison keys — `work`/`Work`/`WORK` collapse because Phase 1 stored them lower-cased, so the union/dedup is automatic at the data layer).
- A project hit with an EMPTY `Tags` slice, OR a project miss (no record), OR an empty `Session.Dir` → emit exactly ONE item flagged for the Untagged catch-all (`CatchAll = true`, routed via task 2-4 — do not set a tag).
- Pattern B count contract: a multi-tag session contributes N items; the sum of items therefore exceeds the live session count when any session has >1 tag — this is intended (drives the per-heading counts in task 2-5).
- Sort resolvable (tagged) items by `GroupKey` (canonical tag) ascending, then `Session.Name` ascending. The Untagged items are appended last by task 2-4.
- Every emitted instance shares the same underlying `tmux.Session` (so selecting any attaches the same session — task 2-6).
- Pure function — no tmux/I/O — unit-testable with seeded sessions/projects, exercisable without the toggle.
- Defensive canonical comparison: if two project records carry the "same" tag in different stored casings (should not happen post-Phase-1, but be robust), they must still collapse to one `GroupKey`. Compare/group on the canonical (lower-cased) form so a stray non-canonical stored value cannot split a heading.

**Acceptance Criteria**:
- [ ] A project tagged `[work, personal]` with one live session yields two items, one per tag, each `GroupHeading` = its canonical tag.
- [ ] Two projects tagged `work` and `Work` collapse so their sessions appear under a single `work` `GroupKey`/`GroupHeading`.
- [ ] A zero-tag session contributes exactly one item flagged for the Untagged bucket (`CatchAll == true`).
- [ ] A session whose directory has no matching project record contributes exactly one Untagged item.
- [ ] Resolvable (tagged) items are ordered by canonical tag, then session name.
- [ ] Each emitted instance references the same underlying `tmux.Session` as its siblings (shared attach target).
- [ ] The sum of By-Tag items exceeds the live session count when any session carries more than one tag.

**Tests**:
- `"it emits one item per tag for a multi-tag session"`
- `"it collapses work, Work and WORK into a single tag heading"`
- `"it emits exactly one Untagged item for a zero-tag session"`
- `"it emits one Untagged item for a session whose dir has no project record"`
- `"it orders tagged items by canonical tag then session name"`
- `"it shares the underlying session across a multi-tag session's instances"`

**Edge Cases**:
- Multi-tag session yields N items (sum exceeds live session count — intended for Pattern B counts).
- `work`/`Work`/`WORK` collapse to one heading (canonical form).
- Zero-tag session yields exactly one item (Untagged), never zero.
- Session whose dir has no project record → one Untagged item.

**Context**:
> By Tag — multi-valued → Pattern B. A session appears once under each tag it has. A multi-tag session is materialised as one list item per tag; every instance attaches the same underlying session.
> By Tag: one item per `(session, tag)` pair. A session with tags `[work, personal]` materialises as two list items. A zero-tag session contributes one item, pinned into the Untagged group.
> The same canonical form (trim + lower-case) is used everywhere a tag is compared: per-project dedup, the cross-project union, and By-Tag grouping. `Work`, `WORK`, and `work` are the same tag — they collapse into one By-Tag heading.
> By Tag: no project record → no tags → the session falls to Untagged.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Grouping Semantics → Modes, The grouping-key problem; § Item model; § Tag value normalisation & validation)

## session-tagging-and-grouping-2-4 | approved

### Task session-tagging-and-grouping-2-4: Pinned catch-all buckets (Unknown / Untagged) with empty-suppression

**Problem**: Sessions that do not resolve to a known project (By Project) or carry no tags (By Tag) must never be silently dropped — they collect under a single pinned catch-all (Unknown / Untagged) rendered last with a count. But an empty catch-all must be suppressed: when every session resolves / is tagged, no spurious `Unknown`/`Untagged` heading should appear. The grouping builders (tasks 2-2 / 2-3) flag these sessions but do not yet own the pinned-last ordering or the suppression rule.

**Solution**: Add a small helper that takes the alphabetically-sorted resolvable items plus the catch-all items (flagged `CatchAll = true` with `GroupHeading` set to `"Unknown"` or `"Untagged"`) and returns the final ordered slice: resolvable groups first (already alphabetical), catch-all items appended last; catch-all items sorted by session name among themselves; the catch-all heading uses a fixed `GroupKey` sentinel that pins it after all real keys; when there are zero catch-all items, none are appended (suppression — no header materialises because no items carry that group key, and task 2-5's boundary logic only emits a header when items exist under it).

**Outcome**: When at least one session falls to the catch-all, the returned slice ends with the catch-all items grouped under a single pinned `Unknown` (By Project) / `Untagged` (By Tag) heading, sorted by session name; when zero sessions fall to the catch-all, no catch-all items and therefore no catch-all heading appear; a session that is unresolvable (no git-root / empty Dir) or whose project was deleted appears in Unknown (By Project) / Untagged (By Tag) and is never dropped from the rendered set.

**Do**:
- Add a helper, e.g. `func appendCatchAll(resolved []list.Item, catchAll []list.Item, heading string) []list.Item`, in `internal/tui/grouping.go`, and wire it into `buildByProject` (heading `"Unknown"`) and `buildByTag` (heading `"Untagged"`).
- Assign catch-all items a fixed pinning `GroupKey` that sorts AFTER every real group key. Two robust options: (a) set `CatchAll = true` and make the final assembly append catch-all items after the sorted resolvable items unconditionally (ordering by append position, not by string key — simplest and avoids a magic sentinel string colliding with a real path/tag); (b) use a sentinel key. Prefer (a): keep resolvable items sorted by their real `GroupKey`, then append catch-all items (themselves sorted by `Session.Name`) so they are pinned last regardless of the heading's alphabetical position. Set each catch-all item's `GroupHeading` to the passed heading and `GroupKey` to a constant (e.g. the heading itself) so the boundary logic in task 2-5 still treats them as one contiguous group.
- Empty-suppression: if `len(catchAll) == 0`, return `resolved` unchanged — no catch-all heading is introduced. Because task 2-5 injects a heading only at a group-key boundary among existing items, absence of catch-all items guarantees absence of the heading. Add a test asserting the rendered output (or the item slice's headings) contains no `Unknown`/`Untagged` when the catch-all is empty.
- Both `Unknown` and `Untagged` carry a count like any other heading (the count is computed by task 2-5 from the rows beneath the heading) — this helper just guarantees they form one contiguous, pinned-last group.
- Never drop a session: every session handed to `buildByProject`/`buildByTag` must appear at least once in the returned slice (resolvable group or catch-all). Add a test that asserts the count of distinct `Session.Name`s in the output ≥ the count of input sessions for By Project (exactly equal for Pattern A), and that every input session name appears at least once.
- Pure function — unit-testable with seeded item slices.

**Acceptance Criteria**:
- [ ] Catch-all items are appended LAST, after all alphabetically-sorted resolvable groups (overrides alphabetical ordering).
- [ ] Catch-all items are ordered by `Session.Name` among themselves and form one contiguous group under a single heading.
- [ ] By Project uses heading `"Unknown"`; By Tag uses heading `"Untagged"`.
- [ ] When there are zero catch-all items, no catch-all heading appears (empty-suppression).
- [ ] An unresolvable-dir session and a deleted-project session both appear in the catch-all (Unknown / Untagged respectively) and are never dropped.
- [ ] Every input session appears at least once in the output (By Project: exactly once; By Tag: at least once).

**Tests**:
- `"it pins the catch-all group last after alphabetical headings"`
- `"it orders catch-all items by session name"`
- `"it suppresses the Unknown heading when no session is unresolvable"`
- `"it suppresses the Untagged heading when every session is tagged"`
- `"it routes an unresolvable-dir session to Unknown in By Project"`
- `"it routes a deleted-project session to Untagged in By Tag"`
- `"it never drops a session from the rendered set"`

**Edge Cases**:
- Empty bucket suppressed (no `Unknown`/`Untagged` header when membership is zero).
- Unresolvable dir (no git-root / empty `Dir`) → Unknown (By Project) / Untagged (By Tag).
- Deleted-project session (stamped dir, no matching record) → Unknown (By Project) / Untagged (By Tag).
- Catch-all pinned last after alphabetical headings.
- Session never dropped — every input session appears at least once.

**Context**:
> Untagged bucket. In By Tag mode, sessions whose directory has no tags collect under a single Untagged group, pinned last.
> Catch-all bucket rendering. The pinned Untagged (By Tag) and Unknown (By Project) buckets are ordinary group headers: they carry a count like any other heading (e.g. `Untagged ··· 3`), and they are rendered only when their membership is ≥ 1. An empty catch-all is suppressed.
> Catch-all buckets pinned last (overrides alphabetical): Untagged in By Tag mode, Unknown in By Project mode.
> Unresolvable directory: a session whose `@portal-dir` is absent and whose lazy fallback cannot derive a git-root collects under a single pinned Unknown group at the end. The Unknown bucket covers both "no derivable directory" and "directory resolved but not a known project".

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Grouping Semantics → Catch-all bucket rendering, Ordering; § Mode Persistence & Empty States → Empty states)

## session-tagging-and-grouping-2-5 | approved

### Task session-tagging-and-grouping-2-5: Header-injecting counted delegate render (dimmed heading + row count at group boundary)

**Problem**: With the item slice pre-sorted into grouped order (tasks 2-2 / 2-3 / 2-4), the headings must be injected at render time as dimmed, counted visual separators — never as list items. Today `SessionDelegate.Render` (`internal/tui/session_item.go`) renders only the session line; it has no notion of a group boundary, a heading, or a per-group count.

**Solution**: Extend `SessionDelegate.Render` to inject a dimmed group heading line above an item when that item begins a new group — detected by comparing the current item's `GroupKey` against the previous list item's `GroupKey` (the leading item always starts a group). Each heading carries a count of the rows rendered beneath that heading (computed from the contiguous run of same-`GroupKey` items in `m.Items()`). Flat items (empty `GroupKey`) inject no heading, preserving today's output byte-for-byte. The delegate styles the heading with a dimmed lipgloss style (layered into the existing delegate styles) and renders the count in the `Heading ··· N` form.

**Outcome**: Rendering a grouped slice produces a dimmed heading line before the first item of each group (including a leading heading before the very first item), each heading reading `<GroupHeading> ··· <count>` where count is the number of contiguous items sharing that `GroupKey`; a By-Tag slice where one session appears under two tags renders that session's row beneath both tag headings and counts it under each (so the sum of By-Tag header counts exceeds the live session count); a Flat slice (all items with empty `GroupKey`) renders with no headings — identical to today's output; the last group's heading carries the correct count of its trailing run.

**Do**:
- In `/Users/leeovery/Code/portal/internal/tui/session_item.go`, modify `SessionDelegate.Render(w, m, index, item)`:
  - Cast `item` to `SessionItem` (existing guard).
  - Determine "starts a new group": if `si.GroupKey == ""` (Flat) → no heading. Otherwise, fetch the previous item via `m.Items()[index-1]` (guard `index == 0` → always a leading heading for a grouped slice) and compare its `GroupKey`; if different (or `index == 0`), this item starts a new group and a heading is emitted above the session line.
  - Compute the group count: walk `m.Items()` forward from the first item of this group (this index, since headings only emit at the first item of a run) counting contiguous items whose `GroupKey == si.GroupKey`. Because the slice is pre-sorted into grouped contiguous runs, a forward scan from `index` until the key changes yields the count. (Alternatively precompute counts once — but the delegate is called per visible row; a bounded forward scan from the group's first index is acceptable for ~15–20 sessions. Document the choice.)
  - Render the heading with a new dimmed lipgloss style (e.g. `headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))` or `.Faint(true)` — layer into the existing `detailStyle`/delegate styles; match the spec's "dimmed" intent) in the form `<GroupHeading> ··· <count>`, on its own line above the session line. The session line itself is rendered exactly as today (cursor + name + detail).
  - Use the existing separator glyph convention if one exists; the spec's examples use `···` (e.g. `Portal ··· 2`, `Untagged ··· 3`).
- The heading is purely a render-layer string written to `w` — it is NOT a list item and NEVER increments the list index. The cursor/selection logic (task 2-6) is unaffected because `m.Items()` still contains only `SessionItem`s.
- Flat-mode guarantee: when every item has empty `GroupKey`, no heading is ever emitted, so `Render` output is byte-identical to today's. Add a regression test asserting a Flat item renders with no heading line.
- The Height/Spacing contract: today `Height() == 1`. A heading line adds vertical space only for the first item of a group. NOTE: `bubbles/list` uses `Height()` for pagination math, so a per-item variable height is a known tension. For v1, keep the heading rendered within the delegate's output for the boundary item and document that the leading-heading row consumes the item's single line plus the heading line; if pagination miscounts, prefer rendering the heading as a prefixed line within the same `Render` write (the list measures by `Height()` returning 1; the extra heading line is drawn but not counted — accept the minor pagination imprecision in v1, or pin `Height()` handling in a follow-up). Document the tradeoff explicitly in a code comment and a test note; do NOT route through `lipgloss/tree` to solve it (build constraint).

**Acceptance Criteria**:
- [ ] A grouped slice renders a dimmed heading line before the first item of each group, including a leading heading before the first item.
- [ ] Each heading reads `<GroupHeading> ··· <count>` with count = the number of contiguous items sharing that group key.
- [ ] A By-Tag slice where one session appears under two tags renders the session beneath both headings and counts it under each (sum of header counts exceeds live session count).
- [ ] A Flat slice (empty group keys) renders with no headings — byte-identical to today's output.
- [ ] The last group's heading carries the correct count of its trailing run.
- [ ] Headings are written to the render writer only; `m.Items()` still contains only `SessionItem`s (no header items).
- [ ] The picker is not routed through `lipgloss/tree`.

**Tests**:
- `"it injects a dimmed heading before the first item of each group"`
- `"it injects a leading heading before the very first item"`
- `"it renders a per-group count of the rows beneath the heading"`
- `"it counts a multi-tag session under each of its tag headings"`
- `"it injects no heading for flat items (byte-identical to today)"`
- `"it carries the correct count for the last group"`

**Edge Cases**:
- Count reflects rows beneath the heading (By-Tag sum exceeds live session count).
- Leading header before the first item.
- Flat items inject no header (no-regression).
- Last group's count (trailing run).

**Context**:
> Group headers are dimmed (styled distinct from session rows), non-selectable (render-layer approach — headings injected at render time as visual separators, not list rows), and counted — each header carries a count of the rows rendered under that heading, e.g. `Portal ··· 2`. In By Tag mode a multi-tag session is counted under each of its tag headings, so the sum of By-Tag header counts exceeds the live session count.
> The flat item slice fed to `bubbles/list` is pre-sorted into grouped order, and headers are injected at each group-key boundary (when the current item's group key differs from the previous item's).
> `lipgloss` styles the grouped look (dimmed headers + counts), layered into the existing `SessionDelegate` styles. No new library. Build constraint: the picker must not be routed through `lipgloss/tree`.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ TUI Rendering & Toggle Behaviour → Group headers, Rendering stack; § Item model)

## session-tagging-and-grouping-2-6 | approved

### Task session-tagging-and-grouping-2-6: Cursor, initial position and g/G land only on session instances; any instance attaches same session

**Problem**: Because headings are render-layer separators and never list items, the cursor must only ever land on session instances — the initial cursor, `g`/`G` (GoToStart/GoToEnd, bound by `bubbles/list`), and ordinary navigation all operate on the all-`SessionItem` list. In By-Tag mode a session can be reachable from multiple cursor positions (one per tag instance); selecting any of them must attach the same underlying session. This needs to be proven against the grouped item slices so the selection contract holds before the toggle is wired in Phase 3.

**Solution**: Verify and, where needed, harden the cursor/selection path so that (a) the initial cursor and `g`/`G` land on a session instance (guaranteed for free because every list item is a `SessionItem`), and (b) `selectedSessionItem` (`model.go:1521`) returns the underlying `tmux.Session` from whichever instance the cursor sits on — so attaching from a duplicate By-Tag instance attaches the same session as any other instance of it. No custom cursor-skip logic is added (the render-layer design makes it unnecessary); this task is primarily a verification-and-test task that locks the contract, plus any minimal adjustment to `selectedSessionItem` to read the underlying session by name.

**Outcome**: Loading a grouped item slice into the session list leaves the initial cursor on the first session instance; `g`/`G` move to the first / last session instance (never a heading, because headings are not items); pressing select on any By-Tag instance of a multi-tag session yields the same `tmux.Session` (same `Name`) as selecting any other instance of that session; `selectedSessionItem` returns the underlying session for the highlighted instance.

**Do**:
- Confirm `selectedSessionItem` (`/Users/leeovery/Code/portal/internal/tui/model.go:1521`) returns the highlighted `SessionItem` and that callers attach by `Session.Name` — so a By-Tag duplicate instance attaches the same session. If any caller keys on the item identity rather than `Session.Name`, adjust to key on the underlying session. (Expected: no change needed; assert with a test.)
- Verify the initial cursor: after `SetItems(groupedSlice)`, `bubbles/list` defaults `Index()` to 0; since item 0 is a session instance (the leading heading is render-only), the initial cursor is on the first session. Add a test loading a grouped slice and asserting `selectedSessionItem` returns the first instance's session.
- Verify `g`/`G`: these are bound by `bubbles/list` to GoToStart/GoToEnd over list items. Since all items are `SessionItem`s, they land on the first / last session instance. Add a test that drives the list (or asserts the binding's effect) so the first/last selected item is a session, never a header.
- Multi-instance attach contract: build a By-Tag slice where one session has two instances (two tags). Move the cursor to each instance in turn and assert `selectedSessionItem().Session.Name` is identical for both — proving "selecting any instance attaches the same underlying session."
- Do NOT add custom cursor-skip logic — the spec's render-layer design (headers never items) makes it unnecessary, and adding it would contradict the build note. If a test reveals the cursor could land on a non-session item, that is a defect in tasks 2-2/2-3/2-4 (a header leaked into the item slice) — fix there, not here.
- These tests use the real `bubbles/list` model with the grouped slice and the extended delegate from task 2-5; no toggle is needed (the slice can be built and loaded directly).

**Acceptance Criteria**:
- [ ] After loading a grouped slice, the initial cursor's `selectedSessionItem` is the first session instance.
- [ ] `g`/`G` land the cursor on the first / last session instance, never a header (headers are not list items).
- [ ] In a By-Tag slice, the two instances of a two-tag session both resolve via `selectedSessionItem` to the same `Session.Name`.
- [ ] `selectedSessionItem` returns the underlying session for the highlighted instance.
- [ ] No custom cursor-skip logic is introduced (headers are render-layer only).

**Tests**:
- `"it places the initial cursor on the first session instance"`
- `"it lands g/G on the first and last session instance, never a header"`
- `"it resolves two By-Tag instances of one session to the same underlying session"`
- `"it returns the underlying session from selectedSessionItem for the highlighted instance"`

**Edge Cases**:
- Initial cursor on first session instance (leading header is render-only).
- `g`/`G` land on session, not header.
- Duplicate By-Tag instances resolve to one session (same `Session.Name`).
- `selectedSessionItem` returns the underlying session.

**Context**:
> Group headers are non-selectable — the cursor jumps session-to-session and never lands on a header. This is achieved by the render-layer approach: the list's items remain session items only; headings are injected at render time as visual separators, not as list rows. Because headers are never list items, the cursor cannot land on one and no custom skip logic is required. Initial cursor: the first session row. GoToStart / GoToEnd (`g`/`G`): land on the first / last session.
> Selection/cursor contract: every instance of a session is independently selectable, and selecting any instance attaches the same underlying session (the duplicate instances are views of one session, not distinct targets). It is acceptable and expected that the same session is reachable from more than one cursor position in By Tag mode.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ TUI Rendering & Toggle Behaviour → Group headers; § Item model → Selection/cursor contract; § Filter Composition → Build note)
