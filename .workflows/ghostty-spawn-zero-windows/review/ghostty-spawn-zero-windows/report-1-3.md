TASK: Rider #2 / Fix 3 — make the partial-failure banner honest when nothing opened. Change PartialFailureMessage(failed []string) → PartialFailureMessage(failed []string, othersOpened bool) string, with both callers deriving othersOpened = len(confirmed) > 0 from spawn.PartitionResults.

ACCEPTANCE CRITERIA:
1. PartialFailureMessage takes othersOpened bool: false → '… failed to open — nothing opened' (single + multi); true → unchanged '… failed to open — others left open'.
2. Both callers derive othersOpened = len(confirmed) > 0 from PartitionResults: cmd/spawn.go (CLI exit-1 error) and internal/tui/burst_partial_failure.go (burstPartialFailureFlash).
3. Trigger self-attach never counts as an 'other' (never in the confirmed set).
4. Permission-wall branch (returns Guidance) and degenerate empty-failed branch (returns '') unchanged.
5. Copy single-sourced in message.go; no spawn: prefix, no ⚠ glyph; '— nothing opened' mirrors GoneMessage/UnsupportedNoopMessage.
6. Parity tests (message_test.go + burst_partial_failure_test.go) assert byte-identical CLI/picker output for total failure (single + multi → nothing opened) and genuine partial (others left open).
7. go test ./... passes.

STATUS: Complete

SPEC CONTEXT:
Fix 3 (Rider #2) of the ghostty-spawn-zero-windows spec. The old PartialFailureMessage hard-coded '— others left open'; on a total failure (every external window failed, nothing confirmed) that clause is false — the observed banner "'portal-EfVRkk', 'portal-agent-first-3' failed to open — others left open" was emitted with opened=0. The spec prescribes exactly the delivered signature PartialFailureMessage(failed []string, othersOpened bool) string, both callers deriving othersOpened from the shared PartitionResults chokepoint (othersOpened = len(confirmed) > 0), with the trigger self-attach never in confirmed. The copy table and single-sourcing / no-prefix / no-glyph constraints match the implementation exactly. Testing & Validation Requirements Rider #2 scopes the parity tests to message_test.go + burst_partial_failure_test.go.

IMPLEMENTATION:
- Status: Implemented (matches spec byte-for-byte)
- Location:
  - internal/spawn/message.go:61-66 — PartialFailureMessage(failed []string, othersOpened bool) string; othersOpened true → "%s failed to open — others left open", false → "%s failed to open — nothing opened" (QuoteJoin, no count-aware verb). Em-dash U+2014 matches GoneMessage (line 41) and UnsupportedNoopMessage (lines 79/81).
  - cmd/spawn.go:179,210 — confirmed, failed := spawn.PartitionResults(results); return fmt.Errorf("spawn: %s", spawn.PartialFailureMessage(failed, len(confirmed) > 0)). CLI adds the "spawn: " prefix at the call site.
  - internal/tui/burst_partial_failure.go:120-124 — confirmed, failed := spawn.PartitionResults(results); returns spawn.PartialFailureMessage(failed, len(confirmed) > 0). Bare body (⚠ added by the notice band via statusGlyph).
- Notes:
  - Trigger-never-an-other holds structurally: runSpawn splits via spawn.SplitNetN(sessions) → burster runs over `external` only, so `results` (and therefore `confirmed`) can never contain the trigger. Picker mirror: burstExternal = names[:last], burstTrigger = names[last]; msg.Results are external-only. Verified against classify.go PartitionResults (confirmed = Confirmed()==AckConfirmed windows, list order).
  - Permission-wall branch unchanged: cmd/spawn.go:191-199 (FirstPermission → Guidance, before the generic branch) and burst_partial_failure.go:117-119. Degenerate empty-failed branch unchanged: burst_partial_failure.go:121-123 (len(failed)==0 → return ""). Only the final PartialFailureMessage(...) call was touched.
  - Exactly two non-test callers exist (grep confirmed); both pass len(confirmed) > 0. No stale single-arg call sites remain.
  - Copy single-sourced in message.go; the CLI/picker both route through it, so a future edit lands in one place.

TESTS:
- Status: Adequate
- Coverage:
  - internal/spawn/message_test.go TestPartialFailureMessage — five focused subtests: true single (others left open), true multi, false single (nothing opened), false multi, and a no-"spawn:"-prefix / no-⚠ check looping both variants. Directly pins both branches and the no-prefix/no-glyph invariant on the renderer.
  - internal/tui/burst_partial_failure_test.go — existing assertions updated to the two-arg form: line 142 (…, true), line 245 (…, true). TestBurstPartialFailure_StaysInMultiSelectMode (lines 370-403) repurposed as the total-failure picker parity case: external=[alpha] alone times out, nothing else confirmed, asserting rm.flashText == spawn.PartialFailureMessage([]string{"alpha"}, false) — the required byte-identical total-failure assertion.
  - CLI parity: cmd/spawn_test.go:1018,1048 assert err.Error() == "spawn: " + spawn.PartialFailureMessage([...], true) for genuine partials.
- Notes:
  - Parity is proven structurally: every CLI/picker assertion compares against the shared spawn.PartialFailureMessage output (not a hard-coded literal), so the two surfaces cannot drift and the literal copy is pinned once in message_test.go. Correct, robust technique.
  - Not over-tested: the five renderer subtests map 1:1 to the acceptance matrix (true/false × single/multi + prefix/glyph). No redundant checks.
  - Minor under-coverage (non-blocking, spec-scoped out): there is no cmd-level Execute test driving an all-external-failing CLI burst to assert the "— nothing opened" total-failure body end-to-end (the two CLI partial-failure Execute tests both use genuine partials → othersOpened=true). The total-failure CLI path is covered by construction (identical shared expression + shared PartitionResults) and by message_test.go's false-branch tests, and Rider #2 deliberately scopes the parity tests to message_test.go + burst_partial_failure_test.go — so this is optional, not a gap against the acceptance criteria.
- go test ./internal/spawn/ ./internal/tui/ ./cmd/ pass; full go test ./... reports no failures.

CODE QUALITY:
- Project conventions: Followed. Single-sourced renderer in internal/spawn, callers derive from the designated PartitionResults chokepoint (classify.go), no raw copy at call sites, CLI-only "spawn: " prefix, glyph left to the notice band. Consistent with GoneMessage/UnsupportedNoopMessage vocabulary.
- SOLID principles: Good. One renderer owns the copy; callers own only the boolean derivation from a shared predicate — single responsibility preserved, no duplication.
- Complexity: Low. A single two-branch conditional; no count-aware verb needed.
- Modern idioms: Yes. Idiomatic Go, fmt.Sprintf + shared QuoteJoin.
- Readability: Good. The doc comment on PartialFailureMessage (lines 44-60) explains the othersOpened semantics, the trigger-never-an-other rule, and the mirrored '— nothing opened' clause.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/spawn_test.go:952 (TestSpawnPartialFailure) — optionally add a subtest that Executes the CLI body with every external window failing (othersOpened=false) and asserts err.Error() == "spawn: " + spawn.PartialFailureMessage([]string{"s2"}, false), giving the CLI total-failure path the same end-to-end coverage the picker has in TestBurstPartialFailure_StaysInMultiSelectMode. Deliberately optional: Rider #2 scopes the parity tests to message_test.go + burst_partial_failure_test.go, and the CLI total-failure body is already covered by construction (shared expression + shared PartitionResults) plus the message_test.go false-branch tests.
