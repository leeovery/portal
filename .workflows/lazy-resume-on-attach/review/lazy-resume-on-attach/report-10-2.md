TASK: A Pane Restored Away From Its Saved Address Keeps Its Own Transcript (lazy-resume-on-attach-10-2, tick-d1af4d)

ACCEPTANCE CRITERIA:
- Saved windows 0, 2 and 3 of `work` restore as 0, 1 and 2: Y (token-shaped token, saved record names `scrollback/work__2.0.bin` holding Y's saved bytes) sits skeleton-marked at window 1, X (unregistered, unmarked, saved at window 3) sits at window 2. A saver tick with only Y skeleton-marked, then a tick with Y pending and its skeleton marker gone, leave `pane-<Y token>.bin` holding Y's saved bytes and `work__2.0.bin` holding X's capture; after each tick the committed index names `pane-<Y token>.bin` for Y and `work__2.0.bin` for X, with no two committed records naming one file
- A capture cycle over that moved, skeleton-marked Y, taken before any dump runs, returns an index naming `pane-<Y token>.bin` for Y, and both that file and `work__2.0.bin` (the path Y's hydrate helper was baked with) hold Y's saved bytes; `portal state commit-now` over the same shape commits Y's record naming `pane-<Y token>.bin`
- A tokened, skeleton-marked pane restored at its saved address, whose record names its own live key's positional file, keeps that path, and no token-named file appears for it
- A moved, skeleton-marked pane whose saved record already names its token-named file keeps that path and that file's bytes; `TestDaemonTick_KeepsARenumberedWaitingPaneTranscriptWhileSkeletonMarked` and `TestCaptureAndRefileKeepsARestoredWaitingPaneTranscript` stay green
- A moved, skeleton-marked pane carrying a token the pane-token rule refuses keeps the positional path its merged record names, and nothing is written under a token-derived name for it
- Once Y's skeleton marker clears with no pending marker set (eager), the next capturing tick captures Y at its live address and commits its record naming `scrollback/work__1.0.bin`, and the housekeeping pass removes `pane-<Y token>.bin`

STATUS: complete

SPEC CONTEXT: The spec's section on the file name being a second address: the saver's scrollback write derives its path from the pane's live address and never consults the record, so a record carrying a positional name another live pane now occupies silently collides, and `ComputeReferencedSet` cannot see it. A frozen pane's bytes leave the positional namespace for a token-derived name (`scrollback/pane-<token>.bin`), which no live address can produce. The corrigendum landed with this task (commit 3262f454a) moves the first exit earlier: when a mid-restore pane with a token-shaped token holds a record naming a positional file that another record in the same capture also names, the capture hard-links that file to the token name (refusing an existing name) and points the record there before any scrollback is written that cycle. It links rather than moves so a helper that has not yet opened its baked path still finds it. It acts only on a shared name so that a file only the pane's own record names stays referenced through that commit's housekeeping pass.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/scrollback.go:185-219 — `linkMovedSkeletonScrollback`: walks the skeleton set and reaches only records whose `ScrollbackFile` is named by 2+ records in the index.
  - internal/state/scrollback.go:221-231 — `recordsPerScrollbackFile`.
  - internal/state/scrollback.go:233-247 — `linkMovedPane`: pane-token rule via `PendingScrollbackPath`; skips a record already on its token path or on its own live key's file; `os.Link` through `placeStoredScrollback`; WARN and record left alone on failure.
  - internal/state/scrollback.go:143-152 — `placeStoredScrollback`: the former `renameStoredScrollback`, now parameterised over the placement, so the re-file keeps `moveNoClobber` and the link uses `os.Link`. A missing source or an existing token name is absorbed as the adopt path.
  - internal/state/scrollback.go:285 — the link runs inside `captureAndRefile`, after `CaptureStructure` and before `refilePendingScrollback`. So `RunCommitCycle` (internal/state/commit_cycle.go:58), and through it the daemon tick and commit-now, take it before any caller's `Dump`.
  - internal/state/paths.go:95 — `positionalScrollbackFile`; `buildPanes` routes through it (internal/state/capture.go:434).
- Notes:
  - Deliberate, sound divergence from the Do's reach condition. The Do reaches every tokened skeleton pane whose record names "a positional file other than its own live key's". The code adds one condition: another record in the index must also name that file. Linking with no occupant would take the positional file out of every record, and that same commit's `gcOrphanScrollback` would delete the baked path before a late hydrate helper opened it. That is the exact case the task chose a link over a move to protect. The narrowing still reaches every collision the dump can create. The dump walks only `capture.Index`, and every non-skipped pane's record names its own live key's file, so any file a dump writes that a skeleton record also names is counted twice before the dump runs (cmd/state_daemon.go:297-311). The spec corrigendum records the narrowed rule and CLAUDE.md's `state` row states it. Not a loss.
  - Known residual, accepted by the task's own derivation and not a finding here: a helper that opens its baked positional path after the occupant's atomic rename reads the occupant's bytes for this boot, although the record (and so the next reboot) keeps the pane's own transcript.
  - The link keeps the positional key's dedup entry. That is correct, since the name still holds those bytes until the occupant's differing write replaces it.
  - A later pending re-file finds the record already on its token path and returns (scrollback.go:126-128), as the Do requires.

TESTS:
- Status: Adequate
- Coverage:
  - AC1 — cmd/state_daemon_run_test.go:613 `TestDaemonTick_KeepsAMovedPaneTranscriptFromThePaneAtItsSavedAddress`, over the staged shape at :542 (saved 0/2/3, live 0/1/2, X unmarked at window 2, hash map seeded from disk). Tick 1 is skeleton-only and tick 2 is pending-only. After each tick it asserts the token file bytes, X's capture in `work__2.0.bin`, and via `assertRenumberedCommit` (:592) that no two committed records share a file and that each file is named by the right pane.
  - AC2 — internal/state/capture_refile_test.go:216 (index names the token file; both token and baked positional files hold the saved bytes; occupant present) and cmd/state_commit_now_test.go:1403 (commit-now commits Y's record on the token path and the occupant on its positional file, with the token file holding the saved bytes).
  - AC3 — capture_refile_test.go:237.
  - AC4 — capture_refile_test.go:305. The two named pre-existing tests are unchanged and store the token path, so neither the count guard nor the link reaches them.
  - AC5 — capture_refile_test.go:320 (the occupant is present, so the count guard passes and the pane-token rule is what refuses; asserts the scrollback directory holds only `work__2.0.bin`).
  - AC6 — cmd/state_daemon_run_test.go:643.
  - The narrowed reach is pinned by :249 (no occupant: the record keeps its positional path, and the file survives `state.Commit`) and :267 (the occupant is itself skeleton-marked and pending: no link, and the file survives commit).
  - The link-failure WARN branch (:339) is asserted for attrs and for record-left-alone.
  - Token-file adoption (:370).
- Notes: Removing the link fails the AC1/AC2/AC6 tests. Removing the count guard fails :249 and :267. Swapping `os.Link` for a move fails :216's baked-path assertion. Tests are focused, with no redundant assertions.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel`, the failure WARN reuses the re-file's attr keys (`pane_key`/`path`/`error`), the seam stays unexported with a test-only alias in export_test.go, and the helpers are small and single-purpose.
- SOLID principles: Good. `placeStoredScrollback` takes the placement as a function, so the re-file and the link share the missing-source and adopt-existing policy with no second copy.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The doc comments on `linkMovedSkeletonScrollback` and `captureAndRefile` state the shared-name condition, why it links rather than moves, and the failure path, and they hold against the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`TestDaemonTick_KeepsARenumberedWaitingPaneTranscriptWhileSkeletonMarked` and `TestCaptureAndRefileKeepsARestoredWaitingPaneTranscript` stay green" — requires a unit-lane run of `go test ./cmd ./internal/state`. Reading finds that neither test's path is reached by the change: each stores the token path, which a single record names.
