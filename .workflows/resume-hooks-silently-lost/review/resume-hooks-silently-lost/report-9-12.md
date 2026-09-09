TASK: resume-hooks-silently-lost-9-12 — The Hook-Staleness Cycle Is A Self-Contained Domain Living In cmd (lift the cycle into `internal/hooksweep`)

ACCEPTANCE CRITERIA:
- [x] `cmd/run_hook_stale_cleanup.go` no longer exists; `cmd` holds only the two call sites and the outcome rendering.
- [x] The new package binds the `hooks` component itself and emits the whole cycle under it.
- [x] Both copy tables and `phraseFor` sit in `cmd` beside `reportSkippedPrune` and `staleHooksNotEvaluable`, keyed on the exported reason type.
- [x] The daemon's throttled sweep and `portal doctor --fix` produce byte-identical output and byte-identical log lines to before the move.
- [x] Every existing stand-down, phrase, coverage and copy guard passes, re-pointed at the new home.

STATUS: complete

SPEC CONTEXT:
The specification's §5 work (component C) makes the stale sweep shape-aware, names what it deleted, gives every declined cycle one grep-able line shape (`op=clean-stale-skipped`, `via=internal`, `reason=<one of six>`), and requires the sweep and the read-only diagnosis to take the same `@portal-restoring` gate so neither judges an entry the other protects. The spec locates that cycle at `cmd/run_hook_stale_cleanup.go` and states "no new component binding is needed — `cmd` already holds one for `hooks`". This task deliberately re-homes the cycle to `internal/hooksweep` with its own `log.For("hooks")` binding. That divergence is a prose-location divergence only: the emitted component is unchanged (`hooks`), the message/attr shapes are unchanged, and the per-package binding rule is the one CLAUDE.md already states (the `theme` component spans two packages by the same rule). CLAUDE.md was amended in the same commit to carry the new package's architecture row, and the shared review context states a phase 9 task's authority is its own body. No behavioural spec requirement is lost.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hooksweep/reason.go:1` (package doc), `:12` (`Reason`), `:18-25` (the six constants), `:35` (`Reasons`)
  - `internal/hooksweep/standdown.go:12` (`standDownMsg`), `:22` (`StandDown`, unexported fields), `:35` (`emit`, unexported), `:54` (`declinedError`)
  - `internal/hooksweep/sweep.go:14` (`var logger = log.For("hooks")`), `:20-24` (message consts), `:30` (`PaneHookLister`), `:37` (`Reader`), `:45` (`View`), `:57` (`Outcome`), `:71` (`StalenessStandDown`), `:85` (`JudgeAgainstLivePanes`), `:152` (`Run`), `:174` (`declinedSweep`), `:199` (`standDownOutcome`)
  - Call sites left in `cmd`: `cmd/state_daemon.go:214` (daemon throttled sweep) and `cmd/doctor.go:201` (`--fix`)
  - Rendering left in `cmd`: `cmd/doctor.go:236` / `:247` (the two copy tables, keyed on `hooksweep.Reason`), `:260` (`phraseFor`), `:266` (`reportSkippedPrune`), `:352` (`staleHooksNotEvaluable`)
  - `cmd/run_hook_stale_cleanup.go` deleted (confirmed absent from the working tree; commit a4898f41 removes 332 lines)
- Notes:
  - The move is faithful line-for-line against `git show a4898f41^:cmd/run_hook_stale_cleanup.go`: same control flow, same branch order in `declinedSweep`, same levels (`declineDebug` for restoring/marker-read-failed, `declineWarn` for the rest), same attrs (`op`/`via=internal`/`reason` + the per-case extra), same message strings (the old inline literals `"stale-hook cleanup counts"` / `"stale-hook cleanup removed"` are now named consts with identical values). Byte-identity of the log lines therefore holds by construction, and the component is unchanged because `log.For("hooks")` resolves the same component wherever it is bound.
  - The exported surface is exactly what the two callers need: `Reason` + its constants + `Reasons`, `Outcome`, `Reader`/`PaneHookLister`, `Run`, plus `StalenessStandDown`/`JudgeAgainstLivePanes` and `View` for `checkStaleHooks`'s read-only count. `StandDown`'s fields and `emit` stay unexported, which is a strengthening: `cmd` can now read a decline's reason (`Reason()`/`Declined()`) but structurally cannot emit one, which is what the type's own doc claims and what the diagnosis must not do.
  - Nothing cobra-shaped was carried: `hooksweep` imports only `internal/{hooks,log,state,tmux}` and no `cmd` symbol. No production or documentation reference to the old file or its symbols survives outside `CHANGELOG.md` (release-owned) and historical `.workflows/` records.
  - `hooksweep.PaneHookLister` duplicates the name and doc of `cmd.PaneHookLister` (`cmd/hooks.go:25`), which `hook list`'s location column still needs. Two consumer-side one-method interfaces is the idiomatic Go shape here, not a leftover.
  - The optional Do-list item ("add a leaf-style deps guard if the dependency set warrants one") was correctly not taken: `hooksweep` is not a leaf.

TESTS:
- Status: Adequate
- Coverage:
  - All four named tests exist and assert what they claim: `internal/hooksweep/sweep_move_test.go:17` (one cycle, what it removed, the live key survives), `:47` (a table standing the cycle down under all six reasons, with a closing loop over `Reasons` that fails a reason no case drives), `:134` (the whole cycle — counts, removed, stand-down and failure messages — captured under `component=hooks`, with every record asserted on that component), and `cmd/doctor_fix_hook_prune_move_test.go:15` (the three literal `doctor --fix` lines, written out rather than composed from the tables they are rendered from, so a re-homing that changed the words cannot agree with itself and pass).
  - The cycle's own suites moved wholesale and were re-pointed, not thinned: lock-timeout (`internal/hooksweep/lock_timeout_test.go`), read failure at both of `CleanStale`'s reads (`read_failure_test.go`), snapshot-before-enumeration (`snapshot_order_test.go`), the decline-reason-rides-the-error pair plus its AST guard (`decline_error_test.go`, `decline_error_guard_test.go`), and the whole `TestRunCycle` / hazard-guard / restore stand-down / row-count-not-token-count / nothing-persisted body (`sweep_test.go`).
  - The reason-enumeration source guard moved with the vocabulary (`internal/hooksweep/reason_enumeration_guard_test.go`), keeps its own negative case over synthetic source, and keeps its scanned-nothing tripwire. The phrase-coverage and copy guards stayed in `cmd` re-keyed onto `hooksweep.Reason` (`cmd/doctor_stand_down_phrase_guard_test.go:47`, `:131`, `:299`), so a reason added in the new package with no `cmd` copy still fails.
  - The user-facing halves stayed in `cmd`: the locked-prune diagnosis (`cmd/hook_prune_locked_test.go`), unjudgeable-key retention across `--fix` and a plain diagnosis (`cmd/hook_prune_unjudgeable_retention_test.go`), and the caller-parity case proving the daemon and `--fix` emit the identical component+message sequence and that neither adds a line of its own (`cmd/hook_prune_one_component_test.go:42`, `:66`, `:85`).
  - The moved suites' assertions on messages are positive (`Only(...)`, `count == 1`), so a message value drifting from the new consts fails rather than passing vacuously; `cmd/hookkey_vocabulary_test.go:218-221` deliberately re-spells the two observable messages with a comment saying why, and asserts on them positively.
- Notes: no over-testing found — the split avoids duplicating the same subject on both sides of the boundary (the cycle's internals are pinned once in `hooksweep`, the rendered face once in `cmd`), and the two same-named `TestUnjudgeableHookKeyRetention` functions assert genuinely different subjects (file contents after a sweep vs the two commands' output/exit).

CODE QUALITY:
- Project conventions: Followed. Per-package component binding matches CLAUDE.md's stated rule and the `theme` precedent; CLAUDE.md gained the `hooksweep` architecture row in the same commit; every new test is unit-lane and touches no real tmux server, no daemon and no binary; `hookstest`/`logtest`/`sourceguardtest` are used by name rather than re-implemented.
- SOLID principles: Good. The package now owns one responsibility (judging staleness and the conditions forbidding judgement) and the callers own only rendering; the seams are the two small consumer-side interfaces.
- Complexity: Low — unchanged from the pre-move body.
- Modern idioms: Yes.
- Readability: Good. The doc comments were re-worded for the new home and each claim holds against the code (the emission claim on `logger`, `View.PaneRows`'s "meaningful only where `Enumerated` is set" — the only non-enumerated return always carries a decline, and `checkStaleHooks` returns before reading it; `StandDown`'s "emission is the sweep's" — now structurally true).
- Issues: none rising to a finding. Two duplications were considered and dismissed as extraction preferences rather than defects: the `stubReader` / `stubStaleSweepReader` fakes (both packages independently need a fake of the same interface, and `hookstest`'s stated role is hooks.json staging, not tmux pane fakes), and the `forEachValueSpec` / `parseSyntheticSource` AST test helpers now present in both guard files (cross-package test helpers, each guard reading its own package's sources).

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
