TASK: restore-host-terminal-windows-9-5 — Give Outcome a zero sentinel so a zero-value Result is not silently a success

ACCEPTANCE CRITERIA:
- Outcome's zero value is OutcomeUnknown; Result{}.OK() returns false.
- OutcomeSuccess/OutcomeSpawnFailed/OutcomePermissionRequired and their constructors behave identically; OK() is still Outcome == OutcomeSuccess.
- No code depends on the prior numeric value of any Outcome member; go build ./... and go test ./internal/spawn/... green.
- The self-attach gate (Burster.Run branching on result.OK()) is unchanged for all constructed results.

STATUS: Complete

SPEC CONTEXT: This is an analysis-cycle (Phase 9) hardening task on the spawn adapter's generic Result taxonomy. The spec's Permissions & Error Quarantine boundary fixes a closed three-member outcome taxonomy (success / spawn-failed / permission-required); general code classifies solely on Outcome via OK()/Confirmed()/FirstPermission and never inspects driver Detail/Guidance. The concern: with OutcomeSuccess previously at iota 0, a bare Result{} silently read as a success and could gate the irreversible self-attach exec (Burster.Run branches on result.OK()). The fix aligns Outcome with the same package's RecipeKind zero-invalid convention.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/adapter.go:35-53 (const block), :86-88 (OK)
- Notes:
  - OutcomeUnknown Outcome = iota is the first const member (adapter.go:42), carrying a doc comment (adapter.go:36-41) stating it is the invalid/unset sentinel, that OpenWindow must NEVER return it, and that it mirrors RecipeKind's zero-invalid treatment in recipe.go. Matches the task's Do-step 1 verbatim.
  - OutcomeSuccess/OutcomeSpawnFailed/OutcomePermissionRequired follow (now implicitly 1/2/3). Constructors Success/SpawnFailed/PermissionRequired (adapter.go:68-81) unchanged; OK() unchanged as Outcome == OutcomeSuccess (adapter.go:87).
  - Result{}.OK(): zero-value Outcome is OutcomeUnknown (0), != OutcomeSuccess (1), so OK() returns false. Confirmed.
  - No numeric dependence: grep of internal/ + cmd/ finds every classification site is symbolic — classify.go:45 (== OutcomePermissionRequired), burst.go:165 (result.OK()), burst.go:177 (== OutcomePermissionRequired). No int(Outcome), no %d formatting, no JSON marshal, no String() method on Outcome, so renumbering is invisible everywhere. Confirmed Do-step 3.
  - No production path constructs a bare Result{} that relied on old success-at-zero. classify.go:49 returns WindowResult{}, false (a zero WindowResult) but the paired false tells callers to ignore its embedded Result. Confirmed Do-step 4.
  - Self-attach gate: Burster.Run gates awaitToken on result.OK() (burst.go:165); every constructed result's OK() is byte-identical to before, so the gate behaviour is unchanged.
  - Consistency: recipe.go:9-17 confirms RecipeKind uses a zero-invalid sentinel (RecipeArgv RecipeKind = iota + 1). Outcome now matches that intent with an explicit named zero member — arguably clearer than RecipeKind's unnamed iota+1 offset. Fully satisfies the task's stated cross-consistency goal.

TESTS:
- Status: Adequate
- Coverage:
  - TestResultZeroValue_IsUnknownNotSuccess (adapter_test.go:48-60) — the task-specific test: asserts var zero Outcome == OutcomeUnknown AND (Result{}).OK() == false. Directly covers the primary acceptance criterion.
  - TestResultOK_TrueOnlyForSuccess (adapter_test.go:33-46) — Success(...).OK() true; SpawnFailed/PermissionRequired .OK() false. Covers the constructor-behaviour criterion.
  - TestResultOutcomes_AllThreeDistinct (adapter_test.go:5-31) — three distinct Outcome values + each constructor stamps its designated constant. Regression guard against a renumber collapsing two members.
  - TestResult_RoundTripsDetailAndGuidance (adapter_test.go:62-89) — Detail/Guidance passthrough, unaffected by the change but still green.
- Notes: Tests read behaviour (OK() truthiness, zero-value identity, constructor stamping), not the concrete iota numbers — so they remain valid regardless of the offset and would fail if the sentinel regressed. Not over-tested: the new test is a single focused case; no redundant assertions, no unnecessary mocking. Not under-tested: every acceptance bullet (zero=unknown, Result{}.OK()==false, constructor parity, OK() semantics) has a direct assertion. classify/burster regression is covered by their own existing suites, which switch symbolically and so are insulated from the renumber.

CODE QUALITY:
- Project conventions: Followed. Explicit named zero-invalid sentinel matches the RecipeKind convention in the same file and the project's stated enum convention (zero = unset). Doc comments are thorough and explain intent, not mechanics.
- SOLID principles: Good. Change is localized to the Outcome enum; OK() remains the single classification predicate (single source of truth preserved).
- Complexity: Low. Pure const-block edit plus doc; no branching added.
- Modern idioms: Yes. Idiomatic Go iota sentinel-at-zero.
- Readability: Good. The OutcomeUnknown doc comment states the invariant (OpenWindow must never return it) at the definition site.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/spawn/adapter.go:24 — the type-level comment "The three members are the whole closed taxonomy fixed by the spec's ... boundary" predates OutcomeUnknown; with four const members now present, a one-clause note that OutcomeUnknown is the sentinel excluded from that three-member taxonomy would remove a momentary "why does it say three when there are four consts" reading. Marginal — the const-level doc (:36-41) already documents the exclusion, so the type comment is not inaccurate; purely a clarity nicety, zero logic impact.
