---
topic: tui-session-picker
status: in-progress
work_type: feature
date: 2026-02-27
---

# Discussion: TUI Session Picker

## Context

The current TUI (`portal open` / `x`) has three separate views: session list, project picker, and file browser. The session list is the default view, with "[n] new in project..." as a gateway to the project picker sub-screen. This creates UX friction — projects aren't discoverable, section context is missing, keybindings are hidden, and navigation requires learning the sub-screen flow.

The proposal (from `tui-session-picker-ux.md`) is to replace the multi-view session list + project picker with a single scrollable screen showing both Sessions and Projects as sections, with context-sensitive keybinding hints in a bottom bar. The file browser remains a separate sub-view.

### Current Architecture

- `internal/tui/model.go` — Main model with `viewState` enum (`viewSessionList`, `viewProjectPicker`, `viewFileBrowser`)
- `internal/ui/projectpicker.go` — Separate project picker sub-view with its own model
- `internal/ui/browser.go` — File browser sub-view
- Views communicate via message types (`BackMsg`, `ProjectSelectedMsg`, `BrowseSelectedMsg`, etc.)
- `strings.Builder` rendering with basic lipgloss styles, no borders/frames/sections

### References

- [tui-session-picker-ux.md](../../tui-session-picker-ux.md) — UX design document
- [tui-redesign discussion](.workflows/discussion/tui-redesign.md) — Prior discussion on visual frame/layout

## Questions

- [x] How should the unified single-screen list be modeled — one model or composed sub-models?
- [ ] How should section headers ("Sessions", "Projects") and the cursor interact?
- [ ] How should the context-sensitive bottom bar be rendered and updated?
- [ ] How should filter (`/`) work across both sections?
- [ ] What happens to command-pending mode (`portal open -e cmd`) in the unified view?
- [ ] How should empty states be handled (no sessions, no projects, both empty)?
- [ ] What's the right approach for the `[b] browse for directory...` item?
- [ ] How should the `n` key auto-execute behavior work?

---

*Each question above gets its own section below. Check off as concluded.*

---

## How should the unified single-screen list be modeled?

### Options Considered

**Option A — Single unified model.** One flat list of items (sessions + projects + browse entry), one cursor, one `Update`. Item type determines available actions. `ProjectPickerModel` retired — its edit/remove logic moves into the unified model or becomes modal.

**Option B — Composed models.** Keep session and project sub-models, render together in a single `View()`. A coordinator tracks which section the cursor is in and delegates key events.

### Decision

**Single unified model (Option A).** The whole point is one scrollable list with one cursor — composition adds coordination complexity for something that's conceptually one list. The existing `ProjectPickerModel` is retired. Project-specific actions (edit, remove) will be handled within the unified model, likely as modal states.

---
