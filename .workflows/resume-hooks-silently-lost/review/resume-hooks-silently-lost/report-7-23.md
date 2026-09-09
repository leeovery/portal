TASK: resume-hooks-silently-lost-7-23 — "The Seed-Key Vocabulary Is Re-Declared In Four Packages" (tick-99dc58, phase 7, implementation-analysis cycle)

ACCEPTANCE CRITERIA:
- [x] `internal/hookstest` is the only declaration site for the named seeds and the two-entry seed body.
- [x] No package re-derives a seed index inline or re-declares a seed name.
- [x] No local named `liveKey` is bound to a reapable seed.
- [x] The two daemon suites seed the same stale command string.
- [x] Every converted assertion judges the same key it judged before — a reapable seed stays reapable, a live seed stays live.
- [ ] Both lanes pass. (Not executable by this reviewer — no test execution. Judged by reading; see TESTS.)

STATUS: complete

SPEC CONTEXT: This is a phase-7 implementation-analysis task, so its authority is its own body rather than the
specification. Its subject is the shared hook-key **seed vocabulary** the suites are written against — the
distinction the specification's own retention rule turns on: a token-shaped key (`nanoid.IsTokenShaped`) absent
from the live pane set is reapable, while a legacy `<session>:<window>.<pane>` key is unjudgeable and retained
forever, because reaping it would be data loss with no route back. A fixture that names a key "live" while
binding it to a key from the reapable half stops measuring liveness and starts measuring the retention rule —
which is exactly the defanging this task removes.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hookstest/hooks.go:167-199` — the single declaration site: the reapable half
    (`ReapableSeedA`…`ReapableSeedD`), the live half (`LiveSeedA`…`LiveSeedC`), the unjudgeable half
    (`UnjudgeableSeedA`…`UnjudgeableSeedC`) and the role-free `SubjectSeed*` set, each documented for what the
    role means rather than for which index it wraps.
  - `internal/hookstest/hooks.go:201-209` — `StaleHookSeed`, the exported two-entry stale-beside-live
    `hooks.json` body, keyed on `ReapableSeedA` / `LiveSeedA`.
  - `internal/hookstest/hooks.go:129,159` — the constructors behind the seeds are now unexported
    (`tokenShapedHookKey`, `unjudgeableHookKey`), which makes "no package re-derives an index" compiler-enforced
    rather than convention. (The unexporting landed after this task's own commit `d8ead3e7`; at that commit the
    conversion was already complete — verified by `git grep 'ReapableHookKey(' d8ead3e7` returning no hit
    outside `internal/hookstest`.)
  - Consumers converted: `cmd/hookkey_vocabulary_test.go` (local `reapableSeedA`…`D` / `liveSeedA`…`C` /
    `unjudgeableSeedA`…`B` and the local `staleHookSeed` all deleted; the file now holds only bodies, rows and
    seam fakes, and its header at lines 1-8 states where the seeds live),
    `internal/hooks/store_test.go`, `internal/hooks/cleanstale_snapshot_test.go:26`,
    `internal/hooks/store_shape_test.go:15`,
    `cmd/state_daemon_hook_cleanup_integration_test.go:100-103` (package `cmd_test`, previously re-deriving
    indices 0/1) and `cmd/bootstrap/transient_listpanes_helpers_integration_test.go:108-109`.
  - Daemon fixtures reconciled onto one body: `cmd/state_daemon_hook_cleanup_test.go:68` and
    `cmd/state_daemon_run_test.go:562` both stage `hookstest.StaleHookSeed`; the two divergent stale command
    strings collapsed to `cmd-gone`.
- Notes: The two `liveKey` locals bound to `ReapableHookKey(0)` are gone. Every remaining `liveKey` in the tree
  (`cmd/noncontiguous_window_reboot_integration_test.go`, `cmd/bootstrap/phase5_integration_test.go`,
  `internal/restore/session.go` and siblings) is a pane-key / FIFO identifier with no relation to hook seeds.

  AC5 verified by reading the conversion diff rather than trusting it. `cmd`'s local vocabulary mapped 1:1 onto
  the exported names at identical indices (`reapableSeedA`…`D` = 0-3, `liveSeedA`…`C` = 4-6), so every `cmd`
  conversion is value-preserving. The four value changes are all in `internal/hooks` and the daemon integration
  fixture, and each preserves the role its assertion needs:
  - `cleanstale_snapshot_test.go`: `liveKey` 0→`LiveSeedA` (4), `staleKey` 1→`ReapableSeedA` (0),
    `lateKey` 2→`ReapableSeedB` (1) — all token-shaped before and after, all mutually distinct.
  - `store_shape_test.go`: same live/stale move; `retainedKey` stays `UnjudgeableSeedA`.
  - `state_daemon_hook_cleanup_integration_test.go`: the pane the fixture *stamps* now carries `LiveSeedA`
    instead of a reapable-half key — the correction the task was written for. The setup-collision guard
    (`liveHookKey == hookstest.ReapableSeedA` → fatal) survives the conversion.

  Role consistency holds across the wider tree, not just the converted files: every fixture staging
  `StaleHookSeed` drives an enumeration reporting `LiveSeedA` as live
  (`cmd/state_daemon_hook_cleanup_test.go:23` `livePaneRowOut`, consumed at :42/:69/:100/:132/:183 and by
  `cmd/state_daemon_run_test.go:566`), so the seed's documented outcome — reap `ReapableSeedA`, retain
  `LiveSeedA` — is the outcome those fixtures actually measure.

TESTS:
- Status: Adequate
- Coverage: The task correctly specifies no new behavioural test (a vocabulary consolidation with no production
  change). The vocabulary itself is nonetheless guarded by `internal/hookstest/hooks_test.go:37-75`, which
  enumerates every named seed by name and asserts: token shape for the judgeable set, authorship at the live
  pane-token width, non-token shape for the unjudgeable set, and mutual distinctness across the whole
  vocabulary. That last assertion is the one that matters here — two seeds collapsing onto one value is the
  failure mode that would silently defang a reap-vs-retain fixture, and it now fails loudly. The mint-side
  panics in `tokenShapedHookKey` (`hooks.go:132,139`) and `fitPrefix` (`hooks.go:148`) close the same gap from
  the other side: an id charset or width move panics rather than turning a reap fixture into a retention one.
  The existing subjects verify the conversion: the daemon cleanup/tick suites, `internal/hooks`'s shape and
  snapshot suites, and the destructive daemon and transient-listpanes integration fixtures.
- Notes: Not over-tested — the vocabulary test asserts four distinct properties over one enumerated set, with
  no duplication between subtests. Test execution was not attempted (out of this reviewer's remit); the "both
  lanes pass" criterion is unverifiable here. Reading finds nothing that would break either lane: no orphaned
  local declarations remain (a missed one would be a compile error, not a silent pass), and the converted files'
  imports are all still consumed (`cmd/hookkey_vocabulary_test.go` no longer imports `hookstest` and no longer
  references it).

CODE QUALITY:
- Project conventions: Followed. `internal/hookstest` is a test-only package outside `_test.go` so any package
  can import it, with production import prohibited — the `internal/hooks` suites reach it from `package
  hooks_test` (verified across all 14 `internal/hooks/*_test.go` files), so the `hookstest → hooks` edge creates
  no cycle. The architecture row in `CLAUDE.md` describing the seed vocabulary matches the code as written.
- SOLID principles: Good. One package owns the naming rule; the constructors that could reintroduce a
  competing vocabulary are unexported.
- Complexity: Low.
- Modern idioms: Yes. `sync.OnceValue` for the width read; seeds derived from the mint rather than restated.
- Readability: Good. Each half of the vocabulary is documented for the role it plays, not for the index it
  wraps, which is what makes a mis-named fixture visible on sight.
- Issues: None found.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
