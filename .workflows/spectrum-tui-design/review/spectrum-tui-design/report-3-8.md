TASK: spectrum-tui-design-3-8 — Two-mode edit-project modal: interaction state machine + immediate-persist (deliberate behaviour change, NOT a parity-preserve)

ACCEPTANCE CRITERIA:
- Navigate mode: Tab/Shift+Tab (+ ↑/↓) move NAME/ALIASES/TAGS; ←/→ move across chips + trailing + add slot; entering a chip field lands on + add; x deletes a focused chip immediately; Esc closes.
- Edit mode: Enter/e on chip, Enter/e on Name, Enter/+ on + add slot (spawns new empty chip in edit mode); landing on + add via Tab/←/→ is navigate-only; type edits; ←/→ move text cursor; Enter commits & persists; Esc discards element edit.
- Immediate per-item persist (Name via Rename, aliases via SetAndSave/DeleteAndSave, tags via AddTag/RemoveTag); NO dirty/save/batch; Esc never discards saved work.
- Falling-out: empty-on-commit = delete; empty Name reverts; duplicate-on-commit = silent dedupe (tags case-sensitive); brand-new empty chip vanishes on Esc.
- Close refreshes cached project records + grouping index for all three fields.
- handleEditProjectConfirm save key + editRemoved batch buffer removed.

STATUS: Complete

SPEC CONTEXT: §8.2 (revised per the top-of-spec CORRIGENDUM, 2026-06-22) defines a uniform two-mode (Navigate/Edit) immediate-persist model replacing the old asymmetric Name/Aliases-batch + tags-live design. The CORRIGENDUM adds ↑/↓ as field-nav aliases in navigate mode. Parity does NOT apply — this is a deliberate behaviour change. Persistence is per-item on exit-edit; the four falling-out rules (empty=delete, empty Name reverts, duplicate=silent dedupe, Esc backs out one level) are normative. Tags are case-sensitive via project.NormaliseTag.

IMPLEMENTATION:
- Status: Implemented (clean, matches spec)
- Location: internal/tui/model.go:2362-2911 (the state machine), :116-135 (editField/editMode enums), :430-457 (model fields). Render layer (task 3-9) is internal/tui/edit_modal.go.
- The state machine lives in model.go (the task description's "model.go" pointer is correct; the briefing's "look at edit_modal.go" is the render dependency, not this task's core).
- updateEditProjectModal dispatches on editMode to updateNavigateModeKey / updateEditModeKey. Navigate: Esc→closeEditModal, Tab/Shift+Tab + Down/Up→focusField(next/prev), Left/Right→moveElement, Enter→enterEditFromNavigate, x→deleteFocusedChip, e→edit (Name or focused chip only), +→edit (add slot only). Edit: Esc→discardEdit, Enter→commitEdit, Left/Right→text-cursor move, Backspace, else literal insert.
- focusField lands a chip field on the trailing + add slot (index == len). focusedOnChip / focusedOnAddSlot disambiguate. moveElement clamps to [0, len].
- commitName: empty→revert (no Rename), unchanged→no-op, else Rename(path, value, "cli"). commitAlias: empty→delete; within-field duplicate→silent dedupe (drops the edited chip if a rename onto a dup); cross-project collision pre-check via Load (rejects if alias maps to a different path) → silent drop; existing rename → DeleteAndSave(old)+SetAndSave(new); new → SetAndSave + append. commitTag: empty→delete; NormaliseTag (case-sensitive); within-field dup→silent dedupe; existing rename→RemoveTag(old)+AddTag(new); new→AddTag+append.
- discardEdit: brand-new chip never entered the slice so it just vanishes; existing chip/Name retains prior value; re-clamps the element index. closeEditModal: refreshes via loadProjects() only when editChanged (a refresh-needed signal, NOT a dirty flag — everything is already on disk).
- Verified the asymmetric batch is gone: grep for handleEditProjectConfirm / editRemoved / editError across internal/tui returns nothing. ProjectEditor interface (model.go:98) carries Rename/AddTag/RemoveTag; AliasEditor carries Load/SetAndSave/DeleteAndSave.
- Traced the tricky paths: rename-existing-chip-onto-duplicate (drops edited chip, deletes its old store name, leaves the original) and rename-onto-own-value (idempotent, no spurious delete) both behave correctly for aliases and tags.

TESTS:
- Status: Adequate (thorough, focused, not over-tested)
- Coverage: internal/tui/edit_modal_state_machine_test.go holds 37 TestSM_* cases driving updateEditProjectModal directly with recording test doubles (smProjectEditor / smAliasEditor record Rename/AddTag/RemoveTag/SetAndSave/DeleteAndSave; smProjectStore satisfies loadProjects). Every acceptance criterion + every edge case is exercised: field nav (Tab/Shift+Tab/↑/↓ incl. wrap), land-on-add-slot, ←/→ element + text-cursor movement (bounded), x-delete (alias + tag + add-slot no-op), enter-edit via Enter/e on Name and chips, e/x-as-literal in edit mode, Tab/↑/↓ ignored in edit mode, +/Enter spawns new chip, land-on-add-slot-is-navigate-only, commit persistence for all three fields, collision pre-check silent revert, existing-chip delete-then-set / remove-then-add, empty-on-commit delete, empty-Name revert (no Rename, no blocking modal), duplicate dedupe, case-sensitive tag distinction, new-empty-chip vanishes on Esc, existing-chip Esc keeps prior value, Esc-close preserves saved work + fires refresh, Esc-close with no changes → nil cmd (no refresh).
- The doubles assert the exact persist call args (path/name/via, ordered slices via reflect.DeepEqual), so a test would fail if the feature broke — not just smoke coverage.
- The four test files named in the briefing (edit_modal_tab_cycle_test, edit_modal_add_tag_test, edit_modal_tag_keys_test, edit_modal_tags_state_test) do NOT exist — the prior tags-only tests were consolidated into the single state-machine file. This is correct for a behaviour-change task (the old asymmetric tests would assert removed behaviour); not a gap.
- Not over-tested: each test targets one transition/rule; no redundant duplication. edit_modal_test.go covers the 3-9 render layer (out of scope here).

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(); small DI interfaces (ProjectEditor/AliasEditor 3 methods each); value-receiver Model with copy-mutate-return; absolute-path discipline N/A. Comments cite §8.2/§13.1 and explain the falling-out rationale.
- SOLID principles: Good. Single-responsibility helpers (commitName/commitAlias/commitTag, deleteAliasAt/deleteTagAt, focusField/moveElement, enterEditFromNavigate/discardEdit/commitEdit). Persistence is behind injected editor interfaces.
- Complexity: Low/Acceptable. commitAlias is the densest function (collision pre-check + dup + existing-vs-new branches) but each branch is clearly delimited and commented; reads cleanly.
- Modern idioms: Yes. slices.Clone/Delete/IndexFunc, []rune-correct cursor arithmetic, clampInt bound helper.
- Readability: Good. Self-documenting names; the editChanged field is explicitly annotated as a refresh-signal, not a dirty flag, pre-empting the obvious misread.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/model.go:2786-2801 (commitAlias) — the cross-project collision pre-check and SetAndSave are both gated inside `if m.aliasEditor != nil`; when aliasEditor is nil a new chip is appended to editAliases (m.editAliases = append at :2811) with nothing persisted, so the in-memory chip diverges from disk. Production always wires aliasEditor (handleEditProjectKey:2367 refuses to open the modal when aliasEditor == nil), so this is unreachable in prod — but the nil-branch silently desyncs. Consider early-returning (drop the chip) when aliasEditor == nil, mirroring commitTag's projectEditor handling, to make the invariant explicit.
- [quickfix] internal/tui/model.go:2762-2815 (commitAlias) vs :2819-2872 (commitTag) — the two commit functions share the same skeleton (empty→delete, within-field dup→dedupe, existing rename→remove-then-add, new→add). The branching is near-identical save for the store seam and NormaliseTag. A small shared helper parameterised over a chip-field accessor would dedupe the duplicate-detection loop and the existing-vs-new branch; defer unless a third chip field appears (premature abstraction risk noted).
