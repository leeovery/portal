TASK: 7-6 — Document the bootstrap.Logger four-method contract

STATUS: Complete

SPEC CONTEXT: analysis-architecture-c1 finding-4. `bootstrap.Logger` gained `Info` in T4-3; mandatory surface is Debug/Info/Warn/Error. Risk: future contributor adds Trace/Fatal without realising interface tightly coupled to emission sites.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/bootstrap/bootstrap.go:159-183`
- Godoc block immediately above `type Logger interface` (178):
  - Maps each method to emission semantics (step-entry Debug, best-effort INFO with Component B example, degrade-and-continue Warn, fatal Error)
  - Documents nil-safe contract
  - Warns verbatim: "Adding a fifth method (Trace, Fatal, etc.) requires a corresponding new emission site inside Run; do not widen this interface speculatively because every implementer (noopLogger, *state.Logger, test recorders) must satisfy the full set"
  - Cross-references spec's Observability section and production log routing
- Method signatures (178-183) match documented contract 1:1
- `noopLogger` (190-202) implements all four with matching per-method godoc

TESTS:
- Status: N/A (documentation-only)
- Four-method shape already enforced structurally by every implementer at compile time

CODE QUALITY:
- Project conventions: Followed; godoc style matches surrounding interface blocks
- SOLID: Good; comment reinforces ISP
- Complexity: Low (zero code change)
- Modern idioms: variadic `...any`
- Readability: Good; dense but every clause earns space

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
