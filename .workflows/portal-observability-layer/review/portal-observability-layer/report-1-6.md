TASK: Resolve process_role from os.Args longest-prefix match against the closed table (portal-observability-layer-1-6)

ACCEPTANCE CRITERIA:
- [state,daemon]→daemon; [state,hydrate]/[state,signal-hydrate]→hydrate.
- [hooks,set,--on-resume,x]→hooks_cli; [clean]/[clean,--logs]→clean.
- [open,.],[x],[attach,foo],[] (bare)→tui.
- Unknown subcommand ([version],[init],[alias,add])→bootstrap.
- Interleaved flags ignored: [--verbose,state,daemon] and [state,--foo,daemon]→daemon.
- Closed result space is exactly the 6 values.

STATUS: Complete

SPEC CONTEXT:
Spec § The internal/log package → process_role resolution (87-100). main resolves process_role from a lightweight os.Args inspection (flags ignored), longest-prefix match, first-match-wins, bootstrap default.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/log/process_role.go (ResolveProcessRole :41, subcommandPath :79, role consts :9-19); wired at main.go:32 → log.Init :33.
- Notes: Pure function. subcommandPath strips any token beginning with `-` (short/long/lone `-`), preserves order. Two-token `state …` arm checked before single-token switch (correct longest-prefix). All 6 const values used; default → roleBootstrap. All AC rows + edge cases (bare state→bootstrap, state wat→bootstrap, interleaved flags) trace correctly to spec table.

TESTS:
- Status: Adequate
- Location: internal/log/process_role_test.go (3 tests)
- Coverage: TestResolveProcessRole (21-case table covers every AC + edge), TestResolveProcessRole_ClosedResultSpace (15 inputs incl nil/empty/flags-only), TestResolveProcessRole_DriftTripwire (canonical argv per non-default role; guards against silent degradation to bootstrap).
- Notes: Behaviour-focused, would fail if a mapping dropped. Drift-tripwire is a justified addition (a second copy of command-routing divorced from Cobra registration; can't import cmd → import cycle).

CODE QUALITY:
- Project conventions: Followed (no t.Parallel, internal/log, documented export).
- SOLID: Good — single responsibility, pure.
- Complexity: Low.
- Modern idioms: Yes (strings.HasPrefix, pre-sized slice, switch table).
- Readability: Good — doc comments restate table + rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] subcommandPath filters ALL flag tokens, not just leading ones. A positional value starting with `-` (e.g. `portal open -weird-dir`) would be stripped, but matching only inspects path[0]/path[1] so the role is never affected. Spec-conformant; noting for awareness.
