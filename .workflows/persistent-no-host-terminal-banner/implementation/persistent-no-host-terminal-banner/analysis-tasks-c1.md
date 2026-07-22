---
topic: persistent-no-host-terminal-banner
cycle: 1
total_proposed: 1
---
# Analysis Tasks: persistent-no-host-terminal-banner (Cycle 1)

## Task 1: Extract a shared cmd-side unsupported-burst no-op test helper
status: pending
severity: medium
sources: duplication

**Problem**: The new CLI copy-regression test `TestRunOpenBurst_UnsupportedTerminal_CopyIsPlainLanguage` (cmd/open_burst_run_test.go:522-598) re-implements, near line-for-line (~30 lines), the entire arrange block (events / inner / adapter / conn / mint / `openBurstDepsForTest` / the `bursterBuilt` NewBurster spy / the two-attach surfaces slice) AND the six no-op invariant assertions (`err != nil`, `err.Error() == want`, `!bursterBuilt`, `len(inner.Calls)==0`, `len(conn.calls)==0`, `len(mint.calls)==0`) already present in the pre-existing `TestRunOpenBurst_UnsupportedTerminal_AtomicNoop` (cmd/open_burst_run_test.go:477-520) in the same file. The only substantive divergence is intentional and spec-mandated (§5/§7): the new test asserts against a hardcoded byte-literal `want` (to catch spec copy drift) plus a NULL row, whereas the old test asserts against the computed `spawn.UnsupportedNoopMessage(id)` (to catch picker/CLI renderer drift). The tui side already factored the identical no-op invariants into a shared `assertAtomicNoOp` helper (internal/tui/burst_unsupported_noop_test.go:56); the cmd side has `openBurstDepsForTest` but no equivalent assert helper, so this boilerplate was copy-pasted across the task boundary.

**Solution**: Extract a shared cmd-side helper mirroring the tui side's `assertAtomicNoOp` — a `runUnsupportedOpenBurstNoOp(t, id)` that owns the arrange + Execute and returns `(err error, inner *spawntest.FakeAdapter, conn *recordingConnector, mint *recordingMint, bursterBuilt bool)`, plus an `assertOpenBurstAtomicNoOp(...)` for the structural no-op invariants. Route both `AtomicNoop` and the new `CopyIsPlainLanguage` cases through it, keeping only their two divergent `err.Error()` assertions (computed `spawn.UnsupportedNoopMessage(id)` vs byte-literal `want`) at the call sites.

**Outcome**: The two tests share one arrange+assert scaffold; the ~30 duplicated setup lines and the six repeated invariant assertions collapse into a single helper pair, while the deliberate two-purpose drift-detection split (computed message vs byte-literal want) is preserved at the call sites. The cmd side gains the shared assert helper that the tui side already has.

**Do**:
- Add a helper in the cmd test file (cmd/open_burst_run_test.go) that builds the events / inner / adapter / conn / mint / `openBurstDepsForTest` wiring, the `bursterBuilt` NewBurster spy, and the two-attach surfaces slice, executes the burst, and returns `(err, inner, conn, mint, bursterBuilt)`.
- Add an assert helper `assertOpenBurstAtomicNoOp(t, err, inner, conn, mint, bursterBuilt)` covering the structural invariants: `err != nil`, `!bursterBuilt`, `len(inner.Calls)==0`, `len(conn.calls)==0`, `len(mint.calls)==0`.
- Refactor `TestRunOpenBurst_UnsupportedTerminal_AtomicNoop` to call both helpers, keeping its computed `spawn.UnsupportedNoopMessage(id)` err assertion at the call site.
- Refactor `TestRunOpenBurst_UnsupportedTerminal_CopyIsPlainLanguage` to call both helpers, keeping its byte-literal `want` + NULL-row err assertion at the call site.
- Do not alter production (non-test) code, and do not change what either test asserts about message content.

**Acceptance Criteria**:
- The duplicated arrange block and the shared no-op invariant assertions exist in exactly one place (the new helper pair).
- Both tests retain their distinct message assertions — computed `spawn.UnsupportedNoopMessage(id)` for `AtomicNoop`, byte-literal `want` (plus NULL row) for `CopyIsPlainLanguage` — at their call sites.
- No production (non-test) code changes.
- `go test ./cmd` passes.

**Tests**:
- `go test ./cmd -run TestRunOpenBurst_UnsupportedTerminal` — both `AtomicNoop` and `CopyIsPlainLanguage` pass through the shared helper and pass.
- Confirm the two divergent assertions remain independently load-bearing: the byte-literal copy-drift assertion still fails if the spec copy string changes, and the computed-message assertion still fails if the renderer drifts.
