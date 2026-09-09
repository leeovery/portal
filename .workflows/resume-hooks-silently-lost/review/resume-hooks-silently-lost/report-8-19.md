TASK: resume-hooks-silently-lost-8-19 — "Two Daemon Fixtures Assert The Stale Seed Is Absent Without First Asserting It Was Present" (phase 8, implementation-analysis / near-miss)

ACCEPTANCE CRITERIA:
- Mutating `StaleHookSeed`'s stale half to an unjudgeable key fails both fixtures (verified by hand, then reverted).
- No `cmd` suite hand-rolls a hook-key literal.
- One spelling for the stale command across the two suites and the shared seed.
- The `hookstest` self-test covers every index the named vocabulary mints.

STATUS: complete

SPEC CONTEXT: The specification says nothing about the test seed vocabulary (grep for `hookstest` / `seed vocabulary` / `ReapableSeed` in the spec returns nothing) — this is a phase-8 self-authored quality task, so its authority is its own body, per the shared verifier context. The substantive backdrop it protects is CLAUDE.md's "Resume hooks" rule: a persisted key is stale only when absent from the live token set *and* judgeable by shape (`nanoid.IsTokenShaped` or empty); an unjudgeable key is retained forever, and deleting one is data loss. That asymmetry is exactly what makes an "absent afterwards" assertion with no presence precondition silently satisfiable — re-point the seed at an unjudgeable key and the sweep correctly retains it, but the fixture's absence check never notices because the key it names was never staged.

IMPLEMENTATION:
- Status: Implemented (commit `c7f7bd50`).
- Location:
  - `cmd/hookkey_vocabulary_test.go:197-211` — the new `assertHookKeysStaged(t, store, keys...)` precondition helper: loads the staged store through `hooks.ViaInternal` and `t.Fatalf`s naming any key the seed did not hold.
  - `cmd/state_daemon_hook_cleanup_test.go:75` and `cmd/state_daemon_run_test.go:573` — the two named fixtures now assert `hookstest.ReapableSeedA` and `hookstest.LiveSeedA` present before the sweep runs. Both stage `hookstest.StaleHookSeed` (`cmd/state_daemon_hook_cleanup_test.go:68`, `cmd/state_daemon_run_test.go:562`), which is the indirection that created the near-miss.
  - `internal/hookstest/hooks.go:129` / `:159` — `ReapableHookKey` / `UnjudgeableHookKey` unexported to `tokenShapedHookKey` / `unjudgeableHookKey` (Do item 5). No reference to the old exported names survives anywhere in the tree — a repo-wide grep returns only two unrelated *test function* names (`TestUnjudgeableHookKeyRetention` at `cmd/hook_prune_unjudgeable_retention_test.go:16` and `internal/hooksweep/sweep_test.go:224`).
  - `internal/hookstest/hooks.go:195-198` — the new `SubjectSeedA..D` half (indices 7-10), the vocabulary route for the previously hand-rolled `cmd` literals.
- Notes: I verified the mutation criterion structurally rather than by re-running the experiment (test execution is out of scope for this role). Re-pointing `StaleHookSeed`'s stale half (`internal/hookstest/hooks.go:206-209`) at any other key — `UnjudgeableSeedA` or another token-shaped seed alike — makes `ReapableSeedA` absent from the staged file, so `assertHookKeysStaged` fatals in both fixtures before the sweep is even called. The precondition is therefore sound against both directions of the mutation, and it also covers the live half.

  Criterion 2 verified by sweep. `aaa111`, `tok999`, `ghost9`, `tok000` and `tok123` are all gone from `cmd` (repo-wide grep leaves them only in `internal/hooks/lock_write_test.go`, which is not a `cmd` suite and whose subjects — lock timeouts, denied writes, removals — carry no staleness class an id-width move could flip). Every `mockKeyResolver{key:…}`, `PaneHookRow{Token:…}`, `--pane-key` argument and `hooksBody(...)` argument in `cmd` now names a `hookstest` seed; the one remaining `Token: ""` at `cmd/hooks_test.go:142` is the deliberate unstamped-pane case, not a key. The legacy-shaped literals still present in `cmd` (`cmd/state_hydrate_exec_log_test.go`, `cmd/state_hydrate_file_missing_log_test.go`, `cmd/state_daemon_test.go:772`) sit outside the hazard the criterion names: they contain `:` and `.`, neither of which is in `nanoid.Alphabet`, so no width or charset move can ever reclassify them, and the hydrate path does a literal lookup rather than a staleness judgement.

  Criterion 3 verified: `cmd-stale` appears nowhere in the tree; `cmd-gone` is the single spelling across `internal/hookstest/hooks.go:207`, the two daemon suites, and the `internal/hooksweep` suites the sweep later moved to.

  Criterion 4 verified by enumeration: the vocabulary mints 14 named seeds (`ReapableSeedA-D`, `LiveSeedA-C`, `UnjudgeableSeedA-C`, `SubjectSeedA-D`), and `internal/hookstest/hooks_test.go:16-34` names all 14 across `tokenShapedSeeds` (11) and `unjudgeableSeeds` (3). The old `for n := range 4` prefix sweep is gone.

  Naming/derivation checked: `fitPrefix(paneTokenWidth()-disambiguatorWidth)` = `fitPrefix(4)` and `seedKeyPrefix` is `"seed"` (4 bytes), so the seeds render `seedaa`…`seedak`. `SubjectSeedA/B/C` are `seedah`/`seedai`/`seedaj`, which preserves the lexicographic order `cmd/hooks_test.go:94-104`'s sorted-listing assertion depends on. `livePaneRowOut` (`cmd/state_daemon_hook_cleanup_test.go:23`) is derived from `hookstest.LiveSeedA`, so the reap/retain split in both guarded fixtures is driven by the vocabulary end to end.

TESTS:
- Status: Adequate.
- Coverage: The task's named test surface is delivered in substance. `"it mints a token-shaped key for every named seed index"` exists verbatim (`internal/hookstest/hooks_test.go:38`), beside the width, unjudgeable-shape and distinctness sweeps (`:45`, `:58`, `:66`) — all four now iterate the named maps rather than an index prefix, so none of them can pass by having stopped looking at index 4. The `"it seeds the stale key before the sweep runs"` case is delivered as an inline precondition inside the two existing fixtures rather than as a separately named subtest. That is the correct shape, not a shortfall: the precondition has to run inside the same function that stages the store and drives the sweep, and a sibling subtest could not observe it.
- Notes: No coverage lost. The dropped `"successive n give distinct keys"` subtest (n over 0-7 across both constructors) is strictly subsumed by `"every named seed is distinct"`, which now covers all 14 named seeds including the previously unnamed indices 7-10. The added `store.Load` in each fixture is inert with respect to what those fixtures measure: it issues no commander call (so `fc.callsContaining("list-sessions")` at `cmd/state_daemon_run_test.go:588` is untouched), creates nothing (a read passes no `O_CREATE`), and lands before `maybeRunHookCleanup` in the one fixture that timestamps `beforeCall` — which only makes the `lastCleanup.Before(beforeCall)` assertion at `cmd/state_daemon_hook_cleanup_test.go:89` more, not less, likely to hold for the right reason. Neither fixture asserts on a log sink, so a possible `load-unlocked` DEBUG breadcrumb cannot pollute anything.

  Blast-radius check on the seed re-point: I enumerated all 26 `StaleHookSeed` call sites. Beyond the two this task guards, none of them carries the same near-miss — the `cmd/hook_prune_*` fixtures assert on a reported count (`"1 stale hook entry"`, `cmd/hook_prune_locked_test.go:23`) or on the file being byte-unchanged (`cmd/hook_prune_single_report_test.go:61`), and the `internal/hooksweep` fixtures assert on enumeration call counts and decline reasons. All of those fail loudly on a re-point rather than silently passing. The remaining absence-after-sweep assertions in the two touched files (`cmd/state_daemon_hook_cleanup_test.go:111`, `cmd/state_daemon_run_test.go:598-670`) seed inline from `hookstest.ReapableSeedA` in the same function body, so the seeded key and the asserted key are one expression and cannot drift apart.

CODE QUALITY:
- Project conventions: Followed. Both touched files are unit-lane (`package cmd`, no build tag), consistent with CLAUDE.md's lane rule — nothing here builds or spawns a binary. `internal/hookstest` remains test-only and its stdlib-plus-`harnesstest`/`hooks`/`nanoid`/`xdg` dependency shape is unchanged. The CLAUDE.md `hookstest` architecture row was updated in the same commit to describe the delivered vocabulary, and at HEAD it still matches the code (named `ReapableSeed*` / `LiveSeed*` / `SubjectSeed*` / `UnjudgeableSeed*` plus `StaleHookSeed`, constructors unexported).
- SOLID principles: Good. The precondition is one helper with one job; unexporting the two constructors narrows `hookstest`'s surface to the named vocabulary, which is what makes "no package re-derives a seed index inline" structural rather than aspirational.
- Complexity: Low. The helper is a load plus a loop; the self-test traded two index loops for two named maps.
- Modern idioms: Yes — `for name, key := range m`, `sync.OnceValue` for the width probe (pre-existing), variadic `keys ...string`.
- Readability: Good. `assertHookKeysStaged`'s doc comment states the failure mode it exists to prevent rather than restating the code, and the seed var block explains why the reapable and live halves are kept apart rather than sharing indices.
- Issues: None. Comment accuracy checked against the code: `seedKeyPrefix`, `disambiguatorWidth`, `tokenShapedHookKey`, the seed var-block comment ("the only route to one … because the constructors behind it are unexported") and `assertHookKeysStaged`'s comment all hold. Placement of `assertHookKeysStaged` in `hookkey_vocabulary_test.go` rather than `testhelpers_test.go` sits slightly against that file's own header ("Staging … lives in testhelpers_test.go"), but the helper is an assertion about the vocabulary's keys rather than a staging fixture, and nothing observable turns on it — noted, not reported.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
