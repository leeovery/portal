TASK: The Commit Cycle Owns The Empty-Capture Confirmation Of Every Scrollback Write (killing-all-sessions-wipes-restore-state-3-1, tick-ecdc01)

ACCEPTANCE CRITERIA:
- A live pane has a saved non-empty transcript and its capture comes back empty; the read after that capture is refused, answered by another server on the socket, or answered naming no server. The daemon's tick and its shutdown flush each leave the transcript unchanged and each log one `empty capture not confirmed; saved transcript kept` line at WARN carrying `pane_key` and `error`.
- Same saved transcript, own server answering the read after the empty capture: tick and flush each write the empty capture, log no refusal line, and send exactly one confirmation read for that capture beside the cycle's own.
- A daemon scrollback write failing for any other reason logs `write scrollback failed` with `pane_key` and `error`, and no refusal line. A refused write and a failed write each count once in `tick complete`'s `anomalous` tally.
- Through the writer the cycle hands its dump, three captures are written without a confirmation read (non-empty over saved, empty with none saved, empty over empty saved); a capture matching the dedup entry writes nothing and reports no change.
- Through the same writer, an empty capture over an uninspectable saved transcript, confirmed by another server, writes nothing and returns an error matching `ErrUnconfirmedEmptyCapture` with `ErrNotOwnServer` reachable.
- A non-test file outside `internal/state` naming `state.WriteScrollbackIfChanged` (called, or as a value under an import alias) fails the unit-lane guard; test files, files inside `internal/state`, and a same-named selector on another package are not findings; the guard reports nothing over the current tree.

STATUS: complete

SPEC CONTEXT: §2.4 — the daemon's dump (tick and shutdown flush; commit-now dumps nothing) may replace a saved transcript with an empty capture only once the committer's own server answers a read sent after that capture; an unconfirmed empty capture is not written and the refusal is logged (§4.2, attrs `pane_key`/`error`). The 2026-10-06 corrigendum records the landed shape: the dump reaches `WriteScrollbackIfChanged` only through the `ScrollbackWriter` the commit cycle hands it, and the commit guard refuses `WriteScrollbackIfChanged` in any non-test file outside `internal/state`. §2.2 supplies the own-server confirmation rule (`confirmOwnServer`).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/commit_cycle.go:48 — `CommitCycle.Dump` is `func(CaptureCycle, ScrollbackWriter) (bool, error)`
  - internal/state/commit_cycle.go:52-95 — `ScrollbackWriter` (unexported fields) and `Write(paneKey, data, hash)`: `confirmEmptyCapture` (:87) runs before `WriteScrollbackIfChanged` (:90)
  - internal/state/commit_cycle.go:135-149 — `RunCommitCycle` binds the writer to `cycle.Client`, `cycle.OwnServer`, `cycle.Dir`, `cycle.HashMap`
  - internal/state/scrollback.go:288-303 — `confirmEmptyCapture` unexported; refusal wraps `ErrUnconfirmedEmptyCapture` and the `confirmOwnServer` cause
  - internal/state/scrollback.go:75-90 — `WriteScrollbackIfChanged` stays exported, documented as for test fixtures
  - cmd/state_daemon.go:342-353 — `dumpPane` makes one `writer.Write` call and tells the refusal line from `write scrollback failed` with `errors.Is(err, state.ErrUnconfirmedEmptyCapture)`; both increment `anomalous` once. `dumpPane` no longer reads `OwnServer`, `Dir` or `HashMap` from deps (it still uses `d.deps.Client` for `CaptureAndHashPane`, as planned)
  - internal/state/commit_guard_test.go:173-178 — `guardedStateNames` adds `WriteScrollbackIfChanged` beside `Commit`
- Notes: Both the tick (cmd/state_daemon.go:203) and the shutdown flush (cmd/state_daemon.go:389) go through `captureAndCommit` and so through the same writer. Grep over non-test `.go` files finds `WriteScrollbackIfChanged` / `confirmEmptyCapture` named only in internal/state/scrollback.go and internal/state/commit_cycle.go, so the guard is clean over the current tree. The only other production code outside `internal/state` that touches scrollback paths (internal/tui/preview_adapter.go) reads them. The writer has since gained skip-set and held-transcript behaviour from later tasks (5-1, 3-5, 7-x); that behaviour does not change this task's contract, and the current comments describe it accurately.

TESTS:
- Status: Adequate
- Coverage:
  - AC1: cmd/state_daemon_empty_capture_test.go:75-100 covers tick × flush × {refused, another server, no server}, checks the transcript is unchanged and asserts exactly one refusal line with keys `component, pane_key, error`. WARN level is pinned at :137.
  - AC2: :102-122 covers tick × flush: the empty capture is written, no refusal line, and `display-message` count == 2 (the cycle's confirmation plus the capture's).
  - AC3: :124-143 covers a refused write giving `anomalous` == 1 with no `write scrollback failed`. :145-172 covers a failed write (blocked by a directory at the transcript path) giving a WARN `write scrollback failed` with `pane_key`/`error`, no refusal line, and `anomalous` == 1, for tick and flush.
  - AC4: internal/state/empty_capture_test.go:93-123 checks that the three cases write with zero later confirmation reads. The later reads are answered by another server, so an extra confirmation would also turn into a refusal and fail the test. :125-138 covers the dedup hit.
  - AC5: :140-166 covers a chmod-000 scrollback dir with another server answering: no write, `ErrUnconfirmedEmptyCapture` and `ErrNotOwnServer` both reachable, exactly one later read, and the transcript intact.
  - AC6: internal/state/commit_guard_test.go:81-109 has the call and alias-value cases (findings), test files outside state, a production file inside state and a selector on an unrelated package (no findings). `TestNoProductionCommitOutsideState` scans the real tree.
- Notes: Each test would fail if the confirmation were dropped, reordered after the write, or bound to the wrong own-server pid. AC2's write-on-own-server and AC5's refuse-on-another-server together pin the binding. The anomalous-tally test repeats the refusal-line assertion, but it is what pins the line at exactly WARN, so the overlap is not bloat.

CODE QUALITY:
- Project conventions: Followed. The guard extends the existing `state.Commit` precedent in the same table shape. No `t.Parallel`. Logging uses the existing `daemon` component and closed attr keys.
- SOLID principles: Good. The cycle owns the confirmation rule. The dump depends on one narrow write operation instead of four inputs it re-plumbed before.
- Complexity: Low
- Modern idioms: Yes (`new(expr)` under go 1.26, `%w: %w` multi-wrap, `slices`/`maps` in the guard)
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
