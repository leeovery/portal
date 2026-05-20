AGENT: duplication
STATUS: issues_found
FINDINGS_COUNT: 1

FINDINGS:
- FINDING: Adjacent KeyEsc / KeySpace arms have byte-identical bodies
  SEVERITY: low
  FILES: internal/tui/pagepreview.go:467-470
  DESCRIPTION: The newly-added `case tea.KeySpace:` arm in `previewModel.Update`'s `tea.KeyMsg` switch returns exactly the same expression as the sibling `case tea.KeyEsc:` immediately above it — `return m, func() tea.Msg { return previewDismissedMsg{} }`. Two consecutive case labels in the same switch with literally identical one-line bodies. Spec states Space must mirror Esc "exactly"; any future change to dismiss dispatch (attaching a refresh cmd, swapping message type) would have two sites to move in lockstep.
  RECOMMENDATION: Collapse into a single comma-separated case label:
  ```go
  case tea.KeyEsc, tea.KeySpace:
      return m, func() tea.Msg { return previewDismissedMsg{} }
  ```
  No behaviour change; the "mirror Esc exactly" contract is enforced structurally rather than by parallel branches. Note: the original plan explicitly instructed "duplicate the one-liner" — this recommendation contradicts that guidance and should be weighed against the plan author's intent.

SUMMARY: One small near-duplicate flagged: the new `KeySpace` arm duplicates the adjacent `KeyEsc` arm byte-for-byte and can be merged via a comma-separated case label. No other cross-file or near-duplicate logic detected; the test file appropriately follows the existing hermetic pattern used by peer preview tests.
