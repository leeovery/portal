TASK: Each Commit Cycle Merges And Carries From The Index It Read Under The Lock (killing-all-sessions-wipes-restore-state-6-2, tick-adfe17)

ACCEPTANCE CRITERIA:
1. Panes X and Y wait in session S. A commit-now files X under its token-named transcript, its housekeeping removes X's positional file, and the user then answers X. The daemon's in-memory previous index predates that commit-now and names X at its positional path. On the daemon's next cycle, S misses the capture (renamed mid-capture, or its environment read failing) while Y still waits. The daemon's tick and its shutdown flush each carry S forward with X's record naming its token-named transcript, commit it, and leave that file on disk with its bytes. (§2.4, §1.1)
2. sessions.json holds a commit-now's commit and the daemon's in-memory previous index predates it. The daemon's next cycle commits the same index, and leaves the same files on disk, as a cycle whose in-memory index matches sessions.json, for skeleton-marked, waiting and carried panes alike. (§2.4)
3. A cycle over a sessions.json that reads and decodes under the lock never calls the caller's LoadPrev. (§2.4)
4. With sessions.json absent or not decodable, the cycle calls LoadPrev once, under the lock; the skeleton merge, waiting-pane merge, carry and answered-pane hold take their records from that index; commit-now logs the absent / unreadable WARN. (§2.4)
5. The lag repairs stay: linkStoredScrollback still adopts a missing source or an existing token-named file; a cycle that linked and ended uncommitted still leaves sessions.json naming only files on disk, and the existing tests for those ends pass unchanged. (§2.3)

STATUS: complete

SPEC CONTEXT: §1.1 — every session not killed stays restorable with its scrollback intact. §2.4 (with its 2026-10-06 corrigenda) — a pane's saved transcript is the file its last committed record names; from a lazy pane's answer until a dump writes its confirmed capture, every commit names the token-named transcript and leaves it on disk. §2.3 — a cycle ending uncommitted never leaves sessions.json naming a missing file. §4.1 already measures drops against the on-disk index rather than the daemon's in-memory one, for the same lag reason this task extends to the merges and the carry.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/state/commit_cycle.go:121-133 (`prev := committed`; `cycle.LoadPrev()` only when `committed == nil`; `captureAndRefile` and `keepAnsweredTranscripts` both take that one `prev`); contract docs at internal/state/commit_cycle.go:39-42 (CommitCycle.LoadPrev) and :104-108 (RunCommitCycle).
- Notes:
  - The change is the minimal one the task prescribed: the index read under the lock feeds the skeleton merge, waiting-pane merge, carry and hold, and LoadPrev is the fallback. `cmp.Or(committed, prev)` and the `cmp` import are gone.
  - `committed` and `prev` are now the same pointer, and `commitOver` still measures the no-change test and the drop log against `committed` (commit_cycle.go:150). I checked whether the merges could write through `prev` into `committed`. They cannot. `carriedSession` clones Environment and every Panes slice (internal/state/capture.go:245-270). The skeleton and waiting merges copy `Pane` values, and `Pane` holds only scalars (internal/state/schema.go:39-46). `keepAnsweredTranscripts` takes the index by value and reads it through `indexPrevPanes`, so the no-change comparison stays sound.
  - The daemon's in-memory index is replaced only on a successful cycle (cmd/state_daemon.go:283), and commit-now's fallback is the empty index (cmd/state_commit_now.go:114), so the fallback paths behave as they did before.
  - Later sanctioned work changed parts of the plan text for criterion 4 and the Do list. Task 8-1 moved the unreadable-index WARN out of commit-now and into the cycle (`logUnreadIndex`, commit_cycle.go:156-163), and reworded it to "…; committing without the saved index". Task 6-3 reduced commit-now's LoadPrev to `&state.Index{}`. Both are deliberate follow-ons. The code is the record, so neither divergence is a finding against this task.
  - CLAUDE.md's `state` row matches the shipped contract: the cycle's previous index is sessions.json read under the lock, and LoadPrev is the fallback.

TESTS:
- Status: Adequate
- Coverage:
  - Criterion 1 is covered at both layers.
    - internal/state/commit_cycle_prev_test.go:92-107 (`TestDaemonCycleCarriesAnAnsweredPaneFromTheIndexACommitNowLeft`) covers a rename landing after the pane read and an environment read failing. It asserts X is in the carried set and names `pane-<token>.bin` with its bytes, and that every file sessions.json names is on disk.
    - cmd/state_commit_prev_lag_test.go:85-140 (`TestDaemonCarriesAnAnsweredPaneFromTheIndexACommitNowLeft`) drives the real `portal state commit-now`, then the daemon's `captureAndCommit` and `defaultShutdownFlush` with `deps.PrevIndex` set to the lagging index. It asserts the record, the bytes, the other session's committed capture, `flush_completed=true` and every saved file on disk.
    - Under the old code both tests would fail: the carry would copy X's positional record, and gcOrphanScrollback would delete `pane-<token>.bin`.
  - Criterion 2: commit_cycle_prev_test.go:116-154 runs the same world against a lagging-index directory and a current-index directory and compares the committed index (SavedAt ignored) and the scrollback contents. The worlds cover a skeleton-marked pane, a waiting pane beside an answered one, both panes waiting, and both carried shapes. The old code would diverge on at least the skeleton-marked and carried rows.
  - Criterion 3:
    - commit_cycle_prev_test.go:156-185 asserts zero LoadPrev calls over a readable sessions.json.
    - The rewritten serialisation subtest (internal/state/commit_cycle_test.go:328-375) asserts that the second commit-now loads no index of its own and logs no repeat drop.
    - The `readable` row of cmd/state_commit_prev_lag_test.go:142-197 asserts no WARN.
  - Criterion 4: commit_cycle_prev_test.go:203-263 covers {absent, not decodable} × {carry, waiting-pane merge, skeleton merge, answered-pane hold}. Each case asserts exactly one LoadPrev call, made while a second open file description is refused the commit lock (`commitLockHeldElsewhere`). It also asserts X's committed CWD and command, which shows each site took its record from LoadPrev's index rather than the live pane, or the live pane's record for the hold. The cmd test asserts each WARN fires exactly once, with its current wording.
  - Criterion 5: `linkStoredScrollback` (internal/state/scrollback.go:148-157) is byte-identical to the pre-task version. internal/state/commit_cycle_uncommitted_test.go, cmd/state_daemon_cancelled_refile_test.go and internal/state/scrollback.go have no commits from this task onward.
- Notes:
  - Mild overlap only. The "commit-now" row of `TestRunCommitCycleReadsNoCallerIndexOverAReadableSessionsJSON` exercises nothing the "daemon tick" row does not, since LoadPrev is never reached. Nothing fails because of it.
  - The state-level and cmd-level carry tests overlap by design: the state test covers both carry shapes, and the cmd test proves the daemon tick and the flush are wired to the cycle.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel, deps staged through withCommitNowDeps/withOwnTmuxServer, logtest.Sink queries composed from Records(), test name style matches siblings)
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good — the three-line fallback states the contract directly; the doc comments on CommitCycle.LoadPrev and RunCommitCycle hold true against the code
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "A cycle that linked and then ended uncommitted (stood down, cancelled mid-dump, or with a failed `sessions.json` write) still leaves `sessions.json` naming only files on disk, and the existing tests for those ends pass unchanged." — the "unchanged" half is settled by reading (no commits to those test files or to scrollback.go since before this task); that they pass needs a run of `go test ./internal/state -run 'Uncommitted|StoodDown|Cancelled'` and `go test ./cmd -run 'Cancelled|StoodDown'` on the current tree.
