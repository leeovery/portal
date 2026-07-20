TASK: cli-verb-surface-redesign-7-5 — Extract expandSessionGlobAll to collapse the duplicated session-glob expansion block

ACCEPTANCE CRITERIA:
- Zero-match returns a single MissResult{Target: pattern}
- Non-zero returns K SessionResult{Domain: "glob"} — behaviour unchanged
- ResolveAliasPinAll (validated-path body) left untouched
- Consumed by both ResolveBareAll and ResolveSessionPinAll

STATUS: Complete

SPEC CONTEXT:
Spec § Glob targets (specification.md:158-166): a bare target containing glob metacharacters (*, ?, […]) is session-domain by construction — it expands against the finite live user-visible session set and skips the path/alias/zoxide chain; zero matches ⇒ unresolvable ⇒ hard fail (spec:51). § Target-set composition / burst pre-flight: in the K-surface multi-target context a miss is COLLECTED (a *MissResult in the returned slice), not a hard error, so pre-flight can report every unresolvable target. The extracted helper is the single session-glob fan-out primitive feeding both the bare and -s pinned All-variants.

IMPLEMENTATION:
- Status: Implemented (clean mechanical extraction)
- Location: internal/resolver/query.go:174-184 (expandSessionGlobAll); consumed at query.go:206 (ResolveBareAll) and query.go:227 (ResolveSessionPinAll)
- Notes:
  - Commit e9e2fe6e shows the two previously byte-identical glob branches in ResolveBareAll and ResolveSessionPinAll replaced by a single call to expandSessionGlobAll(query, names). The diff is a pure extraction — helper body is identical to both old inline blocks (pattern == query in both callers, so MissResult{Target: pattern} == the old MissResult{Target: query}). Behaviour is byte-for-byte unchanged.
  - Zero-match → []QueryResult{&MissResult{Target: pattern}}; non-zero → K &SessionResult{Name: m, Domain: "glob"} in MatchSessions order. Matches acceptance exactly.
  - names is threaded in as a parameter (not fetched inside) because the callers source it differently — ResolveBareAll fetches inside its glob branch, ResolveSessionPinAll reuses an earlier fetch. This keeps the helper pure/side-effect-free; the reasoning is documented in the doc comment.
  - ResolveAliasPinAll (query.go:250-279) correctly left untouched: it expands over the alias-key namespace (Keys()) with a per-key validatedPath reduction and a per-key miss carrying the matched KEY — a genuinely different shape, not the same duplicated block. Not a false-negative omission.
  - The single-target ResolveSessionPin (query.go:289) retains its own first-match-only block (returns matches[0], hard error on miss) — different contract (single QueryResult, not K), correctly outside this All-variant collapse.

TESTS:
- Status: Adequate
- Coverage (internal/resolver/query_all_test.go):
  - ResolveBareAll: "session glob expands to K SessionResults with domain glob" (query_all_test.go:32, non-zero + order + Domain=="glob") and "session glob with zero matches is a single collected miss" (:54, zero → MissResult{Target:"api-*"}).
  - ResolveSessionPinAll: "glob expands to K SessionResults with domain glob" (:205) and "zero-match glob is a collected miss" (:275).
  - Both consumers exercise both the zero-match and non-zero paths, so the extracted helper's full behaviour is verified through each public caller. Order preservation and the Domain=="glob" tag are asserted.
- Notes:
  - No direct unit test of the unexported expandSessionGlobAll — correct: exercising it through its two exported callers tests behaviour, not implementation detail. Testing the private helper directly would couple tests to structure.
  - These tests predate the refactor and continue to pass as the behavioural safety net; a refactor that changed behaviour would break them (e.g. wrong Target on the zero-match miss, dropped Domain tag, or lost ordering). Not over-tested — each assertion covers a distinct property.

CODE QUALITY:
- Project conventions: Followed. Small pure helper, thorough doc comment in house style, make([]QueryResult, 0, len(matches)) preallocation, idiomatic Go.
- SOLID principles: Good — single responsibility (session-glob fan-out), no hidden side effects (names injected).
- Complexity: Low — one branch, one loop.
- Modern idioms: Yes — slice preallocation, range loop.
- Readability: Good — intent and the names-as-parameter rationale are documented.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
