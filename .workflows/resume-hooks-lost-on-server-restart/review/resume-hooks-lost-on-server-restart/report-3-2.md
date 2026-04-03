TASK: Add Key Resolver and Migrate hooks set

ACCEPTANCE CRITERIA:
- StructuralKeyResolver interface exists in cmd/hooks.go
- HooksDeps has KeyResolver field
- hooks set resolves structural key before store.Set and MarkerName
- Resolution failure returns user-facing error
- All TestHooksSetCommand tests pass with structural key values
- New test for ResolveStructuralKey failure

STATUS: Complete

SPEC CONTEXT: The specification requires changing hook registration from raw $TMUX_PANE values (e.g. "%3") to structural keys ("session_name:window_index.pane_index"). Hook registration in cmd/hooks.go must "query tmux for the current pane's session name, window index, and pane index. Build the structural key and use it as the hook storage key." The tmux.Client.ResolveStructuralKey method (added in Phase 2 Task 2) provides the resolution capability.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/hooks.go:22-27 -- StructuralKeyResolver interface definition
  - cmd/hooks.go:37 -- KeyResolver field on HooksDeps
  - cmd/hooks.go:56-78 -- resolveCurrentPaneKey() shared helper (DRY extraction used by both set and rm)
  - cmd/hooks.go:115 -- hooksSetCmd calls resolveCurrentPaneKey()
  - cmd/hooks.go:130 -- store.Set uses structuralKey
  - cmd/hooks.go:140 -- MarkerName uses structuralKey
  - cmd/hooks.go:170 -- hooksRmCmd also uses resolveCurrentPaneKey() (bonus: rm was also migrated)
- Notes: The implementation goes beyond what the task strictly required by also extracting a shared resolveCurrentPaneKey() helper and migrating hooks rm in the same pass. This is a sensible DRY choice -- the resolution logic is identical between set and rm. The production fallback at line 69 creates a tmux.Client which satisfies StructuralKeyResolver because Phase 2 added the method to Client.

TESTS:
- Status: Adequate
- Coverage:
  - "sets hook and volatile marker for current pane" (line 173) -- verifies structural key used for both store and marker
  - "reads pane ID from TMUX_PANE environment variable" (line 210) -- verifies raw pane ID is NOT used as key, structural key IS used
  - "returns error when TMUX_PANE is not set" (line 247) -- verifies error message and no side effects
  - "returns error when on-resume flag is not provided" (line 279) -- flag validation
  - "overwrites existing hook for same pane idempotently" (line 303) -- idempotency with structural keys
  - "writes correct JSON structure to hooks file" (line 341) -- verifies JSON structure uses structural key
  - "sets volatile marker with correct option name" (line 380) -- dedicated marker format test with different structural key
  - "ResolveStructuralKey failure returns user-facing error" (line 411) -- NEW test per acceptance criteria, verifies error contains "resolve" and no side effects
  - mockKeyResolver (line 147-154) -- clean mock implementation
- Notes: Tests are well-structured and focused. Each test verifies a distinct behavior. The ResolveStructuralKey failure test properly checks both the error message and the absence of side effects (no hooks file created, no SetServerOption calls). No over-testing observed -- tests that look similar actually test different aspects (e.g., marker format vs store key vs JSON structure).

CODE QUALITY:
- Project conventions: Followed. Uses the established hooksDeps package-level DI pattern with t.Cleanup. No t.Parallel (per CLAUDE.md). Mock types follow existing naming conventions.
- SOLID principles: Good. StructuralKeyResolver is a single-method interface (interface segregation). resolveCurrentPaneKey() has single responsibility. Dependency inversion via interface injection.
- Complexity: Low. resolveCurrentPaneKey() is a straightforward linear flow: validate env var, resolve key, return. No branching beyond the nil-check for DI.
- Modern idioms: Yes. Error wrapping with %w, interface-based DI, table-driven subtests.
- Readability: Good. Clear naming (resolveCurrentPaneKey, StructuralKeyResolver), well-documented interface and helper function. The two-step pattern (requireTmuxPane then resolve) is easy to follow.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The resolveCurrentPaneKey() function was extracted as a shared helper used by both hooks set and hooks rm. While the plan task only asked for hooks set migration (rm is Task 3-3), the shared extraction is a good architectural choice that avoids duplication. The rm command was effectively migrated as part of this task rather than in its dedicated task.
