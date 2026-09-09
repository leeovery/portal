TASK: resume-hooks-silently-lost-7-3 — Restore And Reattach Integration Fixtures Escape The Mandated State Isolation

ACCEPTANCE CRITERIA:
- No fixture in the named class points `PORTAL_STATE_DIR` at a bare `t.TempDir()`.
- In every retrofitted fixture, `IsolateStateForTest` and `RegisterStateDirTeardownGuard` are both called before `tmuxtest.New`.
- `IsolateStateForTest` sets `HISTFILE` in both the process env and the returned slice, and no local `HISTFILE` workaround survives in `cmd`.
- The composite harness's teardown wait observes the live `_portal-saver` pane pid, not the overwritten `daemon.pid`.
- The widened guard fails on a fixture that sets `PORTAL_STATE_DIR` and starts a server without either required call, and passes over the retrofitted tree.
- The guard still fatals when it scans zero qualifying files.
- The full integration lane passes, and the fingerprint backstop reports no delta against the developer's real state dir.

STATUS: complete

SPEC CONTEXT: The specification carries nothing on test isolation — this is a phase-7 implementation-analysis task, so its authority is its own body (per the shared verifier context). The governing standard is CLAUDE.md's ABSOLUTE INVARIANT and its three isolation boundaries (filesystem/state via `IsolateStateForTest`, tmux via per-test `-L` sockets, processes via the default-deny daemon-pgrep sandbox), plus the LIFO teardown-ordering rule for `RegisterStateDirTeardownGuard`. The defect the task names is real against that standard: ten fixtures drove a live server against a hand-rolled state dir with the sandbox neither enabled nor registered, so a subprocess orphan sweep had an unfiltered pid list and the developer's live daemon was a legal kill candidate.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/portaltest/isolated_env.go:46` — `t.Setenv("HISTFILE", os.DevNull)`, set before the `os.Environ()` read at `:94`, so the returned slice carries it. (`:47`'s `ZDOTDIR` pin is a later task's addition, not this one's.)
  - `internal/portaltest/teardown_guard.go:18-51` — `SaverPIDSource`, the `cleanupT` narrowing, `registerTeardownGuard` extracted, and the new `RegisterStateDirTeardownGuardWithPIDSource`.
  - `cmd/bootstrap/composition_e2e_harness_integration_test.go:46-81,121,139` — `saverPaneProbe` with the deliberate `pid` / `livePID` split; the teardown wait takes the fallback-bearing `saver.pid`, the SIGKILL allowlist takes the live-only `saver.livePID`.
  - `internal/portaltest/teardown_guard_coverage_test.go:78-80` — the widened trigger: `ServerLine != 0 && (NamesStateDir || IsolateLine != 0)`.
  - Retrofits, each isolating and registering the guard before `tmuxtest.New`: `internal/restore/armed_restore_integration_test.go:22,29,31` and `:90,97,99`; `internal/restore/integration_test.go:53,60,62`; `internal/restore/integration_full_test.go:40,69,71`; `internal/restore/exit_closes_pane_integration_test.go:103,115,117`; `cmd/reattach_integration_test.go:70,77,79`; and the five `cmd/bootstrap` fixtures through `newIntegrationStateDir` (`cmd/bootstrap/helpers_integration_test.go:18-27`), called before the server at `eager_signal_hydrate_integration_test.go:50/115`, `:110/115`, `:181/186`, `phase2_hook_fire_integration_test.go:29/50`, `phase5_integration_test.go:23/25` and `:75/77`, `phase5_marker_suppression_integration_test.go:26/28`, `scrollback_resumption_test.go:54/56`, `:104/106`, `:150/152`.
- Notes:
  - AC1 verified repo-wide, not just over the named ten: no `*_test.go` that calls `tmuxtest.New` names `PORTAL_STATE_DIR` without also calling `IsolateStateForTest`.
  - The three inverted orderings named in the task (`armed_restore`, `integration_test`, `reattach`) are fixed — the commit diff moves `tmuxtest.New` below the isolation and the guard in each.
  - The binary-build-before-isolation trap that `helpers_integration_test.go:16-17` documents is respected everywhere the retrofit landed: every `BuildPortalBinaryDir` / `StagePortalBinary` / `ensurePortalOnPATH` call precedes its fixture's `IsolateStateForTest`.
  - The task's "keep the per-file scoping" instruction is superseded, not violated: task 8-11 (`7f92f712`) later moved the rule to per-function judgement with a one-hop local-arrange resolution and an added order check. That is a strictly stronger rule and the current shape is what I judged against.
  - The retrofit also collapsed a genuine double-isolation bug in `cmd/bootstrap/reboot_roundtrip_test.go` (a second `IsolateStateForTest` was overwriting `PORTAL_TEST_SANDBOX_REGISTRY` with a registry naming a state dir the test never wrote to, leaving subprocess sweeps with zero owned dirs); it now takes both returns from one `newIntegrationStateDir` call.
  - AC7's lane run is not verifiable by reading. Nothing in the change suggests a break: no production source is touched, the helper additions are additive, and the retrofitted fixtures keep their prior `state.EnsureDir()` and env-pinning sequence.

TESTS:
- Status: Adequate
- Coverage: every test the task named exists and asserts the property it names.
  - `internal/portaltest/teardown_guard_coverage_rule_test.go:160` fails a server fixture with no guard; `:174` fails a hand-rolled state dir with no isolation (the widening's own arm); `:185` passes a fixture making all three calls; `:276` pins the `scanned == 0` fatal, including the "stopped looking" wording.
  - `:199` (`TestCoverageRuleFailsIsolatedFixtureTakingItsStateDirFromASharedArrange`) covers the `|| IsolateLine != 0` arm, and first asserts the fixture does *not* name `PORTAL_STATE_DIR` — so it cannot silently start passing on the other arm. That was the round-1 fix-tracking issue and it is closed.
  - `internal/portaltest/isolated_env_test.go:230` pins `HISTFILE` on the process env *and* exactly one entry in the returned slice, with a decoy value pre-set so an unset-and-inherited pass is impossible.
  - `internal/portaltest/teardown_guard_test.go:26` waits out a caller-supplied live pid (a real 300ms subprocess, asserted against a poll-tick tolerance and bounded by the 3s budget), `:58` pins the early return over a quiescent dir.
  - `cmd/bootstrap/composition_e2e_harness_integration_test.go:85` is the round-2 addition: it pins the `livePID`/`pid` divergence with a nil socket, and its failure message names the SIGKILL-allowlist consequence. This is the property that keeps a stale pid out of the default-deny set, and it now fails rather than relying on a comment.
- Notes: `presenceDefect`'s first branch (a fixture calling *neither* required call) is reached by no rule test — both single-omission branches are covered, and the branch exists only to render a better message, so the rule's verdict is still pinned. Nothing is over-tested: the rule suite's ten cases are each a distinct arm of the trigger, the presence check, the order check or the hop, with no duplicated assertion.

CODE QUALITY:
- Project conventions: Followed. The new helper is test-only and lives in `internal/portaltest` with a `*testing.T`-first parameter; the guard remains a unit-lane source scan reading through `sourceguardtest.RepoSources`; the retrofitted fixtures keep their `//go:build integration` tags (`internal/restore/integration_test.go`'s untagged status is pre-existing and it neither builds nor spawns a portal binary).
- SOLID principles: Good. `registerTeardownGuard` is the one implementation both public registrars delegate to, with the pid source as the single injected difference — the file-based lookup is now just one `SaverPIDSource` among possible others.
- Complexity: Low. The probe's two methods are five lines each; the guard's widening is one boolean.
- Modern idioms: Yes (`slices.Backward` in the cleanup recorder, `os.DevNull` rather than a literal).
- Readability: Good. Every load-bearing ordering decision carries a comment that says *why* it is load-bearing (LIFO between kill-server and RemoveAll; build-before-HOME-repoint; live-only read for the allowlist), and the comment corrections the fix rounds called for are all present in the current text.
- Issues: None found. The `livePID` latch of `p.seen` is a side effect its name does not advertise, but it is documented at `composition_e2e_harness_integration_test.go:63-66`, the composite suite is single-goroutine, and the divergence it enables is now guarded by a test.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
