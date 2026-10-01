TASK: A Waiting Pane's Transcript Survives a Capture That Misses Its Session (tick-7f8dbc, lazy-resume-on-attach-18-1)

ACCEPTANCE CRITERIA:
- A committing cycle's capture misses `foo` in one of three ways: (a) the session-name read returns `foo` but the enumeration lists X, still pending, under `bar`, and `foo`'s environment read answers no-such-session; (b) the enumeration lists X under `foo` and `foo`'s environment read answers no-such-session; (c) another session captures normally and `foo`'s environment read fails with some other error. After the commit, `pane-T.bin` is on disk holding the bytes it held before, and `sessions.json` holds `foo` as the previous index had it, with X's record naming `pane-T.bin`. A daemon tick and `portal state commit-now` both leave this state.
- Continuing from any of those, the next committing cycle reaches X's session (as `bar` after (a)/(b), as `foo` after (c)) and commits X's record at its live address naming `pane-T.bin`, with the file still on disk holding the same bytes. After (a)/(b), `foo` is no longer in `sessions.json`.
- A daemon tick that carries `foo` after (c) captures no scrollback from X and writes no scrollback file for it.
- `foo` killed after the enumeration listed X: that cycle commits `foo` carried forward; the next cycle, whose enumeration no longer lists X, commits an index without `foo`.
- Nothing is carried for a pane the same enumeration does not list with the pending marker. A session killed before the enumeration, and a missed session holding no waiting pane, are both left out of the committed index.
- When a carried session holds a record whose token a pane in the fresh capture also carries, the committed index holds that token on one record only.
- A carry landing on the name of a session that reached the capture commits nothing, leaving `sessions.json` and the scrollback directory unchanged: the daemon's tick logs its `tick failed` WARN and re-touches `save.requested`; `commit-now` exits non-zero through its failure route, touching `save.requested`. The next cycle in which X's session reaches the capture commits X's record naming `pane-T.bin`.
- Where no waiting pane's session misses the capture, every committing cycle commits what it commits today.

STATUS: issues_found

SPEC CONTEXT: Section 7.2 ("A waiting pane whose session the capture misses keeps its place too") states the rule exactly as built: when the capture's one enumeration lists a live waiting pane whose token no fresh record carries but a previous record does, the previous session holding that record is carried whole, under its previous name, for that cycle. No token lands on a second record. The carried copy may hold a sibling that has since closed or moved. A carry onto a reached session's name refuses the commit. Section 7.3 adds the third scrollback-skip condition: every pane of a carried session. Section 7.2 also holds that a waiting pane is restored with its original content however many reboots it waits through. Because a frozen pane has no second source, losing the token-named file is permanent.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/capture.go:83-153 (captureStructure: per-session loop unchanged; carry runs after both merges at :144-149)
  - internal/state/capture.go:155-167 (waitingTokens — every non-internal row of the one enumeration, including names the session-name read did not return)
  - internal/state/capture.go:169-215 (carryMissedWaitingSessions — canonical-order token resolution, `placed` = fresh tokens after the merges, name-collision refusal via errCarryNameTaken at :195-199)
  - internal/state/capture.go:221-246 (carriedSession — deep-enough copy, strips any token already placed, registers kept tokens)
  - internal/state/capture.go:471-493 (parsePaneRows no longer filters to the session-name read)
  - internal/state/scrollback.go:256-283 (CaptureCycle.Carried + SkipsScrollback third condition), :293-306 (captureAndRefile threads Carried)
  - cmd/state_daemon.go:200-206, :272 (existing failure route; PrevIndex takes the carried index); cmd/state_commit_now.go:114-125 (failCommitNow route)
- Notes:
  - No second tmux read. The rule sits in the shared capture, so the daemon tick, the shutdown flush (cmd/state_daemon.go:373) and commit-now all take it.
  - A refused carry returns the empty capture before the link and re-file run (scrollback.go:298-302), so nothing on disk moves.
  - When nothing is carried, the carry returns an empty set and leaves the fresh index untouched. The extra groups parsePaneRows now returns are read only by waitingTokens. That settles the "commits what it commits today" criterion by reading.
  - One gap, listed under FINDINGS: a carried waiting record bypasses the re-file/adoption step that the merge path relies on. A stale previous index therefore still loses the transcript in a compound race.

TESTS:
- Status: Adequate
- Coverage:
  - internal/state/capture_carry_test.go:167-214 covers scenarios (a), (b) and (c), each under both committers. It checks file bytes, foo DeepEqual to the previous index, then the next cycle's live-address record and foo dropping out.
  - :216-232 checks the Carried/Pending sets for (c).
  - :234-251 covers killed mid-capture, then dropped.
  - :253-277 has five no-carry worlds, including the internal-session row.
  - :279-318 checks that a stripped token leaves one holder per token.
  - :320-361 checks that a collision commits nothing, leaves files untouched, and that the settled next cycle files X.
  - :363-379 checks that a refused capture returns no partial index.
  - cmd/state_carry_missed_session_test.go:83-121 covers a real daemon tick for (c): capture-pane targets only `=other:0.0`, the scrollback set is unchanged and foo is committed as before.
  - :123-155 checks the collision through the tick: `tick failed` WARN, save.requested re-touched, sessions.json and files unchanged, PrevIndex not replaced.
  - :157-187 covers commit-now carrying a renamed session.
  - :189-220 covers commit-now collision: errCommitNowFailed and save.requested touched.
  - Each test would fail if the carry broke: without it the committed index drops foo and the housekeeping pass deletes the seeded files.
- Notes: Focused, with no redundant cases. Nothing exercises a carry from a previous index whose waiting record predates a re-file. That is the gap in the finding below.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel, commandertest strict fake, seams staged through existing helpers, no process-artifact references in comments)
- SOLID principles: Good
- Complexity: Acceptable
- Modern idioms: Yes (maps.Clone, slices.Clone, strings.SplitSeq)
- Readability: Good
- Issues: None beyond the finding below

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [spreading] internal/state/scrollback.go:304 — The re-file pass runs only over `capture.Pending`. That set covers sessions the capture reached (capture.go:123, :152), so a carried waiting record (capture.go:221-246) goes into the index naming whatever path the previous index gave it. It never gets the adoption that `placeStoredScrollback` (scrollback.go:143-152) gives a merged waiting record whose positional source has already moved to `pane-<token>.bin`. The previous index can name that pre-re-file positional path in three ways:
  1. A `commit-now` re-files X and commits after the daemon's last tick. The daemon's carry reads its in-memory `deps.PrevIndex` (cmd/state_daemon.go:260, :272), not `sessions.json`.
  2. A tick is cancelled mid-dump after its re-file, and the shutdown flush runs on the old PrevIndex.
  3. A commit write fails after the re-file.

  Fix: route each carried record whose token is in the waiting set through the same re-file/adoption step, for example by returning those carried keys alongside `Pending` and passing their union to `refilePendingScrollback`. Pin it with a case where the previous index names X at its positional path, `pane-T.bin` is on disk, and foo misses the capture. — FAILS: if X's session misses that next capture (rename, kill or environment-read failure, the very race this task closes), the committed index names a positional file that no longer exists. Commit's housekeeping pass (commit.go:39, :77-111) then deletes `pane-T.bin`. The next cycle adopts a file that is gone, and X restores empty with `scrollback file not found`. That is the permanent loss the task exists to prevent, and it contradicts the task's outcome that after every commit the token-named transcript is on disk and named by `sessions.json`. It needs the base race to coincide with a stale previous index, so it is rare, but the merge path already guards this exact staleness and the carry path does not.

UNSETTLED:
- None
