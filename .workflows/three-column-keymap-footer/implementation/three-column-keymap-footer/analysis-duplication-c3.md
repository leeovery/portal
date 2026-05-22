# Duplication Analysis — Cycle 3

STATUS: clean
FINDINGS_COUNT: 0

## Summary

No significant duplication. Shared helpers (`listNavAndFilterBindings`, `chunkBindingsIntoThreeColumns`, `renderKeymapFooter`, `applyListSize`) already absorb the cross-page pattern; per-page wrappers (`applySessionListSize`/`applyProjectListSize`, `sessionFooterBindings`/`projectFooterBindings`) are intentional pairing-safety boundaries from cycle 2, not duplication.

Considered and rejected (below proportionality threshold for a quick-fix):
- Extracting `lipgloss.JoinVertical(Left, listView, renderKeymapFooter(...))` from `viewSessionList`/`viewProjectList` — 2 lines each; net churn negative.
- Extracting the `if m.termWidth > 0 || m.termHeight > 0` reapply guard from `applySessions` and `ProjectsLoadedMsg` handler — 3 lines each, lists differ.
