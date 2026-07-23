---
topic: remote-trigger-spawns-on-local-terminal
cycle: 2
total_proposed: 1
---
# Analysis Tasks: Remote Trigger Spawns On Local Terminal (Cycle 2)

## Task 1: Collapse the seven happy-path detectInsideTmux subtests into one table-driven test
status: pending
severity: medium
sources: duplication

**Problem**: `internal/spawn/detect_inside_test.go`'s `TestDetectInsideTmux` contains seven happy-path subtests that share a byte-identical arrange/act/assert skeleton and differ only in the input `[]ClientActivity` slice and the expected outcome. Each one builds `&fakeClientLister{clients: []ClientActivity{...}}`, calls `walker, reader := localWalkSeams()`, invokes `detectInsideTmux("dev", lister, walker, reader)`, asserts `err == nil` with the same `t.Fatalf` shape, then asserts either `got.IsNull()` or `got.BundleID`/`got.Name`. That is roughly 100 lines of repeated structure exercising a single behaviour — winner-select-then-locality-gate — across a matrix of client sets: all-remote → NULL (line ~46), sole local → Ghostty (~65), two locals with the higher listed second → Terminal (~83), two locals with the higher listed first → Terminal (~101), an exact activity tie → first-listed Ghostty (~117), remote-most-active with a local bystander → NULL (~133, the reported bug's shape), and local-most-active with an idle remote bystander → Ghostty (~156). This is exactly the "multiple scenarios of one behaviour" case the project's Go testing conventions call for as a table-driven test, and the same package already uses that form for equivalent matrices (`TestAppBundlePath`, `TestParsePSProcessInfo` in `walk_test.go`; the phase-1 scenario table in `detect_test.go`). Left as parallel copy-paste subtests, adding a new selection scenario means restating the whole skeleton, and the assertion shapes can drift row-to-row.

**Solution**: Collapse the seven happy-path subtests into a single table-driven test whose rows carry `{name string, clients []ClientActivity, wantNull bool, wantBundleID, wantName string}`, with a shared body doing the single `localWalkSeams()` + `detectInsideTmux("dev", ...)` call and the `err == nil` + IsNull-or-bundle assertion once per row. Keep the three error-path subtests separate — they wire bespoke walker/reader fakes and assert on `ErrDetectTransient` / underlying-cause chains rather than a resolved identity. This is a test-only structural change: no scenario, assertion, or coverage is added or removed, and no production code is touched.

**Outcome**: One table-driven happy-path test replaces the seven near-duplicate subtests; every currently-pinned scenario and its assertion survive with identical semantics; adding a new selection scenario becomes a one-row change; the error-path and zero-client coverage is preserved. `internal/spawn/detect_inside.go` is unchanged and the full `internal/spawn` suite stays green.

**Do**:
1. In `internal/spawn/detect_inside_test.go`, inside `TestDetectInsideTmux`, define a row type `{name string; clients []ClientActivity; wantNull bool; wantBundleID, wantName string}` and a slice of rows — one per current happy-path subtest — carrying each subtest's description as the row `name` and its exact input slice and expected outcome:
   - every client remote/mosh (`{PID:601,Activity:100}`,`{PID:602,Activity:200}`) → `wantNull`.
   - single local client (`{PID:501,Activity:0}`) → `wantBundleID: com.mitchellh.ghostty`, `wantName: Ghostty` (zero activity must not matter for a sole client).
   - two locals, higher-activity listed SECOND (`{PID:501,Activity:100}`,`{PID:502,Activity:200}`) → `com.apple.Terminal`.
   - two locals, higher-activity listed FIRST (`{PID:502,Activity:200}`,`{PID:501,Activity:100}`) → `com.apple.Terminal`.
   - exact activity tie (`{PID:501,Activity:150}`,`{PID:502,Activity:150}`) → first-listed `com.mitchellh.ghostty`.
   - most-active client remote + local bystander (`{PID:601,Activity:9999}`,`{PID:501,Activity:1}`) → `wantNull` (the reported bug's shape).
   - local most-active + idle remote bystander, remote listed FIRST (`{PID:601,Activity:50}`,`{PID:501,Activity:200}`) → `com.mitchellh.ghostty` / `Ghostty`.
2. Write the shared per-row body: `lister := &fakeClientLister{clients: tt.clients}`; `walker, reader := localWalkSeams()`; `got, err := detectInsideTmux("dev", lister, walker, reader)`; `if err != nil { t.Fatalf(...) }`; then `if tt.wantNull { assert got.IsNull() } else { assert got.BundleID == tt.wantBundleID; if tt.wantName != "" { assert got.Name == tt.wantName } }`.
3. Preserve the session-passthrough assertion — the current all-remote subtest also asserts `lister.calls` equals exactly `[dev]`. Keep it (either in the shared body for every row, or pinned to a dedicated row) so it is not lost.
4. Keep the three error-path subtests exactly as-is and separate: list-clients failure (~line 185), single-client walk failure (~205), and most-active-winner walk transient (~230). They assert `errors.Is(err, ErrDetectTransient)` plus the underlying cause and must stay distinct from the resolved-identity table.
5. The zero-clients clean-NULL subtest (~line 265) may optionally fold into the table as a `wantNull` row (`clients: nil`); if standalone is clearer, leave it — either way its coverage must remain.

**Acceptance Criteria**:
- The seven happy-path subtests are replaced by one table-driven test; each original scenario survives as a named row with its exact input slice and expected outcome (NULL vs bundle/name), including the deterministic first-listed-on-tie assertion and the max-by-activity assertions for the higher client listed both first and second.
- The session-passthrough assertion (`lister.calls == [dev]`) is preserved.
- The three error-path subtests (list-clients failure, single-client walk failure, winner-walk transient → `ErrDetectTransient` + preserved underlying cause) remain separate and behaviourally unchanged.
- The zero-clients clean-NULL coverage remains (in the table or standalone).
- No production code changes — `internal/spawn/detect_inside.go` is untouched; this is a test-file-only edit.
- The spec-pinned invariants all still hold: pure-remote→NULL, single-local→drive, 2+ locals highest-activity wins, exact tie first-listed wins, remote-most-active→NULL, local-most-active→drive.

**Tests**:
- The refactored table-driven test is itself the coverage. Run `go test ./internal/spawn/...` (unit lane) and confirm every row plus the retained error-path and zero-client subtests pass, with no net change in assertions — only structure.
