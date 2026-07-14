TASK: 2.2 — Adapter resolver: identity → native Ghostty adapter / unsupported (tick-44ecd2, restore-host-terminal-windows-2-2)

ACCEPTANCE CRITERIA:
- ResolveAdapter(NewIdentity("com.mitchellh.ghostty","Ghostty")) → non-nil *ghosttyAdapter + ResolutionNative.
- Channel-suffixed Ghostty bundle id (com.mitchellh.ghostty.debug) → (*ghosttyAdapter, ResolutionNative) via family glob.
- NULL identity (NewIdentity("","")) → (nil, ResolutionUnsupported).
- Known-but-undriven identity (com.apple.Terminal) → (nil, ResolutionUnsupported).
- Passthrough/unknown identity (com.example.MyTerm) → (nil, ResolutionUnsupported).
- Never a non-nil adapter with ResolutionUnsupported; never ResolutionConfig in Phase 2.

STATUS: Complete

SPEC CONTEXT:
Spec "Adapter Contract & Extensibility → Detection is separate from the adapter / Resolution precedence": detection yields an identity; a separate resolver maps identity → adapter via precedence config override → native adapter → unsupported; a NULL/unmatched identity → unsupported. "Config Schema → Precedence": config can override a built-in; Phase 4 inserts the terminals.json tier ahead of native — Phase 2 ships the native→unsupported stub with a placeholder for the config step. "Observability → Attr keys": `resolution` is a closed attr with values config|native|unsupported, so Resolution's string values must match for direct logging.

IMPLEMENTATION:
- Status: Implemented (and correctly evolved past the Phase-2 stub — see Notes).
- Location: internal/spawn/resolver.go:7-130 (Resolution type + registry + Resolve/ResolveAdapter); internal/spawn/ghostty.go:77-89,148 (ghosttyAdapter, newGhosttyAdapter, Adapter assertion); internal/spawn/identity.go:24-90 (IsNull, NewIdentity, MatchesFamily).
- Notes:
  - Resolution string values (resolver.go:12-18) are exactly "config"/"native"/"unsupported" — matches the closed log-attr vocabulary.
  - Native registry (resolver.go:32-37) is the ordered slice with exactly one Phase-2 entry: family "com.mitchellh.ghostty*", build=func()Adapter{return newGhosttyAdapter()}. Family glob resolves both the bare id and channel-suffixed variants via MatchesFamily/path.Match.
  - Precedence order in Resolve (resolver.go:80-96): NULL short-circuit BEFORE config (so a `*` catch-all can never hijack NULL), then config tier, then native loop, then unsupported fall-through. All six acceptance criteria trace correctly through ResolveAdapter (resolver.go:128-130), which pins an empty TerminalsConfig — so the config tier never matches and behaviour reduces exactly to the Phase-2 native→unsupported stub (ResolutionConfig unreachable, unsupported always carries nil).
  - newGhosttyAdapter (ghostty.go:87-89) constructs the driver without touching osascript/tmux — only OpenWindow does — so the registry may build it freely, per the task's purity requirement.
  - EVOLUTION (not drift): the code is the Phase-4-evolved form. Task 2.2 specified a free function ResolveAdapter with a *commented* Phase-4 config placeholder; Phase 4 replaced that placeholder with a real Resolver struct + resolveConfig tier (resolver.go:44-123), and kept ResolveAdapter as a thin zero-config wrapper preserving the Phase-1/2 contract. This is the expected build-forward, and every Phase-2 acceptance criterion still holds through the zero-config wrapper. ResolveAdapter remains a live production consumer (internal/tui/spawn_detect.go:64) — not orphaned.
  - Purity: the task asked the resolver stay pure (no logging/I/O). The Phase-4 config path (resolveConfig → newScriptRecipeAdapter) does stat/WARN, but that is out of task 2.2's scope; the Phase-2 surface (ResolveAdapter, empty config) never enters that path and stays pure.

TESTS:
- Status: Adequate.
- Coverage: internal/spawn/resolver_test.go (TestResolveAdapter) is table-driven with the five named acceptance cases (Ghostty native, channel-suffixed native, NULL unsupported, known-undriven unsupported, passthrough-unknown unsupported). Each case additionally asserts the two cross-cutting invariants inline for EVERY row: resolution is never ResolutionConfig, and an unsupported resolution always carries a nil adapter (resolver_test.go:55-60). The native cases type-assert the returned adapter to *ghosttyAdapter; the unsupported cases assert nil. No OpenWindow call, so no real osascript is executed — matches the task constraint.
- Notes:
  - Would fail if the feature broke: a wrong resolution string, a nil adapter on the Ghostty path, or a non-nil adapter on an unsupported path all trip a distinct assertion.
  - Not over-tested: exactly the five acceptance cases + the two invariants; no redundant assertions, no unnecessary mocking (the free function needs none).
  - Minor overlap with internal/spawn/resolver_config_test.go (Ghostty→native, NULL→unsupported) is justified, not redundant: that suite exercises the Phase-4 Resolver METHOD with config wiring, whereas resolver_test.go pins the zero-config free-function entry point that is task 2.2's acceptance surface. Different entry points, different guarantees.

CODE QUALITY:
- Project conventions: Followed. Small ordered-slice registry, 1-method DI seams, unexported helpers, compile-time interface assertion (ghostty.go:148). Package-spawn white-box test, no t.Parallel(). Doc comments explain the load-bearing precedence and the NULL-before-config ordering.
- SOLID principles: Good. ResolveAdapter/Resolve is a single-responsibility pure mapping; the registry keeps the driver-set open for extension (config + native tiers) without touching the resolve algorithm.
- Complexity: Low. Linear precedence ladder; single-entry registry loop.
- Modern idioms: Yes (path.Match glob, typed string enum, func-valued registry builders).
- Readability: Good. Intent is self-documenting; the precedence and the NULL carve-out are called out in comments.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
