# Review Report: built-in-session-resurrection-13-5

**TASK**: Documentation precision cleanup across spec citations, error-warning wording, godoc scope, and historical record

**ACCEPTANCE CRITERIA**:
- Verify spec section titles cited in code comments are accurate.
- Update error-warning wording (test fixtures asserting old "corrupt" headline must use new "Portal state file unusable" wording).
- Tighten godoc scope.
- Add reconciliation note to the historical review record (`.workflows/built-in-session-resurrection/review/built-in-session-resurrection/report.md`) — note, NOT a rewrite.

**STATUS**: Issues Found (one missing edge case; non-blocking documentation drift)

**SPEC CONTEXT**:
The spec's "Observability & Diagnostics" section (line 1314) contains a "Proactive Health Signals" subsection (line 1363) with verbatim warning copy ("Portal state file unusable — restoration skipped." line 1379, "Portal save daemon failed to start..." line 1372). The headline noun is "Portal state file unusable", NOT the older "corrupt sessions.json".

**IMPLEMENTATION**:
- Status: Mostly done; one edge case (historical record reconciliation note) appears not delivered.
- Locations verified:
  - `internal/state/index_reader.go:10-19, 21-38` — godoc text mentions both "permission denied" and "corrupt-index path"; doc-comment is internally consistent. Permission errors ARE wrapped with `ErrCorruptIndex` (line 45). No "corrupt" old headline lurking here.
  - `cmd/bootstrap/errors.go:47-69` — `CorruptSessionsJSONWarning()` and `SaverDownWarning()` carry SPEC headline text exactly. Docs say "Wording matches the Observability section of the specification verbatim."
  - `internal/warning/warning.go:21-22` — uses precise "Observability → Proactive Health Signals → TUI interaction" form.
  - `cmd/state_cleanup.go:119-141` — symlink-traversal claim is aligned with `TestStateCleanup_PurgeAllowsSymlinkedIntermediatePathComponents`.
  - `cmd/bootstrap_warnings_test.go:222-227` — test fixtures assert `"Portal state file unusable — restoration skipped."` (new headline); no "corrupt sessions.json" old wording lingering.
  - `cmd/bootstrap/errors_test.go:67-97` — `TestCorruptSessionsJSONWarning_returnsExactSpecCopy` and `TestSaverDownWarning_returnsExactSpecCopy` assert new headline text verbatim.

- Drift / open items:
  - `cmd/bootstrap/errors.go:52-53` and `:62-63` cite "Observability section of the specification" (umbrella) rather than the more precise "Observability → Proactive Health Signals" path that `internal/warning/warning.go:21-22` uses. Borderline; may not have been within task scope.
  - The historical review record at `.workflows/built-in-session-resurrection/review/built-in-session-resurrection/report.md` ends at line 137 with the original Bug 46 entry. There is NO reconciliation note (no cycle-6 update, no addendum, no annotation on items 19/40/41/42 — the items most directly relevant to cleanup work). The task description explicitly calls out "historical review record gets a reconciliation note (not a rewrite)" as a load-bearing edge case.

**TESTS**:
- Status: Adequate where applicable.
- Spec-headline copy tests verbatim-assert both warning lines.
- Symlink-traversal regression: `TestStateCleanup_PurgeAllowsSymlinkedIntermediatePathComponents`.
- Documentation-only cleanup tasks not generally test-able beyond fixture assertions already in place.

**CODE QUALITY**:
- Readability: `internal/state/index_reader.go` doc-comments precise. `cmd/bootstrap/errors.go` cites spec at umbrella section level — less precise than `internal/warning/warning.go`. `cmd/state_cleanup.go:119-141` exemplary.
- Issues:
  - Inconsistent spec-citation precision between `internal/warning/warning.go` and `cmd/bootstrap/errors.go`.
  - Missing historical-review reconciliation note.

**BLOCKING ISSUES**:
- None. Missing historical-review reconciliation note is a documentation-precision miss in a workflow audit-trail file, not a code defect. Does not block ship.

**NON-BLOCKING NOTES**:
- [quickfix] `.workflows/built-in-session-resurrection/review/built-in-session-resurrection/report.md` — task 13-5 explicitly calls for "historical review record gets a reconciliation note (not a rewrite)". File ends at line 137 with no addendum. Recommend appending a short "## Reconciliation Notes (post-cycle-6)" section pointing to Phase 13 cleanup tasks resolving findings 19, 40, 41, 42 (where applicable) and any items the analysis cycle deferred.
- [quickfix] `cmd/bootstrap/errors.go:52-53` and `:62-63` — citations say "Observability section of the specification verbatim". For consistency with `internal/warning/warning.go:21-22`, tighten to "Observability → Proactive Health Signals". One-line edit per godoc; no behavioural impact.
- [idea] If cycle-6 analysis flagged other godoc-scope issues (e.g., the `CorruptSessionsJSONWarning` doc's reference "T12-8" — a task ID rather than a spec section), consider one sweep through `cmd/bootstrap/errors.go` doc-comments.
