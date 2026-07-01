TASK: Fix the stale ListAllPanes doc-comment on cleanStaleAdapter (session-rename-orphans-resume-hook-analysis-1-2, tick-9b5a22)

ACCEPTANCE CRITERIA:
- cmd/bootstrap_production.go:71 names ListAllPaneHookKeys, not ListAllPanes.
- No code change (comment-only edit); go build ./... passes.
- cmd/bootstrap/stale_marker_cleanup.go:43 is unchanged.

STATUS: Complete

SPEC CONTEXT:
Per the spec (Hook-Key Derivation; Risks -> Missed key-producing site), retiring stale
name-based-keying doc-comments was an explicit deliverable: a comment asserting the wrong
enumeration "would invite a future caller back into name-based keying." The Stage 2 hook
enumeration was switched to the paneKey-based ListAllPaneHookKeys; the cleanStaleAdapter
doc-comment was the one residual prose pointer still naming the retired name-based
ListAllPanes. Correcting it eliminates a misleading breadcrumb toward the old keying model.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/bootstrap_production.go:71
- Notes: The doc-comment now reads "satisfies the interface via ListAllPaneHookKeys." This
  matches the actual interface method AllPaneLister.ListAllPaneHookKeys() declared at
  cmd/clean.go:23, which *tmux.Client satisfies (compile-time assertion at
  bootstrap_production.go:94). git show f2921a33 confirms the change is a single-line diff:
  "ListAllPanes" -> "ListAllPaneHookKeys" on the comment line, with no surrounding code
  touched. `grep -n ListAllPanes cmd/bootstrap_production.go` returns no matches, so no stale
  name-based reference remains in this file.
- Sibling reference: cmd/bootstrap/stale_marker_cleanup.go:43 still reads
  "(*tmux.Client).ListAllPanes" and is correctly UNCHANGED — its last commit (2b2276f7)
  predates this task, and that path is the name-based skeleton-marker cleanup that
  legitimately references ListAllPanes.

TESTS:
- Status: Adequate (N/A — documentation-only correction)
- Coverage: No new test required and none added, per the task's Tests note. A prose comment
  is not behaviour and cannot be meaningfully unit-tested. Existing cmd tests remain the
  regression backstop for the adapter's actual wiring.
- Notes: The correctness of the underlying wiring (interface method name) is already pinned
  by the compile-time assertion `var _ AllPaneLister = (*tmux.Client)(nil)` at line 94 —
  if the interface method drifted, the build would fail.

CODE QUALITY:
- Project conventions: Followed. Comment style, GoDoc format, and em-dash prose match the
  surrounding file.
- SOLID principles: N/A (comment-only)
- Complexity: Low (single-word substitution in prose)
- Modern idioms: N/A
- Readability: Good. The comment now accurately describes the interface contract, and the
  three-sentence block (interface field / production wiring / test-substitution rationale)
  remains internally consistent.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
