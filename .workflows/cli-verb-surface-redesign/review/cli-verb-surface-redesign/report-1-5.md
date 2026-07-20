TASK: cli-verb-surface-redesign-1-5 — `-f/--filter` picker redirect + mutual exclusivity

ACCEPTANCE CRITERIA (plan row 1-5 + Phase 1 AC bullet):
- `-f` + positional target → usage error
- empty `-f` value → usage error
- `-f` alone opens the filtered Sessions picker, skipping resolution
- no-arg `open` still launches the picker unchanged (regression)
(Phase 1 AC: "`-f/--filter <text>` opens the picker on the Sessions page pre-filled with `<text>`, skipping resolution; it is mutually exclusive with a positional target (usage error otherwise).")

STATUS: Complete

SPEC CONTEXT:
Spec § "-f/--filter is the sole non-composing flag": `-f` is not a target — it is a "skip resolution, open the picker pre-filtered" redirect, mutually exclusive with positional targets and with every pin flag (usage error otherwise). Plain `-f <text>` (no command) opens the picker on the default Sessions page with the text pre-filled; the command variant `-f <text> -e <cmd>` is the Phase-2 exception (filtered Projects picker). Spec § "Pinned-domain contract": only bare positionals run the guessing chain and only `-f` opens the picker. Note the Phase-2 task 2-5 extended the exclusivity + error wording to include all pins; the code under review is the merged current state, which fully satisfies the narrower Phase-1 acceptance as well.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/open.go:206-217 (the `-f` guard in openCmd.RunE); flag registration cmd/open.go:998; no-arg picker fall-through cmd/open.go:274-276; ordered-target scan exclusion cmd/open_targets.go:33; bootstrap TUI-path classification cmd/root.go:301-303 / 310-311.
- Notes:
  - The `-f` branch is correctly placed AFTER the `--ack` validation but BEFORE both the multi-target routing gate and the pin-dispatch loop. This ordering is load-bearing: `open -f x -s y` is rejected as a usage error rather than resolving the pin or bursting. Verified against the multi-target gate at open.go:230-233 and pin loop at 268-272.
  - Mutual exclusivity checks `destination != ""` (positional) OR `anyPin` (any of session/path/zoxide/alias changed) → single usage error. Empty value guarded separately. Both are `NewUsageError` → exit 2 (via main.classify).
  - `-f` alone routes to `openTUIFunc(cmd, filterVal, command, serverWasStarted(cmd))` with `command == nil`, which yields the default Sessions page (Projects mode is only entered when a command is present — Phase 2). The Sessions-vs-Projects page selection itself is TUI-layer logic keyed on command presence, correctly deferred to that layer.
  - No-arg regression path (open.go:274-276) is untouched by the `-f` feature and reached only when the `-f` branch does not fire.
  - `-f`'s value is additionally excluded from the raw ordered-target scan (open_targets.go:33, empty domain) — defense-in-depth, since the guard fires before that scan anyway.
  - No drift from the plan/spec. Error wording matches the Phase-2-extended message verbatim; Phase-1's narrower "positional only" intent is a strict subset.

TESTS:
- Status: Adequate
- Coverage (cmd/open_test.go):
  - TestOpenCommand_Filter_OpensPickerPrefilteredAndSkipsResolution (2426) — `-f blog` opens picker with initialFilter="blog", command=nil, and asserts the resolver's SessionLister was NEVER consulted (recordingFilterLister.called) → proves resolution is skipped, not merely that the picker opened.
  - TestOpenCommand_Filter_WithPositionalTarget_UsageError (2474) — `-f blog api` → exact usage-error string + *UsageError type; TUI not called, resolver not consulted.
  - TestOpenCommand_Filter_EmptyValue_UsageError (2649) — `-f ""` → "-f/--filter value must not be empty", *UsageError, TUI not called.
  - TestOpenCommand_NoArgs_NoFilter_LaunchesPicker (2683) — regression: bare `open` still opens the picker with empty filter.
  - TestOpenCommand_Filter_WithPin_UsageError (2521, table over -s/-p/-z/-a) + TestOpenCommand_Filter_WithMultiplePins_UsageError (2603) — the Phase-2 pin-exclusivity extension; both assert no attach/mint outcome fires and the resolver is untouched.
  - TestOpenCommand_Filter_ThreadsCommandToPicker (2853) — Phase-2 `-f web -e claude` threads the command through.
- Notes:
  - Well-balanced. Each test asserts a distinct behaviour (redirect+skip, positional conflict, empty value, no-arg regression). No redundant happy-path duplication.
  - Failure-detecting: breaking the skip-resolution property flips lister.called; removing the exclusivity guard drops the error; breaking no-arg flips tuiCalled/gotFilter. Every acceptance bullet has a test that would fail if the feature broke.
  - The `lister.called == false` assertion is the strongest signal here — it verifies `-f` behaves as a redirect (not a resolved target), which is the core of the spec contract.
  - Not over-tested: no assertions on implementation details; the Sessions-page proxy is command==nil (correct level for the cmd layer — the actual page machinery is tested in internal/tui).

CODE QUALITY:
- Project conventions: Followed. Guard uses the package `NewUsageError` (exit-2 seam), the `openTUIFunc` override seam for tests, and `cmd.Flags().Changed`/`GetString` per the cobra convention. Comment block accurately cites the spec sections and explains the load-bearing ordering.
- SOLID principles: Good. The `-f` branch has a single responsibility and delegates picker launch to the existing `openTUIFunc`.
- Complexity: Low. Straight-line guard with two early returns and a tail delegate.
- Modern idioms: Yes.
- Readability: Good — intent-revealing comment, clear branch.
- Issues: One minor DRY observation (non-blocking, below).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/open.go:208-209 — the inline `anyPin := cmd.Flags().Changed("session") || ... || cmd.Flags().Changed("alias")` duplicates `anyOpenDomainPin(cmd)` (cmd/root.go:314-317), which is the byte-identical predicate over the same four pins. The pin set is now enumerated in three places (this inline expression, `anyOpenDomainPin`, and the `pinDispatch` table at open.go:263-266); a future pin must be added to all three. Replace the inline with a call to `anyOpenDomainPin(cmd)` to collapse two of the three. (Concrete, mechanical, touches control-flow logic → quickfix; note the two predicates are semantically the same today but named for different intents, so confirm the shared meaning holds before merging.)
