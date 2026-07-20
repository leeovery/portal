TASK: cli-verb-surface-redesign-7-2 — Extract a single domain-pin dispatch helper for the four copy-paste arms in openCmd.RunE

ACCEPTANCE CRITERIA:
- Byte-identical behaviour per pin (-s/-p/-a/-z): same resolution, error propagation, and openResolved handoff
- Short-circuit on first changed pin flag preserved
- Adding a future pin must touch one place (the table), not four

STATUS: Complete

SPEC CONTEXT:
Spec § Domain-pinning flags — each pin (-s/--session, -p/--path, -a/--alias, -z/--zoxide) resolves its value in ONE domain only and dispatches the hit through the shared outcome switch. Pins are checked in a FIXED precedence order (session → path → alias → zoxide), the first changed pin short-circuits, and a pin never mints-to-picker: a miss hard-fails (spec § Pinned-domain contract) and emits no resolve line (pins are deterministic, not guesses). The block sits before the no-target early-return so `open -s <name>` with an empty positional resolves the pin rather than launching the picker.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/open.go:259-272 (pinDispatch table + short-circuit loop), cmd/open.go:317-328 (resolvePinAndOpen helper). Resolver pin methods: internal/resolver/query.go:289/325/346/376.
- Notes: The four previously-copy-pasted ~13-line if-arms are collapsed into a 4-row dispatch table keyed by flag name + a resolver method expression ((*resolver.QueryResolver).ResolveSessionPin, etc.), iterated by a loop that `return`s on the first Changed flag, plus one shared body (resolvePinAndOpen). Refactor is behaviour-preserving: the diff (commit 44b04465) shows the extracted helper performs the identical sequence — GetString(flag) → buildQueryResolver → resolve(qr, val) → error propagation → openResolved handoff — as each original arm. All four resolver methods share the uniform (string) (QueryResult, error) signature, so the method-expression table is type-safe. Sole dispatch site confirmed: the only other Changed("session"/…) calls (open.go:208-209) are the -f/--filter mutual-exclusion anyPin guard, not a pin arm.
  - Byte-identical: verified line-for-line against the old arms in the commit diff.
  - Short-circuit: loop returns on the first Changed pin in fixed session→path→alias→zoxide order — identical to the old sequential if-return arms.
  - One-place extensibility: a future pin is a single table row; the loop and helper are pin-agnostic. Acceptance met.

TESTS:
- Status: Adequate
- Coverage: Per-pin behavioural coverage drives the full RunE via rootCmd.Execute() with real argv, exercising the extracted table + helper end-to-end (cmd/open_test.go):
  - Session: ExactHit_RoutesToConnector (269), Glob_AttachesFirstMatch (413), Miss_HardFailsNoPicker (447), WithCommand_UsageError (370), EmitsNoResolveLine (502).
  - Path: Mints_NoPicker (535), GlobNamedDir_Mints (593), ThreadsCommandIntoMint (634), EmitsNoResolveLine (675).
  - Alias: Mints_NoPicker (710), UnknownKey_HardFailsNoPicker (772), ThreadsCommandIntoMint (827), EmitsNoResolveLine (868).
  - Zoxide: Mints_NoPicker (903), NotInstalled_ErrorsNoPicker (962), NoMatch_HardFailsNoPicker (1012), ThreadsCommandIntoMint (1067), EmitsNoResolveLine (1108).
  These assert the three acceptance dimensions per pin — correct outcome (attach vs mint vs picker), error propagation (miss / not-installed / unknown-key), and the openResolved handoff (command-threading + resolve-line suppression) — and would fail if the extraction mis-routed any pin. This is the correct level for a behaviour-preserving refactor: tests are behavioural (through RunE), not coupled to the helper's internals, so no new tests were needed and none is redundant.
- Notes: No dedicated "short-circuit precedence" test (e.g. two pins set, session wins), but this is not a gap: two changed pins are recovered as two targets by orderedOpenTargets and route to the multi-target burst BEFORE reaching pinDispatch, so the loop is only ever reached with exactly one changed pin — the precedence order is structurally preserved and its multi-pin branch is unreachable by design. No over-testing observed.

CODE QUALITY:
- Project conventions: Followed. Idiomatic Go table-driven dispatch with method expressions; matches the codebase's single-source/chokepoint pattern (cf. openResolved, emitResolveDecision, logExecHandoff). Comments carry the spec-anchored rationale per pin.
- SOLID principles: Good. Open/closed satisfied — a new pin is one table row, not a fifth arm. Single responsibility: resolvePinAndOpen owns exactly the read→resolve→handoff sequence.
- Complexity: Low. Four near-duplicate branches reduced to one loop + one helper; net -32 lines in the commit.
- Modern idioms: Yes. Method expressions ((*T).Method) as first-class table values is the correct Go idiom for this dispatch.
- Readability: Good. The consolidated doc block documents each pin's domain semantics in one place; resolvePinAndOpen has a clear docstring explaining the method-value contract.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
