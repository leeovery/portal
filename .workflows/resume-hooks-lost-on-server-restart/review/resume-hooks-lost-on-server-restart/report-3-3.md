TASK: Migrate hooks rm to Structural Keys

ACCEPTANCE CRITERIA:
- hooks rm resolves structural key before store.Remove and marker deletion
- Resolution failure returns user-facing error
- All TestHooksRmCommand tests pass with structural key values
- New test for ResolveStructuralKey failure

STATUS: Complete

SPEC CONTEXT: The specification requires hooks rm to resolve $TMUX_PANE to a structural key (session_name:window_index.pane_index) before removal. The store.Remove call and volatile marker deletion must both use the structural key. Resolution failures must produce a user-facing error. This is part of the broader migration from ephemeral pane IDs to structural keys that survive tmux server restarts.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/hooks.go:165-196 (hooksRmCmd RunE)
- Notes: The rm command calls `resolveCurrentPaneKey()` (cmd/hooks.go:59-78) which reads TMUX_PANE, resolves it via StructuralKeyResolver, and returns the structural key. The resolved key is used for both `store.Remove(structuralKey, "on-resume")` at line 180 and `deleter.DeleteServerOption(hooks.MarkerName(structuralKey))` at line 190. The implementation reuses the shared `resolveCurrentPaneKey()` helper introduced in Task 3-2, avoiding code duplication between set and rm. The error path at line 74 wraps resolver failures with a user-facing message: "failed to resolve structural key for current pane: %w".

TESTS:
- Status: Adequate
- Coverage:
  - "removes hook and volatile marker for current pane" -- verifies end-to-end structural key flow (line 456)
  - "reads pane ID from TMUX_PANE and resolves structural key" -- verifies raw pane ID is not used as key (line 495)
  - "returns error when TMUX_PANE is not set" -- validates env var requirement (line 536)
  - "returns error when on-resume flag is not provided" -- flag validation (line 565)
  - "silent no-op when no hook exists for pane" -- graceful no-op with structural key (line 589)
  - "removes correct JSON entry from hooks file" -- multi-key selective removal (line 618)
  - "deletes volatile marker with correct option name" -- verifies marker format @portal-active-{structural_key} (line 656)
  - "cleans up pane key when last event removed" -- verifies empty map cleanup (line 688)
  - "ResolveStructuralKey failure returns user-facing error" -- NEW, verifies error propagation and no side effects (line 721)
- Notes: All 9 tests use structural key values consistently. The new ResolveStructuralKey failure test verifies both the error message and the absence of side effects (hook data preserved, no DeleteServerOption calls). Tests are well-focused with no redundancy -- each tests a distinct behavior or edge case.

CODE QUALITY:
- Project conventions: Followed. Uses package-level hooksDeps with t.Cleanup for DI, no t.Parallel, Cobra command pattern matches existing code.
- SOLID principles: Good. Single responsibility for resolveCurrentPaneKey (shared between set and rm). Interface segregation with separate ServerOptionDeleter and StructuralKeyResolver. Dependency inversion through hooksDeps injection.
- Complexity: Low. The rm command is a straightforward pipeline: resolve key -> load store -> remove -> delete marker.
- Modern idioms: Yes. Uses fmt.Errorf with %w for error wrapping. Clean interface-based DI.
- Readability: Good. The flow is clear and mirrors the set command structure. Variable names are descriptive.
- Issues: None.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- The rm command's structure is nearly identical to set (resolve key, operate on store, operate on tmux option). This symmetry is good for maintainability. Phase 4 Task 4-2 already plans to extract a shared helper if warranted.
