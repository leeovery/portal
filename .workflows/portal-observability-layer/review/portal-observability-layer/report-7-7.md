TASK: Add a drift-tripwire test tying ResolveProcessRole to the real command set (portal-observability-layer-7-7)

ACCEPTANCE CRITERIA:
- A test asserts ResolveProcessRole returns the correct role for each canonical argv shape (daemon, bootstrap, hydrate, hooks_cli, tui, clean).
- The default-fallback behavior for an unrouted verb is explicitly asserted.
- The test fails visibly if a role's expected argv shape stops resolving correctly.
- go test ./internal/log/... passes.

STATUS: Complete

SPEC CONTEXT:
Spec § process_role resolution (87-100): closed 6-value space, static longest-prefix table over os.Args[1:]. process_role is a mandatory baseline attr (204,215). Resolution happens before Cobra parses argv. Task is Phase-7 cleanup: the role table is a second independent copy of command-routing knowledge in internal/log, structurally divorced from cmd/ Cobra registration.

IMPLEMENTATION:
- Status: Implemented (additive test only; process_role.go pre-existing, untouched)
- Location: internal/log/process_role_test.go:59-140 (TestResolveProcessRole_DriftTripwire); resolver process_role.go:41-72; invoked main.go:32.
- Notes: All four Do items: per-role canonical argv (perRole :86-95 covers daemon/hydrate/hooks_cli/clean/tui); default-fallback (fallbackInputs :127-139: version/init/alias/bare state/unknown state subcommand/never-registered verb); contributor comment naming cmd/ files (:69-79); Do #4 (Cobra enumeration) deliberately NOT done with correct justification (cmd imports internal/log → import cycle). Canonical argv shapes verified against real Cobra Use: registration (state daemon, state hydrate, state signal-hydrate, hooks set, clean, open).

TESTS:
- Status: Adequate
- Location: internal/log/process_role_test.go (three complementary functions, no redundancy)
- Coverage: TestResolveProcessRole (broad table incl interleaved-flag stripping); DriftTripwire (per-role + seen-map completeness guard going red on a declared-but-unexercised role + explicit state signal-hydrate + default-fallback); ClosedResultSpace (6-value invariant). Tripwire fails visibly on drift two ways: dropped/renamed mapping → "drifted from cmd/" red; new role without fixture → seen guard red.
- Notes: Pure resolver → no mocks. Distinct purposes. Not over/under-tested.

CODE QUALITY:
- Project conventions: Followed (table-driven subtests; no t.Parallel).
- SOLID: Good — pure function, contract-tested.
- Complexity: Low.
- Modern idioms: Yes (t.Run subtests, clear t.Errorf with offending argv).
- Readability: Good — doc comments explain why the second copy exists + why Cobra enumeration is impossible.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The `case "x"` arm (process_role.go:68) is effectively dead at os.Args level (portal init generates a shell function x()→portal open, so the binary always sees `open`); harmless/spec-aligned (spec lists x …→tui); a one-line comment noting it's the defensive case for a hypothetical direct `portal x` would clarify.
- [idea] The tripwire cannot catch the inverse — a new non-tui/non-bootstrap subcommand added to cmd/ that SHOULD get a dedicated role but falls through to bootstrap (the optional Do #4 would address this but can't due to import cycle). Inherent to the hand-listed approach; no action unless taxonomy expands.
