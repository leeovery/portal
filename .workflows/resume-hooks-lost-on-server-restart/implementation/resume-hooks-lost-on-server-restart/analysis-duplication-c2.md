AGENT: duplication
FINDINGS:
- FINDING: Structural key format string repeated three times in tmux.go
  SEVERITY: low
  FILES: internal/tmux/tmux.go:161, internal/tmux/tmux.go:237, internal/tmux/tmux.go:249
  DESCRIPTION: The tmux format string "#{session_name}:#{window_index}.#{pane_index}" is duplicated across ResolveStructuralKey, ListPanes, and ListAllPanes. This is the canonical structural key format -- a core concept of this implementation. If the format changes, all three sites must be updated in lockstep.
  RECOMMENDATION: Extract a package-level constant (e.g. structuralKeyFormat = "#{session_name}:#{window_index}.#{pane_index}") in internal/tmux/tmux.go and reference it in all three methods.
SUMMARY: Cycle 1 findings (duplicate test helpers, duplicate pane-resolution blocks) are fully resolved. One new low-severity finding: the structural key tmux format string is repeated three times in tmux.go and should be a constant.
