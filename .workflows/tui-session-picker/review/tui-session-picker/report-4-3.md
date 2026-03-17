TASK: Wire edit-project dependencies in production (tick-a6dd98)

ACCEPTANCE CRITERIA:
- `buildTUIModel` in `cmd/open.go` passes both `WithProjectEditor` and `WithAliasEditor` options
- Pressing `e` on the Projects page in a running `portal open` session opens the edit project modal
- All existing tests pass

STATUS: Complete

SPEC CONTEXT: The specification requires that `e` triggers a modal overlay with the project's name field, alias list, and full edit controls. The `ProjectEditor` and `AliasEditor` interfaces exist in `internal/tui/model.go` (lines 57-67), with functional options `WithProjectEditor` and `WithAliasEditor` (lines 331-342). The `handleEditProjectKey` method (line 738) guards on both being non-nil and silently returns if either is missing. This task ensures those dependencies are wired in production so the `e` key actually works.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `/Users/leeovery/Code/portal/cmd/open.go:271-283` - `tuiConfig` struct includes `projectEditor` and `aliasEditor` fields
  - `/Users/leeovery/Code/portal/cmd/open.go:296-301` - `buildTUIModel` conditionally passes `WithProjectEditor` and `WithAliasEditor` options
  - `/Users/leeovery/Code/portal/cmd/open.go:332-340` - `openTUI` loads `aliasStore` via `loadAliasStore()` and assigns both `store` (as `projectEditor`) and `aliasStore` (as `aliasEditor`) into the config
  - `/Users/leeovery/Code/portal/cmd/open.go:352-353` - Concrete wiring: `projectEditor: store` and `aliasEditor: aliasStore`
- Notes: `project.Store` satisfies `tui.ProjectEditor` via its `Rename(path, newName string) error` method (store.go:176). `alias.Store` satisfies `tui.AliasEditor` via its `Load`, `Set`, `Delete`, and `Save` methods (store.go:37,75,102,108). Both are production-ready implementations using existing infrastructure. No new adapter types were needed.

TESTS:
- Status: Adequate
- Coverage:
  - `/Users/leeovery/Code/portal/cmd/open_test.go:893-924` - `TestBuildTUIModel/"project and alias editors wired enables edit modal"` verifies that when `projectEditor` and `aliasEditor` are set in `tuiConfig`, pressing `e` on the projects page opens the edit modal (checks for "Edit:" in view output)
  - `/Users/leeovery/Code/portal/internal/tui/model_test.go:4795-5216` - `TestEditProject` comprehensively tests the edit modal flow with test doubles (20+ sub-tests covering: modal open, tab focus, rename, alias add/remove, alias collision, Esc dismiss, empty name rejection, no-editor guard, empty list guard)
  - `/Users/leeovery/Code/portal/cmd/open_test.go:701-729` - `stubProjectEditor` and `stubAliasEditor` test doubles are defined for cmd-level testing
- Notes: The task correctly identifies that existing TUI tests already cover the edit modal flow with test doubles. The new cmd-level test verifies the integration wiring. Manual smoke test (running `portal open`, navigating to Projects, pressing `e`) is noted as an acceptance criterion but is inherently not automatable. Test coverage is well-balanced.

CODE QUALITY:
- Project conventions: Followed. Uses functional options pattern consistent with all other TUI dependencies. Go interfaces are small and focused. Error handling follows `fmt.Errorf("%w", err)` wrapping convention.
- SOLID principles: Good. Dependency inversion is exemplary - the TUI depends on interfaces (`ProjectEditor`, `AliasEditor`), and production implementations are injected via the command layer. Single responsibility maintained - `openTUI` handles construction, `buildTUIModel` handles assembly, `handleEditProjectKey` handles behavior.
- Complexity: Low. The wiring is straightforward assignment of existing store instances to config fields, with nil-guarded conditional injection in `buildTUIModel`.
- Modern idioms: Yes. Functional options pattern, interface satisfaction via structural typing (no explicit `var _ Interface = (*Type)(nil)` needed but it works implicitly).
- Readability: Good. The `tuiConfig` struct clearly documents all injectable dependencies. The conditional nil checks in `buildTUIModel` (lines 296-301) make optional dependency injection explicit.
- Issues: None

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- The nil checks in `buildTUIModel` for `projectEditor` and `aliasEditor` (lines 296-301) are defensive but arguably unnecessary since `openTUI` always sets both. However, they protect against future callers of `buildTUIModel` that might not set these fields, so the guard is reasonable.
