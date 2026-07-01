TASK: 3-2 — Extend capture to read #{@portal-id} into Session.PortalID (append column, bump captureFieldCount 10->11, parser lockstep) [tick-f19ccd]

ACCEPTANCE CRITERIA:
- captureFormat ends with |||#{@portal-id} and has exactly 11 |||-separated fields; fields 0-9 byte-identical (append-only).
- captureFieldCount == 11.
- paneRow has portalID string, populated from parts[10].
- Stamped session captures Session.PortalID == '<id>'.
- Un-stamped session captures Session.PortalID == "".
- PortalID lifted from the FIRST row of grouped[name]; multi-pane same-id yields that id once.
- Wrong-arity row (10 or 12 fields) rejected with 'unexpected pane row field count %d'.
- Zero-row session yields PortalID == "" and empty Windows, not specially guarded (rejected downstream); no panic on the empty lift.
- go build succeeds; go test ./internal/state/... passes.

STATUS: Complete

SPEC CONTEXT:
Spec § Cross-Reboot Persistence → 2. Capture. tmux user-options are in-memory and do not survive a reboot; sessions.json is the only durable record of a session's immutable @portal-id across a reboot. For the headline case (session renamed then rebooted) the saved Name is the post-rename name, so restore must recover the id from the snapshot to bake the matching --hook-key. captureFormat is fixed-arity paired with captureFieldCount; the field MUST be appended as the last column so every existing index is unchanged, and the count + parser reads must move in lockstep or every row is rejected/mis-slotted. The opaque token is alphanumeric so it cannot contain the ||| delimiter — trailing position is delimiter-safe. Zero-row session (natural churn between name enumeration and pane read) yields "" and empty Windows; benign because Restore already rejects a windowless entry, so no capture-time guard is added.

IMPLEMENTATION:
- Status: Implemented (append-only, in lockstep)
- Location:
  - internal/state/capture.go:42 — captureFormat appends |||#{@portal-id} as the 11th column; fields 0-9 byte-identical (confirmed against git show 8bccbeca: only the trailing field added).
  - internal/state/capture.go:44 — captureFieldCount = 11 (was 10).
  - internal/state/capture.go:31-41 — doc-comment updated: trailing @portal-id, session-scoped, resolves per-pane, alphanumeric token cannot contain "|||", un-stamped resolves to "".
  - internal/state/capture.go:347-349 — paneRow.portalID field added with session-scoped doc-note (first-row consumption).
  - internal/state/capture.go:403 — parsePaneRow reads portalID: parts[10]; parts[0]..parts[9] unchanged.
  - internal/state/capture.go:136-145 — Session assembly lifts PortalID from the first row of grouped[name], guarded: portalID := ""; if rows := grouped[name]; len(rows) > 0 { portalID = rows[0].portalID }.
  - The existing len(parts) != captureFieldCount guard at capture.go:381 now enforces arity 11 automatically (no code change needed — the const bump does the work). parsePaneRows returns the parse error, and CaptureStructure at :111-114 propagates it as a pre-loop fail-fatal returning the empty index.
  - findOrAppendSession (capture.go:260-265) also copies PortalID into the merge-append branch (added by follow-up commit 0a3ee38e / T-analysis-1-1) — closes the erase-on-merge gap; covered by capture_internal_test.go.
- Notes: No zero-row guard/skip added, exactly as the spec mandates — the empty entry falls through to Restore's rejection. Correct.

TESTS:
- Status: Adequate
- Coverage:
  - internal/state/capture_test.go:533-707 (TestCaptureStructurePortalID) covers all six planned test names:
    - stamped → PortalID == "<id>" (:534)
    - un-stamped → PortalID == "" (:556)
    - multi-pane lift once from first row (:578) — three rows across two windows, all carrying the same id
    - wrong-arity 10-field row rejected with 'unexpected pane row field count' + empty Sessions (:606)
    - every existing field index unchanged after append — full Session/Window/Pane shape asserted with a stamped id proving no bleed into earlier fields (:631)
    - zero-row session → "" + empty Windows, no panic (:679)
  - Test helper paneLineWithID (:98) formats the 11-field row; paneLine (:90) delegates with an empty id — the whole legacy suite now exercises the un-stamped-column shape, giving broad append-only regression coverage for free.
  - capture_internal_test.go:20 (TestFindOrAppendSessionCopiesPortalID) white-box-pins the merge-append branch copying PortalID (the follow-up gap).
  - cmd/ daemon fixtures (state_daemon_run_test.go et al.) updated append-only with a trailing ||| blank column — required lockstep maintenance so the shared captureFormat's arity-11 rows are not rejected in the daemon tests.
- Notes: Tests verify behaviour, not implementation detail. The wrong-arity test uses only the 10-field (short) case; the plan's acceptance also names a 12-field (long) case, but both are rejected by the identical len(parts) != captureFieldCount comparison, so the 12-field variant would exercise no distinct code path — its omission is not under-testing. No t.Parallel() used (conforms to project rule). No over-testing observed — each sub-test targets a distinct facet.

CODE QUALITY:
- Project conventions: Followed. Doc-comments explain the non-obvious (session-scoped repetition, first-row lift, zero-row-benign rationale) per golang-documentation norms; no colour/style concerns apply here.
- SOLID principles: Good. Single-responsibility parse/assemble split preserved; the lift is a 4-line inline guard, not a new abstraction.
- Complexity: Low. One added conditional; no new branches in the hot parse path.
- Modern idioms: Yes. Idiomatic if-with-init scoping (if rows := grouped[name]; len(rows) > 0).
- Readability: Good. Intent is self-documenting; the zero-row rationale is inlined at the lift site.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
