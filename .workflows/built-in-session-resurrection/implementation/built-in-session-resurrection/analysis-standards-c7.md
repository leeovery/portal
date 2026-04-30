---
agent: standards
cycle: 7
findings_count: 2
status: issues_found
---
# Standards Analysis (Cycle 7)

## Summary

Phase 13 conforms to spec; the Fatal Bootstrap Errors clear-marker bullet, Observability copy update, and marker-suppression scope godoc are all consistent across spec/errors.go/errors_test.go/bootstrap_warnings_test.go. Two low-severity observations — neither warrants a code change.

---

## Spec Conformance Gaps

### FINDING S1: T13-3 `TestReattachIntegration_OpenPathResolvesSavedOnlySession` does not exercise `portal open <path>` resolving the saved session **by name**

- **SEVERITY:** low
- **FILES:** `cmd/reattach_integration_test.go:750-848`
- **DESCRIPTION:** phase-5-tasks.md L942 says "portal open PATH resolves a session name present only in sessions.json." The test deliberately decouples the alias query (`mysaved` → `projectDir`) from the saved name (`open-ghost`) and only asserts (a) the path-arg branch reaches `openPath` via `PathResult` and (b) `has-session -t open-ghost` succeeds post-bootstrap. That is sufficient for the underlying Phase 5 acceptance criterion ("skeleton is created before the command's own attach logic runs"), and the planning's edge-case note (phase-5-tasks.md L952) explicitly endorses this shape. The literal bullet text could be read as requiring `portal open` to *connect to* the saved name, which would require contriving the `{project}-{nanoid}` naming dance — explicitly out of scope. The godoc at lines 743-749 documents this trade-off honestly.
- **RECOMMENDATION:** No code change. Optionally reconcile the planning bullet text with what the test proves; the godoc already explains the gap.

## Convention Violations

### FINDING S2: `DriveSignalHydrateBinary` exec's only the inner subcommand, not the full `command -v portal && ...` hook-content wrapper

- **SEVERITY:** low
- **FILES:** `internal/restoretest/restoretest.go:218-250`
- **DESCRIPTION:** Production hook content (internal/tmux/hooks_register.go:39) is `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"`. The helper exec's the absolute binary directly and notes "the wrap is unnecessary" (lines 222-226). Strictly correct (the wrap is a no-op when binary is on PATH), but this means binary-driven coverage argv is byte-identical to the *inner half* only, not the full hook-content shape. A future regression in the wrapper's quoting under tmux's run-shell parser would not surface here. The godoc is honest about the divergence.
- **RECOMMENDATION:** Optional — exec via `sh -c "command -v portal >/dev/null 2>&1 && <binary> state signal-hydrate <session>"` to cover the full hook-content shape. Defer if Phase 13 scope is tight.

## Documentation Drift

None.

## Verification (clean)

- **Spec § Fatal Bootstrap Errors** (specification.md:1395) now explicitly enumerates "@portal-restoring clear (unset) fails at step 6" as fatal. Wording is clear and symmetric with the set-option bullet.
- **Spec § Observability** (specification.md:1379) wording change to "Portal state file unusable — restoration skipped." is consistently applied across spec, errors.go:56, errors_test.go:70, bootstrap_warnings_test.go:225,236. No stale "Portal state file is corrupt" references remain.
- **T13-4 godoc** at phase5_marker_suppression_integration_test.go:50-70 accurately scopes the test to "Restore-side write discipline only" and explicitly disclaims daemon-tick suppression as out-of-scope.
- **T13-4 cycle-6 S5 revert is well-justified**: production session.go:108 keys hooks via `tmux.PaneTarget` (target-format), not `state.SanitizePaneKey` (FS-safe key). Comment at reboot_roundtrip_test.go:208-216 explains why both formats coexist.
- **`openPathFunc` seam** follows the existing `openTUIFunc` pattern with parallel godoc, init() registration, and compile-time assertions.
- **Build tags consistent**: `//go:build integration` present on all integration files.
- **The "switch-client substitute" disclaimer** at reboot_roundtrip_test.go:744-752 is explicit about what is NOT covered (real PTY-attached `tmux switch-client`) and why.
