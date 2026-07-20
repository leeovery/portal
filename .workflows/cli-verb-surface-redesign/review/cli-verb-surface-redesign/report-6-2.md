TASK: cli-verb-surface-redesign-6-2 — Fully hide the `state` namespace (parent hidden, children argv-invocable)

ACCEPTANCE CRITERIA:
- Parent `stateCmd` gains `Hidden:true`
- The six children (daemon/hydrate/signal-hydrate/notify/commit-now/migrate-rename) are already `Hidden:true` (assert each)
- All six stay fully invocable by argv (`Hidden != disabled`) so hook firing / hydrate still work
- `state` prefix preserved (no rename — `notifyCommand`/`commitNowSubstring`/`migrateRenameSubstring`/`PortalDaemonArgvPattern` substring matching unchanged)
- state stays in `skipTmuxCheck`
- Gone from `--help` top-level listing and generated completion
- Precondition: status/cleanup deleted in Phase 4 so zero user-facing children remain

STATUS: Complete

SPEC CONTEXT:
Specification § "`state` Namespace — Fully Hidden" (lines 337-348). The namespace becomes fully hidden but cannot stop being a command: every remaining `state` child is a separate-process argv entry point (daemon runs in the _portal-saver pane; hydrate exec'd via respawn-pane -k; signal-hydrate/notify/commit-now/migrate-rename fired by tmux hooks as run-shell "portal state …"), so each must stay invocable. Once status→doctor and cleanup→uninstall (Phase 4) left zero user-facing children, the whole namespace is marked hidden. The spec explicitly mandates keeping the `state` prefix because hook idempotency matches those command strings by substring (`notifyCommand`, `commitNowSubstring`, `migrateRenameSubstring`, `PortalDaemonArgvPattern`); renaming would churn internal matching for zero user benefit.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state.go:15-19 — parent `stateCmd` now carries `Hidden: true`, with a thorough doc comment (lines 5-14) explaining that Hidden is visibility-only, the subtree stays argv-invocable, and the `state` prefix + child names are preserved for the substring matchers.
  - Six children each `Hidden: true`: cmd/state_daemon.go:724, cmd/state_hydrate.go:474, cmd/state_signal_hydrate.go:108, cmd/state_notify.go:29, cmd/state_commit_now.go:157, cmd/state_migrate_rename.go:19.
  - All six registered on `stateCmd` via `stateCmd.AddCommand(...)` in each file's init (verified: commit_now:292, migrate_rename:97, daemon:846, signal_hydrate:139, hydrate:517, notify:51).
  - `state` stays in `skipTmuxCheck` — cmd/root.go:65 (`"state": true`), with an accurate doc comment (lines 34-38) noting the children are internal argv entry points and re-running bootstrap would recursively register hooks / spawn nested daemons.
  - `state` prefix preserved: substring/pattern constants unchanged and still literal `portal state …` strings — `notifyCommand` (internal/tmux/hooks_register.go:152 → "portal state notify"), `commitNowSubstring` (:189 → "portal state commit-now"), `migrateRenameSubstring` (:80 → "portal state migrate-rename"), `PortalDaemonArgvPattern` (internal/state/daemon_identity.go:45 → `^portal state daemon( |$)`).
- Notes: The change is minimal and correct — the parent gains one `Hidden: true` line; children were pre-hidden. No rename anywhere; all substring anchors intact. Precondition holds: no user-facing `state` children remain (status/cleanup removed in Phase 4).

TESTS:
- Status: Adequate
- Coverage (cmd/state_test.go):
  - TestStateParentIsHidden (276) — locks `stateCmd.Hidden==true` AND `IsAvailableCommand()==false`; a strong invariant that survives future child churn.
  - TestStateHiddenSubcommandsAreHidden (285) — two subtests: (a) each of the six package-level child vars (`stateChildCommands`, 262) is Hidden; (b) every *registered* child is Hidden, with a `len(children)==len(stateChildCommands)` count guard so a new un-hidden child fails loudly. Referencing children by var (not string) makes a rename/dropped-registration a compile error.
  - TestStateChildrenRemainInvocableByArgv (315) — proves Hidden != disabled: all six resolve via `rootCmd.Find([state <name>])` to the correct command. This is the direct argv-invocability proof for all six.
  - TestStateInternalSubcommandsAcceptValidArgv (157) — actually Executes daemon/notify/signal-hydrate/hydrate/migrate-rename with valid argv (exit 0, no stderr noise), with proper per-subtest state-dir isolation (PORTAL_STATE_DIR=t.TempDir()) and run-func stubs for the blocking daemon/hydrate paths.
  - TestStateHiddenSubcommandsAbsentFromShellCompletions (331) — bash/zsh/fish generated completions omit all six children AND a bare `state` entry (whole-word `\bstate\b` regex avoids the "statement(s)" boilerplate false-positive — a nice touch).
  - TestStateCommandRegistration (48) — state is registered on root yet absent from `portal --help` Available Commands; `portal state --help` lists no user-facing children (only cobra's help/completion tolerated); hidden children never surface at root.
  - TestStateBareInvocationPrintsHelp (140) — bare `portal state` exits 0 and prints help.
  - Micro-acceptance mapping: parent-hidden ✓, each-child-hidden ✓, argv-invocable ✓, absent-from-help ✓, absent-from-completion ✓.
- Notes: Not under-tested — every acceptance sub-claim has a dedicated assertion, and the count-guard + var-reference design defends against future drift. Not over-tested — the two TestStateHiddenSubcommandsAreHidden subtests overlap slightly (each-var vs each-registered) but the second adds the distinct drift/count guard, so it earns its place. `commit-now` is exercised for argv-invocability via Find (TestStateChildrenRemainInvocableByArgv) and for Hidden (stateChildCommands) but is the one child not driven through a full Execute in TestStateInternalSubcommandsAcceptValidArgv; this is a reasonable omission (commit-now needs tmux/state to do real work and its invocability is already proven via Find), not a coverage gap for this task.

CODE QUALITY:
- Project conventions: Followed. Consistent with the codebase's cobra pattern (package-level command var + init AddCommand), the substring-matcher single-source-of-truth convention, and the skipTmuxCheck documentation style. Tests obey the "no t.Parallel in cmd" rule (file header comment at line 1) and isolate PORTAL_STATE_DIR per subtest.
- SOLID principles: Good. Single obvious responsibility; the visibility flag is orthogonal to resolution/execution.
- Complexity: Low. One added field.
- Modern idioms: Yes (e.g. `strings.SplitSeq` in the test helper).
- Readability: Good. cmd/state.go's doc comment is precise about why Hidden is visibility-only and why the prefix must be preserved — it captures the load-bearing rationale future maintainers need.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
