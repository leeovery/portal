AGENT: duplication
FINDINGS:
- FINDING: Session-list refresh-and-resize block duplicated between SessionsMsg and previewSessionsRefreshedMsg handlers
  SEVERITY: medium
  FILES: internal/tui/model.go:802-809, internal/tui/model.go:896-901
  DESCRIPTION: Both handlers run the same four-step sequence: assign `msg.Sessions` to `m.sessions`, compute `m.filteredSessions()`, call `m.sessionList.SetItems(ToListItems(filtered))`, and re-apply terminal size when non-zero. The new previewSessionsRefreshedMsg arm at lines 896-901 is a near-verbatim copy of the existing block at lines 802-809 (only the inside-tmux title rewrite at 811-813 is omitted). This is the pattern code-quality.md flags under "Compose, don't duplicate" — a second author of the second handler can drift the resize gate, the filter call, or the SetItems shape independently of the original. The `previewSessionsRefreshedMsg` handler is brand-new code in this work unit, so the duplication is fresh and consolidatable.
  RECOMMENDATION: Extract a `(*Model).applySessions(sessions []tmux.Session)` helper that performs assign → filter → SetItems → conditional SetSize, and call it from both arms. The SessionsMsg arm additionally performs the inside-tmux title rewrite, which can stay at the call site (session-specific policy, not list-mutation policy). Locating the helper next to `filteredSessions` keeps the cluster cohesive.

- FINDING: Three near-duplicate cycle-key handlers in previewModel.Update (Tab / ] / [)
  SEVERITY: low
  FILES: internal/tui/pagepreview.go:259-266, internal/tui/pagepreview.go:276-283, internal/tui/pagepreview.go:284-291
  DESCRIPTION: The Tab, `]`, and `[` arms each follow the same four-line shape: guard against degenerate length (`<= 1`), mutate an index field by modular arithmetic, call `m.readFocusedPaneIntoViewport()`, and return `(m, nil)`. The post-mutation tail (`m.readFocusedPaneIntoViewport(); return m, nil`) is the most clearly repeated slice across all three. The arms differ on four axes: (a) which field they mutate (paneIdx vs windowIdx), (b) which length they take modulo, (c) step direction (+1 vs -1), (d) whether paneIdx is reset (yes for `]`/`[`, no for Tab). A single shared "cycle index" helper would need either four parameters or a closure, which would not improve readability over the current three explicit branches.
  RECOMMENDATION: Either accept the three branches as-is (the shape divergence justifies the repetition under code-quality.md > "Avoid premature abstraction") or extract only the post-mutation tail as a tiny helper that locks the "always re-read after mutation" invariant in one place. Do not introduce a generic cycle-index abstraction.

STATUS: findings
