TASK: lazy-resume-on-attach-6-6 — The Picker Resolves Pending State From One Read

ACCEPTANCE CRITERIA:
- A list load takes exactly one pending read, in the same command as the session enumeration, and the delegate renders from that set.
- A refresh after a kill, after a rename and on preview dismissal each re-read both together; a session that stopped being pending between loads loses its dot on the next list.
- A pending read that fails renders no dots, renders the session list normally, does not quit the picker and does not block the first paint.
- An unwired seam marks nothing and panics on nothing.
- A grouped rebuild — the `s` cycle, a `ProjectsLoadedMsg`, a filter — re-renders from the cached set and issues no second read.
- A multi-tag session marked pending shows its dot on each of its By-Tag rows, because the keying is on the session name.
- Flat mode, the filter and the multi-select marks are unaffected by the set's presence or absence.
- No picker test reaches a real tmux server: the seam is injected wherever a test Executes the open body, and a failed read is the degradation rather than a failure.

STATUS: complete

SPEC CONTEXT: Spec section 8.3 gives the picker's session row a second (pending) dot, lit when any pane in the session is waiting, keyed per session rather than per pane. Section 8.2 fixes the marker as the pane option `@portal-resume-pending`, read in one whole-server enumeration (`tmux.ListPendingResumePanes`), so the picker costs one read per list load. The plan context adds the design rule: pending state can change under an open picker, so it rides every session-list load rather than a once-per-picker cache. A failed read costs the dots, never the picker, and `rebuildSessionList` must not issue the read.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Seam + option + read helper: internal/tui/pending_resume.go:5-35 (`PendingResumeReader`, `WithPendingResumeReader`, `readSessionList` reading sessions then pending and returning the session error only, `readPendingSessions` returning nil for a nil seam or a failed read)
  - Deps wiring: internal/tui/build.go:32 (`Deps.PendingReader`), :138-140 (option applied, nil-guarded)
  - Production wiring: cmd/open.go:514 (tuiConfig field), :589 (passed to tui.Deps), :723 (`pendingReader: client`, the same *tmux.Client the lister reads through)
  - Message carriage: internal/tui/model.go:114-120 (`SessionsMsg.Pending`), internal/tui/pagepreview.go:296-301 (`previewSessionsRefreshedMsg.Pending`)
  - Fetch sites routed through the helper: model.go:1388 (refreshSessionsAfterPreviewCmd), :1494 (fetchSessionsCmd), :2833 (killAndRefresh), :2896 (renameAndRefresh). The post-restore refetch (model.go:1507, added later) also goes through it. `readSessionList` is now the only `ListSessions()` caller in the package (pending_resume.go:19)
  - Chokepoint: model.go:1158-1172. `applySessions` takes the set and replaces `m.pendingSessions` wholesale next to `m.sessions` / `m.derivedDirs`, via `applyPendingSessions`, which re-points the delegate. `m.sessions` is written only at model.go:1159
  - Delegate: model.go:951 (`Pending: m.pendingSessions` next to `Selected` / `GoneFlagged`). The row renders via `isSelected(d.Pending, it.Session.Name)` at internal/tui/session_item.go:323
  - Update arms: model.go:1643 (SessionsMsg) and :1787 (previewSessionsRefreshedMsg) pass `msg.Pending`. `SessionsMsg.Err` still quits, but it can never carry the pending error
- Notes: Skipping the pending read when the session read fails is sound: that error quits the picker (SessionsMsg) or keeps the old list (preview refresh), so no set would be used. The helper is split into two small functions rather than one; that is equivalent to the plan. `rebuildSessionList` (model.go:1249-1284) does no pending read. No drift.

TESTS:
- Status: Adequate
- Coverage: internal/tui/session_list_pending_read_test.go covers all eight planned tests plus one extra:
  - first load through `Build` (wiring included), with the read order checked as exactly [sessions, pending] inside the command (:140-160)
  - kill, rename and preview-dismissal table, checking both reads counted twice and the dots from the second set (:162-217)
  - a dot dropped between loads, which catches both merging and a stale delegate map (:223-233)
  - a failed pending read on the list-load and preview routes: no quit (via `loadSessions`), rows rendered, no dots (:235-262)
  - the pending error kept out of Err on all four routes (:264-299)
  - an unwired seam on all four routes plus render (:301-323)
  - grouped rebuild over the `s` cycle, `ProjectsLoadedMsg` and a typed filter, with every follow-up command drained, so a hidden re-fetch would be counted (:325-361)
  - a multi-tag By-Tag session showing its dot on both rows (:363-382)
  - multi-select marks untouched by the set (:384-397)
  The 36 existing `applySessions` call sites in the tui suites pass nil. The cmd test config (`defaultTestTUIConfig`, cmd/open_test.go:1605-1614) leaves the seam nil. `pendingReader` is the same client as the lister, so any test that injects a commander-backed client injects both.
- Notes: Focused, not bloated; each test would fail if its behaviour broke.

CODE QUALITY:
- Project conventions: Followed (small single-method seam, nil-tolerant option applied by Build, one chokepoint for session state, and bare exported seams, which match their neighbours in model.go)
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (min builtin, strings.SplitSeq/FieldsSeq in the test helpers)
- Readability: Good; comments on SessionsMsg, the model field and readSessionList match the code
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
