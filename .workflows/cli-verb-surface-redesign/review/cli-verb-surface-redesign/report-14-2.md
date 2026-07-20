TASK: cli-verb-surface-redesign-14-2 — Extract a shared exact-session-match helper and unify its error handling in the resolver

ACCEPTANCE CRITERIA:
- One helper owns the ListSessionNames + slices.Contains + lister-error policy; no site re-implements the match.
- Resolve, ResolveSessionPin, and ResolveSessionPinAll all route through the helper.
- The err == nil happy path behaves identically to today; the lister-error path is now consistent across all three (no match, no panic).
- go build -o portal ., go test ./..., and golangci-lint run are clean.

STATUS: Complete

SPEC CONTEXT: Task is an internal refactor within internal/resolver — no user-facing behaviour change. Spec § Target resolution precedence (exact session name → path → alias → zoxide) and § Pinned-domain contract (-s/--session is session-domain only; a miss hard-fails, no picker fallback). The helper must preserve Resolve's pre-existing lister-error tolerance (a lister error / nil-empty list collapses to "no match" and falls through), and unify the two pin variants (previously `names, _`) onto that same policy. Task explicitly scopes to the shared exact-match helper only; the broader *All-vs-single-pin symmetry restructure is out of scope.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/resolver/query.go
  - Helper: isExactSession(query string) bool — query.go:123-129. Owns the ListSessionNames fetch (124), the one lister-error policy (`if err != nil { return false }`, 125-127), and the slices.Contains membership test (128). slices.Contains appears exactly once in the file, inside this helper — no inline match survives.
  - Resolve routes through it — query.go:154.
  - ResolveSessionPinAll routes through it — query.go:245.
  - ResolveSessionPin routes through it — query.go:314.
- Notes: The two remaining `qr.sessions.ListSessionNames()` calls (query.go:221 ResolveBareAll, :241 ResolveSessionPinAll) are the glob-branch fetches feeding expandSessionGlobAll — they need the full names slice for MatchGlob, not a membership test, so they legitimately do NOT route through isExactSession. No missed exact-match site. Helper doc comment (114-122) accurately documents it as the single authority and the "lister error → no match, never escalated" policy, and notes callers keep their own downstream non-match behaviour (fall-through / hard-fail / collected miss). Happy path is byte-identical to the former `err == nil && slices.Contains(...)`; the two pin variants that formerly discarded the lister error (`names, _`) now share the tolerant policy — a strict improvement, no behaviour regression on the happy path.

TESTS:
- Status: Adequate
- Coverage: Dedicated cross-entry-point suite TestQueryResolver_ExactSessionMatch_UnifiedAcrossEntryPoints (query_test.go:715-829):
  - Exact hit resolves via Resolve (725), ResolveSessionPin (741), ResolveSessionPinAll (757) — asserts SessionResult{Name, Domain:DomainSession}.
  - Lister error (nil names + non-nil err) is no-match with no escalation via Resolve (781 → MissResult, nil err), ResolveSessionPin (796 → nil result + verbatim "No session found" hard-fail, NOT the lister error), ResolveSessionPinAll (811 → collected MissResult, nil err).
  - mockSessionLister (query_test.go:54-61) returns both names and err, correctly exercising the err != nil branch. Both acceptance test bullets are directly satisfied.
- Notes: Pre-existing per-method lister-error subtests remain — Resolve "lister error collapses to no match" (query_test.go:503) and ResolveSessionPin "lister error collapses to a miss" (query_test.go:677). These overlap the new unified suite (781/796) for those two entry points. Mild, defensible redundancy: the per-method tests are broader single-method characterization suites; the new suite documents the cross-cutting shared-helper invariant. Not over-tested to a blocking degree. query_all_test.go carries ResolveSessionPinAll glob/exact-hit/exact-miss cases but no lister-error case — that gap is filled only by the unified suite (811), so no duplication there.

CODE QUALITY:
- Project conventions: Followed. Interface-based DI preserved (SessionLister seam untouched); unexported helper on the receiver; house-style capitalised user-facing miss messages with the //nolint:staticcheck directive retained on the unchanged pin error paths.
- SOLID principles: Good — single-responsibility helper; the match rule now has exactly one owner.
- Complexity: Low — helper is a 3-line fetch/guard/contains.
- Modern idioms: Yes — slices.Contains, clean early-return error guard.
- Readability: Good — the helper and all three call sites carry accurate comments explaining the fetch/membership/error-policy ownership and each caller's downstream behaviour.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/resolver/query_test.go:503, :677 — Consider consolidating the two pre-existing per-method lister-error subtests (Resolve, ResolveSessionPin) now that TestQueryResolver_ExactSessionMatch_UnifiedAcrossEntryPoints (:781, :796) covers the same policy across all three entry points; requires a decide-whether judgment since the per-method suites double as full single-method characterization, so keeping them is also defensible.
