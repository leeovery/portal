TASK: enter-attaches-from-preview-2-3 — Add flashTickMsg with generation guard for tick-based auto-clear

ACCEPTANCE CRITERIA:
- `flashTickMsg` struct with `Gen uint64` field
- `flashTickCmd(gen)` returns non-nil `tea.Cmd` emitting `flashTickMsg{Gen: gen}`
- `flashAutoClearDuration` constant exists, documented, in spec range
- `Model.Update` handles `flashTickMsg`: clears flash iff `msg.Gen == m.flashGen`
- No caller schedules ticks in this task

STATUS: Complete

SPEC CONTEXT: Spec § Inline flash > Clear conditions mandates auto-clear via `tea.Tick` after a short duration ("~3s recommended"). Spec § Replacement on rapid successive bails requires pending ticks from superseded flashes never early-clear a newer flash — chosen mechanism is generation-guarded self-discriminating ticks.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/sessions_flash.go:35 — `flashAutoClearDuration = 3 * time.Second`
  - internal/tui/sessions_flash.go:42-45 — `flashTickMsg{Gen uint64}`
  - internal/tui/sessions_flash.go:52-56 — `flashTickCmd` factory captures gen by value via closure
  - internal/tui/model.go:1023-1037 — `Update` handler with generation guard
- Notes: Handler placement is at the top-level message switch. Documentation references spec sections inline.

TESTS:
- Status: Adequate
- Coverage: internal/tui/sessions_flash_tick_test.go covers all six required test scenarios plus a bonus fresh-model lifecycle test. The TestFlashTickCmd_InvokeProducesFlashTickMsgWithCapturedGen test waits the full 3s real-time tick — slow but verifies actual `tea.Tick` end-to-end behaviour.
- Notes: Tests are focused, not bloated.

CODE QUALITY:
- Project conventions: Followed.
- SOLID principles: Good — single-responsibility helpers, message type is a simple value carrier, factory is pure.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good — comments dense with spec references and rationale.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
