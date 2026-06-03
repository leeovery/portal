TASK: Add ShowGlobalHooksForEvent per-event read seam (state-notify-cascade-on-binary-upgrade-1-1)

STATUS: Complete

ACCEPTANCE CRITERIA: argv exactly ["show-hooks","-g",<event>]; verbatim output + nil err on success; ("",err) wrapping `failed to show global hooks: %w` on failure; ("",nil) on zero entries; build + tests pass.

SPEC CONTEXT: §§ Solution Strategy, Concrete mechanism — new per-event read seam preserving the removed no-arg contract so ParseShowHooks needs zero changes.

IMPLEMENTATION:
- Status: Implemented (faithful to spec)
- Location: internal/tmux/tmux.go:766-779 — uses c.cmd.Run("show-hooks","-g",event) (trimming variant); error path returns "", fmt.Errorf("failed to show global hooks: %w", err); success returns output verbatim; empty output → ("",nil). Doc comment documents the tmux 3.6b blind spot.
- Note: the no-arg ShowGlobalHooks is already fully deleted (its deletion is task 1-5); end state matches AC6. The "do not delete" line was a sequencing precaution.

TESTS:
- Status: Adequate (not over/under-tested)
- internal/tmux/hooks_test.go:11-75 TestShowGlobalHooksForEvent — 3 subtests: verbatim output + exact argv (ACs 1,2 + no-normalization); empty → ("",nil) (AC4); error → ("",err) with errors.Is + prefix contains (AC3). MockCommander records Calls faithfully. Real-tmux blind-spot coverage correctly deferred to 1-6/1-7.

CODE QUALITY:
- Project conventions: Followed. SOLID: Good. Complexity: Low. Modern idioms: Yes. Readability: Good. Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
