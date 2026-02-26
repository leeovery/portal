---
topic: tui-redesign
status: open
date: 2026-02-26
---

# Discussion: TUI Redesign

## Context

The current Portal TUI implementation is functional but visually basic. The specification calls for a full-screen, bordered, centered layout built with Bubble Tea and lipgloss, but the current implementation uses raw string building with minimal styling.

### Current State

- Three views: session list (`internal/tui/model.go`), project picker (`internal/ui/projectpicker.go`), file browser (`internal/ui/browser.go`)
- All use manual `strings.Builder` concatenation with basic cursor prefix
- Only 5 lipgloss styles defined (cursor, name, detail, attached, divider colors)
- No borders, centering, padding, or terminal size adaptation
- Hardcoded divider strings instead of dynamic borders
- Kill/rename/filter prompts rendered as inline text

### Spec Expectation

Full-screen framed layout per the specification mockups:

```
┌─────────────────────────────────────┐
│                                     │
│           SESSIONS                  │
│                                     │
│    >  cx-03          ● attached     │
│       api-work       2 windows      │
│       client-proj                   │
│                                     │
│    ─────────────────────────────    │
│    [n] new in project...            │
│                                     │
└─────────────────────────────────────┘
```

### Gap Summary

1. No borders/framing — needs lipgloss `Border()`
2. No centering — needs `lipgloss.Place()` with terminal dimensions
3. No padding — needs `lipgloss.Padding()`
4. No terminal size tracking — needs `tea.WindowSizeMsg` handling
5. No structured layout — manual string concat vs component-based rendering
6. Prompts/modals not styled as distinct elements

## Questions

- [ ] How polished should the styling be? Minimal bordered frame, or richer theming (gradients, color palette)?
- [ ] Should all three views (session list, project picker, file browser) share a common frame/layout component?
- [ ] Should prompts (kill confirm, rename, filter) be styled as modal overlays or remain inline?
- [ ] Any preference on color scheme or border style (rounded, thick, double, etc.)?
