---
topic: tui-redesign
status: in-progress
work_type: feature
date: 2026-02-27
---

# Discussion: TUI Redesign

## Context

The Portal TUI is functional but visually basic. All three views (session list, project picker, file browser) use raw `strings.Builder` concatenation with minimal lipgloss styling. The specification calls for a full-screen, bordered, centered layout built with Bubble Tea and lipgloss.

### Current State

- Three views: session list (`internal/tui/model.go`), project picker (`internal/ui/projectpicker.go`), file browser (`internal/ui/browser.go`)
- Manual `strings.Builder` concatenation with basic cursor prefix (`> ` / `  `)
- 5 lipgloss styles defined (cursor, name, detail, attached, divider colors)
- No borders, centering, padding, or terminal size adaptation
- Hardcoded divider strings instead of dynamic borders
- Kill/rename/filter prompts rendered as inline text

### Spec Expectation

Full-screen framed layout per the specification mockups — bordered, centered, padded, responsive to terminal size.

### References

- [Portal Specification — TUI Design section](.workflows/specification/portal/specification.md#tui-design)
- [tui-redesign.md](tui-redesign.md) — initial notes

## Questions

- [x] Should all three views share a common frame/layout component, or be styled independently?
- [ ] How should prompts (kill confirm, rename, filter) be rendered — modal overlays or styled inline?
- [ ] What border style and color scheme should be used?
- [ ] Should terminal size tracking be handled at the top-level Model or within each sub-view?
- [ ] How should the "SESSIONS" / "PROJECTS" title be rendered?

---

*Each question above gets its own section below. Check off as concluded.*

---

## Should all three views share a common frame/layout component?

### Context

All three spec mockups (sessions, projects/command-pending, empty states) show the same bordered frame pattern. The question is whether to build one shared layout component or have each view manage its own frame.

### Options Considered

**Option A — Shared frame component**
- A single function like `Frame(content, title string, width, height int) string` that wraps any view's inner content with border, padding, and centering
- Top-level Model owns terminal dimensions via `tea.WindowSizeMsg`, passes them down
- Each view just returns its inner content as a plain string
- Pros: No duplication of border/centering/sizing logic; consistent frame across all views; single place to change border style
- Cons: Slight coupling — views must know they'll be framed (content sizing)

**Option B — Each view styles independently**
- Each view handles its own `WindowSizeMsg`, border, padding, centering
- Pros: Views are fully self-contained
- Cons: Triple duplication of identical frame logic; risk of visual inconsistency

### Decision

**Shared frame component.** All spec mockups use the same visual frame, so duplicating it is pure waste. Lipgloss handles this cleanly — `lipgloss.NewStyle().Border(...).Width(w).Padding(...)` plus `lipgloss.Place()` for centering. The top-level Model tracks terminal size and the frame function wraps each view's content.

---
