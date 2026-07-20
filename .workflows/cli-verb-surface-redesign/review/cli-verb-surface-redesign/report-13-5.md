TASK: cli-verb-surface-redesign-13-5 — Close the test-coverage parity gaps across the redesigned surface

ACCEPTANCE CRITERIA:
1. cmd/open_test.go — `-f <text> -- <cmd>` case alongside the `-e` filter-threads-command case.
2. cmd/open_test.go — an attach-guard case using `open dev -- claude` alongside the `-e` case.
3. cmd/open_test.go — `TestOpenCommand_PathPin_Miss_HardFailsNoPicker` mirroring the `-s` pin-miss test.
4. cmd/state_daemon_run_test.go — a `tick()`-idle-branch test for the project prune mirroring `TestDaemonTick_RunsHookCleanupOnIdleTick`.
5. cmd/open_surfaces_test.go — engine-level subtest for a `Domain:"session"` (`-s`) surface.
6. cmd/open_targets_guard_test.go — reverse assertion (every pin key maps to a live openCmd flag); cmd/open_targets_test.go — excluded flag's value between two positionals.
7. cmd/completion_test.go — `open` flag completion excludes `--ack`; top-level completion excludes `state`.
- No production behaviour changes.

STATUS: Complete

SPEC CONTEXT: This is a Phase-13 review-remediation task consolidating seven symmetric QA-flagged test gaps where one spelling/branch is covered but its sibling is not. It is a test-only task (no production-code change) drawn from reports 2-5, 2-6, 2-2, 4-8, 3-3, 3-2, 6-3. The redesigned surface (single `open` verb with `-s/-p/-a/-z` pins, `-e`/`--` command spellings, multi-target burst, hidden `--ack`, hidden `state` namespace) is exercised by these siblings.

IMPLEMENTATION:
- Status: Implemented (test-only, as required)
- Location: all seven additions present and verified by reading:
  1. cmd/open_test.go:2998 `TestOpenCommand_Filter_ThreadsDashDashCommandToPicker` — `open -f web -- claude` threads filter=web + command=[claude] to picker; exact sibling of `TestOpenCommand_Filter_ThreadsCommandToPicker` (:2967, `-e`).
  2. cmd/open_test.go:370 `TestOpenCommand_BareSessionAttach_WithDashDashCommand_UsageError` — `open dev -- claude` → UsageError (exit 2), no attach; sibling of `..._WithCommand_UsageError` (:325, `-e`).
  3. cmd/open_test.go:686 `TestOpenCommand_PathPin_Miss_HardFailsNoPicker` — non-existent dir → "Directory not found: <path>" plain error (not UsageError), no TUI, no mint; mirrors the `-s` pin-miss test.
  4. cmd/state_daemon_run_test.go:766 `TestDaemonTick_RunsProjectCleanupOnIdleTick` — idle tick prunes the gone-dir project, retains live, runs no capture cycle; mirrors `TestDaemonTick_RunsHookCleanupOnIdleTick` (:608). Helpers `seedProjectsJSON` / `projectPaths` and daemon fields `ProjectStore`/`lastProjectCleanup`/`projectCleanupInterval`/`maybeRunProjectCleanup` all exist.
  5. cmd/open_surfaces_test.go:105 `TestResolveOpenSurfaces_SessionPinSurface` — engine-level `Domain:"session"` surface; covers exact hit (one attach surface) and miss (collected `*MissResult`, not the single-pin hard error).
  6. cmd/open_targets_guard_test.go:104 `TestOpenTargetPinsKeysAreLiveFlags` (reverse: every openTargetPins key names a live openCmd flag); cmd/open_targets_test.go:46 case "excluded exec value between two positionals" (`blog -e claude api` → `[blog, api]`).
  7. cmd/completion_test.go:317 `TestCompletionHidesInternalSurface` — subtest "open flag completion excludes the hidden --ack flag" (:318) and "top-level completion excludes the hidden state namespace" (:329).
- Notes: Commit 9784aecb (Tcli-verb-surface-redesign-13-5) changed only `*_test.go` files plus workflow metadata (.tick/tasks.jsonl, manifest.json) — no production source touched, satisfying "No production behaviour changes". The guard test file (item 6) now carries `map[string]resolver.Domain` typing; this is downstream of the later task 14-1 (typed Domain) updating openTargetPins and is not a drift/defect — the reverse-mapping assertion behaves identically.

TESTS:
- Status: Adequate
- Coverage: All seven siblings present and each verifies real behaviour, not just existence:
  - Command-spelling siblings (1,2) assert both the routed value AND the negative (no attach on the guard case).
  - Path-pin miss (3) asserts the verbatim message, no-TUI, no-mint, and that the error is a plain error not a UsageError — a precise, non-redundant set.
  - Project-prune idle tick (4) asserts both the prune outcome and the structural no-capture invariant (zero list-sessions), matching its hook-cleanup peer exactly.
  - Session-pin surface (5) covers both the hit and the collected-miss branch (the documented `ResolveSessionPinAll` divergence from the single-pin hard error).
  - Argv-scan (6): reverse guard is the true mirror of the forward guard; the mid-list case proves the excluded value is dropped, not misrouted as a third positional.
  - Completion (7): both hidden-surface subtests additionally assert a VISIBLE sibling (`--session`, `open`) is offered, so the negative probes are non-vacuous.
- Notes: No over-testing — each test is focused with no redundant assertions. Tests are hermetic (seams overridden via t.Cleanup; the one real-tmux completion test uses a per-test `-L` socket in the unit lane, per CLAUDE.md). No t.Parallel (correctly noted in file headers).

CODE QUALITY:
- Project conventions: Followed — seam-injection + t.Cleanup restore, no t.Parallel, table-driven subtests, unit-lane real-tmux client only, well-commented intent tying each test to its sibling/spec section.
- SOLID principles: N/A (test code); shared predicate `valueTakingFlagMissingPins` is cleanly reused by both forward and reverse guards (DRY).
- Complexity: Low — straightforward arrange/act/assert.
- Modern idioms: Yes (slices.Equal, strings.SplitSeq, t.Context()).
- Readability: Good — each test's doc comment states the sibling it mirrors and the invariant it pins.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
