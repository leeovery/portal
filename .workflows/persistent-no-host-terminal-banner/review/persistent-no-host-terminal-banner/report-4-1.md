TASK: 4-1 — Extract A Shared Cmd-Side Unsupported-Burst No-Op Test Helper (chore / duplication cleanup, tick-2b34c3)

ACCEPTANCE CRITERIA:
- The duplicated arrange block and the shared no-op invariant assertions exist in exactly one place (the new helper pair).
- Both tests retain their distinct message assertions — computed spawn.UnsupportedNoopMessage(id) for AtomicNoop, byte-literal want (plus NULL row) for CopyIsPlainLanguage — at their call sites.
- No production (non-test) code changes.
- go test ./cmd passes.

STATUS: complete

SPEC CONTEXT:
This task did not originate from a spec section — it is an analysis-phase duplication finding (severity medium). The new copy-regression test TestRunOpenBurst_UnsupportedTerminal_CopyIsPlainLanguage had copy-pasted ~30 arrange lines + six no-op invariant assertions from the pre-existing TestRunOpenBurst_UnsupportedTerminal_AtomicNoop. The tui side already had the equivalent shared assertAtomicNoOp helper (internal/tui/burst_unsupported_noop_test.go:56); the cmd side had openBurstDepsForTest but no assert-helper counterpart. Spec §5/§7 mandate the deliberate two-purpose divergence: the computed message (spawn.UnsupportedNoopMessage) catches renderer drift between the CLI and picker; the byte-literal want catches spec copy drift.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/open_burst_run_test.go:477-590
  - runUnsupportedOpenBurstNoOp(t, id) — cmd/open_burst_run_test.go:486-508 (arrange + Execute; returns inner, conn, mint, bursterBuilt, err — error last per ST1008).
  - assertOpenBurstAtomicNoOp(t, err, inner, conn, mint, bursterBuilt) — cmd/open_burst_run_test.go:515-532 (structural invariants: err != nil, !bursterBuilt, len(inner.Calls)==0, len(conn.calls)==0, len(mint.calls)==0).
  - TestRunOpenBurst_UnsupportedTerminal_AtomicNoop — cmd/open_burst_run_test.go:534-548 (routes through both helpers, keeps computed spawn.UnsupportedNoopMessage(id) assertion at line 545).
  - TestRunOpenBurst_UnsupportedTerminal_CopyIsPlainLanguage — cmd/open_burst_run_test.go:550-590 (routes through both helpers, keeps byte-literal tt.want assertion at line 585; NULL/remote row present at line 571-573).
- Notes:
  - git show 5b6352ab --stat touches exactly three files: cmd/open_burst_run_test.go (the test) plus two bookkeeping files (.tick/tasks.jsonl, .workflows/persistent-no-host-terminal-banner/manifest.json). NO production (non-test) code changed. Acceptance criterion "No production code changes" satisfied.
  - The signature ordering (error last) mirrors the tui-side counterpart intent and follows the ST1008 idiom; both helpers call t.Helper() so failures report the caller's line.
  - The duplicated arrange block was removed from CopyIsPlainLanguage's t.Run closure (diff shows the ~30 lines deleted) and now lives solely in runUnsupportedOpenBurstNoOp. The six invariant assertions live solely in assertOpenBurstAtomicNoOp. Single-location criterion satisfied.

TESTS:
- Status: Adequate (test-only chore — no new coverage expected; verified by completeness against baselines)
- Coverage: Both TestRunOpenBurst_UnsupportedTerminal_AtomicNoop and TestRunOpenBurst_UnsupportedTerminal_CopyIsPlainLanguage (named + NULL sub-cases) route through the shared helper pair. The structural no-op invariants are asserted once, applied to all three cases (1 AtomicNoop + 2 CopyIsPlainLanguage sub-tests).
- Notes:
  - Both divergent message assertions remain independently load-bearing, confirmed against the live renderer (internal/spawn/message.go:79-84):
    - Named literal "can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened" byte-matches fmt.Sprintf("can't open new windows in %s · %s — nothing opened", id.Name, id.BundleID) for the Apple Terminal identity.
    - NULL literal "can't open new windows over a remote connection — nothing opened" byte-matches the IsNull() branch.
    So the byte-literal is currently accurate and WILL fail on any spec-copy wording change (copy-drift catch), whereas AtomicNoop's computed assertion self-references the renderer and instead catches the CLI diverging from the shared renderer (renderer-drift catch). The two-purpose split is preserved and genuinely non-redundant.
  - Behaviour of both tests is unchanged by the refactor: the assertion set is byte-for-byte equivalent to the pre-refactor duplicated blocks (verified against the diff), only relocated. The only textual change is a benign message-string tweak ("NewBurster must not be called" → "must not be built") in the shared assert helper — an assertion failure message, not a checked value.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel (per CLAUDE.md cmd-package rule); t.Helper() on both helpers; error-last return (ST1008 / golangci modernize-friendly); mirrors the established tui-side assertAtomicNoOp pattern the task explicitly cites as the model.
- SOLID principles: Good — single-responsibility split between arrange+execute (runUnsupportedOpenBurstNoOp) and structural assertion (assertOpenBurstAtomicNoOp), matching the tui-side decomposition.
- Complexity: Low. Straight-line helpers, no branching beyond the assertions.
- Modern idioms: Yes — idiomatic table-driven sub-tests retained; helper returns are cleanly destructured at call sites.
- Readability: Good. Both helpers carry precise doc comments explaining the divergence contract and why each caller keeps its own err.Error() assertion; call-site comments explain the computed-vs-literal split.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
