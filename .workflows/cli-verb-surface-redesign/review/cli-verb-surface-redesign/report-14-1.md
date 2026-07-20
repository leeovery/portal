TASK: cli-verb-surface-redesign-14-1 — Introduce a typed Domain across the cmd↔resolver routing boundary

ACCEPTANCE CRITERIA:
- cmd.Target.Domain and the resolver QueryResult Domain fields are the typed Domain, not string.
- globExpandableDomain and resolveOpenSurfaces switch on typed constants, not string literals.
- The `resolve` component log line's `domain` attr values are unchanged (session/path/alias/zoxide/miss).
- Glob routing and every domain-pin route behave identically to before.
- `go build -o portal .`, `go test ./...`, and `golangci-lint run` are clean.

STATUS: Complete

SPEC CONTEXT:
Spec § "Wrong-guess feedback — tmux is the receipt" governs the `resolve` component INFO decision
line: one line per bare positional resolved through the guessing chain, with a closed `domain` attr
taxonomy (session/path/alias/zoxide, or miss) that must be byte-identical. Glob and pinned targets are
deterministic, not guesses, and emit no line. This task is a pure internal refactor (typed enum for a
previously string-keyed domain vocabulary) that must preserve that log wording exactly — the anti-pattern
being fixed is "runtime string checks where the signature should be a specific type."

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/resolver/domain.go:15-41 — new `type Domain string` closed constant set
    (DomainBare/Session/Path/Alias/Zoxide/Glob/Miss) + `String()`. Doc comment ties the underlying
    strings to the spec-governed log attr values.
  - internal/resolver/query.go:47,57 — PathResult.Domain / SessionResult.Domain are now `Domain`;
    every producer (Resolve, ResolveSessionPin/All, ResolvePathPin, ResolveAliasPin/All,
    ResolveZoxidePin, expandSessionGlobAll, validatedPath) sets typed constants.
  - cmd/open_targets.go:15,41 — Target.Domain is `resolver.Domain`; openTargetPins is
    `map[string]resolver.Domain` with typed values (excluded flags → zero value). Cobra flag-name keys
    correctly stay string (the third, legitimately-string vocabulary).
  - cmd/open_burst.go:67,76-83 — globExpandableDomain takes/switches on `resolver.Domain` typed
    constants (DomainBare/Session/Alias).
  - cmd/open_surfaces.go:66-106 — resolveOpenSurfaces switches on typed `t.Domain` constants.
  - cmd/open.go:435,444-455 — emitResolveDecision emits `"domain", domain.String()`; resolveDecision
    returns `resolver.Domain`, reads r.Domain / DomainMiss.
- Notes: No drift. Grep confirms zero string-literal domain switches or `Domain string` fields remain in
  production (internal/resolver + cmd). All seven constants are referenced (no unused-symbol lint risk).
  resolveOpenSurfaces' switch has no default arm, but Target.Domain is structurally constrained to
  bare/session/path/alias/zoxide (empty-domain targets are never emitted by orderedOpenTargets), so the
  absence is safe and matches prior behaviour.

TESTS:
- Status: Adequate
- Coverage:
  - internal/resolver/domain_test.go:13-31 — TestDomain_String pins all seven constants to their exact
    strings at the source of truth (the byte-identical contract for the log attr).
  - cmd/open_domain_routing_test.go:16-34 — TestGlobExpandableDomain_TypedConstants asserts
    bare/session/alias classify true and path/zoxide/glob/miss + the empty zero value classify false,
    now via typed constants — directly satisfies the "classifies identically to the prior string switch"
    routing test.
  - cmd/open_test.go:2175-2296 — resolve decision-line tests assert identical `domain` attr for a session
    hit ("session"), a zoxide mint ("zoxide"), and a miss ("miss"); 2298-2338 confirms a glob target
    emits no line AND still fans out via the burst (runOpenBurstFunc, driven through the real
    multi-target gate with an injected raw argv). 2340-2372 exercises the shared emitResolveDecision
    helper (non-glob emits, glob suppressed).
  - cmd/open_targets_guard_test.go — updated predicate signature to map[string]resolver.Domain; still
    guards flag↔pin drift. open_pin_source_guard_test.go untouched (flag-name/resolver domain, no Domain
    coupling) and unaffected.
  - internal/resolver/query_test.go / query_all_test.go / glob_test.go — result-Domain assertions compare
    against untyped string constants (e.g. `pr.Domain != "path"`), which remain valid against the named
    string type and continue to pin per-arm domain provenance.
- Notes: The acceptance groups "path/alias/zoxide hit" as one; the zoxide test covers the shared
  PathResult arm of resolveDecision (path/alias/zoxide are all *PathResult, discriminated only by the
  Domain value the resolver already sets and query_test.go independently pins), so a dedicated alias/path
  resolve-line test would be redundant — not under-tested. Not over-tested: assertions are focused, no
  redundant happy-path duplication.

CODE QUALITY:
- Project conventions: Followed. Typed constant set with String() matches Go idiom for closed
  enumerations; comments are load-bearing and accurate. internal/resolver stays a pure, log-free library
  (the log-attr coupling is documented but the emission lives only in cmd/open.go). Domain lives in the
  resolver package (the shared vocabulary owner), consumed by cmd — correct dependency direction.
- SOLID principles: Good. Single closed type is the one source of truth for both vocabularies; routing
  switches now key off it (open/closed — adding a domain is one constant + compile-surfaced switch sites).
- Complexity: Low. Pure type substitution; no new branches or control flow.
- Modern idioms: Yes. `domain.String()` is called explicitly at the log site rather than passing the
  named-string value to slog — the correct defensive choice that guarantees byte-identical rendering
  independent of slog's handling of named string kinds.
- Readability: Good. Self-documenting constants; the doc comment on open_targets.go explicitly notes the
  three distinct vocabularies (typed domain values vs. legitimately-string cobra flag-name keys).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] cmd/open_surfaces.go:66 — the `switch t.Domain` has no default arm. It is provably safe today
  (Target.Domain is constrained to the five emitted domains), but since the task's stated outcome is
  "switches become exhaustive-checkable", consider whether to wire the `exhaustive` linter (or a
  defensive default) so a future Target-producing domain is caught mechanically rather than silently
  yielding no surface. Decide whether the added coverage is worth the tooling/dead-arm cost.
