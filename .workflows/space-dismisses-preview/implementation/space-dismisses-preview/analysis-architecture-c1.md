AGENT: architecture
STATUS: issues_found
FINDINGS_COUNT: 1

FINDINGS:
- FINDING: Esc and Space arms are independent duplicates rather than a shared case label
  SEVERITY: low
  FILES: internal/tui/pagepreview.go:467-470
  DESCRIPTION: The spec's contract is that Space dismisses "exactly like" Esc — same emitted message, same downstream pathway. The implementation expresses this as two adjacent arms with byte-identical bodies (`return m, func() tea.Msg { return previewDismissedMsg{} }`). Go's switch supports comma-separated case expressions, so `case tea.KeyEsc, tea.KeySpace:` with a single body would make the "mirrors exactly" invariant structural rather than maintained by editor discipline. As written, a future edit to one arm (e.g., adding a side-effect cmd, swapping the message type, or attaching telemetry) can silently desync from its sibling. Low impact today because both bodies are one line, but the seam quality is weaker than spec intent.
  RECOMMENDATION: Collapse to a single case label: `case tea.KeyEsc, tea.KeySpace:` followed by the existing single-line return. Note: the original plan explicitly instructed "duplicate the one-liner" — this contradicts the plan author's stated preference and should be weighed against intent.

SUMMARY: Implementation is otherwise clean — minimal surface change, no new public API, reuses the existing dismiss pathway and sessions-refresh transition documented in CLAUDE.md, and the hermetic unit test mirrors the package's established pattern. One low-severity structural duplication flagged.
