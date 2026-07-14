TASK: restore-host-terminal-windows-4-3 — Identity match + within-config most-specific precedence

ACCEPTANCE CRITERIA:
1. Identity matching a raw bundle id, a .app name, a friendly alias, AND a *-glob all at once resolves to the RAW BUNDLE ID entry (highest tier).
2. A friendly-alias key ("ghostty") matches a com.mitchellh.ghostty identity via the bundle-id family and outranks a bare * glob.
3. Among two matching globs, the one with the longer literal prefix wins (com.mitchellh.* beats com.* beats bare *).
4. A bare * catch-all is the lowest-precedence match — wins only when nothing more specific matches.
5. An identity matching NO key returns ok=false (fall through to native).
6. matchConfig is deterministic regardless of Go map iteration order (residual tier+literals tie resolves by key string).

STATUS: Complete

SPEC CONTEXT:
Spec "Config Schema → Precedence" (specification.md:391): within config, most-specific wins; deterministic order highest-first: exact raw bundle id → exact .app name / friendly alias → *-glob (longer glob beats broader; bare * lowest). "Config keys accepted: layered" (:274-277): friendly alias / .app name / raw bundle id / *-glob, all reducing internally to bundle-id-family matching (MatchesFamily). Task is a PURE ranker; recipe validity (4.2) and invalid→native fall-through (4.6) are out of scope; NULL identity never reaches matchConfig (short-circuited in 4.6).

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/configmatch.go (friendlyAliases table :11-14; tier consts :20-24; matchScore + better :30-43; matchConfig :53-69; scoreKey :76-97; countLiterals :102-110)
- Notes: Matches the plan exactly. scoreKey form-classification order is load-bearing and correct: glob (strings.Contains "*") first → exact bundle id (tierBundleID=3) → friendly alias via MatchesFamily (tierNamed=2) → .app name (tierNamed=2) → no-match. Glob matching reuses the Phase 1 family matcher (MatchesFamily, path.Match semantics), so config glob semantics equal native family semantics as required. friendlyAliases ships {ghostty: com.mitchellh.ghostty*, warp: dev.warp.Warp-*} per spec. matchConfig is pure — no logging, no validation, no adapter construction.
  - Tie-break logic verified correct by trace: the switch's case 1 (!ok OR score.better) handles first-match and strictly-better; case 2 (!bestScore.better(score) && k < key) is reached only when score is not strictly better, so !bestScore.better(score) provably narrows to a genuine tier+literals equality (total order: differing tier ⇒ one side better; equal tier + differing literals ⇒ one side better), and the lexicographically-smaller key is kept. Minimum key wins regardless of iteration order for N tied keys (bestScore left unchanged on the tie-swap is safe since scores are equal). No off-by-one or ordering bug found.

TESTS:
- Status: Adequate
- Location: internal/spawn/configmatch_test.go
- Coverage: All six acceptance criteria are covered:
  - AC1 → "it picks the exact raw bundle id over a .app name, alias, and glob" (all four forms present, expects raw bundle id).
  - AC2 → "it matches a friendly-alias key through the bundle-id family" (+ warp variant with dev.warp.Warp-Stable → dev.warp.Warp-*); both outrank a co-present bare *.
  - AC3 → "it prefers a longer glob over a broader glob and both over a bare catch-all".
  - AC4 → "it selects the bare * catch-all only when nothing more specific matches".
  - AC5 → "it returns no match for an identity absent from the config" (wantOK=false).
  - AC6 → TestMatchConfig_DeterministicTieBreak: 200 iterations against two equal-scored globs (a.b.* / *.b.c both literals=4), asserts the lexicographically-smaller key wins every time. The fixture's countLiterals/path.Match reasoning is sound (verified independently).
  - Plus "it prefers a named .app name over a glob", TestMatchConfig_ReturnsWinningEntry (proves the entry travels with the winning key, not just the key string), and TestCountLiterals (pins literal counts incl. bare *, "**", all-literal key).
- Notes: Not under-tested — every AC and the load-bearing tie-break are exercised, and a broken feature would fail (wrong tier, wrong glob length, or nondeterministic tie-break all caught). Not meaningfully over-tested — TestCountLiterals targets an internal helper directly but pins the load-bearing specificity numbers that drive glob precedence, so it earns its place. Assertions are focused, no excessive setup or mocking (pure fabricated configs/identities).

CODE QUALITY:
- Project conventions: Followed. White-box unit test in package spawn (unit lane, no daemon/tmux — correct; no integration tag needed). Table-driven subtests with behaviour-phrased "it ..." names, matching the codebase style. Doc comments explain the "why" (why order is load-bearing, why ties must not decide the winner) per house style.
- SOLID principles: Good. Single responsibility (matchConfig only ranks keys; scoring split into scoreKey/countLiterals; ordering isolated in matchScore.better). Reuses MatchesFamily rather than re-implementing glob semantics (DRY, no drift from native).
- Complexity: Low. One linear pass over the config; branch-per-form in scoreKey; the only subtle spot (the two-case switch) is correct and well-commented.
- Modern idioms: Yes. `for i := range 200` (Go 1.22+ int range), rune-ranged countLiterals, idiomatic multi-value switch-true.
- Readability: Good. Intent is self-documenting; the tier constants and matchScore.better read cleanly.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/spawn/configmatch_test.go — Add a case where a friendly-alias key and a .app-name key BOTH match the same identity (e.g. cfg {"ghostty":{}, "Ghostty":{}} with NewIdentity("com.mitchellh.ghostty","Ghostty")). Both score tierNamed/literals=0, so the winner is decided purely by the key-string tie-break — a plausible real config (user pastes both forms) whose determinism is currently only proven via the glob fixture. Low priority; behaviour is already correct by the shared tie-break path.
