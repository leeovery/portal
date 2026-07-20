TASK: Single-Source The Domain-Pin Set So The Exclusivity Guard Cannot Miss A Future Pin (cli-verb-surface-redesign-13-2)

ACCEPTANCE CRITERIA:
- cmd/open.go:208-209 no longer hand-lists pin flags; it calls anyOpenDomainPin.
- The pin-name set is declared once and consumed by both the guard and the dispatch table.
- The exclusivity guard's behaviour is unchanged for the existing four pins.

STATUS: Complete

SPEC CONTEXT:
The open command has four domain-pin flags (-s/--session, -p/--path, -a/--alias, -z/--zoxide) that skip the resolution grammar and force a single domain. -f/--filter is the sole non-composing flag: mutually exclusive with a target and every domain pin. This task is a maintainability refactor (not a behaviour change) closing a latent drift risk where the pin-name set was hand-listed in three places, so a future fifth pin could be added to the dispatch table yet silently omitted from the exclusivity guard.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/open.go:295 (openDomainPinFlags single source), cmd/open.go:208 (call site), cmd/open.go:245-249 + :317-322 (dispatch loop + pinResolvers), cmd/root.go:319-321 (anyOpenDomainPin guard).
- Notes:
  - AC1 met: cmd/open.go:208 now reads `if destination != "" || anyOpenDomainPin(cmd) {` — the former inline `anyPin := cmd.Flags().Changed("session") || …` is gone. The -f/pin exclusivity guard routes through anyOpenDomainPin.
  - AC2 met: `openDomainPinFlags = []string{"session", "path", "alias", "zoxide"}` (open.go:295) is the single canonical pin-name list. The guard consumes it: `anyOpenDomainPin` = `slices.ContainsFunc(openDomainPinFlags, cmd.Flags().Changed)` (root.go:320). The dispatch table consumes it: RunE iterates openDomainPinFlags for precedence order and looks each flag's resolver up in `pinResolvers` (open.go:245-247). Adding a pin now requires editing openDomainPinFlags + pinResolvers only.
  - AC3 met: the guard iterates the same four pins {session, path, alias, zoxide} — behaviour for the existing pins is byte-identical.
  - Naming note (not drift): the plan text references a `pinDispatch` table (cmd/open.go:259-272). The shipped code names the dispatch table `pinResolvers` (a map keyed by openDomainPinFlags) and the shared name list `openDomainPinFlags`. This is a benign naming evolution from a prior task in the phase; the drift-guard test explicitly labels pinResolvers keys as "(dispatch) key". Semantic match to the acceptance criteria — the "pin-name set" and the "dispatch table" both exist and both consume the single source.

TESTS:
- Status: Adequate
- Coverage:
  - Required drift guard present: cmd/open_pin_source_guard_test.go TestPinResolversKeysCoveredByFlagList asserts every dispatch (pinResolvers) key is present in the shared openDomainPinFlags list — exactly the "every pinDispatch key is present in the shared pin-name list" test the task requires, in the existing guard-test style.
  - TestFlagListFullyResolved gives reverse coverage (every listed flag has a resolver — no nil-resolver dispatch).
  - TestOpenDomainPinFlagsAreRegistered asserts every name is a real openCmd flag (catches a typo that would silently disable the guard for that pin).
  - TestAnyOpenDomainPinCoversEveryPin drives anyOpenDomainPin with each declared pin marked Changed and asserts it fires — proves the guard covers every dispatchable pin.
  - Existing pin-exclusivity tests still exercise the refactored guard: TestOpenCommand_Filter_WithPin_UsageError (open_test.go:2635, table over all four pins) and TestOpenCommand_Filter_WithMultiplePins_UsageError (open_test.go:2717) both drive `open -f … <pin>` through the anyOpenDomainPin call site and assert the exact usage-error wording, no resolver consultation, no TUI launch.
- Notes:
  - Not under-tested: the drift vectors (forward map→list, reverse list→map, list→registered-flag, guard OR-composition) are each covered, plus behavioural exclusivity end-to-end.
  - Not over-tested: the four guard tests each target a distinct, non-overlapping drift vector. TestAnyOpenDomainPinCoversEveryPin and TestOpenDomainPinFlagsAreRegistered share the openProbeCmdWithFlags fixture and both touch per-pin Changed(), but prove different properties (guard OR-composition vs flag-registration correctness) — justifiable, not redundant.
  - Would fail if broken: adding a pinResolvers entry without listing it in openDomainPinFlags fails TestPinResolversKeysCoveredByFlagList; reverting open.go:208 to a hand-list that omits a pin would not be caught by these guards, but the existing -f/pin exclusivity tables would catch a missed pin at the call site.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel in the cmd-package guard test (per CLAUDE.md); guard test uses no package-level state mutation / cobra Execute / tmux.
- SOLID principles: Good. Single canonical source with two consumers; no responsibility bleed.
- Complexity: Low. `slices.ContainsFunc(openDomainPinFlags, cmd.Flags().Changed)` collapses the guard to one line.
- Modern idioms: Yes — slices.ContainsFunc with a method value is idiomatic Go 1.21+.
- Readability: Good. The single-source invariant and load-bearing precedence order are documented at openDomainPinFlags, pinResolvers, anyOpenDomainPin, and the RunE dispatch loop.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
