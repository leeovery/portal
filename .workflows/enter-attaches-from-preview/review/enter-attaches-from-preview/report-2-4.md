TASK: enter-attaches-from-preview-2-4 — Clear flash on actionable KeyMsg without swallowing keystroke

ACCEPTANCE CRITERIA:
- Active flash + actionable KeyMsg on Sessions page calls clearFlash once
- KeyMsg continues to normal handler (not consumed)
- WindowSizeMsg/FocusMsg/BlurMsg/MouseMsg do not clear flash
- No-flash path is near-zero cost
- isActionableKey is documented and small

STATUS: Complete

SPEC CONTEXT: Spec § Inline flash > Clear conditions mandates that the next actionable tea.KeyMsg clears the flash; spec § Flash interaction with filter input pins "one key, one intent" — flash must not swallow the keystroke. Modifier-only/resize/focus events do not clear.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/sessions_flash.go:75-77 — `isActionableKey` helper with documented defensive shape (`msg.Type != 0 || len(msg.Runes) > 0`)
  - internal/tui/model.go:1390-1394 — clear-flash guard at top of `case tea.KeyMsg` inside `updateSessionList`; deliberate fall-through preserves keystroke
- Notes: Placement is correct (top of Sessions-page KeyMsg branch, before KeyCtrlC / `?` / SettingFilter routing). Comment explicitly documents the fall-through intent. Non-KeyMsg events never enter this branch.

TESTS:
- Status: Adequate
- Coverage at internal/tui/sessions_flash_clear_test.go:
  - First keystroke clears flash AND opens filter input ("one key, one intent" — load-bearing assertion)
  - WindowSizeMsg / FocusMsg / BlurMsg / MouseMsg do not clear flash (4 separate tests)
  - Keystroke with no flash is normal no-op
  - Successive keystrokes after clear all land in filter input
  - KeyDown advances cursor AND clears flash (list binding case)
  - Esc clears flash AND returns tea.Quit (Esc edge case)
  - Enter clears flash AND runs Enter handler (Enter edge case)
  - Table-driven TestIsActionableKey_Defensive locks the helper contract
- Notes: Tests use the shared `flashModelWithSessions` helper. Coverage is balanced, not over-tested.

CODE QUALITY:
- Project conventions: Followed.
- SOLID principles: Good — `isActionableKey` is a single-purpose pure function; clearFlash side-effect is documented as deliberate.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
