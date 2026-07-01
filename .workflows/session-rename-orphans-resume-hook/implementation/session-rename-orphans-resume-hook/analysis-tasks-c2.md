---
topic: session-rename-orphans-resume-hook
cycle: 2
total_proposed: 2
---
# Analysis Tasks: Session Rename Orphans Resume Hook (Cycle 2)

## Task 1: Add a fast tmux-less guard binding session.PortalIDOption to the hook-key format strings
status: pending
severity: medium
sources: architecture

**Problem**: The fix's central invariant requires three independent embeddings of the literal `"@portal-id"` to stay byte-identical: the source-of-truth constant `session.PortalIDOption` (internal/session/create.go:29), the literal inside `tmux.HookKeyFormat` (internal/tmux/tmux.go:849), and the literal inside `state.captureFormat` (internal/state/capture.go:42). `internal/tmux` and `internal/state` cannot import `internal/session` (import cycle), so the literals are necessarily duplicated. The two cycle-1 static guards that run in tmux-less CI — `internal/tmux/hookkey_test.go` (`TestHookKeyFormatContainsPortalIDLiteral`) and `internal/state/portal_id_literal_guard_test.go` (`TestCaptureFormatContainsPortalIDLiteral`) — each assert only that their own format string contains a *locally-hardcoded* copy `const portalIDLiteral = "@portal-id"`. Neither imports or compares against `session.PortalIDOption`. Consequently, if the source-of-truth constant were ever changed (e.g. to `@portal-uid`), BOTH cycle-1 guards still pass (each checks its own copy), stamping would write the new value while both format strings still read `@portal-id`, and every stamped session would silently orphan its hook — the exact bug this fix removes, reintroduced at scale. The only tests that catch a change to the constant itself are the `//go:build integration` + `SkipIfNoTmux`-gated end-to-end tests, which do not run in the default tmux-less `go test ./...` path. This is a genuine residual gap: the cycle-1 guards pin each format string to a local copy of the literal, but nothing pins the source-of-truth constant to that same literal in a fast tmux-less path.

**Solution**: Add a fast, non-tmux-gated binding guard test in the `cmd` package (which already imports both `internal/session` and `internal/tmux` — verified cycle-free, since `internal/session` imports `internal/tmux`, so a `cmd`-package test importing both is fine). The guard asserts `session.PortalIDOption == "@portal-id"` (catches a change to the constant) AND `strings.Contains(tmux.HookKeyFormat, session.PortalIDOption)` (ties the constant to the tmux format-string embedding). Combined with the two existing cycle-1 guards (each of which already pins its own format string to the `"@portal-id"` literal), this transitively closes the loop: constant == "@portal-id" == HookKeyFormat literal == captureFormat literal, all verifiable without tmux. `state.captureFormat` is unexported so it cannot be reached from `cmd`; its cycle-1 guard already pins it to the shared literal, so the transitive chain still holds via the shared literal value. Do NOT attempt to reach `captureFormat` from `cmd`.

**Outcome**: A change to `session.PortalIDOption` (or a drift between the constant and `tmux.HookKeyFormat`) fails a fast `go test ./cmd` run with NO tmux present, giving the "Missed key-producing site" drift class — the spec's named primary risk — a fast tripwire it currently lacks. All existing tests continue to pass unchanged (the current values all agree).

**Do**:
1. Create a new test file `cmd/portal_id_binding_guard_test.go` in package `cmd_test` (or package `cmd` — either compiles; `cmd_test` is preferred for an external-facing binding assertion). It MUST NOT be gated by `//go:build integration` and MUST NOT call `tmuxtest.SkipIfNoTmux` — it must run under plain tmux-less `go test ./cmd`.
2. Import `strings`, `testing`, `github.com/leeovery/portal/internal/session`, and `github.com/leeovery/portal/internal/tmux`.
3. Write a single test, e.g. `TestPortalIDOptionBindsHookKeyFormat`, that:
   - Fatals if `session.PortalIDOption != "@portal-id"` (the source-of-truth constant must stay the canonical literal that both format strings embed).
   - Errors if `!strings.Contains(tmux.HookKeyFormat, session.PortalIDOption)` (the tmux format string must embed the source-of-truth constant's exact value, not a copy that can drift).
4. Add a doc-comment explaining WHY this guard exists in `cmd` (the only package that can import both `session` and `tmux` without an import cycle), what drift class it catches (a change to `session.PortalIDOption` silently orphaning every stamped session's hook), and that it deliberately runs tmux-less to complement the `SkipIfNoTmux`-gated end-to-end guards. Reference the two cycle-1 sibling guards (`internal/tmux/hookkey_test.go`, `internal/state/portal_id_literal_guard_test.go`) so a future reader sees the full three-way picture.
5. Do NOT modify the two existing cycle-1 guards, the constant, or the format strings — this task adds coverage only.

**Acceptance Criteria**:
- A new tmux-less guard test exists in the `cmd` package asserting both `session.PortalIDOption == "@portal-id"` and `strings.Contains(tmux.HookKeyFormat, session.PortalIDOption)`.
- The test is NOT `//go:build integration`-tagged and does NOT call `SkipIfNoTmux`; it runs and passes under `go test ./cmd` with no tmux server present.
- Mutating `session.PortalIDOption` to any other value (verify manually or by reasoning) causes the new test to fail fast without tmux.
- `go test ./...` remains green; no production code, existing test, or the two cycle-1 guards are altered.

**Tests**:
- The task IS a test. Verify it passes tmux-less: `go test ./cmd -run TestPortalIDOptionBindsHookKeyFormat`.
- Confirm the full suite still passes: `go build -o portal .` and `go test ./...`.

## Task 2: Delete redundant verifyRenameHookFiredOnce; reuse the shared assertHookFireCount helper
status: pending
severity: low
sources: duplication

**Problem**: `verifyRenameHookFiredOnce` (internal/restore/rename_reboot_hook_integration_test.go:385-395) reads the hook-fire side-effect file, counts `"HOOK_FIRED"` occurrences, and asserts exactly one. `assertHookFireCount` (internal/restore/rename_reboot_durability_integration_test.go:307-317) does the identical read-and-count and asserts an arbitrary `want`. Both files are `//go:build integration` and live in the SAME `restore_test` package. `verifyRenameHookFiredOnce(t, file)` is exactly `assertHookFireCount(t, file, 1)` — its own doc-comment even notes it "Mirrors reboot_roundtrip_test.go's verifyHookFiredOnce shape". Two functions in the same package now embody the same read-count-assert logic; the fixed `HOOK_FIRED` marker string and the count-mismatch error message are maintained in two places. `verifyRenameHookFiredOnce` has exactly one call site (line 344); `assertHookFireCount` is already the package's shared helper, used in three places across two other files.

**Solution**: Delete `verifyRenameHookFiredOnce` and replace its single call site with the already-shared `assertHookFireCount(t, hookFireFile, 1)`. This yields one read-count-assert implementation for the whole `restore_test` package and reduces coupling (the call site moves to an already-consumed shared helper).

**Outcome**: A single `assertHookFireCount` owns the `HOOK_FIRED` read/count/message contract for the entire package; a future change to the marker string or count-mismatch diagnostics is made in exactly one place. The 3-5 headline test's behaviour is unchanged (still asserts exactly one firing).

**Do**:
1. In `internal/restore/rename_reboot_hook_integration_test.go`, replace the call at line 344 `verifyRenameHookFiredOnce(t, hookFireFile)` with `assertHookFireCount(t, hookFireFile, 1)`.
2. Delete the `verifyRenameHookFiredOnce` function definition and its doc-comment (lines ~378-395).
3. If any import (e.g. `strings`) or referenced identifier becomes unused in that file after the deletion, remove it so the file compiles clean. (Note: `os` and `strings` are likely still used elsewhere in the file — verify before removing; only remove genuinely-orphaned imports.)
4. Do NOT alter `assertHookFireCount` itself, and do NOT touch the 3-6 durability or 3-7 multipane files.

**Acceptance Criteria**:
- `verifyRenameHookFiredOnce` no longer exists anywhere in `internal/restore/`.
- The single former call site now calls `assertHookFireCount(t, hookFireFile, 1)` and the 3-5 headline test still asserts the hook fired exactly once.
- The integration build compiles clean: `go build -tags integration ./internal/restore/...` (or the project's standard integration build) succeeds with no unused-import/unused-function errors.
- No behaviour change to the 3-5 test's pass/fail semantics.

**Tests**:
- Existing 3-5 headline integration test (`internal/restore/rename_reboot_hook_integration_test.go`) continues to exercise the exactly-once assertion via `assertHookFireCount` — no new test needed.
- Verify tmux-less compile of the standard suite (`go test ./...`) and, where tmux is available, the integration-tagged restore suite still passes (`go test -tags integration ./internal/restore/...`).
