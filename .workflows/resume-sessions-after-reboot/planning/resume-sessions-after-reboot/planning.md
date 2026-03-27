# Plan: Resume Sessions After Reboot

## Phase 1: Hook Registry and CLI Surface
<!-- status: approved | approved_at: 2026-03-27 -->

**Goal**: Deliver the persistent hook store, volatile marker tmux operations, and the complete `hooks set`/`hooks rm`/`hooks list` CLI commands so external tools can register and manage restart hooks.

**Rationale**: This is the foundational capability everything else depends on. External tools (like Claude Code) need the `xctl hooks set` command to register hooks before execution logic has any work to do. The store, tmux marker operations, and CLI form a cohesive vertical slice — the registry is usable end-to-end after this phase. This follows the existing `alias` command pattern closely, reducing design risk. It must come first because Phase 2 (execution) reads from this store and checks these markers.

**Acceptance Criteria**:
- [ ] `internal/hooks` package exists with a JSON-backed store at `~/.config/portal/hooks.json` supporting load, save, set, remove, list, and get-by-pane operations, using the atomic write pattern from `project/store.go`
- [ ] `tmux.Client` supports `SetServerOption`, `GetServerOption`, and `DeleteServerOption` methods for `@`-prefixed user options at server level
- [ ] `hooks set --on-resume "cmd"` writes a persistent entry for `$TMUX_PANE` and sets the volatile marker `@portal-active-{pane_id}`
- [ ] `hooks rm --on-resume` removes the persistent entry for `$TMUX_PANE` and removes the volatile marker; silent no-op if no hook exists
- [ ] `hooks list` outputs all registered hooks in tab-separated format (pane ID, event type, command)
- [ ] `hooks set` and `hooks rm` error with "must be run from inside a tmux pane" when `$TMUX_PANE` is unset
- [ ] `hooks` is added to `skipTmuxCheck` in `root.go` so the command bypasses Portal's tmux bootstrap
- [ ] The event type flag (`--on-resume`) is required for `set` and `rm`; running without it produces an error

#### Tasks
<!-- status: draft -->

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| resume-sessions-after-reboot-1-1 | Hook Store | missing file returns empty map, malformed JSON returns empty map, atomic write creates parent directory, set is idempotent (overwrites existing entry for same pane and event) |
| resume-sessions-after-reboot-1-2 | Tmux Server Option Methods | GetServerOption returns not-found when option does not exist, DeleteServerOption for non-existent option |
| resume-sessions-after-reboot-1-3 | Hooks List Command | empty store produces no output, hooks bypasses tmux bootstrap |
| resume-sessions-after-reboot-1-4 | Hooks Set Command | TMUX_PANE unset produces error, idempotent overwrite of existing hook, on-resume flag is required |
| resume-sessions-after-reboot-1-5 | Hooks Rm Command | TMUX_PANE unset produces error, silent no-op when no hook exists, on-resume flag is required |

## Phase 2: Hook Execution in Connection Flow
<!-- status: approved | approved_at: 2026-03-27 -->

**Goal**: Implement the core resume behavior — when connecting to a session via Portal, check each of the session's panes for hooks that need execution (persistent entry exists, volatile marker absent) and fire restart commands via `send-keys` before connecting.

**Rationale**: This is the feature's reason for existing and depends on the registry and markers from Phase 1. It touches all three connection paths (TUI picker, direct path, `portal attach`) and introduces new tmux operations (`list-panes`, `send-keys`). The execution logic is a distinct concern from the CLI surface — it has different testing needs (mocking connection flows, verifying send-keys calls, testing the two-condition check) and a different risk profile (modifying critical connection paths). It must come after Phase 1 because it reads from the hook store and checks volatile markers.

**Acceptance Criteria**:
- [ ] `tmux.Client` supports `ListPanes(sessionName)` returning pane IDs for a session and `SendKeys(paneID, command)` for delivering commands
- [ ] Hook execution logic checks each pane in the target session: executes the on-resume command (via `send-keys`) only when a persistent entry exists AND the volatile marker is absent
- [ ] After executing a hook, the volatile marker `@portal-active-{pane_id}` is set to prevent re-execution
- [ ] Hook execution is inserted before session connection in all three paths: TUI selection (`processTUIResult`/`openTUI`), direct path (`openPath`), and `portal attach`
- [ ] Multiple panes with hooks are executed sequentially; `send-keys` failures for individual panes are silently ignored
- [ ] Hook execution is scoped to the target session's panes only — hooks for other sessions are not touched

## Phase 3: Stale Hook Cleanup
<!-- status: approved | approved_at: 2026-03-27 -->

**Goal**: Implement lazy cleanup of hook entries for panes that no longer exist, and extend the `xctl clean` command to include hook cleanup.

**Rationale**: This is an additive, low-risk concern that depends on the store (Phase 1) and is naturally tested after execution (Phase 2) has validated the full flow. It mirrors the existing `CleanStale` pattern in the project store. Separating it keeps Phase 2 focused on the critical execution path, and this phase can be validated independently by creating stale entries and verifying pruning behavior. It comes last because the system is fully functional without it — cleanup is a hygiene concern, not a correctness one.

**Acceptance Criteria**:
- [ ] Hook store supports a `CleanStale` method that cross-references pane IDs against live tmux panes and removes entries for panes that no longer exist
- [ ] Stale cleanup runs automatically when hooks are read during the execution flow (Phase 2's execution path), invisible to the user
- [ ] `xctl clean` command output includes removed stale hook entries alongside removed stale projects
- [ ] Stale cleanup does not error when no tmux server is running (returns gracefully with no changes)
