TASK: cli-verb-surface-redesign-7-4 — Single-source the single-target "nothing resolved" miss error string

ACCEPTANCE CRITERIA:
- U+2014 em-dash and `-f %s` suffix preserved byte-identical
- Both the bare-positional (open.go) and N=1 glob-to-zero burst (open_burst.go) sites call the one `singleMissError` helper
- Multi-target `aggregatedMissError` left as-is

STATUS: Complete

SPEC CONTEXT:
specification.md § "Miss handling — total miss is a hard fail" (line 68): a target resolving to nothing is a hard failure at every arity; the TUI-picker-with-filter fallback is removed and the error points at the escape hatch, e.g. `nothing resolved for 'blog' — try -f blog` (line 73). Line 85 (§ Atomic pre-flight) confirms the `-f` suggestion appears only in the single-target miss message, while the multi-target abort omits it. The refactor is a pure single-sourcing of that wording; behaviour is unchanged.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Helper: cmd/open_burst.go:108-110 (`singleMissError`) — sole authoring site.
  - Bare-positional caller: cmd/open.go:303 (`return singleMissError(miss.Target)`).
  - N=1 glob-to-zero burst caller: cmd/open_burst.go:142 (`return singleMissError(misses[0])`).
  - Untouched multi-target helper: cmd/open_burst.go:91-93 (`aggregatedMissError`), still the separate `nothing resolved for: %s` (no `-f`) form.
- Notes:
  - Byte-identical em-dash verified by hexdump of cmd/open_burst.go:109 — bytes `e2 80 94` (U+2014), and `-f %s` suffix intact.
  - `git show a0309160` confirms the refactor moved the two former inline `fmt.Errorf("nothing resolved for '%s' — try -f %s", …)` sites into the helper verbatim (string unchanged), so the format is provably byte-identical to pre-refactor.
  - `grep -rn "nothing resolved"` confirms no residual inline copy of the single-target string remains in production code — only the helper, the aggregated helper, comments, and tests reference the wording.
  - Plan row 7-4 cites stale site line numbers (`open.go:344`, `open_burst.go:125`); the actual call sites are open.go:303 and open_burst.go:142. Line drift only — the correct sites both route through the helper.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/open_multitarget_test.go:206 `TestSingleMissError_ByteIdenticalFormat` — direct unit test asserting `singleMissError("blog").Error()` equals the exact U+2014 wording (the precise byte-identical guard this task needs).
  - cmd/open_test.go:2040-2058 — bare-positional total miss end-to-end (`open blog`): asserts the string, that the TUI is not launched, and that it is a plain (non-usage) error → exit 1.
  - cmd/open_multitarget_test.go:180-204 — single non-glob target via the multi-target seams: asserts the single-target wording and that the burst is not invoked.
  - cmd/open_multitarget_test.go:215-242 `TestOpenCommand_SingleGlobExpandingToZero_KeepsMinusF` — the N=1 glob-to-zero burst call site (`nomatch-*`): asserts the single-target `-f` wording and no burst.
  - cmd/open_multitarget_test.go:104/139/170 — aggregated multi-target misses assert the separate `nothing resolved for: …` wording, confirming `aggregatedMissError` is untouched.
- Notes: Not over-tested. Each site (helper, bare-positional, N=1 glob burst, aggregated) has a focused assertion; no redundant bloat introduced by this task. Tests would fail if the em-dash, the double substitution, or the routing regressed.

CODE QUALITY:
- Project conventions: Followed. Single-authoring-site pattern matches the codebase's established "SOLE authoring site" idiom (mirrors `commandAttachOnlyMessage`, `emitResolveDecision`, `logExecHandoff`). Doc comment cites the spec section and names both callers.
- SOLID principles: Good — single responsibility, one source of truth, no drift surface.
- Complexity: Low — a one-line formatter.
- Modern idioms: Yes — idiomatic `fmt.Errorf`, plain error (correctly not a `*UsageError`, so exit 1).
- Readability: Good — intent-revealing name and a comment that explains the arity split against `aggregatedMissError`.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
