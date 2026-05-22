TASK: Add `portal state commit-now` happy-path subcommand (killed-session-resurrects-within-tick-window-1-1)

ACCEPTANCE CRITERIA:
- New hidden `portal state commit-now` subcommand registered under `state`.
- Happy path: `state.ReadIndex` → `state.CaptureStructure(client, nil, &prev)` → `state.Commit(dir, idx, false, logger)`.
- Edge cases: zero sessions; one session; multi-window multi-pane; underscore-prefixed sessions filtered out (via `keepSessionNames`); missing/corrupt `sessions.json` → zero-value `PrevIndex` + WARN log (never fatal).
- Successful sync commit does NOT touch `save.requested`.
- Writes only `sessions.json`; no `.bin` files (`anyScrollbackChanged=false`).

STATUS: Complete

SPEC CONTEXT:
Spec § Fix Approach mandates synchronous commit reads prior `sessions.json` via `state.ReadIndex` to preserve scrollback-hash/content fields for live sessions, then calls `state.CaptureStructure` and `state.Commit` with `anyScrollbackChanged=false`. PrevIndex resolution failure (ENOENT or decode error) falls back to zero-value `PrevIndex` with WARN log, never fatal. The synchronous path writes only `sessions.json` — no `.bin` files, no marker management. On success the daemon's dirty flag (`save.requested`) is not touched.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/state_commit_now.go:151-219` — `stateCommitNowCmd` + `RunE`.
  - `cmd/state_commit_now.go:270-281` — `loadPrevIndex` ENOENT/decode fallback with WARN log under `state.ComponentDaemon`.
  - `cmd/state_commit_now.go:65-84` — `CommitNowDeps` DI struct.
  - `cmd/state_commit_now.go:92-124` — `resolveCommitNowDeps` per-field nil fallback.
  - `cmd/state_commit_now.go:283-285` — `init()` registers under `stateCmd`.
  - `Hidden: true`, `Args: cobra.NoArgs`, `SilenceErrors: true`, `SilenceUsage: true`.
- Notes:
  - `Commit` invoked with `anyScrollbackChanged=false` (line 213).
  - `CaptureStructure(client, nil, &prev)` — correct skipSet nil, prev pointer.
  - `loadPrevIndex` distinguishes ENOENT skip from decode-failure WARN.
  - Successful path does not touch `save.requested`.

TESTS:
- Status: Adequate
- Coverage (`cmd/state_commit_now_test.go`):
  - Zero sessions: `TestStateCommitNow_WritesEmptySessionsJSONWhenZeroLiveSessions` (line 164).
  - One session: `TestStateCommitNow_WritesSessionWithWindowsAndPanes` (line 191).
  - Multi-window multi-pane: `TestStateCommitNow_WritesMultiWindowMultiPaneSession` (line 237).
  - PrevIndex passthrough: line 295.
  - Underscore filter (real `state.CaptureStructure`): line 358.
  - Missing `sessions.json` → zero Prev + WARN: line 398.
  - Corrupt `sessions.json` → zero Prev + WARN: line 448.
  - `save.requested` untouched on success: line 491.
  - No `.bin` written, `anyScrollbackChanged=false`: line 515.
  - Subcommand registration: line 1262.
- Notes: Every plan-task edge case has a corresponding test. WARN log assertions check level + component + message tokens. No `t.Parallel`.

CODE QUALITY:
- Project conventions: Followed. Matches `*Deps` + `resolve*Deps` + per-field nil-fallback idiom.
- SOLID principles: Good. Function-field collaborators, single-responsibility helpers.
- Complexity: Low. `RunE` is linear, ~30 lines.
- Modern idioms: Yes. `errors.Is`/`errors.Unwrap`, `%w` wrap, descriptive sentinel.
- Readability: Good. Doc comments cite spec sections.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] Typo `restoringCals` at `cmd/state_commit_now_test.go:80` — should be `restoringCalls`. Field is unused.
- [idea] `resolveCommitNowDeps` mirrors `defaultBootstrapDeps`/`openDeps`/`hooksDeps` per-field nil-fallback. A small generic helper could DRY this; defer unless duplication grows.
- [idea] `cmd/state_commit_now_test.go` is 1,273 lines bundling 1-1/1-2/1-3 tests. Splitting into per-task files would aid navigation; section banners make it scannable today.
- [idea] `loadPrevIndex` duplicates `state.ReadIndex` (skip, err) discrimination. Could become `state.ReadIndexOrZero(dir, logger, component)` if a second consumer emerges.
