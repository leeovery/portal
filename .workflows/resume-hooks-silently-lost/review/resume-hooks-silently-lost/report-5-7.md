TASK: resume-hooks-silently-lost-5-7 — The Seam Fakes And The Stale Seed Go To Their Declared Home (ad hoc consolidation task added to phase 5; the planning file's phase 5 declares `total: 5`, so this task's authority is its own body)

ACCEPTANCE CRITERIA:
- `sideEffectPaneLister` is gone and `stubAllPaneLister` carries an optional `during` hook
- All four named fakes are declared in `cmd/hookkey_vocabulary_test.go`
- The three hook-key seam fakes remain three distinct types — none merged
- One `staleHookSeed` literal remains; the three copies are gone and all five uses re-point
- No assertion changes and no test changes its verdict
- `go test ./...` and `go test -tags integration -p 1 ./cmd/...` pass

STATUS: issues_found (0 blocking; one orphaned test-fake field)

SPEC CONTEXT:
The specification's §9.3 ("Existing tests to re-point or retire") names the three `cmd`-side enumeration-seam fakes by their then-current homes — `stubAllPaneLister` at `cmd/bootstrap_production_test.go:91`, `fakeHookLister` at `cmd/doctor_test.go:802`, `recordingHookKeyLister` at `cmd/run_hook_stale_cleanup_test.go:19` — as files that must follow the seam change; it states no requirement about where a fake is declared, so this task's relocation is house-keeping the spec neither asks for nor forbids. The substance the fakes serve is §5 (stale cleanup, shape-aware staleness, the restore stand-down) and §6.3 (snapshot-before-enumeration ordering) — the seed body the task single-sources is the stale-beside-live fixture those cases measure against. §9.2's "Lost update" row pins the case the folded `during` hook exists for: an entry registered after the sweep's snapshot and after the pane enumeration must survive.

IMPLEMENTATION:
- Status: Implemented; subsequently carried forward (renamed and re-homed) by later phases, with every property this task delivered intact.
- Location (delivery, commit d316c794):
  - `cmd/hookkey_vocabulary_test.go` — gained `staleHookSeed`, plus the moved declarations of `stubAllPaneLister` (now carrying `during func()`), `mockKeyResolver`, `paneStampCall`/`recordingPaneStamper` and `stampedPane`.
  - Declarations removed from `cmd/bootstrap_production_test.go`, `cmd/hooks_test.go`, `cmd/hooks_pane_token_test.go`, `cmd/hooks_write_lock_test.go`; `sideEffectPaneLister` deleted from `cmd/hook_sweep_snapshot_order_test.go` with its 3 call sites re-pointed; `lockedStaleSeed` and the two `staleSeed` locals deleted from `cmd/hook_sweep_lock_timeout_test.go` and `cmd/run_hook_stale_cleanup_test.go`.
- Location (current tree): `cmd/hookkey_vocabulary_test.go:93-114` (`stubStaleSweepReader`, the renamed descendant of `stubAllPaneLister`, now typed against `hooksweep.Reader` and carrying a `calls` counter), `:120-129` (`mockKeyResolver`), `:133-155` (`paneStampCall`/`recordingPaneStamper`), `:159-170` (`stampedPane`); the stale seed is single-sourced one package further out at `internal/hookstest/hooks.go:206` (`StaleHookSeed`), with its comment naming `ReapableSeedA`/`LiveSeedA` as the two keys it stages.
- Criterion-by-criterion:
  - `sideEffectPaneLister` gone: confirmed — no occurrence anywhere in the tree; the fold is behaviour-preserving (the deleted fake returned `l.rows, nil` and `restoringOption(false, nil)`; the merged fake returns `s.rows, s.err` and `restoringOption(s.restoring, s.restoringErr)`, whose zero values are exactly those at all three re-pointed sites).
  - Four fakes in the declared home: confirmed at delivery and in the current tree.
  - Three hook-key fakes still distinct: confirmed — `mockKeyResolver`, `recordingPaneStamper` and `stampedPane` are three separate types; the fixed-key resolver and the poisoned stamper are still what `paneKeyPathSeams`/`assertNoPaneTmuxCalls` (`cmd/hookkey_vocabulary_test.go:183-195`) discriminate on.
  - One stale-seed literal: confirmed. The remaining multi-entry `cmd-gone` bodies in `cmd/state_daemon_hook_cleanup_test.go` (:38, :96, :151) and `cmd/state_daemon_run_test.go` (:597, :630, :661) are single-entry seeds, not copies of the stale-beside-live body. The plan's "five uses" undercounts the actual re-points (3 in `cmd/hook_sweep_lock_timeout_test.go`, 10 in `cmd/run_hook_stale_cleanup_test.go`); the substance of the criterion — three literals gone, one home, every use re-pointed — is met.
  - Banked work respected: the four-way lister merge the task explicitly excluded was not attempted here.
- Notes: the two import removals the diff performed (`fmt` from `cmd/hook_sweep_lock_timeout_test.go`, `internal/tmux` from `cmd/hook_sweep_snapshot_order_test.go`) are safe — neither package is referenced anywhere else in those files at that commit.

TESTS:
- Status: Adequate — this is a declaration-relocation and literal-consolidation task, correctly authored with no new test.
- Coverage: every touched suite keeps its assertions. The only assertion-adjacent edits in `cmd/run_hook_stale_cleanup_test.go` replace the local alias `staleKey` with `reapableSeedA`, which is the value it was assigned; no verdict changes. The task's own verification instrument — mutation-testing the folded `during` invocation against the snapshot-order suite — is the right check for a fold whose only behavioural content is that one branch.
- Notes: I did not execute the suites (reading only). The delivered ordering `during()` before returning `s.err` matters only for the three re-pointed sites, none of which arm `err`, so the fold cannot have changed a verdict.

CODE QUALITY:
- Project conventions: Followed. Unit-lane test scaffolding only, no lane change; no `t.Parallel()`; the package-level `staleHookSeed` is an immutable string so sharing it across tests carries no cross-test coupling; the file's header comment is kept true by the move ("the seam fakes that answer with them").
- SOLID principles: Good — the fold is a strict superset (an optional nil-checked hook), and the task's refusal to merge the three hook-key fakes preserves the discriminations several cases rest on.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. Each relocated fake arrived with a comment saying what it is for and why its shape matters (the fixed key, the poisoned error, the stamp→resolve round-trip), which is more than the scattered originals carried.
- Issues: one orphaned field in the current tree, below.

BLOCKING ISSUES:
- None.

FINDINGS:
- [in-scope] [contained] cmd/hookkey_vocabulary_test.go:98 — the `during func()` field this task folded onto the fake (read at `:104-105`, described at `:89-92`) now has no setter in `cmd`: the three snapshot-order cases it was folded for moved to `internal/hooksweep/snapshot_order_test.go:21,48,76` (plus `read_failure_test.go:26`), which drive that package's own `stubReader` (`internal/hooksweep/helpers_test.go:67`). Repo-wide, `during` is set at exactly those four sites and read at exactly two (`cmd/hookkey_vocabulary_test.go:104` and `internal/hooksweep/helpers_test.go:73`). Delete the field, its two-line nil-checked invocation and the trailing clause of the doc comment that describes it from the `cmd` copy; the sibling `calls` counter must stay (it is asserted at `cmd/doctor_test.go:1080,1102`), and no `cmd` call site changes because none of them name `during`. — FAILS: the `cmd` fake advertises a concurrency-window seam no test in that package uses, so the branch is never executed in the unit lane that owns the file and a future edit to it would go unobserved, while a reader looking for the case it serves is pointed at a package where it no longer lives.
