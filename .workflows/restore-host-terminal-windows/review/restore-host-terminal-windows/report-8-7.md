TASK: restore-host-terminal-windows-8-7 — Remove (or document) the unreachable OutcomeUnsupported Result taxonomy member

ACCEPTANCE CRITERIA:
- OutcomeUnsupported and Unsupported() are either removed, or retained with an explicit declaration-site comment forbidding their return from OpenWindow.
- The resolution-tier unsupported handling (ResolutionUnsupported → atomic no-op) is unchanged.
- The package builds and all spawn tests pass; no dangling reference to a removed symbol remains.

STATUS: Complete

SPEC CONTEXT: The Adapter Result taxonomy (internal/spawn/adapter.go) is the generic, terminal-agnostic classification general code switches on. "Unsupported" is a resolution-tier concept: Resolver.Resolve returns (nil, ResolutionUnsupported) when no driver matches, and OpenWindow is only ever invoked on an already-resolved (supported) adapter. The tick flagged OutcomeUnsupported + Unsupported() as not merely dead but a latent trap: a future driver returning Unsupported() from OpenWindow would be classified !OK() → AckFailed (burst.go) and FirstPermission (classify.go) only recognises OutcomePermissionRequired, so it would never route to the atomic unsupported no-op the resolution tier owns.

IMPLEMENTATION:
- Status: Implemented (preferred route taken — removal, plus a bonus declaration-site rationale comment)
- Location:
  - internal/spawn/adapter.go — Outcome const block (lines 35-53) now carries exactly OutcomeUnknown (zero sentinel), OutcomeSuccess, OutcomeSpawnFailed, OutcomePermissionRequired. OutcomeUnsupported is gone. Constructors (lines 67-81) are Success / SpawnFailed / PermissionRequired only; the Unsupported() constructor is gone.
  - adapter.go:26-32 — added doc comment explaining "Unsupported" is deliberately NOT an Outcome and is a resolution-tier decision handled before any OpenWindow call. This directly closes the future-author trap the tick identified (exceeds the minimum removal ask).
  - internal/spawn/resolver.go — ResolutionUnsupported and the resolution-tier no-op path unchanged: NULL identity → (nil, ResolutionUnsupported) at line 82, fall-through → (nil, ResolutionUnsupported) at line 95. Doc/precedence comments intact.
- Notes: Whole-repository Grep for OutcomeUnsupported and the standalone Unsupported() constructor returns ZERO matches. All apparent "Unsupported()" hits are the unrelated tui Model method DetectUnsupported() and the surviving ResolutionUnsupported resolution-tier symbol. No dangling reference remains. burst.go / classify.go classify on OK() / OutcomePermissionRequired, neither of which was touched, so their classification is unchanged.

TESTS:
- Status: Adequate
- Coverage: internal/spawn/adapter_test.go was updated in lockstep:
  - TestResultOutcomes_AllThreeDistinct (lines 5-31) now iterates only Success/SpawnFailed/PermissionRequired, asserts 3 distinct Outcomes, and each constructor stamps its designated constant. Comment (lines 6-8) documents that "unsupported" is a resolution-tier outcome, not an Adapter Outcome.
  - TestResultOK_TrueOnlyForSuccess and TestResultZeroValue_IsUnknownNotSuccess exercise the OK() predicate and the zero-value OutcomeUnknown sentinel — the invariant that a bare Result{} is never mistaken for success is preserved.
  - TestResult_RoundTripsDetailAndGuidance covers Detail/Guidance passthrough.
  - Resolution-tier coverage intact: resolver_test.go (lines 27/33/39/58-59) and resolver_config_test.go (lines 102/119/135) still assert ResolutionUnsupported → nil adapter, including the NULL-identity-skips-config case. logemit_test.go pins the unsupported log-attr literal.
- Notes: No test referenced the removed Unsupported() constructor, so no orphaned test remains. Tests verify behaviour (distinct outcomes, OK() semantics, resolution routing), not implementation detail. Not over-tested — each assertion targets a distinct guarantee. A comment-only change would need no assertion, and the removal is transitively proven green by the existing enum/routing tests.

CODE QUALITY:
- Project conventions: Followed. Zero-invalid sentinel (OutcomeUnknown) mirrors RecipeKind per codebase convention; closed typed taxonomy with constructor helpers is the established spawn pattern.
- SOLID principles: Good. Removing the member tightens the Adapter contract (interface segregation / single responsibility) — "unsupported" now lives solely on the tier that decides and handles it.
- Complexity: Low. Net reduction of surface area.
- Modern idioms: Yes.
- Readability: Good — the added declaration-site comment proactively steers future driver authors away from the identified trap.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
