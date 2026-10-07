TASK: A Renamed Session Is Not Logged As Dropped (killing-all-sessions-wipes-restore-state-2-7, tick-357799)

ACCEPTANCE CRITERIA:
- A live session is renamed, and the next commit's index holds it under its new name. No drop line is logged, for its old name or its new one. (§4.1, §6.3)
- A commit's index lacks one or more of the sessions `sessions.json` holds, and holds no session that `sessions.json` does not. A drop line names each missing session. (§4.1, §4.3)

STATUS: complete

SPEC CONTEXT: §4.1 requires every written commit to log each dropped session by name at INFO, measured against the prior on-disk index (not a caller's in-memory one), and says a rename is not a drop. The saved index names a session by name alone, so the identity that tells a rename from a drop is left to the implementer (Corrigendum 2026-10-05): at shutdown no new session appears beside a dropped one, so the drop lines a reboot's log is read for (§4.3) come out the same under any identity. §6.3 tests that a rename logs no drop line.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/state/commit.go:85-108 (logDroppedSessions), internal/state/commit.go:114-116 (sameSessionUnderAnyName), called from commitOver at internal/state/commit.go:52-54 after the sessions.json write succeeds; the production cycle reaches it through RunCommitCycle at internal/state/commit_cycle.go:150 with `committed`, the index read under the commit lock at internal/state/commit_cycle.go:121.
- Notes: Sessions the new index holds under a name the prior index lacks are collected as "appeared". Each prior session missing by name is checked against those, and the first one whose windows match it layout for layout is consumed one-to-one (slices.Delete), so one new session can stand in for only one saved session. The identity is the ordered window-layout strings. `#{window_layout}` (captureFormat, internal/state/capture.go:34) embeds tmux's server-unique pane ids, and a rename leaves them as they were. Because the identity ignores the environment, window names and pane cwd/command/token, a rename lands cleanly even when those fields lag in the saved record. The layouts on disk stay current: `window-layout-changed`, `window-linked` and `window-unlinked` each trigger a capture, as `session-renamed` does (internal/tmux/hooks_register.go:22-32). So the identity fails only when a rename and a geometry change land inside the same tick. That is the identity trade-off the spec leaves to the implementer, and the earlier fix round weighed it explicitly against a `#{session_id}` schema field. With no appeared session, every missing one is logged (criterion 2). Nothing drifts from the plan.

TESTS:
- Status: Adequate
- Coverage: internal/state/commit_drop_log_test.go covers:
  - a rename logging no drop, with the renamed name committed (TestCommitLogsNoDropForARenamedSession);
  - a rename after the pane's cwd, command, window name and environment moved on (TestCommitLogsNoDropForASessionRenamedAfterItsPaneMovedOn), the exact false-drop shape of the earlier attempt;
  - a drop beside an unrelated new session with a distinct pane id (TestCommitLogsADropBesideAnUnrelatedNewSession);
  - one-to-one pairing, where two identical saved records against one new one log exactly one drop (TestCommitTakesOneNewSessionAsTheRenameOfOneSavedSessionOnly);
  - criterion 2 through TestCommitLogsEachDroppedSessionByName and TestCommitLogsTheLastSessionDroppedByAnEmptyIndex.
  internal/state/commit_rename_realtmux_test.go runs the rename against real tmux on a disposable socket, after the pane has moved on. It then kills a session and creates an unrelated one, asserting the kill is still logged. Each core branch has a test that fails if it breaks: an always-false match, an always-true match, a missing one-to-one consumption, and a whole-record identity.
- Notes: Tests are focused, with no redundant assertions. The real-tmux test is a fast client test on a `tmuxtest` socket, which the unit lane permits.

CODE QUALITY:
- Project conventions: Followed (attr key `session` from the closed vocabulary, INFO level, logger via loggerOrDiscard, no new component)
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (slices.IndexFunc / slices.Delete / slices.EqualFunc)
- Readability: Good
- Issues: None. Both comments (internal/state/commit.go:24-27 and :110-113) hold against the code, including the narrowing to "compare equal only when they share their windows", which correctly accounts for tmux session groups.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
