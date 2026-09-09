# Implementation Review: Resume Hooks Silently Lost

**Plan**: resume-hooks-silently-lost
**Verdict**: Pass

## Summary

The specified defect is closed on all four fronts the specification named. A resume hook is now keyed on the pane's own durable `@portal-pane-id` token — no session component, no coordinates — so no tmux rearrangement, rename or window renumbering can move it, and restore re-stamps each saved token before arming so the key survives the reboot gap. Registration probes the pane's existence with `show-options -p` naming no option, so a `$TMUX_PANE` that answers for nothing fails on exit status before anything is minted, stamped or written; the `:.` silent success is gone, and `hook rm` now exits 0 iff it removed an entry. The stale sweep judges only keys whose shape it can judge — an unconvertible old-format entry is retained permanently rather than reaped — names each deletion at INFO with the command it took, and stands down under six distinct, greppable reasons. The unlocked read-modify-write on `hooks.json` is closed by a sidecar `flock` with a bounded acquire that degrades reads and refuses writes.

Verification covered all 190 completed plan tasks across ten phases (phases 1–5 the specified bugfix; 6–9 the implementation phase's own analysis cycles; 10 a guard-cache fix), with 22 deliberately cancelled tasks excluded and disclosed below. No task was found incomplete and no behaviour was found broken — there are no blocking issues. Thirty-five findings were raised, every one of them assessed valid against the current tree; 27 were contained enough to finish in this session and have been applied, verified together and committed with both lanes green.

## QA Verification

### Specification Compliance

The implementation aligns with the specification as amended. Ten corrigenda were recorded during implementation and are authoritative over the spec body; the substantive ones move the id vocabulary to a stdlib-only `internal/nanoid` leaf (with the pane-token width split from the session-suffix width, so `hooks.json`'s key-recognition contract is decoupled from name generation), give `CleanStale` ownership of its own snapshot-then-enumerate ordering rather than trusting a caller, split the sweep's stand-down reasons from three to six, and add a second, shorter lock bound for the advisory pre-read. Each divergence was checked against the record by its verifier and found to preserve or strengthen the property the spec argued for.

Two divergences from the plan's literal wording were judged sound rather than reported as losses, both because the spec's own corrigenda sanction them: the shape predicate lives beside the generator in `internal/nanoid` rather than `internal/session`, and `StaleKeys` is one exported function rather than an exported/unexported pair.

### Plan Completion

- [x] Phase acceptance criteria met — all ten phases
- [x] All tasks completed or deliberately discarded — 190 completed and verified; 22 cancelled during the implementation phase's own analysis cycles and excluded from review by design. Cancelled: `9-22` … `9-28`, `9-32` … `9-34`, `9-37` … `9-39`, `9-41` … `9-48`, `9-50` (the phase-9 consolidation proposals the implementation phase declined)
- [x] No scope creep — the phase 6–9 analysis tasks are the implementation phase's own consolidation work, not new product surface

### Code Quality

No blocking issues. The findings that survived assessment are overwhelmingly *claims the code falsifies* rather than defects in behaviour: comments and CLAUDE.md rows that a later phase's refactor left describing a shape the tree had moved past. That pattern is the signature of a ten-phase plan whose later phases re-homed code the earlier ones documented — the sweep moving into `internal/hooksweep`, the id vocabulary into `internal/nanoid`, the guard primitives into `sourceguardtest`.

Three findings named something with a real-system consequence rather than a stale claim, and all three are fixed: a test seam case that could write `sessions.json` into the repo's `cmd/` directory under the regression it guards (breaching the no-writes-outside-temp invariant), a `~10ms` timing margin in a saver-readiness fixture that a loaded machine could cross, and a guard whose union-over-collision behaviour had lost its only test.

### Test Quality

Tests adequately verify requirements. Coverage is unusually strong on the properties that matter here: the moved-pane durability suite asserts after each individual move with a "proved it moved" guard; the lost-update case exercises the `AtomicWrite` inode swap specifically, which a lock taken on `hooks.json` itself would fail; the snapshot-before-enumeration ordering is pinned structurally (`lister.calls == 0`) rather than by rendered output, so a reorder fails rather than passing quietly.

Four verifiers noted that criteria naming a suite run or a repeated-run stability check could not be settled by reading, and said so rather than claiming them.

### Blocking Issues

None.

## Findings

### Corrected in this session

**27 actions applied, 0 skipped, 0 reverted.** Committed as `f81b2116` across 40 files. The verifier read the complete diff, settled two applier-reported partials (an `internal/capture/capture_test.go` list entry and a redundant local `t.Setenv` in `cmd/testmain_home_poison_test.go`, both clauses targeting files outside their action's declared file list), repaired one piece of real damage — a replacement `.golangci.yml` rationale that swapped a false claim about `os.Setenv`'s error modes for a differently false one — and flattened seven insert-scarred paragraphs left by piecemeal editing.

**Suite: green.** `go test ./...` all packages ok; `go test -tags integration -p 1 ./...` all packages ok, exit 0; `golangci-lint run` 0 issues; `go vet` clean in both lanes.

| Action | Summary | Files | Source |
|---|---|---|---|
| A1 | `sourceguardtest` docs misdescribe the `InDir` directory | `internal/sourceguardtest/packagedeps.go` | 10-1-1 |
| A2 | CLAUDE.md omits doctor as a `ListAllPaneHookKeys` consumer | `CLAUDE.md` | 4-8-1 |
| A3 | `cmd` test fake carries a `during` hook nothing sets | `cmd/hookkey_vocabulary_test.go` | 5-7-1 |
| A4 | `PaneHookLister` declared twice with identical doc | `cmd/hooks.go` | 6-1-1 |
| A5 | Twelve failure messages name `errors.As`, not `errors.AsType` | 9 test files across `cmd`, `internal/log`, `internal/resolver`, `internal/state`, `internal/tmux` | 6-25-1 |
| A6 | Unaddressable-name sentinel doc names one of three rules | `internal/tmuxerr/errors.go`, `internal/tmux/errors.go` | 7-1-1 |
| A7 | Unreadable `hooks.json` still hand-rolled at five sites | `internal/hooks/lookup_test.go`, `cmd/state_hydrate_test.go`, `cmd/state_hydrate_exec_log_test.go`, `internal/hookstest/doc.go` | 7-16-1, 8-27-1 |
| A8 | `hook rm --pane-key` subtest guards one seam, claims two | `cmd/hooks_rm_exit_test.go` | 7-19-1 |
| A9 | Nothing asserts `NewRestoreOrchestrator` pins `Exe` | `internal/restoretest/orchestrator_staged.go` (+ new integration-tagged test) | 7-22-1 |
| A10 | Failure message names the vanished `quietCommander` | `cmd/spawn_seams_test.go` | 7-26-1, 8-16-1 |
| A11 | `ActivePaneCurrentPath` cites a doc that lost the claim | `internal/tmux/tmux.go` | 7-32-1, 9-2-1 |
| A12 | Teardown guard's blind-spot list misses a third shape | `internal/portaltest/teardown_guard_coverage_test.go` | 8-11-1 |
| A13 | `ShowEnvironment` test subsumed by the new table | `internal/tmux/session_name_test.go` | 8-2-1 |
| A14 | `countCalls` restates the callee-unwrap rule | `internal/capture/swap_harness_test.go` | 8-21-1 |
| A15 | `HooksPath` assertion compares `HooksPath` to itself | `internal/hookstest/staging_test.go` | 8-27-2 |
| A16 | Fourth hydrate wait still spells 10s/50ms inline | `cmd/bootstrap/reboot_roundtrip_test.go` | 8-36-1 |
| A17 | `lock.go` bound comments record results, not reasons | `internal/hooks/lock.go` | 8-40-1, 8-40-2 |
| A18 | `logtest` install guard skips `package log_test` files | `internal/logtest/install_guard_test.go` | 8-49-1 |
| A19 | Flag-prefix refusal string ships with no fixture | `internal/capture/fixtures.go`, `internal/capture/swap_harness_test.go`, `internal/capture/capture_test.go` | 8-6-1 |
| A20 | `Commit` seam case can write `sessions.json` into `cmd/` | `cmd/deps_merge_convention_test.go` | 8-8-1 |
| A21 | Saver-ready fixture leaves a ~10ms timing margin | `internal/tmux/portal_saver_test.go` | 9-1-1 |
| A22 | Copy test's comment denies what the AST guard catches | `cmd/doctor_stand_down_copy_test.go` | 9-10-1 |
| A23 | Union-over-collision behaviour lost its only test | `internal/tmux/target_composition_guard_test.go` | 9-17-1 |
| A24 | CLAUDE.md omits six new `sourceguardtest` primitives | `CLAUDE.md` | 9-19-1 |
| A25 | `TestMain` leaves `XDG_CONFIG_HOME` unpoisoned | `.golangci.yml`, `cmd/testmain_isolation_test.go`, `cmd/testmain_home_poison_test.go` | 9-3-1, 9-3-2 |
| A26 | `sourceguardtest` doc names scratch copy as only route | `internal/sourceguardtest/doc.go` | 9-51-1 |
| A27 | `hooksweep` fixtures label unjudgeable payloads `cmd-live` | `internal/hooksweep/sweep_test.go` | 2-8-1, 2-8-2 |

Six collision groups were collapsed into single actions (A7, A10, A11, A17, A25, A27). Two scope calls were overturned on the record — `8-6-1` and `8-36-1` were scoped against a single task's commit rather than the work unit's change-set, and both turned out to be this work unit's own unfinished delivery. Four findings prescribed remedies that themselves breached the project's comment standard (a consumer count, three comments citing tests); each was amended rather than dropped, the defect surviving and only the remedy changing.

### Out of scope

Held in the manifest for your call. Neither was part of this specification.

1. **`ListAllPanesWithFormat` doc claims untrimmed output** *(quick-fix)* — `internal/tmux/tmux.go` says the method returns "raw untrimmed output" while its body calls the trimming `Run`. A caller needing trailing whitespace preserved — the exact concern the `paneHookRowSeparator` comment forty lines below reasons about correctly — is told it survives, so changing the separator to a space or tab would look safe and would silently drop it from the first row. *(source: 2-1-1)*
2. **Two test socket prefixes miss the `ptl-` sweep pattern** *(quick-fix)* — `internal/spawn/ack_realtmux_test.go` uses `spawnack-` and `internal/tmux/portal_dir_roundtrip_realtmux_test.go` uses `portaldir-`, neither matching the `ptl-*` pattern CLAUDE.md's documented human sweep keys on. A server leaked by either suite is invisible to that sweep and keeps running — the accumulation CLAUDE.md measures at 113 leaked servers, which saturates the machine and manufactures flakes elsewhere. *(source: 2-7-1)*

### Discarded

None. All 35 findings were assessed valid against the current tree — none was stale, already-done, or wrong about its situation.
