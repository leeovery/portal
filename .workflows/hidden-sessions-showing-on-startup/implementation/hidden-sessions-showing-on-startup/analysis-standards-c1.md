# Analysis — Standards (cycle 1)

STATUS: clean
FINDINGS_COUNT: 0

Implementation conforms to specification and project conventions.

- **Fix A — chokepoint filter**: `Client.ListSessions` applies `strings.HasPrefix(name, "_")` filter as the final post-processing step (`internal/tmux/tmux.go:149-163`), returns non-nil empty slice per the return-value contract.
- **Fix B — bootstrap rename**: `PortalBootstrapName = "_portal-bootstrap"` constant exported at `internal/tmux/portal_saver.go:26` (sibling to `PortalSaverName`); `StartServer` invokes `new-session -d -s PortalBootstrapName` (`tmux.go:195`); no literal `"_portal-bootstrap"` strings outside the constant declaration.
- **Capture-path invariant preserved**: `ListSessionNames` remains a thin delegation to `ListSessions` (`tmux.go:171-181`).
- **Doc-comment cleanup**: `PortalSaverName` comment (`portal_saver.go:10-16`) references `Client.ListSessions` as the chokepoint; `StartServer` comment (`tmux.go:183-193`) drops stale tmux-resurrect rationale, retains the `exit-empty on` reasoning reframed against Portal's own Restore step.
- **Test coverage**: `TestListSessionsFiltersUnderscorePrefixed` (`tmux_test.go:139`), updated `TestStartServer` asserting args include `-s PortalBootstrapName` (`tmux_test.go:478`), and `verifyPostBootstrapSessionSet` (`reboot_roundtrip_test.go:641-712`) performing both raw-tmux assertion and `Client.ListSessions` assertion.
- **Empty-list verification**: `cmd/list.go:66-68` already silent-returns on empty slice — no change required.
- **Upgrade note**: `CHANGELOG.md` carries the spec-suggested wording about restarting tmux to clear leftover `0` sessions.
- **Sole production caller verified**: All four production `new-session` invocations (`tmux.go:95, 195, 291, 549`) are paired with `-s`; no unnamed call site remains.
