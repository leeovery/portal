TASK: lazy-resume-on-attach-2-2 — The Per-Pane Capture Read Carries the Pending Marker

ACCEPTANCE CRITERIA:
- A pane row whose twelfth field is `1` puts that pane's live pane key in the returned set; a row whose twelfth field is empty does not.
- The pending set is keyed exactly as the daemon's own per-pane loop keys a pane — `SanitizePaneKey(session, window index, pane index)` — for panes beyond the first window and the first pane.
- A row at the old eleven-field arity fails the parse with the existing `unexpected pane row field count` error and an empty `Sessions`, as does a thirteen-field row; twelve fields parse.
- Panes of a session that vanished between the enumeration and its `show-environment` read, or that failed anomalously, are absent from the pending set as they are from the index.
- Every early return (`ListSessionNames` failure, `ListAllPanesWithFormat` failure, parse failure, all-sessions-failed) returns an empty non-nil set alongside the empty index and the error.
- `sessions.json` written from an index whose panes were pending carries no pending field, and `SchemaVersion` is still 1 with no migration.
- `portal state commit-now` behaves exactly as before: it discards the second value, still passes a nil skip set, and still commits with `anyScrollbackChanged=false`.
- The daemon's tick summary, its per-pane `pane captured` DEBUG and every existing capture test pass unchanged against twelve-field fixtures.

STATUS: complete

SPEC CONTEXT: Spec 7.3 has the saver read the pane-scoped `@portal-resume-pending` marker as one more column of the per-tick `list-panes -a -F` capture read rather than making a second tmux call; `captureFieldCount` moves 11 -> 12, the same contained move the pane token made. Spec 7.3 and 8.2 make the marker's value `1`, and its presence is what gets read. Spec 9.2 says the pending state is never persisted: nothing reaches `sessions.json` and no schema version moves. The plan returns the pending set beside the index instead of storing it on `Pane`. A non-persisted field would be compared against an index decoded from disk, so every tick would read as a structural change.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/capture.go:26 — `captureFormat` gains the twelfth column, built from `ResumePendingOption`
  - internal/state/capture.go:28 — `captureFieldCount = 12`
  - internal/state/capture.go:378, :428 — `paneRow.resumePending`, read through `ResumePendingSet(parts[11])`
  - internal/state/capture.go:48-116 — the new three-value signature. `emptyPending` (non-nil) comes back on each of the four early returns (:56, :65, :69, :98). `addPendingPanes` (:94, :126-136) runs only after a session's `ShowEnvironment` succeeded and the session was appended, so vanished and anomalously failed sessions add nothing. The set is keyed `SanitizePaneKey(session, r.windowIdx, r.paneIdx)` at the live address. `paneKeySet` (:138-144) always returns a non-nil map.
  - internal/state/schema.go:11, :39-46 — `SchemaVersion` is still 1, and `Pane` has no pending field
  - cmd/state_daemon.go:249 — binds `pendingSet` and uses it through `paneSkipsScrollback` (:273, :322-328)
  - cmd/state_commit_now.go:36, :121 — the seam type carries the extra return. The call discards it, passes a nil skipSet, and commits with `false` (:126).
  - CLAUDE.md `state` row — names the twelfth column, `captureFieldCount = 12`, and the second return that is not stored on the record
- Notes: The code has moved past the task's wording in a sound way. Later tasks put the capture behind `state.CaptureAndRefile` (internal/state/scrollback.go:166), and the daemon and commit-now now call that wrapper instead of `CaptureStructure`. The commit-now seam was renamed to `CaptureAndRefile` to match. Every property this task required still holds through the wrapper: it passes the pending set through unchanged and returns it untouched on a failed capture. No criterion is lost.

TESTS:
- Status: Adequate
- Coverage:
  - Every named test is in internal/state/capture_test.go under `TestCaptureStructureResumePending` (:1691-1901):
    - a marked pane lands in the set (:1692)
    - a never-marked pane (:1708) and an empty pending column (:1723) read as unmarked
    - the set is keyed by the live pane key at window 2 / pane 3, beside an unmarked 0.0 row (:1738)
    - vanished (:1757) and anomalously failed (:1780) sessions are left out
    - a four-case table checks the non-nil empty set on every early return (:1803)
    - a pending index committed to sessions.json carries no pending key and version stays 1 (:1852)
  - The arity table (:593-619) rejects eleven and thirteen fields with the existing message and empty `Sessions`. The twelve-field case is exercised by every other test through `paneLine`/`paneLineWithPending` (:83-102).
  - `TestStateCommitNow_DiscardsThePendingSet` (cmd/state_commit_now_test.go:1142) checks the nil skipSet, `anyScrollbackChanged=false`, and a committed index equal to the captured one.
  - Every literal pane-row fixture under cmd/ and internal/ now has twelve fields. The 10-field rows in internal/tmux/tmux_test.go:1410-1419 are an unrelated pass-through test of `ListAllPanesWithFormat` and never reach the parser.
  - In the task commit, the daemon tests (run, capture-logging, cycle-summary, self-supervision) changed only their fixture rows and one fixture comment; no assertion changed.
- Notes: The "never marked" and "empty column" cases feed the parser the same byte (empty). The spec's edge case says tmux cannot tell them apart, so this is the plan's own stated test pair, not redundancy worth raising.

CODE QUALITY:
- Project conventions: Followed. The option name is composed from `ResumePendingOption`, not restated. Presence goes through the single `ResumePendingSet` rule, and nothing parses tmux message text.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (`strings.SplitSeq`, generic-free map sets matching the existing `skipSet` shape)
- Readability: Good. The `CaptureStructure` doc comment (:32-47) is accurate: the set is non-nil on every return, only holds sessions that reached the Index, and is never persisted.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "The daemon's tick summary, its per-pane `pane captured` DEBUG and every existing capture test pass unchanged against twelve-field fixtures." — Reading settles that every fixture carries twelve fields and that the task changed no assertion. Whether the tests actually pass needs a run of `go test ./cmd ./internal/state`, plus the integration lane (`go test -tags integration -p 1 ./cmd/... ./internal/restore`) for the integration-tagged files the task touched.
