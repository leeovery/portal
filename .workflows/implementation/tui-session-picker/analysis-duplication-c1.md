AGENT: duplication
FINDINGS:
- FINDING: Window label pluralization logic duplicated in SessionItem
  SEVERITY: medium
  FILES: internal/tui/session_item.go:39-42, internal/tui/session_item.go:80-83
  DESCRIPTION: The window count pluralization logic (`fmt.Sprintf("%d windows", ...)` with a special case for `Windows == 1`) is implemented identically in both `SessionItem.Description()` (lines 39-42) and `SessionDelegate.Render()` (lines 80-83). The Description method already computes the correct label, but Render re-derives it independently instead of calling Description.
  RECOMMENDATION: Have `SessionDelegate.Render()` call `si.Description()` or extract a `windowLabel(count int) string` helper used by both. The Render method can then apply lipgloss styling to the result of Description (or the helper), eliminating the duplicated branching logic.

- FINDING: viewSessionList and viewProjectList share identical dimension-fallback and modal-overlay structure
  SEVERITY: medium
  FILES: internal/tui/model.go:1123-1143, internal/tui/model.go:1193-1214
  DESCRIPTION: Both `viewProjectList()` and `viewSessionList()` follow the exact same pattern: get the list view string, extract width/height with a fallback to 80x24 when zero, then switch on modal state to overlay content via `renderModal()`. The dimension-fallback block (6 lines) and the structural pattern are duplicated across the two methods.
  RECOMMENDATION: Extract a helper like `viewListWithModal(listModel list.Model, modalContent func() (string, bool)) string` that handles the dimension fallback and modal overlay. Each page view method would only need to supply its list model and a function mapping the current modal state to content.

- FINDING: updateModal and updateProjectModal are structural clones
  SEVERITY: medium
  FILES: internal/tui/model.go:704-718, internal/tui/model.go:980-994
  DESCRIPTION: `updateModal()` (sessions page) and `updateProjectModal()` (projects page) have identical structure: check for Ctrl+C quit, then switch on `m.modal` to dispatch to specific modal handlers. The only difference is which modal constants are dispatched. Both also share the same Ctrl+C guard pattern.
  RECOMMENDATION: Unify into a single modal dispatcher that handles all modal types (kill, rename, delete, edit). Since the model already has a single `modal` field, a single `updateActiveModal(msg)` method can switch on all modal states regardless of which page triggered them, eliminating the page-specific modal router duplication.

- FINDING: selectedSessionItem and selectedProjectItem are near-identical accessor patterns
  SEVERITY: low
  FILES: internal/tui/model.go:603-610, internal/tui/model.go:910-917
  DESCRIPTION: Both methods follow the same 7-line pattern: get SelectedItem from a list, nil-check, type-assert, return. The only differences are the list field and the item type. This is a minor structural duplication.
  RECOMMENDATION: Low priority. The methods are short and type-specific, so extracting a generic helper would add complexity without significant benefit. Acceptable as-is given Go's lack of method generics on struct receivers.

- FINDING: ToListItems and ProjectsToListItems are the same conversion pattern
  SEVERITY: low
  FILES: internal/tui/session_item.go:95-101, internal/tui/project_item.go:77-83
  DESCRIPTION: Both functions allocate a `[]list.Item` of the same length as the input slice and loop to wrap each element. The pattern is identical; only the input/output types differ. Each is 7 lines.
  RECOMMENDATION: Low priority. A generic `func ToItems[T any](slice []T, wrap func(T) list.Item) []list.Item` could unify these, but the current code is clear and idiomatic Go. Not worth extracting unless more item types are added.

SUMMARY: Three medium-severity duplications found in production code: window-label pluralization computed twice in SessionItem, view rendering methods sharing identical dimension-fallback and modal-overlay scaffolding, and two structurally cloned modal dispatchers. Two low-severity patterns (selected-item accessors and slice-to-list-item converters) are acceptable as-is.
