# Analysis — Architecture (cycle 1)

STATUS: findings
FINDINGS_COUNT: 4

## Findings

### drainRefilterCmd is misnamed for its actual generality
- SEVERITY: low
- FILES: internal/tui/pagepreview_refetch_test.go:130-141, internal/tui/kill_refresh_filter_test.go:127
- DESCRIPTION: Helper is described and used as a generic "invoke cmd; feed message back through Update" round-trip — `TestDrainRefilterCmdInvokesCmdAndFeedsResultThroughUpdate` explicitly probes it with a non-refilter `WindowSizeMsg`. The name suggests refilter-specific semantics but the body is domain-agnostic single-step cmd drain. Two consumers exist; future consumers (rename-refresh, ProjectsLoadedMsg propagation) will face friction with the misleading name.
- RECOMMENDATION: Rename to `drainCmdThroughUpdate` (or similar domain-neutral name) and rewrite the doc comment to lead with the general contract. Test-package-internal only.

### WithInsideTmux panic-on-non-nil is sound but stylistically anomalous
- SEVERITY: low
- FILES: internal/tui/model.go:407-410
- DESCRIPTION: Per spec the panic is the intended mechanism to surface a future relocation violating the invariant. Code is correct. Stylistic note: `panic("unreachable: ...")` in a constructor-style method is unusual for this codebase — every other "should never happen" branch in model.go uses silent fall-through. Given the invariant is genuinely tight, the panic is defensible.
- RECOMMENDATION: No change required. Current shape is consistent with spec intent.

### ProjectsLoadedMsg handler diverges from applySessions's encapsulation pattern
- SEVERITY: low
- FILES: internal/tui/model.go:939-951, internal/tui/model.go:662-671
- DESCRIPTION: `applySessions` encapsulates (SetItems → capture cmd → return cmd) behind a helper used by two call sites. The `ProjectsLoadedMsg` handler open-codes the byte-identical sequence inline for the projects list. Currently only one projects call site, so extraction isn't warranted by Rule-of-Three. The asymmetry means a future projects-list mutator will re-discover the lossy plumbing problem this bugfix exists to eliminate.
- RECOMMENDATION: Defer until a second projects-list mutator appears. When it does, mirror `applySessions` with `applyProjects(projects []project.Project) tea.Cmd`.

### visibleSessionNames is shared infrastructure colocated as if file-local
- SEVERITY: low
- FILES: internal/tui/pagepreview_refetch_test.go:458-469, internal/tui/kill_refresh_filter_test.go:149
- DESCRIPTION: `visibleSessionNames` is now the canonical visibility-assertion helper used by both preview-refetch tests and the new kill-refresh test. It sits at the bottom of `pagepreview_refetch_test.go` alongside file-local helpers. As consumers grow, its location implies file-locality that no longer holds.
- RECOMMENDATION: Relocate to a shared `list_test_helpers_test.go` when a third consumer arrives. Not blocking.

## Summary

Implementation cleanly addresses the spec. No high- or medium-severity architectural issues. Four low-severity organisational/naming items, all deferrable until additional consumers materialise.
