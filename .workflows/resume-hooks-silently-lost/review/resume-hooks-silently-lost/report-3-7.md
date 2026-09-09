TASK: resume-hooks-silently-lost-3-7 (tick-7b9f24) — extractHookKey Either Makes Its Distinction Or Drops It

ACCEPTANCE CRITERIA:
- `extractHookKey`'s second return is either consumed or gone, along with the comment describing it
- A `respawn-pane` argv carrying no `--hook-key` fails the test loudly rather than yielding `""`
- No existing subtest changes its verdict; no assertion moves
- `go test ./internal/restore/` passes

STATUS: complete

SPEC CONTEXT:
The spec's restore-side contract is that a pane's hook key is baked purely from saved state
(`collectArmInfos` → `buildHydrateCommand`), that a pane with no saved token gets **no**
`--hook-key` flag at all rather than an empty one (`internal/restore/session.go:347-357`), and
that the baked key is the pane's own durable `@portal-pane-id` token. The two suites this task
touches — `TestSessionRestorer_HydrateBakesOneTokenPerPane` and
`TestSessionRestorer_HydrateBakesKeyFromSavedStateOnly` — are the unit-lane proofs of exactly
that: one token per pane, derived from saved state with no live read. This task is a phase-3
consolidation item whose authority is its own body (it repairs a signature/comment mismatch
introduced by task 3-3), not a new spec behaviour.

IMPLEMENTATION:
- Status: Implemented (direction taken: consume the bool, as the task's primary branch)
- Location: `internal/restore/session_test.go:400-403` (the consumption), `:409-410` (the comment
  it makes true), `:411-423` (`extractHookKey` itself, unchanged by this task)
- Commit: `689e4076` — a 3-line addition to `internal/restore/session_test.go` plus the tick/manifest
  bookkeeping. Nothing else in the tree changed, which matches "no assertion moves".
- Notes:
  - `respawnPaneHookKeys` now reads `key, found := extractHookKey(t, args[4])` and fatals with
    `"respawn-pane args = %v carry no --hook-key flag"` when `found` is false, restoring the pre-3-3
    loudness the task named. An argv missing the flag entirely can no longer contribute `""` to the
    returned slice.
  - The comment at `:409-410` is now true of the code. The two cases it names are genuinely
    distinguished at the one call site: absent flag → `t.Fatalf`; present flag with an empty value →
    `""` appended, which then fails the caller's key comparison as a wrong key rather than being
    conflated with a missing flag. Production cannot emit `--hook-key ''` today
    (`session.go:347-357` omits the flag for an empty token), so the second case is precisely the
    regression shape the distinction exists to report distinctly.
  - The "no current case changes" claim in the Do list holds on reading: the only two callers are
    `:335` and `:373`, and every pane they arm is built with `newPaneWithToken` (`:65-67`) carrying a
    non-empty token, so every `respawn-pane` argv in those runs carries the flag and the new fatal is
    unreachable there. The untokened-pane subtest (`:291-311`) does not route through this helper —
    it uses `respawnPaneHydrateCommand` (`:379-390`) plus a direct `strings.Contains` check — so the
    added fatal cannot reach it either.
  - `extractHookKey` and `respawnPaneHookKeys` are declared once each in the package
    (`internal/restore/session_test.go` only); no other consumer exists whose verdict could move.

TESTS:
- Status: Adequate
- Coverage: The task deliberately prescribes no new test — the change adds a fixture-helper guard,
  and the two existing suites are the coverage that the guard fires on nothing today. Both remain
  byte-unchanged in the commit, which is the strongest available evidence that no verdict moved.
  The loudness itself was to be proven by a reverted mutation rather than a committed test; nothing
  in the tree suggests a mutation artefact was left behind (the diff is the three added lines).
- Notes: Adding a dedicated test for a test helper's fatal path would be over-testing here — the
  package's precedent for that (`commander_fake_loudness_test.go`) exists for a shared fake with
  cross-package consumers, which this single-package fixture helper is not.

CODE QUALITY:
- Project conventions: Followed. Unit-lane test file, no `t.Parallel()`, no new tmux or filesystem
  reach, no lane-tag implications (nothing built or spawned).
- SOLID principles: Good — the helper keeps its single parsing responsibility and the policy decision
  ("a missing flag is a test failure") sits with the caller that has the argv to name in the message,
  which is why the message can print `args` rather than just the hydrate string.
- Complexity: Low — one added branch.
- Modern idioms: Yes — the comma-ok shape is the idiomatic Go form for exactly this distinction, and
  `strings.Cut` already backs it.
- Readability: Good. The fatal names the whole argv, which is the diagnostic a reader of a failure
  needs; the surviving comment now describes behaviour rather than intent.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
