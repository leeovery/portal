TASK: N-Key New Session in CWD (tick-01c27d)

ACCEPTANCE CRITERIA:
- n key creates a session in the current working directory immediately
- Session creation uses CreateFromDir(cwd, nil) -- command forwarding is added in Phase 3 task 3-5
- On success, Selected() returns the new session name and TUI quits
- On error (sessionCreateErrMsg), TUI handles gracefully (no crash)
- n with no session creator configured is a no-op
- The "[n] new in project..." option and divider are removed from the session list

STATUS: Complete

SPEC CONTEXT: The spec says: "n immediately creates a session in the current working directory and attaches. No confirmation, no cursor movement -- equivalent to portal open . / x ." Works from both pages, no spinner. The cwd field on Model is populated via WithCWD option, set from os.Getwd() in cmd/open.go.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/internal/tui/model.go:116 -- `cwd` field on Model
  - /Users/leeovery/Code/portal/internal/tui/model.go:324-328 -- `WithCWD(path string) Option`
  - /Users/leeovery/Code/portal/internal/tui/model.go:1080-1085 -- `handleNewInCWD()` guards on nil sessionCreator
  - /Users/leeovery/Code/portal/internal/tui/model.go:1087-1089 -- `createSessionInCWD()` delegates to `createSession(m.cwd)`
  - /Users/leeovery/Code/portal/internal/tui/model.go:602-610 -- `createSession(dir)` calls `CreateFromDir(dir, m.command)`, returns SessionCreatedMsg or sessionCreateErrMsg
  - /Users/leeovery/Code/portal/internal/tui/model.go:943 -- n key handled in updateSessionList
  - /Users/leeovery/Code/portal/internal/tui/model.go:657 -- n key handled in updateProjectsPage
  - /Users/leeovery/Code/portal/internal/tui/model.go:583-586 -- SessionCreatedMsg handler sets selected and quits
  - /Users/leeovery/Code/portal/internal/tui/model.go:587-588 -- sessionCreateErrMsg handler returns to current page
  - /Users/leeovery/Code/portal/cmd/open.go:294 -- `tui.WithCWD(cfg.cwd)` passed in production
- Notes: Implementation uses a shared `createSession(dir)` method that passes `m.command`. In normal mode `m.command` is nil, which satisfies the acceptance criteria of calling `CreateFromDir(cwd, nil)`. This shared method also naturally supports command-pending mode (Phase 3 task 3-5) without additional changes. No "[n] new in project..." text or totalItems() method or divider rendering found anywhere in the codebase -- confirmed fully removed. The `n` key is wired on both the sessions page (line 943) and the projects page (line 657), matching the spec requirement that n works from both pages.

TESTS:
- Status: Adequate
- Coverage:
  - "n creates session in cwd and quits" (line 575) -- verifies CreateFromDir called with correct cwd, nil command, SessionCreatedMsg triggers Selected() and quit
  - "n with no session creator is no-op" (line 626) -- verifies nil command returned when no sessionCreator
  - "session creation error is handled gracefully" (line 645) -- verifies error returns to session list, Selected() empty, view still renders
  - "n from empty session list creates session in cwd" (line 686) -- extra edge case: n works even with no sessions in list
  - "createSessionInCWD delegates to createSession with cwd" (line 3482) -- verifies the delegation chain
  - "[n] new in project option" removal: confirmed by grep -- no references to "new in project" text remain anywhere in model.go or model_test.go
- Notes: All four planned tests are present plus two additional useful coverage cases. Tests verify behavior (command output, message types, Selected() value) rather than implementation details. The error path test confirms the TUI remains functional after error. No over-testing observed -- each test covers a distinct scenario.

CODE QUALITY:
- Project conventions: Followed -- functional options pattern (WithCWD), interface-based dependency injection (SessionCreator), mock-based testing
- SOLID principles: Good -- handleNewInCWD has single responsibility (guard + delegate), createSessionInCWD delegates to createSession (DRY), SessionCreator interface is minimal (one method)
- Complexity: Low -- handleNewInCWD is a simple nil guard + delegate, createSessionInCWD is a one-liner
- Modern idioms: Yes -- functional options, interface-based DI, Bubble Tea command pattern
- Readability: Good -- method names clearly express intent, guard clause pattern is consistent with other handlers (handleKillKey, handleRenameKey)
- Issues: None

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- The "session list no longer shows new in project option" test from the task plan is implicitly verified by the absence of such text in the codebase rather than an explicit test assertion. This is acceptable since the old rendering code was completely removed (not just hidden behind a flag), so there is nothing to regress to.
