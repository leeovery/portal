# Implementation Review: Kill-Rename Prefix Collision

**Plan**: kill-rename-prefix-collision
**QA Verdict**: Approve

## Summary

The bugfix is implemented exactly as specified, with no blocking issues across all three tasks. The root cause — `KillSession` and `RenameSession` building bare `-t <session>` targets that tmux silently prefix-matches onto a colliding session (`foo` → live `foo-2`) — is closed at the Client-method chokepoint by routing both through a new centralising `exactTarget(session) string` helper (`"=" + session`), the session-level sibling of `PaneTargetExact`. The behaviour-neutral migration of the five existing inline `"="+name` session-target sites is complete and byte-identical, leaving no inline session-target drift surface anywhere in `internal/tmux`. The destructive `newName`-must-stay-bare trap in `RenameSession` is correctly handled and guarded by a dedicated test. `go build` and the full `go test ./...` suite are green.

## QA Verification

### Specification Compliance
Full alignment with the specification. The `exactTarget` primitive matches the spec's prescribed signature/body; the two destructive callers gain the `=` prefix on the target only (`RenameSession`'s `newName` stays bare); the five migration sites (`HasSession`, `HasSessionProbe`, `SwitchClient`, `saverPanePID`, `SaverPaneID`) route through the helper with unchanged argv; and every explicitly out-of-scope site (`PaneTarget`, `display-message -t <paneID>`, bare session reads/sets, `SelectWindow`'s window-level prefix, `PaneTargetExact`) is correctly untouched. No deviations.

### Plan Completion
- [x] Phase 1 acceptance criteria met
- [x] All tasks completed (1-1, 1-2, 1-3)
- [x] No scope creep — change held firmly at the session-target surface; out-of-scope sites verified untouched via grep audit

### Code Quality
No issues. `exactTarget` is a single-responsibility formatter and the single sanctioned session-level construction; the fix lives at the argv-construction chokepoint with no caller-side changes; rationale godoc blocks on both destructive methods mirror the already-fixed sites; error wrapping (`%w`) and signatures unchanged; idiomatic Go throughout.

Independent grep audit confirmed the only non-comment `"=" +` constructions remaining in `internal/tmux` are the `exactTarget` helper body (`tmux.go:594`) and the spec-allowed window-level `SelectWindow` (`tmux.go:983`).

### Test Quality
Tests adequately verify requirements, neither under- nor over-tested.
- `exact_target_internal_test.go` — focused `exactTarget("foo") == "=foo"` assertion, correctly placed in an internal `package tmux` test file (the unexported helper is unreachable from external `tmux_test.go`).
- `TestKillSession` / `TestRenameSession` happy-path assertions updated to the `=`-prefixed argv, replacing the assertions that previously pinned the buggy bare form.
- `TestKillSessionUsesExactMatchPrefix` / `TestRenameSessionUsesExactMatchPrefix` — prefix-collision regression tests mirroring `TestHasSessionUsesExactMatchPrefix`; a dropped-`=` regression hits a `t.Errorf`/`t.Fatalf` arm and fails loudly. `RenameSession`'s bare-`newName` guard is asserted two complementary ways (positional argv equality + `HasPrefix` negative check).
- Migrated sites keep their existing argv pins green — that green state is the proof of behaviour-neutrality. No `t.Parallel()` (project constraint honoured).

### Required Changes (if any)
None.

## Recommendations

### Do now
1. `internal/tmux/tmux.go:378-379` — `RenameSession` godoc parenthetical example reads slightly muddled compared to the clearer `KillSession` example; reword to mirror it (e.g. "renaming \"foo\" when only a live \"foo-2\" exists must NOT rename \"foo-2\""). Pure comment edit, zero logic impact. (Report 1-2)
