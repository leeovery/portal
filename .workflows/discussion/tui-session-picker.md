---
topic: tui-session-picker
status: in-progress
work_type: feature
date: 2026-02-27
---

# Discussion: TUI Session Picker

## Context

The current TUI (`portal open` / `x`) has three separate views: session list, project picker, and file browser. The session list is the default view, with "[n] new in project..." as a gateway to the project picker sub-screen. This creates UX friction — projects aren't discoverable, section context is missing, keybindings are hidden, and navigation requires learning the sub-screen flow.

The original proposal (from `tui-session-picker-ux.md`) was to merge sessions and projects into a single scrollable screen. Through discussion, this evolved into a two-page architecture using `bubbles/list` — see questions below for the full journey.

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
- [x] Should sessions and projects be one list or two pages?
- [x] How should page switching and keybindings work?
- [ ] How should filter (`/`) work?
- [ ] What happens to command-pending mode (`portal open -e cmd`)?
- [ ] How should empty states be handled (no sessions, no projects, both empty)?
- [ ] What's the right approach for the `[b] browse for directory...` item?
- [ ] How should the `n` key auto-execute behavior work?
- [ ] Should we adopt `bubbles/list` and other bubbles components?

---

*Each question above gets its own section below. Check off as concluded.*

---

## How should the unified single-screen list be modeled?

### Options Considered

**Option A — Single unified model.** One flat list of items (sessions + projects + browse entry), one cursor, one `Update`. Item type determines available actions. `ProjectPickerModel` retired — its edit/remove logic moves into the unified model or becomes modal.

**Option B — Composed models.** Keep session and project sub-models, render together in a single `View()`. A coordinator tracks which section the cursor is in and delegates key events.

### Decision

**Single unified model (Option A).** The whole point is one scrollable list with one cursor — composition adds coordination complexity for something that's conceptually one list. The existing `ProjectPickerModel` is retired. Project-specific actions (edit, remove) will be handled within the unified model, likely as modal states.

**Note**: This decision was later superseded by the two-page architecture decision below. The unified model concept still applies — each page is its own `bubbles/list.Model` rather than hand-rolled.

---

## Should sessions and projects be one list or two pages?

### Context

The original UX doc proposed a single scrollable screen with "Sessions" and "Projects" sections. We needed to decide whether this was achievable with the available components.

### Options Considered

**Option A — Single list with contextual rendering.** Sessions and projects in one flat `bubbles/list`. Custom `ItemDelegate` renders sessions differently from projects (attached badge, window count vs path). No headers — visual distinction does the work.

**Option B — Two stacked `bubbles/list` components.** One for sessions, one for projects, rendered vertically in the same view. Tab switches focus. Each has its own cursor, filtering, pagination.

**Option C — Two separate pages.** Each page is a full `bubbles/list` with title, filtering, help bar. A key toggles between them. Only one visible at a time.

### Journey

Started with Option A (single unified list). Discovered `bubbles/list` has no native section/group/header concept — items are a flat `[]list.Item`. Explored rendering section headers via the delegate (e.g., rendering a "Projects" label above the first project item). This felt like fighting the component.

Explored Option B (two stacked lists). Workable but breaks the "one cursor" mental model — you're managing focus between two list components in the same view.

Considered whether contextual rendering alone (Option A without headers) would be enough — sessions show window count + attached badge, projects show path. The visual distinction exists, but with a short list (2 sessions, 2 projects) it could feel like "four things that happen to look different" rather than two clear categories.

Landed on Option C. Two pages, equal hierarchy. Each page is a proper `bubbles/list` with all built-in features (title, filtering, pagination, help). This actually solves the original UX complaints better than the single-screen mockup — projects aren't buried behind a gateway, each page has focused keybindings, section context is the page title itself.

### Decision

**Two separate pages (Option C).** Sessions page and Projects page as equal peers, not parent-child. Each is a full `bubbles/list.Model`. Default page is sessions if any exist, otherwise projects. Toggling to an empty page still works — consistent navigation.

---

## How should page switching and keybindings work?

### Context

With two pages, we need a clear, discoverable way to switch between them, plus per-page keybindings.

### Options Considered

**Tab toggle** — simple, one key. But ambiguous ("which page am I going to?").

**Named keys** — `p` for projects, `s` for sessions. Self-documenting, you always know the destination.

**`x` toggle** — mirrors the shell command (`x` launches Portal). Nice symmetry but same ambiguity as Tab.

### Journey

Started with Tab as the obvious choice. Then explored using `x` as a toggle — there's nice symmetry with `x` being the shell alias for `portal open`. But toggle keys are ambiguous about destination.

Named keys (`p`/`s`) are self-documenting. Decided to use both: `p`/`s` as the advertised keys (shown in help bar), `x` as an undocumented power-user toggle.

For per-page keybindings, explored whether `r` could mean "rename" on sessions and "remove" on projects. Decided against the overloading — too easy to confuse. Remove became "delete" with `d` instead.

All keybindings standardized to lowercase.

### Decision

**Page switching:**
- `p` — go to projects (shown in sessions help bar)
- `s` — go to sessions (shown in projects help bar)
- `x` — toggle between pages (undocumented power-user shortcut)

**Sessions page keybindings:**
`[enter] attach  [r] rename  [k] kill  [p] projects  [n] new here  [/] filter  [q] quit`

**Projects page keybindings:**
`[enter] new session  [e] edit  [d] delete  [s] sessions  [b] browse  [/] filter  [q] quit`

---
