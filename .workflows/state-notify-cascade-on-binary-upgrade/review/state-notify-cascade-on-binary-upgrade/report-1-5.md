TASK: Delete the no-arg ShowGlobalHooks method and migrate its remaining fixtures (state-notify-cascade-on-binary-upgrade-1-5)

STATUS: Complete

ACCEPTANCE CRITERIA: ShowGlobalHooks method removed; `ShowGlobalHooks\b` returns nothing across internal/ + cmd/; ShowGlobalHooksForEvent sole survivor; re-export removed; reboot_roundtrip verifyHydrationHookEntries reads via per-event seam; full suite green.

SPEC CONTEXT: §§ Concrete mechanism + AC6 ("Global read removed").

GREP RESULTS (load-bearing):
- `ShowGlobalHooks\b` across internal/ *.go → no matches; across cmd/ *.go → no matches.
- Whole-tree `ShowGlobalHooks` → only ShowGlobalHooksForEvent. Production call sites: hooks_register.go:325 (convergeEvent), hooks_unregister.go:117 (teardown). Others are test readers/doc comments.
- `ShowGlobalHooksOrWarn|showGlobalHooksOrWarn` → no matches (re-export + helper both gone).

IMPLEMENTATION:
- Status: Implemented
- tmux.go:766-779 — only ShowGlobalHooksForEvent remains (trimming Run, `failed to show global hooks: %w` wrap). export_test.go — no ShowGlobalHooksOrWarn re-export. hooks_test.go:11-75 — TestShowGlobalHooks replaced by TestShowGlobalHooksForEvent; TestAppendGlobalHook/TestUnsetGlobalHookAt preserved. hooks_migration_test.go:41-48 countSignalHydrateEntries delegates to per-event helper. cmd/bootstrap/reboot_roundtrip_test.go:1287 verifyHydrationHookEntries reads via ShowGlobalHooksForEvent.
- One intentional raw no-arg read remains: hooks_register_realtmux_test.go:217 ts.Run(t,"show-hooks","-g") — raw tmux socket (NOT the deleted Client method), the Testing-Requirement-2 blind-spot guard; comment documents this. Correct, does not violate AC6.

TESTS:
- Status: Adequate. Deletion + re-pointing; correctness is full-suite green + grep. TestShowGlobalHooksForEvent gives focused coverage of the surviving seam; re-pointed readers exercise the production seam (higher fidelity). No over/under-testing.

CODE QUALITY:
- Net code removal; tests in external tmux_test package; reader logic deduped into one canonical helper; %w + errors.Is preserved; intentional raw no-arg read well-commented.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
