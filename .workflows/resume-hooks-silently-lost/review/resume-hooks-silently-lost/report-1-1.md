TASK: resume-hooks-silently-lost-1-1 — The Stale Rule Retains Keys It Cannot Judge (tick-d21108)

ACCEPTANCE CRITERIA:
1. `session.IsTokenShaped` derives its width from `suffixLen` and its charset from `NanoIDAlphabet`; neither value restated as a literal in the predicate
2. `internal/hooks` holds exactly one staleness implementation; `StaleKeys` delegates to it and `CleanStale` calls it directly, proven by a source guard that fails on a `StaleKeys` call inside `CleanStale`
3. A non-token-shaped key absent from the live set is retained by `CleanStale` and not reported by `StaleKeys`, from both call sites (daemon sweep, `portal doctor --fix`)
4. A token-shaped key absent from the live set is still deleted; one present in the live set is still retained
5. An empty (`""`) key is deleted
6. A cycle whose every candidate is retained writes no file and emits no batch summary
7. `TestCleanStaleRemovesExactlyStaleKeys` still compares a non-empty removal set against a non-empty prediction
8. `TestDoctorSummary_FixPathRendersTwo` still asserts `6 of 7 checks passed` pre-repair and `7 checks passed` post-repair, with its seed re-pointed rather than its counts relaxed
9. `portal doctor` reports the stale-hooks check passing and exits 0 with only retained non-token-shaped entries present
10. Both lanes pass with the re-pointed fixtures

STATUS: complete

SPEC CONTEXT:
§3.2 fixes the key shape (exactly the mint's width, drawn from the mint's alphabet) and requires the predicate be *derived* from the generator's own constants rather than restated — the named failure mode being a hardcoded `^[A-Za-z0-9]{6}$` in `internal/hooks` beside a width free to move. §5.2 makes deletion shape-aware (token-shaped absent → delete; not token-shaped → retain untouched on every run; empty → delete) and requires the rule have one implementation every reader routes through, so protection travels with the rule rather than sitting at one call site. §5.4 requires the live set be the *non-empty* token subset while the mass-deletion guard counts pane *rows*. §8.1/§8.3 explain why no migration code ships: the retention itself is the safety property.

Two corrigenda are directly authoritative over this task's wording:
- 2026-08-30 / 2026-09-01: the predicate's home is `internal/nanoid` (a stdlib-only leaf), not `internal/session`; the leaf holds its own `paneTokenWidth` behind `NewPaneTokenGenerator`, and `internal/hooks`' leaf guard now *forbids* the `internal/session` import the task's Do list introduced. This also retired the transitive-dependency concern the task's own fix-tracking flagged (`internal/hooks` reaching `internal/state`/`internal/tmux` through `internal/session`).
- 2026-09-06: the exported/unexported `StaleKeys`/`staleKeys` pair was collapsed to one exported `StaleKeys`, both callers reaching it directly, and the source guard re-pointed accordingly. The re-entrancy reasoning the task cited never applied — `StaleKeys` takes an already-loaded snapshot and acquires nothing.

Judged against the delivered code (the source of truth) plus the corrected spec, every criterion holds in substance. Criteria 1 and 2 are met by their corrected form, not their literal wording.

IMPLEMENTATION:
- Status: Implemented (evolved past the task's letter by sanctioned later phases 6-3, 7-5, 9-11)
- Location:
  - `internal/nanoid/nanoid.go:65-75` — `IsTokenShaped`, deriving length from `paneTokenWidth` (`:58`) and membership from `Alphabet` (`:17`), both by reference, no literal in the predicate body. Both tests are byte-based (`len(s)`, `s[i]`, `strings.IndexByte`), so no multi-byte input can satisfy the pair.
  - `internal/hooks/store.go:248-263` — the single `StaleKeys`: absent-from-live **and** (`key == ""` or token-shaped). Its doc comment (`:239-247`) states the retention and disclaims a mass-deletion guard.
  - `internal/hooks/store.go:298-310` (`CleanStale`, doc comment states "A key it cannot judge is retained untouched") → `:318-358` (`deleteStale`), which reaches the rule at `:332` and takes the zero-removals early return at `:334-336` before both the save and the summary.
  - `cmd/doctor.go:387` — `checkStaleHooks` inherits the rule through the same function, unchanged at its call site.
  - `internal/hooks/cleanstale_staleness_guard_test.go:14-47` — the source guard, in its corrected form: it fails on any surviving unexported `staleKeys` declaration (`:19-21`) and on either reader not calling `StaleKeys` (`:25-26`, `:44-46`).
  - `internal/hooksweep/sweep.go:110-118` — `liveTokensFrom` drops empty tokens before they reach the rule, which is what keeps criterion 5 true in production rather than only in fixtures: were an unstamped pane's empty token admitted, the `""` entry would be permanently unreachable by the reaper.
- Notes: production `IsTokenShaped` callers are `internal/hooks/store.go:258` and the `internal/hookstest` seed self-check; production `StaleKeys` callers are `cmd/doctor.go:387` and `internal/hooks/store.go:332` — verified by grep across all non-test `.go` files. No second implementation and no restated shape anywhere.

TESTS:
- Status: Adequate
- Coverage (every named test in the task's Tests list is present, at its post-move home):
  - Predicate — `internal/nanoid/nanoid_test.go:40-96`: six-char accept, 5/7-char reject, out-of-alphabet table (`:`, `.`, `-`, `_`, space, `!`), empty reject, multi-byte reject (a 6-byte / 3-rune fixture *and* a 6-rune fixture, each asserting its own byte/rune count before use so a fixture that stopped being 6 of anything fails rather than passes), old-format shapes. Plus `:98-121` pins recognition against 200 freshly minted tokens and a one-byte-short token — this is what makes the derivation observable rather than merely stated.
  - Rule — `internal/hooks/store_shape_test.go:13-104`: retention of an unjudgeable key (asserting *both* the empty removal set and an unchanged `StaleKeys` prediction and byte-identical file), deletion of a token-shaped absent key with the live one kept, deletion of the empty key, and the all-retained cycle asserting byte-identical file and no `clean-stale` record. That last check is correctly aimed: the batch summary shares the `clean-stale` message with the per-key lines (`internal/storelog` tests pin it), so one filter covers both halves of criterion 6.
  - Daemon sweep — `internal/hooksweep/sweep_test.go:225-249`, retention across a real `Run` with a non-empty live set plus `AssertHooksFileUnchanged`.
  - `doctor --fix` and read-only `doctor` — `cmd/hook_prune_unjudgeable_retention_test.go:17-55`: the entry survives, no `Pruned stale hook:` line names it, and the plain run exits nil with `✓ stale hooks: no stale hooks`.
  - Guard — the source guard is fatal on an empty scan through `sourceguardtest.ParsePackageSources` → `ParseSources` (`internal/sourceguardtest/parsesources.go:84-87`), so it cannot pass by having stopped looking.
  - Anti-vacuity — `internal/hooks/store_test.go:842-844` fails outright on an empty `StaleKeys` prediction, which is the guard criterion 7 asks for stated as an assertion rather than left to a seed choice.
  - `cmd/doctor_summary_test.go:170-192` retains `6 of 7 checks passed` (exactly once) and the `7 checks passed` suffix, with only the seed moved to `hookstest.ReapableSeedA`. Criterion 8 held.
- Notes:
  - The fixture re-pointing is now routed through `internal/hookstest`'s named seed vocabulary (`ReapableSeed*` / `LiveSeed*` / `UnjudgeableSeed*` / `SubjectSeed*`) rather than the scattered literals the task's Do list enumerated. That is strictly stronger than what was asked: `tokenShapedHookKey` derives its width from a live mint and panics if the result is not token-shaped (`internal/hookstest/hooks.go:130-141`), so a width or charset move carries every fixture with it instead of silently converting a reap fixture into a retention one.
  - Both confounds raised in this task's own fix-tracking are closed in the current tree: the empty-pane-read guard fixtures seed judgeable keys (`internal/hooksweep/sweep_test.go:20-22`, `:423-441`), so the guard rather than the shape rule is what those subtests measure.
  - No over-testing observed. The predicate suite is a table of one-line assertions per boundary, and the rule suite has one subtest per rule arm with no duplicated arms across `internal/hooks`, `internal/hooksweep` and `cmd` — each layer asserts its own surface (the rule, the cycle, the user-facing face).
  - Criterion 10 (both lanes green) is not verifiable by reading; no compile-level inconsistency was found — every helper the reviewed suites call (`readFileBytes`, `enumerating`, `hookstest.AssertHooksFileUnchanged`, `HooksFileBytes`, `StageStore`) is declared and in scope.

CODE QUALITY:
- Project conventions: Followed. `internal/nanoid` is stdlib-only with its own leaf guard; `internal/hooks`' leaf guard (`internal/hooks/leaf_guard_test.go:22-27`) admits `internal/nanoid` and nothing else from the session/tmux tree, which is the corrigendum's arrangement enforced structurally rather than in prose. No new log component or attr key. Unit-lane only, correctly — nothing here builds, spawns or execs a portal binary.
- SOLID principles: Good. The predicate answers one question about a string and knows nothing of hooks; the rule knows nothing of locking or of tmux; the pane/token distinction §5.4 requires is held by `liveTokensFrom` in a third package. One rule, one home, two readers.
- Complexity: Low. `IsTokenShaped` is a guard plus a loop; `StaleKeys` is a set build plus a filtered loop.
- Modern idioms: Yes — `for i := range len(s)`, `maps.Clone`, `strings.IndexByte` over a manual scan.
- Readability: Good. The comments carry the *reasons* that would otherwise be lost — `nanoid.go:52-58` marks the pane-token width as a migration event rather than a tuning knob, `store.go:239-247` says why an unjudgeable key is retained, and `hooksweep/sweep.go:107-109` says why an empty token must never enter the live set.
- Comment accuracy: Verified against the code. `nanoid.go:20-21` ("Nothing persisted is classified by it") holds — `IsTokenShaped` reads `paneTokenWidth`, not `width`. `store.go:332`'s surrounding narrowing comment matches `narrowToSnapshot`. The guard's own header comment (`cleanstale_staleness_guard_test.go:10-13`) describes the arrangement the file actually asserts; the earlier draft that asserted a non-existent lock hold was corrected in-phase.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
