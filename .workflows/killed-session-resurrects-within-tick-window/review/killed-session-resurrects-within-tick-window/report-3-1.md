TASK: Delete defaultTouchSaveRequested wrapper (killed-session-resurrects-within-tick-window-3-1)

ACCEPTANCE CRITERIA:
- Wrapper `defaultTouchSaveRequested` removed from `cmd/state_commit_now.go`.
- `CommitNowDeps.TouchSaveRequested` field default symmetric with sibling defaults (`state.X` directly).
- Tests substitute the field, not the wrapper symbol.

STATUS: Complete

SPEC CONTEXT: Cycle-2 cleanup. After cycle-1 promoted `state.TouchSaveRequested` to the state package as single source of truth, the cmd-side `defaultTouchSaveRequested` became a one-line trampoline asymmetric with sibling `CommitNowDeps` defaults (which all reference `state.X` directly). Delete the wrapper, assign `state.TouchSaveRequested` directly.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/state_commit_now.go:99` (`TouchSaveRequested: state.TouchSaveRequested,`) — symmetric with siblings on lines 94-96 (`ReadIndex: state.ReadIndex`, `CaptureStructure: state.CaptureStructure`, `Commit: state.Commit`).
- Notes: Repo-wide ripgrep for `defaultTouchSaveRequested` returns zero hits in source code. Doc-comment at `cmd/state_commit_now.go:82` correctly states "Defaults to state.TouchSaveRequested." with no stale wrapper reference. `IsRestoring` legitimately remains a closure because its signature requires a tmux client.

TESTS:
- Status: Adequate
- Coverage: Tests construct inline `func(dir string) error` at `cmd/state_commit_now_test.go:119-126` and assign to `deps.TouchSaveRequested`. Six existing assertions (`TouchSaveRequested calls = ...`) at lines 616, 750, 808, 861, 940, 1050 cover happy-path no-touch, both short-circuit touches, capture-failure touch, commit-failure touch, and touch-failure-during-failure. Production callsites (`state_notify.go:48`, daemon-merge integration test) invoke `state.TouchSaveRequested` directly.
- Notes: No new tests required — pure trampoline removal behaviour-equivalent to prior state.

CODE QUALITY:
- Project conventions: Followed. Symmetry with sibling defaults is the canonical cmd-package DI idiom.
- SOLID: Good. Removes unnecessary indirection.
- Complexity: Low. One fewer function in the file.
- Modern idioms: Yes.
- Readability: Improved — reader no longer chases a one-line wrapper.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
