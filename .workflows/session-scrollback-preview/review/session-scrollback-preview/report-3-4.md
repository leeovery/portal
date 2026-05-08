TASK: session-scrollback-preview-3-4 — Keymap precedence over embedded viewport

ACCEPTANCE CRITERIA:
- Preview-owned keys (], [, Tab, Esc) never reach m.viewport.Update.
- Viewport default scroll keys (Up, Down, j, k, PgUp, PgDn, ctrl-u, ctrl-d) continue to pass through.
- Home and End (preview-owned via task 2-6) continue to invoke GotoTop/GotoBottom.
- tea.WindowSizeMsg still reaches viewport for re-flow during preview.
- No double-handling: a single Tab keypress produces exactly one Tail call.

STATUS: Complete

SPEC CONTEXT:
Per § Within-preview Key Bindings > Keymap policy: "Preview owns ], [, Tab, Esc. Everything else either passes through to the embedded bubbles/viewport (scroll keys above) or is unbound/no-op." Per § Open Items: confirm ], [, Tab do not collide with any inherited bubbles/viewport or page-level bindings; if they do, preview's binding wins inside the preview page.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/pagepreview.go:252-312
- Notes:
  - Dispatch order correct: tea.WindowSizeMsg handled first; tea.KeyMsg switch matches Esc/Home/End/Tab/]/[ and short-circuits with return. Default falls through to m.viewport.Update(msg).
  - Preview-owned keys (], [, Tab, Esc) all return early before viewport passthrough.
  - WindowSizeMsg still reaches viewport (and updates dimensions).
  - No double-handling: each branch returns explicitly; single Tab → exactly one Tail call.
  - The tea.KeyRunes arm has a nested switch string(msg.Runes) that falls through to viewport passthrough if rune is neither ] nor [ (e.g. j/k), which is correct.

TESTS:
- Status: Adequate
- Location: internal/tui/pagepreview_precedence_test.go
- Coverage: Tab/]/[ owned, Up/PgDn/j/k passthrough, WindowSizeMsg reflow, single-Tab Tail count, and non-key fall-through.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None of consequence.
