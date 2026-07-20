TASK: cli-verb-surface-redesign-4-6 — `portal uninstall` — runtime-only teardown (+ delete `state cleanup`)

ACCEPTANCE CRITERIA (plan row 4-6 + Phase 4 bullet):
- server down → graceful no-op (skip kill/unregister) still prints completion message
- saver already absent / no hooks registered → idempotent success
- leaves all user sessions AND the load-bearing `_portal-bootstrap` anchor running
- touches no state-dir or config files (fully recoverable — `open` re-bootstraps)
- no `--yes` gate or prompt
- kill-before-unregister ordering preserved for the daemon SIGHUP flush
- relocate `killSaver`/`isSessionAbsentError` out of the deleted `state_cleanup.go`
- prints the completion/recovery path message; bootstrap-exempt (skipTmuxCheck)

STATUS: Complete

SPEC CONTEXT:
Spec §"uninstall — Runtime-Only Teardown (replaces `state cleanup`)" (specification.md:269-291): the command IS the teardown (nothing behind a flag), removes ONLY the tmux-server footprint (kill `_portal-saver` daemon + unregister global hooks), touches NO filesystem, prints a two-line completion/recovery path, is fully recoverable via `open` re-bootstrap, is an idempotent graceful no-op on already-clean state, and leaves all sessions running (user sessions + `_portal-bootstrap`). Explicitly: no `--yes` gate, no prompt. Completion message is byte-specified in the spec.

IMPLEMENTATION:
- Status: Implemented (matches spec + all acceptance criteria)
- Location: cmd/uninstall.go
  - RunE (86-116): `client.ServerRunning()` gates both actions; when down, both skipped → graceful no-op, message still prints (97-107). Kill-before-unregister ordering explicit (101 then 104). Errors accumulate via `errors.Join` (95, 102, 105, 114) — no short-circuit. Completion message printed BEFORE the joined return (109-112) so it appears on partial-failure paths.
  - Completion constants (61-62) are byte-exact to spec §275-279.
  - `killSaver` (135-149): `HasSession` probe → skip when absent (idempotent mode 1); `KillSession` error tolerated via `isSessionAbsentError` (idempotent mode 2, the probe→kill race); other errors logged WARN + returned. INFO breadcrumb centralised in `killSaverInfoMessage` (121) to prevent success-path drift.
  - `isSessionAbsentError` (155-157): case-insensitive "can't find session" substring match — relocated here (verified single definition; no leftover in a state_cleanup.go).
  - No cobra flags registered (grep confirms zero `.Flags()` on uninstallCmd) → no `--yes` gate, `Args: cobra.NoArgs`.
  - Touches no files: RunE issues only tmux client calls (ServerRunning/HasSession/KillSession/unregister); zero filesystem/config I/O by construction.
  - Leaves `_portal-bootstrap` + user sessions: production code only ever references `tmux.PortalSaverName` for kill-session; no code path can kill another session.
  - Bootstrap-exempt: `skipTmuxCheck["uninstall"] = true` (cmd/root.go:66).
  - `state_cleanup.go` DELETED (confirmed absent on disk; git shows deletion in the same task commit d9ba504e). `killSaver`/`isSessionAbsentError` each have exactly one definition, now in uninstall.go.
- Notes: `HasSession` (internal/tmux/tmux.go:135) returns `err == nil`, so a genuine "can't find session" probe returns false → kill skipped → idempotent nil. Correct for the absent-saver criterion.

TESTS:
- Status: Adequate
- Coverage (cmd/uninstall_test.go):
  - Ordering has-session < kill < show-hooks < set-hook + exact message — TestUninstall_KillsPortalSaverBeforeRemovingHooks
  - Down server → no kill/unregister calls, message still prints — TestUninstall_NoServerRunningIsGracefulNoOpAndPrintsMessage
  - Saver absent → no kill-session, idempotent success — TestUninstall_IsIdempotentWhenSaverAbsent
  - Exact completion message + empty stderr — TestUninstall_PrintsExactCompletionMessage
  - Hook-removal failure still runs kill, joined error carries "hook removal", message still prints — TestUninstall_AccumulatesHookRemovalFailureWithoutSkippingKill
  - probe→kill race ("can't find session" on kill) tolerated, unregister still proceeds — TestUninstall_ToleratesKillSessionCantFindSessionError
  - non-idempotent kill error ("permission denied") → joined "daemon kill" error, unregister still runs, message still prints — TestUninstall_KillSessionOtherFailureContributesJoinedErrorAndStillRunsUnregister
  - INFO/daemon/SIGHUP breadcrumb on successful kill — TestUninstall_LogsInfoWhenSaverKilledSuccessfully
  - bootstrap-exempt registration — TestUninstall_RegisteredInSkipTmuxCheck
  - PersistentPreRunE does not run bootstrap (panicRunner) — TestUninstall_DoesNotInvokeBootstrap
  - `wantCompletionMessage` is hard-coded in the test (not imported from prod) so a production string drift fails.
- Notes: Balanced — not over-tested (each test carries a distinct assertion; the message-only test adds the unique empty-stderr check). One implicit-only property: no test explicitly asserts that kill-session targets ONLY `_portal-saver` (the RunFunc `case "kill-session"` matches any target), so the "leaves `_portal-bootstrap` running" criterion is covered by construction/inspection rather than an explicit guard. Low risk — production code has no other kill-session site — but see non-blocking note.

CODE QUALITY:
- Project conventions: Followed. `*Deps` package-level injection point (`uninstallDeps`) with `buildUninstallDeps` fallback, matching the cmd DI pattern; test restores via t.Cleanup; no new log component (reuses closed-taxonomy `daemon` component per CLAUDE.md); tests avoid t.Parallel (file header states it).
- SOLID: Good. Single-responsibility helpers, small seam interfaces (`Unregister func(*tmux.Client) error`), constructor fallback keeps prod path nil-injection.
- Complexity: Low. Linear RunE with one guard; killSaver two-branch idempotency.
- Modern idioms: Good — `errors.Join` for accumulation, wrapped `%w` errors.
- Readability: Good. Load-bearing ordering and idempotency rationale documented in-source (64-85, 123-134).
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] cmd/uninstall_test.go:109 (TestUninstall_KillsPortalSaverBeforeRemovingHooks) — add an assertion that no kill-session call targets `_portal-bootstrap` (or that the only kill-session target is `tmux.PortalSaverName`), to explicitly guard the "leaves the load-bearing `_portal-bootstrap` anchor running" acceptance criterion that is currently only implicitly covered.
- [idea] cmd/uninstall.go:136 — `killSaver` relies on `HasSession`, which collapses ALL errors (incl. transient tmux/server faults) to `false`, so a transient probe failure is silently treated as "saver absent": the kill is skipped while the message still claims the runtime was removed. Consider using the discriminating `HasSessionProbe` (already exists at internal/tmux/tmux.go:165) to distinguish a genuine absence from a transient fault and fold the latter into the joined error. Requires a decision on whether to change the idempotency semantics — best-effort no-op is a defensible current choice.
