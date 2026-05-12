---
topic: multiple-state-daemons-running-concurrently
cycle: 2
total_proposed: 1
---
# Analysis Tasks: multiple-state-daemons-running-concurrently (Cycle 2)

## Task 1: Route pre-recycle tmux server PID capture through captureTmuxServerPID helper
status: approved
severity: low
sources: duplication

**Problem**: In `internal/tmux/portal_saver_integration_test.go`, `TestEnsurePortalSaverVersion_SingletonInvariantAcrossRecycle` captures the tmux server PID twice via the exact same `display-message -p '#{pid}'` recipe. The first capture (lines 172-176) is still inlined as `serverPIDOut := sock.Run(t, "display-message", "-p", "#{pid}"); serverPID, err := strconv.Atoi(strings.TrimSpace(serverPIDOut))` with its own error branch, while the second capture at line 248 already routes through `captureTmuxServerPID` (defined at lines 263-271). The helper's doc comment at lines 258-262 states "Extracted as a helper because the test captures the server PID twice" — but only one of the two sites was migrated in cycle 1, so the rationale documented on the helper is currently misleading.

**Solution**: Replace the inline pre-recycle capture block (lines 172-176) with a single `serverPID := captureTmuxServerPID(t, sock)` call so both capture sites share the helper, matching the helper's documented motivation.

**Outcome**: Both server-PID capture sites in the test route through `captureTmuxServerPID`. The helper's doc comment becomes truthful (it really is consumed twice). ~5 lines of duplicated parse/error logic are removed. No behavioural change to the test.

**Do**:
1. Open `internal/tmux/portal_saver_integration_test.go`.
2. Replace lines 169-176 (the comment block "Capture tmux server PID for the on-failure diagnostic dump…" plus the inlined `sock.Run` / `strconv.Atoi` / error branch) with:
   - A one- or two-line comment preserving the "pre-recycle server PID for the on-failure diagnostic dump" intent.
   - A single call: `serverPID := captureTmuxServerPID(t, sock)`.
3. Confirm the local variable name remains `serverPID` so the downstream `dumpDiagnostics(t, dir, serverPID, …)` call sites at lines ~217 and ~221 continue to compile unchanged.
4. Confirm no unused imports are left behind (the existing file already uses `strconv` and `strings` elsewhere, but if those uses were limited to the deleted block, prune them).
5. Re-read the helper's doc comment (lines 258-262) and confirm its "captures the server PID twice" rationale now matches reality. No edit required to the comment.

**Acceptance Criteria**:
- `internal/tmux/portal_saver_integration_test.go` no longer contains an inline `display-message -p '#{pid}'` call for the pre-recycle server PID capture.
- Both pre-recycle and post-recycle server-PID captures call `captureTmuxServerPID(t, sock)`.
- `go build ./...` succeeds.
- `go vet ./...` reports no issues for the modified package.

**Tests**:
- `go test ./internal/tmux/ -run TestEnsurePortalSaverVersion_SingletonInvariantAcrossRecycle` continues to pass (no behavioural change expected; the helper is byte-identical to the inlined recipe).
- `go test ./internal/tmux/...` continues to pass.
