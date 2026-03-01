AGENT: duplication
FINDINGS:
- FINDING: Verbose rune-key matching pattern repeated 17 times
  SEVERITY: medium
  FILES: internal/tui/model.go:637, internal/tui/model.go:639, internal/tui/model.go:645, internal/tui/model.go:651, internal/tui/model.go:653, internal/tui/model.go:658, internal/tui/model.go:663, internal/tui/model.go:716, internal/tui/model.go:722, internal/tui/model.go:931, internal/tui/model.go:933, internal/tui/model.go:935, internal/tui/model.go:937, internal/tui/model.go:939, internal/tui/model.go:942, internal/tui/model.go:996, internal/tui/model.go:1001
  DESCRIPTION: The expression `msg.Type == tea.KeyRunes && string(msg.Runes) == "x"` (and its `keyMsg` variant) appears 17 times across updateProjectsPage, updateSessionList, updateKillConfirmModal, and updateDeleteProjectModal. Each instance is a single-character rune check with the same 3-part comparison. This well exceeds the Rule of Three threshold.
  RECOMMENDATION: Extract a helper like `func isRuneKey(msg tea.KeyMsg, ch string) bool { return msg.Type == tea.KeyRunes && string(msg.Runes) == ch }` in model.go. All 17 call sites become `isRuneKey(msg, "q")` etc., reducing line noise and making the switch cases more scannable.

- FINDING: Kill-confirm and delete-confirm modals are structural clones
  SEVERITY: low
  FILES: internal/tui/model.go:709-731, internal/tui/model.go:989-1009
  DESCRIPTION: updateKillConfirmModal and updateDeleteProjectModal follow an identical y/n/Esc confirmation pattern (~20 lines each). Both check for non-KeyMsg, dispatch y to an action command, handle n/Esc by clearing modal state, and ignore other keys. The only differences are which pending fields get cleared and which action command is returned.
  RECOMMENDATION: Low priority given only 2 instances (below Rule of Three). If a third confirmation modal is added in the future, extract a generic confirmation handler that accepts a confirm callback and a cancel callback. Acceptable as-is for now.

SUMMARY: One medium finding: a verbose rune-key matching expression is repeated 17 times across model.go and should be extracted into a small helper. One low finding: two y/n confirmation modals share identical structure but only have 2 instances, below the Rule of Three threshold.
