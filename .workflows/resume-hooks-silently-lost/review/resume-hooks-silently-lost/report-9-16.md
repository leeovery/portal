TASK: resume-hooks-silently-lost-9-16 — `cmd/uninstall.go` substring-matches tmux stderr to detect an absent session (tick-b82094)

ACCEPTANCE CRITERIA:
- [x] `portal uninstall` treats a `KillSession` error wrapping `tmux.ErrNoSuchSession` as an absent saver and exits reporting a clean removal.
- [x] A `KillSession` error carrying "can't find session" stderr but classified as an unaddressable name is reported as a failure, and no removal is logged.
- [x] `cmd/uninstall.go` contains no `strings.Contains` over an error string.
- [x] An arbitrary non-absence kill failure still WARNs and returns the error.

STATUS: complete

SPEC CONTEXT: This is a phase-9 implementation-analysis task, so its authority is its own body rather than the specification (the spec mentions `portal uninstall` only once, at line 426, about `migrateRenameSubstring`, which is untouched here). The binding contract is the one stated in `internal/tmux/errors.go:11-15`: layers above `internal/tmux` must not substring-match tmux stderr, because tmux's phrasing is not a stable contract. The discrimination the task turns on is `wrapSessionTargetErr` (`internal/tmux/errors.go:118-127`), which checks the name **first**, so an unaddressable name wraps `ErrUnaddressableSessionName` and deliberately never reaches `wrapNoSuchSession` — even though tmux answers it with the very same "can't find session" stderr a vanished session produces.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/uninstall.go:94-101` (`killSaver`'s kill-error branch, now `errors.Is(err, tmux.ErrNoSuchSession)` at `:95`); the `isSessionAbsentError` helper and its substring-stability comment are deleted, as is the `strings` import (commit 66202dcd).
- Notes: The classification is reachable on the real path, not only in tests — `runCommand` (`internal/tmux/tmux.go:52-63`) always returns `WrapCommandError`, so every failed tmux invocation is a `*CommandError` with tmux's stderr attached, which is exactly what `wrapNoSuchSession` (`internal/tmux/errors.go:41-50`) requires to attach the sentinel, and `KillSession` (`internal/tmux/tmux.go:271-277`) routes its error through `wrapSessionTargetErr`. So there is no production shape where a genuinely absent saver loses the sentinel and gets reported as a failure. The two outcomes at `killSaver` stay distinct as the task required: absence logs `killSaverInfoMessage` and returns nil (`:96-97`), everything else WARNs and returns the error (`:99-100`). No `strings` reference remains anywhere in the file, and `isSessionAbsentError` has no remaining reference in the tree.

TESTS:
- Status: Adequate
- Coverage: `cmd/uninstall_test.go:342-405` (`TestUninstall_ClassifiesKillSessionFailureBySentinel`) carries the four sub-tests the task named verbatim, over the shared `killSaverOutcome` driver at `:321-340` that reports both what was logged and what was returned. The fixtures are load-bearing rather than decorative: the absent case (`:346`) injects a real `*tmux.CommandError` with "can't find session" stderr, which is the only shape that reaches the client's sentinel wrap; the unaddressable case (`:360`) puts that same stderr phrase in the message while wrapping `ErrUnaddressableSessionName`, so the deleted substring check would have passed it as a clean removal and the sentinel check reports it as a failure; the reworded case (`:395`) wraps the sentinel with stderr text that says "session vanished", so a re-introduced substring match would fail it. The two directions of the regression are therefore both pinned. Each sub-test asserts the log as well as the returned error, so "reported a removal it did not perform" is caught rather than only the exit status.
- Notes: The commit also removed `TestUninstall_ToleratesKillSessionCantFindSessionError` and the `setHookCalls` helper it used. That test asserted a second property beyond absence-tolerance — that hook removal still runs after a tolerated kill error. It is not directly restated, but `uninstallCmd`'s `RunE` (`cmd/uninstall.go:56-63`) calls `unregister` unconditionally with no branch on `killSaver`'s result, and `TestUninstall_KillSessionOtherFailureContributesJoinedErrorAndStillRunsUnregister` (`:230`) plus `TestUninstall_KillsPortalSaverBeforeRemovingHooks` (`:36`) already pin that sequencing on the failure and success paths, so a regression there would still fail. Not a coverage loss. Nothing here is over-tested — four sub-tests, four distinct classifications, one shared driver, no duplicated setup.

CODE QUALITY:
- Project conventions: Followed. Unit-lane test (no binary built, no daemon spawned, no real tmux), deps injected through `withUninstallDeps` via `installUninstallDeps` rather than assigning the seam directly, so `cmd/seam_guard_test.go` stays satisfied; the scripted commander is `internal/commandertest` rather than a local fake; the captured logger goes through `newCaptureLoggerForComponent` over a `logtest.Sink` rather than a hand-rolled `slog` handler, which the `internal/log` source guard requires. No new log component or attr key is introduced — the existing `daemon`-component borrow is unchanged.
- SOLID principles: Good. The discrimination stays in `internal/tmux`, where the stderr vocabulary lives; `cmd` now consumes only the sentinel.
- Complexity: Low. One predicate replaced by one `errors.Is`; a helper and an import removed.
- Modern idioms: Yes — sentinel-based `errors.Is` over string inspection is the idiomatic Go form and the one the package's own doc comment prescribes.
- Readability: Good. The surviving comment on `killSaver` (`:76-78`) still holds against the code — the probe discrimination it describes is intact at `:80-92`, and its "a session that auto-destroys between probe and kill is success" claim is exactly what `:95-98` now implements. The obsolete substring-stability comment went with the helper.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
