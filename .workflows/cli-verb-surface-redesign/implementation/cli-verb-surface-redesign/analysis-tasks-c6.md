---
topic: cli-verb-surface-redesign
cycle: 6
total_proposed: 1
---
# Analysis Tasks: CLI Verb Surface Redesign (Cycle 6)

## Task 1: Retarget stale source comments that cite redesign-deleted files (cmd/state_cleanup.go, attach.go)
status: pending
severity: low
sources: standards

**Problem**: The redesign deleted `cmd/state_cleanup.go` (folded into `portal uninstall`) and `cmd/attach.go` (folded into `portal open --session`), but four in-source comments still name those non-existent files as live consumers/provenance, so a reader tracing the seam is sent to files that no longer exist:
- `internal/tmux/hooks_unregister.go:14-15` — says the exported `UnregisterPortalHooks` signature is "consumed as a function value by cmd/state_cleanup.go". The real function-value consumer is now `cmd/uninstall.go`'s `buildUninstallDeps` (defaults `Unregister` to `tmux.UnregisterPortalHooks`).
- `internal/tmux/hooks_unregister.go:95-96` — repeats "consumed as a function value by cmd/state_cleanup.go" on `UnregisterPortalHooks` itself. Same real consumer (`cmd/uninstall.go` / `buildUninstallDeps`).
- `internal/tmux/tmux.go:382` — `KillSession`'s chokepoint comment lists "the internal _portal-saver callers (cmd/state_cleanup.go, internal/tmux/portal_saver.go)". The real `_portal-saver` `KillSession` caller is now `cmd/uninstall.go` (`killSaver`, which calls `c.KillSession(tmux.PortalSaverName)`); `internal/tmux/portal_saver.go` remains correct.
- `internal/resolver/query.go:307-308` — justifies the `//nolint:staticcheck` on the capitalised "No session found" message as "silenced per the directive attach.go carried". `attach.go` is deleted; the message is retained verbatim for byte-compat with the retired attach command.

All four are pure documentation drift — the code moved correctly during the redesign; only the provenance comments are stale. No behavioural impact.

**Solution**: Retarget each comment to the surviving consumer, and reword the query.go directive justification so it no longer points at a deleted file. Comment-only — no code, signature, or behaviour changes.

**Outcome**: Every comment in the three files names a file that exists. A maintainer tracing `UnregisterPortalHooks`, the `KillSession` `_portal-saver` path, or the `query.go` staticcheck directive lands on the real surviving consumer (`cmd/uninstall.go`) or an accurate house-style/byte-compat justification, never on a deleted `state_cleanup.go` / `attach.go`.

**Do**:
1. `internal/tmux/hooks_unregister.go:14-15` — replace "consumed as a function value by cmd/state_cleanup.go" with the real consumer: `cmd/uninstall.go` (specifically `buildUninstallDeps`, which defaults the `Unregister` seam to `tmux.UnregisterPortalHooks`).
2. `internal/tmux/hooks_unregister.go:95-96` — replace the second "consumed as a function value by cmd/state_cleanup.go" with the same `cmd/uninstall.go` (`buildUninstallDeps`) reference.
3. `internal/tmux/tmux.go:382` — in the `KillSession` chokepoint comment, replace `cmd/state_cleanup.go` in the `_portal-saver` callers list with `cmd/uninstall.go` (`killSaver`); keep `internal/tmux/portal_saver.go` as-is.
4. `internal/resolver/query.go:307-308` — reword "silenced per the directive attach.go carried" so it no longer names `attach.go`; state instead that the capitalised leading word is a deliberate user-facing message (ST1005 silenced per house style) whose verbatim text is preserved for byte-compat with the former `attach` miss path. Leave the `//nolint:staticcheck` directive and the message string exactly as they are.

**Acceptance Criteria**:
- No comment in `internal/tmux/hooks_unregister.go`, `internal/tmux/tmux.go`, or `internal/resolver/query.go` names `cmd/state_cleanup.go` or `attach.go` (`grep -rn "state_cleanup.go\|attach.go" internal/` returns nothing from these sites).
- The two `hooks_unregister.go` comments name `cmd/uninstall.go` (`buildUninstallDeps`) as the `UnregisterPortalHooks` function-value consumer.
- The `tmux.go:382` `KillSession` comment lists `cmd/uninstall.go` (`killSaver`) and `internal/tmux/portal_saver.go` as the `_portal-saver` callers.
- The `query.go:307` comment justifies the staticcheck `nolint` via house style / byte-compat with the retired attach miss message, without pointing at a deleted file; the `//nolint:staticcheck` directive and the `"No session found: %s"` string are byte-for-byte unchanged.
- Comment-only change: no code, signature, or behavioural edit in any of the three files. `go build ./...` succeeds and `golangci-lint run` is clean on the three touched files.

**Tests**:
- No new tests required (comment-only change). The existing suites exercising `UnregisterPortalHooks` (bootstrap/uninstall), `KillSession` exact-target behaviour, and the resolver's "No session found" miss path must continue to pass unchanged, confirming no behaviour moved.
