---
phase: 4
phase_name: Tag management in the projects edit modal
total: 7
---

## session-tagging-and-grouping-4-1 | approved

### Task session-tagging-and-grouping-4-1: Add Tags field modal state + load editProject.Tags on modal open

**Problem**: The projects edit modal currently holds buffer state for only the Name and Aliases fields (`editAliases`, `editRemoved`, `editNewAlias`, `editAliasCursor`). There is no state to hold the in-modal Tags buffer, and the modal-open handler does not seed any tag state from the project record, so a Tags field has nowhere to live and nothing to display.

**Solution**: Add a parallel set of modal-state fields for tags on the `Model` struct, mirroring the alias-state fields, and extend `handleEditProjectKey` to seed the tag buffer from `editProject.Tags` when the modal opens (alongside the existing alias load and the `editFocus`/`editError`/cursor resets).

**Outcome**: When the user presses `e` on a project, the modal opens with a populated in-memory tag buffer that reflects the project's stored `Tags`, reset cleanly on every open (no leakage from a prior edit), ready for the rendering and key-handling tasks that follow.

**Do**:
- In `internal/tui/model.go`, add tag-buffer fields to the `Model` struct in the "Edit project modal state" block (~226-234), mirroring the alias fields: `editTags []string` (working copy of the project's tags), `editRemovedTags []string` (tags marked for removal during this edit — parallel to `editRemoved`), `editNewTag string` (the in-progress Add-input text — parallel to `editNewAlias`), and `editTagCursor int` (the highlighted row within the Tags block — parallel to `editAliasCursor`).
- In `handleEditProjectKey` (~1344-1377), after the existing resets, seed the tag state: set `m.editTags` from a copy of `pi.Project.Tags` (copy the slice so mutating the buffer never aliases the stored project slice), and reset `m.editRemovedTags = nil`, `m.editNewTag = ""`, `m.editTagCursor = 0`.
- A `Project` with nil/empty `Tags` must seed `m.editTags` to nil/empty (the zero-tag state) without panicking — a `nil` source slice is acceptable; do not allocate a non-nil empty slice unless it falls out naturally.

**Acceptance Criteria**:
- [ ] The `Model` struct carries `editTags`, `editRemovedTags`, `editNewTag`, `editTagCursor` fields parallel to the alias-state fields.
- [ ] Opening the modal on a project with existing tags seeds `m.editTags` with exactly those tags (a copy, not an alias of `pi.Project.Tags`).
- [ ] Opening the modal on a project with nil/empty tags seeds `m.editTags` to empty and does not panic.
- [ ] Re-opening the modal (open → Esc → open again, or open a different project) resets `m.editTags`, `m.editRemovedTags`, `m.editNewTag`, `m.editTagCursor` to their seed/zero values so no buffer leaks from the prior edit.
- [ ] Mutating `m.editTags` after open does not mutate the underlying `pi.Project.Tags` slice (copy isolation).

**Tests**:
- `"it loads existing project tags into the tag buffer on modal open"`
- `"it seeds an empty tag buffer for a project with nil/empty tags"`
- `"it resets the tag buffer when re-opening the modal on a different project"`
- `"it copies the tags slice so buffer mutation does not alias the stored project tags"`

**Edge Cases**:
- Project with nil `Tags` (back-compat record predating the field) — decodes to nil, seeds empty buffer, no panic.
- Modal re-open after a prior edit that added/removed tags — the prior `editTags`/`editRemovedTags`/`editNewTag`/`editTagCursor` must not leak; all reset from the freshly-selected project.
- Slice-aliasing: `m.editTags = pi.Project.Tags` (without a copy) would let later buffer mutations corrupt the in-memory project list; copy explicitly.

**Context**:
> Phase 1 added `Project.Tags []string`; a `projects.json` lacking the field decodes to nil/empty (the zero-tag state, no migration). The modal mirrors the existing alias field exactly. Confirmed anchors: edit-modal state fields at model.go ~226-234; the open handler `handleEditProjectKey` at ~1344-1377 already loads aliases into edit state and resets `editFocus`/`editNewAlias`/`editError`/`editRemoved`/`editAliasCursor` — tag seeding slots in alongside.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Assigning & Managing Tags → Surface; § Tag Data Model & Persistence → Storage, Back-compatibility)

## session-tagging-and-grouping-4-2 | approved

### Task session-tagging-and-grouping-4-2: Three-way Tab field cycle (Name → Aliases → Tags → wrap to Name)

**Problem**: The modal's Tab handler (`updateEditProjectModal`, ~1391-1397) is a binary toggle between `editFieldName` and `editFieldAliases`. With a third field (Tags) it can never reach or focus the Tags field, leaving it unusable.

**Solution**: Add an `editFieldTags` value to the `editField` enum and extend the Tab handler from a binary toggle to a three-way forward cycle: Name → Aliases → Tags → wrap to Name. Initialise the tag cursor when focus enters the Tags field so the highlight starts in a defined position.

**Outcome**: Pressing Tab cycles focus through all three fields in order and wraps from Tags back to Name; on entering the Tags field the cursor is initialised so subsequent Up/Down/`x`/type handlers (task 4-4) operate against a defined position.

**Do**:
- In `internal/tui/model.go`, add `editFieldTags` to the `editField` enum (~93-98), after `editFieldAliases`, so the iota order is `editFieldName`, `editFieldAliases`, `editFieldTags`.
- Rewrite the `tea.KeyTab` case in `updateEditProjectModal` (~1391-1397) as a forward three-way cycle: `editFieldName → editFieldAliases → editFieldTags → editFieldName`. Use an explicit switch or `(m.editFocus + 1) % 3` cast — match the surrounding code style.
- When focus transitions **into** `editFieldTags`, initialise `m.editTagCursor` to a defined value. Set it to `0` on entry to match the alias cursor's reset-on-open convention, OR confirm the cursor is already maintained from task 4-1's seed and only needs clamping; whichever is chosen must leave the cursor within `[0, len(m.editTags)]` (the Add-input row is index `len(m.editTags)`).
- Do not alter the existing add-vs-confirm behaviour for Name/Aliases; this task only changes which field Tab lands on.

**Acceptance Criteria**:
- [ ] `editField` enum has three values in order: Name, Aliases, Tags.
- [ ] From Name, one Tab focuses Aliases; from Aliases, one Tab focuses Tags; from Tags, one Tab wraps to Name.
- [ ] Three Tab presses from Name return focus to Name (full cycle).
- [ ] On entering the Tags field, `m.editTagCursor` is within `[0, len(m.editTags)]` (never out of bounds, even with zero tags).
- [ ] The Tab cycle does not modify any field's buffer text (Name/Aliases/Tags content unchanged by navigation).

**Tests**:
- `"it cycles focus Name to Aliases on Tab"`
- `"it cycles focus Aliases to Tags on Tab"`
- `"it wraps focus Tags to Name on Tab"`
- `"it returns to Name after three Tab presses"`
- `"it initialises the tag cursor within bounds when focus enters Tags"`

**Edge Cases**:
- Wrap boundary: Tab on `editFieldTags` must return to `editFieldName`, not advance past the enum.
- Zero tags: entering the Tags field with an empty `m.editTags` must leave the cursor on the Add-input row (index 0 == `len(m.editTags)`), in bounds.
- The render task (4-6) reads `m.editFocus == editFieldTags` to draw the focus indicator; the cycle must set exactly that value.

**Context**:
> Spec: "The modal's current binary Tab toggle (model.go:1391-1397, Name ↔ Aliases) becomes a three-way cycle. Focus order: Name → Aliases → Tags → (wrap to Name). Tab advances; the Tags field is placed visually after Aliases (last in the modal). Tab still cycles — no new navigation key is introduced." Confirmed anchors: `editField` enum at ~93-98; Tab handler at ~1391-1397.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Assigning & Managing Tags → Surface → Field navigation)

## session-tagging-and-grouping-4-3 | approved

### Task session-tagging-and-grouping-4-3: Add-tag-on-Enter (field-scoped add, blank/dup no-op)

**Problem**: Pressing Enter in the modal currently routes unconditionally to `handleEditProjectConfirm` (~1399-1400). For the Tags field to mirror the alias field, Enter must be **field-scoped**: while the Tags field is focused with non-empty input, Enter adds the tag to the working buffer; a blank/whitespace-only Enter is a no-op (not a confirm); and the existing confirm behaviour for Name/Aliases must be preserved.

**Solution**: In the `tea.KeyEnter` case of `updateEditProjectModal`, special-case the Tags field: when `editFieldTags` is focused, an Enter with non-empty trimmed input appends the tag to `m.editTags` and clears `m.editNewTag`; an Enter with blank/whitespace-only input is a no-op (return without confirming); a duplicate-after-normalisation tag is a no-op. When any other field is focused (or no special case applies), fall through to the existing confirm path unchanged.

**Outcome**: Typing a tag into the Tags field and pressing Enter adds it to the visible buffer (no modal close); pressing Enter on blank input does nothing; pressing Enter while Name or Aliases is focused still confirms/saves as today.

**Do**:
- In `internal/tui/model.go`, in the `tea.KeyEnter` case of `updateEditProjectModal` (~1399-1400), branch on `m.editFocus == editFieldTags` **before** delegating to `handleEditProjectConfirm`.
- For the Tags branch: compute the canonical form. The spec mandates trim + lower-case + non-empty + per-project dedup, and Phase 1 owns normalisation/dedup in the `Store`. **For the in-modal buffer add**, normalise locally for the no-op checks using Phase 1's `project.NormaliseTag` (the helper Phase 1 delivers) so the blank/dup checks match what the store will ultimately persist. If `NormaliseTag` returns empty (blank or whitespace-only input), it is a **no-op**: do not append, leave `m.editNewTag` unchanged or clear it (match the alias field's behaviour — the alias add clears on confirm; here clearing `m.editNewTag` after a successful add is correct, leaving it on a no-op blank is acceptable). Confirm with the alias-field convention.
- If the normalised tag already exists in `m.editTags` (compare in canonical form), it is a **no-op** (do not append a duplicate); clear `m.editNewTag`.
- Otherwise append the canonical tag to `m.editTags`, clear `m.editNewTag`, clear `m.editError`, and return `m, nil` (no modal close).
- The Tags-focused Enter with **empty** input must NOT call `handleEditProjectConfirm` — it is a no-op, mirroring how the alias Add input behaves (Enter on an empty alias Add does not error; it confirms-or-noops per existing behaviour — verify the exact existing alias semantics and match them so the disambiguation is consistent).
- When `m.editFocus` is `editFieldName` or `editFieldAliases`, Enter continues to call `handleEditProjectConfirm` exactly as today.

**Acceptance Criteria**:
- [ ] With Tags focused and `m.editNewTag = "  Work "`, Enter adds `work` to `m.editTags` and clears `m.editNewTag`.
- [ ] With Tags focused and a blank or whitespace-only `m.editNewTag`, Enter adds nothing and does not close the modal.
- [ ] With Tags focused and `m.editNewTag` equal (after normalisation) to a tag already in `m.editTags`, Enter is a no-op (no duplicate appended).
- [ ] With Name focused, Enter still triggers confirm (`handleEditProjectConfirm`), saving the project name as today.
- [ ] With Aliases focused, Enter behaves exactly as it did before this task (no regression to the alias add/confirm path).
- [ ] A successful tag add clears `m.editError`.

**Tests**:
- `"it adds a normalised tag to the buffer on Enter when Tags focused"`
- `"it stores '  Work ' as 'work' on add"`
- `"it is a no-op on Enter with blank/whitespace-only tag input"`
- `"it is a no-op when adding a duplicate-after-normalisation tag"`
- `"it does not close the modal on a Tags-field add"`
- `"it still confirms the modal on Enter when Name is focused"`
- `"it still confirms the modal on Enter when Aliases is focused"`

**Edge Cases**:
- `"  Work "` → trimmed + lower-cased → `work` (normalisation parity with the store, via `project.NormaliseTag`).
- Blank / whitespace-only input → no tag added, no confirm, no error.
- Duplicate after normalisation (`work` already present, user types `WORK`) → no-op, no second entry.
- Tags-focused empty-input Enter must be a no-op, NOT a confirm — this is the central add-vs-confirm disambiguation; verify against the existing alias-field semantics so behaviour is consistent across the two add fields.
- Name/Aliases-focused Enter must remain confirm — adding the Tags field must not alter the existing fields' add-vs-confirm behaviour.

**Context**:
> Spec: "Enter is field-scoped (add), not confirm. While the Tags (or Aliases) field is focused with non-empty input, Enter adds the entry — identical to the existing alias-field behaviour. Modal confirm/save continues to use its existing mechanism, unchanged. Adding the Tags field does not alter the add-vs-confirm disambiguation for the existing fields." And: "Empty / whitespace-only: rejected. Pressing Enter on a blank (or whitespace-only) input is a no-op — no tag is added." And: "Duplicate within a project: adding a tag a project already carries (after normalisation) is a no-op." Phase 1 owns `project.NormaliseTag` (trim + lower-case + reject empty) and the deduped per-project set — the modal reuses `NormaliseTag` for the local no-op checks; final dedup/normalisation on persist is owned by the Phase 1 `Store.AddTag` called in task 4-5. Confirmed anchor: Enter case at model.go ~1399-1400.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Assigning & Managing Tags → Surface → Field navigation; § Tag value normalisation & validation)

## session-tagging-and-grouping-4-4 | approved

### Task session-tagging-and-grouping-4-4: Remove-highlighted-tag via `x` + Tags field text/Backspace/Up/Down keys

**Problem**: The Tags field needs the same key surface as the alias field: highlight a tag entry and press `x` to remove it, type into the Add input, Backspace the Add input, and move the highlight Up/Down bounded to the entries-plus-Add-row. None of these handle the Tags field today — the existing `tea.KeyRunes`/`tea.KeyBackspace`/`tea.KeyUp`/`tea.KeyDown` cases (~1402-1450) only handle Name and Aliases.

**Solution**: Mirror the alias key handling for the Tags field in `updateEditProjectModal`: extend the Runes handler so `x` on an existing tag entry removes it (recording it in `editRemovedTags`) and so typing on the Add row appends to `editNewTag`; extend Backspace to trim `editNewTag` when on the Tags Add row; extend Up/Down to move `editTagCursor` bounded to `[0, len(m.editTags)]`.

**Outcome**: When the Tags field is focused, the user can highlight an entry and remove it with `x`, type a new tag into the Add input, Backspace mistakes in the Add input, and move the highlight up/down through entries and the Add row — identical in feel to the alias field.

**Do**:
- In `internal/tui/model.go` `updateEditProjectModal`:
  - **Runes (`x` remove):** in the `tea.KeyRunes` case (~1427-1449), add a branch parallel to the alias branch (~1430-1438): when `m.editFocus == editFieldTags && text == "x" && m.editTagCursor < len(m.editTags)`, capture `removed := m.editTags[m.editTagCursor]`, append it to `m.editRemovedTags`, splice it out of `m.editTags`, and clamp `m.editTagCursor` to `len(m.editTags)` if it now exceeds it. Return `m, nil`.
  - **Runes (type into Add):** add a branch parallel to ~1440-1444: when `m.editFocus == editFieldTags && m.editTagCursor == len(m.editTags)`, append `text` to `m.editNewTag`, clear `m.editError`, return `m, nil`.
  - **Backspace:** in the `tea.KeyBackspace` case (~1402-1413), add a Tags branch parallel to the alias Add branch: when `m.editFocus == editFieldTags && m.editTagCursor == len(m.editTags)` and `len(m.editNewTag) > 0`, trim the last byte off `m.editNewTag`.
  - **Down:** in `tea.KeyDown` (~1415-1419), add: when `m.editFocus == editFieldTags && m.editTagCursor < len(m.editTags)`, increment `m.editTagCursor`.
  - **Up:** in `tea.KeyUp` (~1421-1425), add: when `m.editFocus == editFieldTags && m.editTagCursor > 0`, decrement `m.editTagCursor`.
- Preserve the existing Name/Aliases branches verbatim — the Tags branches are additive `else if`/`switch`-arm peers, gated on `m.editFocus == editFieldTags`.

**Acceptance Criteria**:
- [ ] With Tags focused and the cursor on an existing entry, `x` removes that entry from `m.editTags` and records it in `m.editRemovedTags`.
- [ ] `x` typed while the cursor is on the Add input row (index `len(m.editTags)`) does NOT remove a tag — it types `x` into `m.editNewTag`.
- [ ] After removing the last remaining entry, `m.editTagCursor` is clamped so it never exceeds `len(m.editTags)` (no out-of-range highlight).
- [ ] Typing characters with the cursor on the Add row appends them to `m.editNewTag`.
- [ ] Backspace on the Add row trims `m.editNewTag` only; Backspace does not affect existing tag entries.
- [ ] Down increments `m.editTagCursor` up to (and including) the Add-row index `len(m.editTags)`; Up decrements down to `0`; neither moves out of `[0, len(m.editTags)]`.
- [ ] Up/Down/`x`/Backspace/type while Name or Aliases is focused are unaffected (no regression).

**Tests**:
- `"it removes the highlighted tag entry on x"`
- `"it records the removed tag in editRemovedTags"`
- `"it types x into the Add input when cursor is on the Add row (x is not a removal there)"`
- `"it clamps the tag cursor after removing the last entry"`
- `"it appends typed characters to the new-tag Add input"`
- `"it backspaces only the new-tag Add input, not existing entries"`
- `"it bounds the tag cursor to entries plus the Add row on Up and Down"`

**Edge Cases**:
- `x` on the Add input row is a literal character, not a removal (cursor == `len(m.editTags)`).
- Cursor clamp after removing the last entry: removing the only/last tag must not leave the cursor pointing past the (now-shorter) slice.
- Backspace must only mutate `m.editNewTag`, never an existing entry's text (entries are immutable in v1; only add/remove).
- Up/Down must stay within `[0, len(m.editTags)]`; the Add row is the final reachable index.
- Removed-then-reads: a removal records into `editRemovedTags` for task 4-5 to persist via `Store.RemoveTag` on confirm.

**Context**:
> Spec: the Tags field behaves "exactly like the existing alias field (model.go:1427-1438): Type a tag + Enter to add it. Highlight an entry + x to remove it." Confirmed anchors: Runes handler ~1427-1449 (alias `x`-remove ~1430-1438, type-into-Add ~1440-1444), Backspace ~1402-1413, Down ~1415-1419, Up ~1421-1425. The alias removal records into `editRemoved` and splices `editAliases`, clamping `editAliasCursor` — mirror that exactly with the tag fields.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Assigning & Managing Tags → Surface)

## session-tagging-and-grouping-4-5 | approved

### Task session-tagging-and-grouping-4-5: Persist tag additions/removals on confirm via ProjectEditor AddTag/RemoveTag seam

**Problem**: Tag additions/removals made in the modal buffer (`m.editTags` additions, `m.editRemovedTags`) are in-memory only. On confirm, `handleEditProjectConfirm` persists the name and alias mutations but knows nothing about tags, so edits are lost on modal close. The `ProjectEditor` seam exposes only `Rename` and has no way to persist tags.

**Solution**: Extend the `ProjectEditor` interface with `AddTag(path, rawTag string) error` and `RemoveTag(path, rawTag string) error` (thin wrappers over Phase 1's `Store.AddTag`/`Store.RemoveTag`), wire the production `*project.Store` and the `mockProjectEditor` test double to satisfy it, and extend `handleEditProjectConfirm` to persist tag removals then additions on confirm — mirroring the alias persistence path, where a persist failure sets `editError` and aborts the modal close.

**Outcome**: On modal confirm, every added tag is persisted via `Store.AddTag` and every removed tag via `Store.RemoveTag` (normalisation/dedup owned by Phase 1); a persist failure sets `editError` and keeps the modal open; an unchanged tag set persists nothing.

**Do**:
- In `internal/tui/model.go`, extend the `ProjectEditor` interface (~75-78) with `AddTag(path, rawTag string) error` and `RemoveTag(path, rawTag string) error`. Keep the existing `Rename`.
- Production wiring: `*project.Store` is the production `ProjectEditor` (wired in `cmd/open.go` ~467 as `projectEditor: store`). Confirm `Store.AddTag(path, rawTag string) error` and `Store.RemoveTag(path, rawTag string) error` (delivered by Phase 1) match the new interface signatures — if the Phase 1 signatures differ (e.g. carry a `via` argument), align the interface to the Store's actual signature rather than inventing one. **Flag any signature mismatch as an ambiguity** rather than silently adapting.
- Test double: extend `mockProjectEditor` (model_test.go ~4728-4741) with `AddTag`/`RemoveTag` methods that record calls (e.g. `addedTags []tagCall`, `removedTags []tagCall` capturing path + rawTag) and honour an injectable error for the failure-path test (parallel to the existing `err` field — consider a separate `tagErr` so name-rename and tag-persist failures can be exercised independently).
- Also extend `cmd/open_test.go`'s `stubProjectEditor` (~711-713) with no-op `AddTag`/`RemoveTag` returning `nil` so the cmd-level build/tests still compile.
- In `handleEditProjectConfirm` (~1455-1510), after the alias mutations and before `m.modal = modalNone`, persist tags: iterate `m.editRemovedTags` calling `m.projectEditor.RemoveTag(m.editProject.Path, tag)`; then persist additions. **For additions**, compute the set of tags in `m.editTags` that were not already on `m.editProject.Tags` (the originally-loaded set) and call `m.projectEditor.AddTag(m.editProject.Path, tag)` for each — OR call `AddTag` for every tag in `m.editTags` and rely on Phase 1's per-project dedup making re-adds a no-op. Choose the diff-on-confirm approach (only persist genuinely-new tags) to minimise store writes, but note that the dedup-tolerant approach is also correct because Phase 1 owns dedup; document the choice inline.
- On any `AddTag`/`RemoveTag` error, set `m.editError` to a clear message (e.g. `"Failed to save tags"`) and `return m, nil` WITHOUT closing the modal — exactly mirroring the alias failure path (~1483-1487, ~1500-1503).
- When the tag set is unchanged (no additions, no removals), no `AddTag`/`RemoveTag` calls are made (the iterate-over-empty-slices naturally yields zero calls).

**Acceptance Criteria**:
- [ ] `ProjectEditor` interface declares `AddTag(path, rawTag string) error` and `RemoveTag(path, rawTag string) error`.
- [ ] `*project.Store` satisfies the extended `ProjectEditor` (production build compiles); `mockProjectEditor` and `stubProjectEditor` satisfy it (test builds compile).
- [ ] On confirm with one added tag, `AddTag(path, tag)` is called once with the project path and the added tag.
- [ ] On confirm with one removed tag, `RemoveTag(path, tag)` is called once with the project path and the removed tag.
- [ ] On confirm with both an add and a removal, both calls are made in one confirm.
- [ ] When the tag set is unchanged, neither `AddTag` nor `RemoveTag` is called.
- [ ] An `AddTag`/`RemoveTag` error sets `m.editError` and leaves the modal open (`m.modal` stays `modalEditProject`).
- [ ] Normalisation/dedup is NOT re-implemented in the modal — the raw tag is passed to the store, which owns canonicalisation (Phase 1).

**Tests**:
- `"it persists an added tag via AddTag on confirm"`
- `"it persists a removed tag via RemoveTag on confirm"`
- `"it persists both an addition and a removal in one confirm"`
- `"it makes no tag persistence calls when the tag set is unchanged"`
- `"it sets editError and keeps the modal open when AddTag fails"`
- `"it sets editError and keeps the modal open when RemoveTag fails"`
- `"it passes the raw tag to the store without re-normalising in the modal"`

**Edge Cases**:
- Persist failure (`AddTag`/`RemoveTag` returns error) → `editError` set, modal stays open, mirroring the alias persist-failure abort.
- No-op confirm (tags unchanged) → zero store calls.
- Additions and removals in the same confirm → both persisted; order is removals-then-additions to avoid a transient duplicate edge if the same canonical tag is removed then re-added (rare; document the chosen order).
- Normalisation/dedup is owned by Phase 1's `Store.AddTag`/`Store.RemoveTag` — the modal passes raw values; do NOT re-trim/lower-case before the store call beyond what task 4-3 already normalised into the buffer for display.
- **Ambiguity to flag if present:** Phase 1's `Store.AddTag`/`RemoveTag` exact signature. The planning summary states `Store.AddTag(path, rawTag) error` / `Store.RemoveTag(path, rawTag) error` with no `via` argument (unlike `Rename(path, newName, via)`). If the implemented Phase 1 signatures carry a `via`/origin argument, align the `ProjectEditor` interface to the real signature and pass `"cli"` (the user-facing-mutation convention used by the alias/rename paths). Surface the mismatch rather than guessing silently.

**Context**:
> Spec: "On the projects-edit → sessions-page transition, dispatch a sessions-list refresh... Tags are read live from projects.json at grouped-render time." Phase 1 delivers `Store.AddTag(path, rawTag) error` / `Store.RemoveTag(path, rawTag) error` which own normalisation (trim + lower-case + reject empty) and per-project dedup. Confirmed anchors: `ProjectEditor` seam at model.go ~75-78; `handleEditProjectConfirm` at ~1455-1510 (alias removals ~1481-1487, alias add ~1489-1504, persist-failure pattern sets `editError` and returns without closing); `mockProjectEditor` at model_test.go ~4728-4741; `stubProjectEditor` at cmd/open_test.go ~711-713; production wiring `projectEditor: store` at cmd/open.go ~467. The alias path uses `via="cli"` for user-facing TUI mutations.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Assigning & Managing Tags; § Tag Data Model & Persistence → Tag value normalisation & validation, Storage)

## session-tagging-and-grouping-4-6 | approved

### Task session-tagging-and-grouping-4-6: Render Tags block after Aliases with "no tags" empty state

**Problem**: `renderEditProjectContent` (~1813-1857) renders only the Name and Aliases blocks. The Tags field has buffer state and key handling but is invisible — the user cannot see existing tags, the highlight, the Add input, or an empty-state message.

**Solution**: Append a Tags block to `renderEditProjectContent`, placed visually after the Aliases block (last), mirroring the alias block's structure: a focus-indicated heading, a list of `[x]`-marked entries with a highlight marker on the focused entry, an always-rendered Add-input row, and a clear "no tags" empty state when there are no tags (replacing the blank).

**Outcome**: The modal renders a Tags block after Aliases showing each tag with a removal marker, the highlight on the focused entry, the Add input, and an explicit "no tags" message when the tag list is empty — visually consistent with the Aliases block.

**Do**:
- In `internal/tui/model.go` `renderEditProjectContent` (~1813-1857), after the Aliases block (after the alias Add row at ~1848) and before the error/footer lines, append a Tags block mirroring the alias block (~1826-1848):
  - A blank-line separator, then a focus-indicated heading: `tagsIndicator := "  "`, `if m.editFocus == editFieldTags { tagsIndicator = "> " }`, write `tagsIndicator + "Tags:\n"`.
  - If `len(m.editTags) == 0`, write a clear empty-state line — e.g. `"    (no tags)\n"` (the alias block uses `(none)`; use `(no tags)` or `(none)` consistent with the alias wording — the spec requires "a clear 'no tags' empty state" rather than a blank, so any explicit non-blank marker satisfies it; prefer wording that reads clearly).
  - Else, iterate `m.editTags`: each entry gets a marker (`"    "` default, `"  > "` when `m.editFocus == editFieldTags && m.editTagCursor == i`) and renders `%s[x] %s\n`.
  - Always render the Add row: `addMarker := "    "`, `if m.editFocus == editFieldTags && m.editTagCursor == len(m.editTags) { addMarker = "  > " }`, write `%sAdd: %s\n` with `m.editNewTag`.
- The focus indicator (`> ` on the heading) must appear on the Tags heading ONLY when `editFieldTags` is focused, and NOT on Name/Aliases at the same time (each block independently reads `m.editFocus`).
- Do not alter the existing Name/Aliases rendering or the error/footer lines (~1850-1854).

**Acceptance Criteria**:
- [ ] The modal view contains a `Tags:` heading rendered after the `Aliases:` block.
- [ ] With existing tags, each tag renders on its own row with a `[x]` removal marker.
- [ ] The focused tag entry shows the highlight marker (`  > `) when the Tags field is focused and the cursor is on that entry.
- [ ] An empty `m.editTags` renders a clear "no tags"/"(none)" empty-state line, not a blank.
- [ ] The Add-input row is always rendered (showing `m.editNewTag`), regardless of how many tags exist.
- [ ] The Tags heading focus indicator (`> `) appears only when `editFieldTags` is focused; when Name or Aliases is focused, the Tags heading shows the unfocused indicator.

**Tests**:
- `"it renders the Tags block after the Aliases block"`
- `"it renders each tag with an [x] removal marker"`
- `"it shows the highlight marker on the focused tag entry"`
- `"it shows a 'no tags' empty state when there are no tags"`
- `"it always renders the tag Add-input row"`
- `"it shows the focus indicator on the Tags heading only when Tags is focused"`

**Edge Cases**:
- Empty tags → explicit empty-state line ("no tags"), never a blank gap (spec: "shows a clear 'no tags' empty state, not a blank").
- Focus indicator must be field-scoped — focusing Tags must not also mark Name/Aliases as focused, and vice versa.
- Highlighted-entry marker appears only on the entry at `m.editTagCursor` and only while Tags is focused; the Add row gets the marker when the cursor is on the Add-input index.
- Add row is always rendered even with zero tags (the cursor can rest there to type a first tag).

**Context**:
> Spec: "Empty tags field in the projects edit modal — shows a clear empty state ('no tags') rather than a blank." And the Tags field "is placed visually after Aliases (last in the modal)." Confirmed anchor: `renderEditProjectContent` at model.go ~1813-1857; the Aliases block (~1826-1848) uses `aliasIndicator`, a `(none)` empty state, per-entry `[x]` markers with a `  > ` cursor marker, and an always-rendered `Add:` row — mirror this exactly for Tags.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Mode Persistence & Empty States → Empty states; § Assigning & Managing Tags → Surface)

## session-tagging-and-grouping-4-7 | approved

### Task session-tagging-and-grouping-4-7: Dispatch re-group sessions refresh on projects-edit → sessions-page transition

**Problem**: After editing tags and returning to the sessions page (projects-page `s`/`x` → sessions, ~1250-1261), the grouped view does not re-read `projects.json`, so a just-added/removed tag is not reflected — the By-Tag/By-Project grouping is stale until some other refresh fires. The transition currently sets `m.activePage = PageSessions` and returns `nil` (no refresh).

**Solution**: On the projects-page → sessions-page transition, dispatch a sessions-list refresh that re-fetches live sessions and re-applies them through the mode-aware re-render path (so tags are re-resolved and the list re-groups), mirroring the existing preview-dismiss refresh contract (`refreshSessionsAfterPreviewCmd` / `exitPreviewToSessions`, ~851-887). Because tags are read live at grouped-render time, re-feeding the sessions through `applySessions` (Phase 3's mode-aware `rebuildSessionList` path) re-resolves the now-changed project tags.

**Outcome**: After adding or removing a tag in the projects edit modal and returning to the sessions page, the next render reflects the change (the session moves under/out of the relevant tag group), respecting the currently-active grouping mode.

**Do**:
- In `internal/tui/model.go` `updateProjectList` (or the projects-page key switch, ~1250-1261), modify the `s` and `x` cases that transition to `PageSessions`: after `m.activePage = PageSessions`, dispatch a sessions-refresh command instead of returning `nil`.
- Reuse the existing live-refresh machinery: build a tea.Cmd that calls `m.sessionLister.ListSessions()` and emits a refresh message the existing handler consumes. The cleanest mirror is `refreshSessionsAfterPreviewCmd("")` → `previewSessionsRefreshedMsg` (~858-871, handled at ~1154-1166), which routes through `applySessions` — the mode-aware re-render entry that Phase 3 makes group-aware. Passing an empty `preserveName` is acceptable here (there is no preview-anchored cursor to preserve on a page switch); if cursor preservation across the page switch matters, capture the currently-selected session name, but the spec does not require it for v1. Prefer the simplest correct mirror.
- Preserve the existing `commandPending` guard (~1251-1253, ~1257-1259): when `m.commandPending`, the handler currently returns `m, nil` early without switching pages — keep that guard intact; the refresh dispatch applies only on the actual transition.
- Return `m, <refreshCmd>` from the transition. The refresh cmd may be nil when no `SessionLister` is wired (test harnesses) — callers must tolerate a nil cmd (the existing `refreshSessionsAfterPreviewCmd` already returns nil in that case; `tea.Batch`/returning a nil cmd is safe).
- Apply the same refresh to both the `s` and `x` transition cases (both set `PageSessions`).

**Acceptance Criteria**:
- [ ] Pressing `s` on the projects page transitions to the sessions page AND dispatches a sessions-list refresh command.
- [ ] Pressing `x` on the projects page transitions to the sessions page AND dispatches the same refresh.
- [ ] The dispatched refresh re-fetches sessions via `SessionLister` and re-applies them through the mode-aware re-render path so the active grouping mode is respected (a By-Tag view re-groups with the updated tags).
- [ ] When `m.sessionLister` is nil (no lister wired), the transition still switches pages and the refresh cmd is nil — no panic.
- [ ] The `commandPending` guard is preserved: in command-pending mode the existing early-return behaviour is unchanged (no page switch, no refresh).
- [ ] No refresh is dispatched when the handler is not transitioning to the sessions page (e.g. other keys on the projects page are unaffected).

**Tests**:
- `"it dispatches a sessions refresh on the projects s -> sessions transition"`
- `"it dispatches a sessions refresh on the projects x -> sessions transition"`
- `"it re-groups with updated tags after returning from a tag edit"`
- `"it tolerates a nil SessionLister on the transition (no panic, nil cmd)"`
- `"it preserves the commandPending guard (no page switch, no refresh)"`
- `"it does not refresh when not transitioning to the sessions page"`

**Edge Cases**:
- Refresh must respect the active grouping mode — route through `applySessions`/the Phase 3 mode-aware re-render, not a flat-only rebuild, so a By-Tag/By-Project view reflects the edit on return.
- Nil `SessionLister` (test harness) → refresh cmd is nil, page still switches, no panic.
- `commandPending` guard preserved → no page switch / no refresh in command-pending mode (existing early-return at ~1251-1253, ~1257-1259 stays).
- This mirrors the existing preview-dismiss contract; it does NOT add a background watch on `projects.json` — re-grouping on page re-entry is sufficient for v1 (spec: "No live cross-page reactivity beyond this is required").

**Context**:
> Spec: "On the projects-edit → sessions-page transition, dispatch a sessions-list refresh that re-resolves project records and re-groups — mirroring the existing refresh dispatched on the preview-dismiss → sessions transition. After adding/removing a tag and returning to the sessions list, the change is reflected on the next render. No live cross-page reactivity beyond this is required — re-grouping on page re-entry (not a background watch on projects.json) is sufficient for v1." Confirmed anchors: projects → sessions transition at model.go ~1250-1261 (currently `m.activePage = PageSessions; return m, nil`, with `commandPending` early-returns); the preview-dismiss refresh contract at `refreshSessionsAfterPreviewCmd` ~858-871 (returns nil when no SessionLister) and `exitPreviewToSessions` ~883-887, consumed by the `previewSessionsRefreshedMsg` handler ~1154-1166 which routes through `applySessions` ~796-808. Phase 3 makes `applySessions`/`filteredSessions`/`ToListItems` mode-aware via `rebuildSessionList`, so re-feeding sessions re-groups per the active mode and re-reads live project tags.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Assigning & Managing Tags → Refresh contract — edits are visible on return to the sessions page)
