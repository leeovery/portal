TASK: cli-verb-surface-redesign-1-4 — `resolve` log component — INFO decision line

ACCEPTANCE CRITERIA (plan row 1-4 + Phase 1 AC bullet 8):
- session hit logs resolved_path = session name
- miss logs domain=miss with empty resolved_path
- glob targets emit no line (deterministic — gate on the resolver's glob predicate)
- INFO level so guesses are reconstructable after the fact
- `resolve` component added to closed taxonomy, bound once via log.For("resolve") in cmd/open.go
- one INFO line with attrs target / domain (session/path/alias/zoxide, or miss) / resolved_path
- internal/resolver stays log-free

STATUS: Complete

SPEC CONTEXT:
Spec § "Wrong-guess feedback — tmux is the receipt" (specification.md:78-90) locks one observability
addition: `open` logs its resolution decision so a confusing guess is reconstructable from portal.log.
It is a governed amendment adding ONE component `resolve` to the closed taxonomy, emitted only from
cmd/open.go (resolution's driver); internal/resolver stays a pure log-free library. Attr keys: target
(raw input), domain (session/path/alias/zoxide, or miss), resolved_path (resolved dir, or session name
for a session hit; empty on miss). Behaviour: INFO level; guessing-chain targets only (pins AND globs
are deterministic → no line); emitted on a miss too (domain=miss, empty resolved_path); one line per
resolved guessing-chain target.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/open.go:30 — `var resolveLogger = log.For("resolve")`, the SINGLE binding (comment cites the spec).
  - cmd/open.go:412 `emitResolveDecision(target, result)` — single-sourced emitter owning the
    `!HasGlobMeta` gate + the locked attr set; emits `resolveLogger.Info("resolved", target, domain,
    resolved_path)`.
  - cmd/open.go:426 `resolveDecision(result)` — derives (domain, resolved_path): SessionResult →
    (r.Domain, r.Name); PathResult → (r.Domain, r.Path); MissResult → ("miss", ""); default → ("","").
  - Two call sites both route through emitResolveDecision: cmd/open.go:295 (single-target bare arm) and
    cmd/open_surfaces.go:67 (burst bare arm) — single source, no drift.
  - Glob gate: emitResolveDecision uses resolver.HasGlobMeta (internal/resolver/glob.go:18) — the
    resolver's own glob predicate, single-sourced, not a cmd-local reimplementation. Satisfies the
    plan's "gate on the resolver's glob predicate" wording exactly.
  - Domain values originate in resolver.Resolve (query.go:139/147/152/159/165): session/path/alias/
    zoxide/miss — read directly off the result, never re-derived.
  - internal/resolver purity is enforced by internal/resolver/log_free_test.go (forbids a log/slog
    import and any log.For binding in non-test resolver files).
- Notes: Closed-taxonomy enforcement is convention, not code (internal/log/log.go:139 states the
  component set is convention); the resolve binding is a legitimate spec-governed amendment. The
  resolveDecision default arm ("","") is defensively unreachable — Resolve only ever returns the three
  handled result shapes or an error (a mid-chain hard error like DirNotFound returns before
  emitResolveDecision, so no line fires — correctly documented at open.go:290-294).

TESTS:
- Status: Adequate
- Coverage (cmd/open_test.go):
  - TestOpenCommand_ResolveLog_SessionHit (2061): domain=session, resolved_path=session name, INFO,
    exactly 1 record.
  - TestOpenCommand_ResolveLog_ZoxideMint (2102): domain=zoxide, resolved_path=resolved dir, INFO.
  - TestOpenCommand_ResolveLog_Miss (2142): domain=miss, empty resolved_path, INFO, and asserts the
    separate stderr hard-fail error is STILL returned (line coexists with the error).
  - TestOpenCommand_ResolveLog_GlobEmitsNoLine (2184): drives the production burst path via injected
    raw argv; asserts 0 resolve records for a glob target.
  - TestEmitResolveDecision_Helper (2226): unit-tests the helper directly — non-glob → 1 INFO line
    with full attr set; glob → 0 (proves the gate lives inside the helper, shared by both call sites).
  - Pin-suppression: TestOpenCommand_SessionPin_EmitsNoResolveLine (502) + the -p/-a/-z pin tests
    (677/870/1110) each assert 0 resolve records — pins are deterministic, no line.
  - Component/attr matching is precise: resolveRecords() (2032) filters on component=="resolve" AND
    message=="resolved"; assertResolveAttr checks the exact string attr — a broken component name or
    attr key would fail.
  - internal/resolver/log_free_test.go guards the log-free invariant.
- Notes: Well-balanced. Would fail if the feature broke (wrong level, wrong domain, wrong resolved_path
  overloading, glob leaking a line, or resolver emitting logs are all caught). Minor coverage gap: no
  dedicated assertion for a bare domain=path or domain=alias line — but the PathResult arm (which
  handles path/alias/zoxide identically via r.Domain/r.Path) is exercised by the zoxide test, and the
  domain is read straight off the result, so the gap is low-value; adding both would border on
  over-testing.

CODE QUALITY:
- Project conventions: Followed. Single log.For binding per component; INFO level consistent with the
  per-open `process: exec` line; emission confined to the cmd layer keeping internal/resolver a leaf
  library (matches the CLAUDE.md logging contract and the closed-taxonomy discipline).
- SOLID principles: Good. emitResolveDecision (gate + emit) and resolveDecision (pure attr derivation)
  are cleanly separated single-responsibility helpers; adding a call site is one call, not a copy.
- Complexity: Low. A type switch and a boolean gate; no branching depth.
- Modern idioms: Yes. Type switch on the result interface; helper reuse.
- Readability: Good. Comments cite the exact spec sections and explain the overloaded resolved_path and
  the gate placement rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/open_test.go:2102 — optionally add a bare domain=alias (or domain=path) resolve-line
  assertion to cover the other PathResult sub-domains explicitly; currently only zoxide exercises the
  PathResult arm. Low value (identical code path), include only if exhaustiveness over the attr matrix
  is wanted.
