# Implementation Review: Slow Open Empty Previews And Zombie Sessions

**Plan**: slow-open-empty-previews-and-zombie-sessions
**QA Verdict**: Approve
**Review cycle**: 2 (Phase 11 review remediation + Phase 12 analysis)

## Summary

The bugfix lands all seven spec components (A–G) with high fidelity to the specification — daemon-identity primitive, kill-barrier SIGKILL escalation, bootstrap orphan sweep, `daemon.pid` pre-check + inode cross-check, daemon self-supervision, `CaptureStructure` per-session log-and-continue, placeholder-then-respawn saver creation, and the test-isolation helper. Four subsequent analysis cycles (phases 7–10) consolidated duplication and tightened API surfaces; the resulting code is well-tested, well-documented, and architecturally sound.

**Cycle-2 remediation** (Phase 11, four tasks) addressed every blocking item from cycle 1: T11-1 flipped the T9-7 test to assert WARN (production-aligned), T11-2 closed the T6-4 empty-baseline + missing-dir silent-pass gaps with plan-verbatim diagnostics, T11-3 amended Component F bullet 3 to match the log-noise-absence assertion shape the implementation actually provides on tmux 3.6b (+ Note flagging `remain-on-exit on` as a future opt-in), and T11-4 rewrote the misleading T7-5 comment and lifted the daemon.version invariant from comment-only to a symmetric `os.Stat` assertion. **Phase 12** (one task) reconciled the Component F design-rationale prose at specification.md:365 with the amended bullet 3 + Note, eliminating the last spec-internal contradiction. Five new task verifiers (T11-1 – T11-4, T12-1) ran and returned no blocking issues. Spec and implementation are now internally consistent end-to-end.

## QA Verification

### Specification Compliance

Implementation aligns with the specification across all components. Documented deviations are reasoned and architecturally justified:

- **Task 2-1**: `ErrNoSuchSession` sentinel factored into new leaf package `internal/tmuxerr` (rather than declared directly in `internal/tmux`) to break a latent import cycle. Matches existing leaf-package precedents.
- **Task 3-5**: Component F acceptance bullet "`_portal-saver` remains present after the daemon exits" reframed to "absence of no-such-session log noise during the lock-loser cascade". On tmux 3.6b without `remain-on-exit on`, the session DOES disappear when the lock-loser daemon exits even with `destroy-unattached=off`. **Resolved in cycle 2** — T11-3 amended bullet 3 + added Note; T12-1 reconciled design-rationale prose at line 365.
- **Task 5-2**: Helper API exports `SaverPanePIDOrAbsent(*Client, string) (int, bool, error)` tri-state instead of the planned `SaverPanePID(*Client, string) (int, error)`, with the rich-sentinel form unexported. Encodes "treat any error as absent" rule at type level rather than caller discipline — improvement, later reinforced by T10-2 and T10-3.
- **Task 6-1 / 6-4**: Composite harness uses per-orphan `PORTAL_STATE_DIR` (rather than shared) to keep all three daemons alive long enough for pgrep N=3 observation; defended in file-headers.

### Plan Completion

- [x] Phases 1–6 (Components A–G + composite verification) acceptance criteria met
- [x] All 73 completed tasks implemented (68 original + 4 cycle-1 review remediation + 1 cycle-6 analysis)
- [x] Analysis cycles 7–10 (29 refactor tasks) completed
- [x] Phase 11 (Cycle-1 review remediation, 4 tasks) completed
- [x] Phase 12 (Cycle-6 analysis, 1 spec coherence task) completed
- [x] No scope creep
- [x] `go build ./...` clean
- [x] CLAUDE.md bootstrap section updated to 11-step ordering with new SweepOrphanDaemons step

### Code Quality

Strong throughout. Highlights:

- DI / seam pattern consistent with established `*Deps` convention; tests swap via `t.Cleanup`-restored package-level vars
- New `internal/portaltest` leaf package became canonical home for cross-package test scaffolding (isolated env, subprocess spawn, fingerprint diff, pgrep, portal-log read); structurally enforces test-only usage via `*testing.T` parameter
- Saver-seams consolidation (T8-6 / T9-6) produced a clean composite `SaverSeams` struct with shared primitives promoted to top level; doc comments on every field call out shared consumption
- Identity-check primitive (`state.IdentifyDaemon`) shared by Components A/B/C with three-result contract + transient-error case; argv pattern constant (`PortalDaemonArgvPattern`) is single source of truth consumed by ~6 sites
- Pgrep enumeration unified into `state.PgrepPortalDaemons` after T9-1, eliminating prod/test drift risk

### Test Quality

Tests adequately verify requirements at unit and integration level. Notable strengths:

- AST-walking adjacency test for `acquireDaemonLock → WritePIDFile` (T4-8) pins source ordering against future intruder statements
- Composite end-to-end test reconstructs the reporter's 3-daemon failure scenario and asserts convergence to singleton within 6s
- No-final-flush snapshot tests (T4-2, T5-7) byte-compare scrollback directory across kill/eject windows
- Lower-bound regression guard `selfSupervisionHysteresisTicks >= 1` (T5-9) prevents accidental zeroing

Gaps closed in cycle 2:

- **Task 9-7**: log-level test fixed by T11-1 (WARN assertion, renamed function).
- **Task 6-4**: empty-baseline + missing-dir silent-pass closed by T11-2 (`(paths, dirExists)` return; plan-verbatim diagnostics).
- **Task 7-5**: stale comment fixed by T11-4 (+ symmetric `os.Stat(daemon.version)` assertion).

Remaining gaps (non-blocking, carried from cycle 1):

- **Task 4-8**: meta-test for "AST diagnostic names intruding statement" was not implemented; `SingleProductionCallSite` scans only `cmd/` non-recursively.
- **Task 6-1**: explicit ppid check that orphan parents != saver pane process is not asserted (structurally guaranteed but not verified).

### Cycle-2 Remediation Coverage

| Cycle-1 finding | Resolved by | Status |
|---|---|---|
| Required Changes #1 (T9-7 broken test) | T11-1 | ✓ Test asserts WARN; passes |
| Bug #49 (T6-4 empty baseline silent-pass) | T11-2 | ✓ `len(baseline) > 0` guard with plan-specified message |
| Bug #50 (T6-4 missing-dir silent-pass) | T11-2 | ✓ `(paths, dirExists)` discriminator; ENOENT fails loudly |
| Bug #51 (T7-5 stale comment) | T11-4 | ✓ Comment rewritten + symmetric stat assertion |
| Recommendation #26 (T3-5 spec/impl mismatch) | T11-3, T12-1 | ✓ Bullet 3 amended + design-rationale prose reconciled |

### Required Changes

None. All cycle-1 blocking items resolved.

## Recommendations

### Quick-fixes

1. **T1-1**: `internal/state/daemon_identity.go:117` — error message reads `"ps failed with output %q"`; "stdout" is more precise than "output"
2. **T1-2 / T1-3**: top-of-file comment in `isolated_env.go:1-3` mostly duplicates package godoc in `doc.go`
3. **T1-3**: tighten `fingerprint.go` "test-only" docstring — exported `SnapshotStateDir`/`DiffFingerprints`/`FormatDelta`/`Fingerprint` are consumed from out-of-package tests and don't take `*testing.T`; structural enforcement only covers `IsolateStateForTest`
4. **T1-4, T1-5, T4-9, T5-7, T5-8, T8-3 et al.**: doc-hygiene sweep on planning artefacts and audit deliverable — multiple plan rows still reference pre-rename helper `NewIsolatedStateEnv`; live symbol is `IsolateStateForTest`
5. **T2-5**: one-line comment in `readPortalLog` noting intentional double-close (flush before on-disk read); update file-level comment to cite live call-site line `cmd/state_daemon.go:327` (planning doc's `:149` is stale)
6. **T3-6**: `parseShowEnvironmentKeys` silently ignores malformed lines — a `t.Logf` for unexpected shapes would aid future debugging (not load-bearing today)
7. **T4-7**: post-loop `return nil, fmt.Errorf(...)` at `daemon_lock.go:173-175` is unreachable (final iteration's mismatch returns first); duplicate format string — convert to `panic("unreachable: bounded retry loop fell through")` or extract constant
8. **T4-8**: replace `positionString`'s `strings.TrimPrefix(p.String(), p.Filename+":")` with `fmt.Sprintf("%d:%d", p.Line, p.Column)`
9. **T4-10**: split 4 spec-named assertions into `t.Run` subtests so failure reports `.../scrollback_stability` rather than parent
10. **T5-1**: memo path drift — plan specifies `planning/...`; committed memo at `specification/...`. Either relocate or amend plan (recommend amend — spec-adjacent more durable)
11. **T6-1**: add sentinel constant `compositeHarnessOrphanCount = 2` and use at pgrep/spawn sites for intent clarity
12. **T6-1**: consolidate shared helpers (`skipIfNoPgrep`, `waitForSaverPanePID`, `readSaverPanePID`, `waitForPgrepCount`, `pidAlive`, `waitForDaemonPID`) into `cmd/bootstrap/integration_helpers_test.go`
13. **T6-2**: file header and constant doc describe "~3.6 s of observation"; actual is 900ms × 3 ≈ 2.7s — bump interval to 1.2s OR correct comments
14. **T6-2**: `dirNames` line 199 — `filepath.Base(e.Name())` is redundant; replace with `e.Name()`
15. **T6-6**: `var _ = syscall.SIGKILL` (431) and `var _ = errors.Is` (432) anchor imports no longer directly referenced — drop
16. **T6-8**: add positive-control `t.Logf` to fingerprint-backstop test that walks `h.StateDir` and reports file count + total bytes
17. **T6-8**: extend preamble to enumerate three documented failure causes (subprocess inherited dev XDG_CONFIG_HOME; direct file write bypassed env; helper snapshot semantics changed)
18. **T8-7**: `internal/portaltest/doc.go` says `*testing.T` "enforces structurally"; `ReadPortalLogSafe` (T9-4) is exception — one-word softening (e.g. "on most exported helpers")

### Ideas

19. **T1-1**: "Zero exit with empty stdout" treated as transient — defensible but not in spec's enumerated branches; one-line rationale tying to `( |$)` regex contract would close the loop
20. **T1-1**: `defaultIdentifyPS` uses `.Output()` discarding stderr; capture stderr into wrapped error for operator debugging
21. **T1-3**: implementation adds `hashed` field-flip delta translated to `hashed-changed` — useful signal not anticipated in plan; consider documenting in spec
22. **T1-3**: backstop's "snapshot BEFORE env mutation" spec requirement intentionally deviated from for host-noise mitigation; materially changes threat model (env-flow tests only); update spec note
23. **T2-3**: log format strings could be harmonised to `"capture: <natural-churn|anomalous> session %q: %v"` for filter-by-classification grep
24. **T2-3**: all-natural-churn branch emits per-session WARNs but no tick-level summary — single INFO/WARN summary would speed postmortems
25. **T2-5**: add "happy path emits no spurious WARN" test for regression safety; extract `makeLoggingTickHarness(t, envErrs)` if Component E tests grow beyond two callsites
26. **T3-5 (significant)**: spec ↔ impl naming mismatch — spec acceptance bullet 3 literally asserts "session persists after the daemon exits"; impl asserts "no log-noise cascade" (driven by tmux 3.6b behaviour). Either amend spec or add `remain-on-exit on` so literal spec assertion holds — **needs discussion**
27. **T3-5**: negative-case detection of literal 3-2 revert is timing-dependent on tmux 3.6b; strengthen via injected daemon-exit delay
28. **T4-1**: `escalateKillToSIGKILL` swallows non-ESRCH errors from SendSIGKILL; DEBUG breadcrumb would aid post-mortems without changing behaviour
29. **T4-3**: SaverPanePID seam widened post-original-spec to tri-state; brief plan-history note for traceability
30. **T4-6**: pre-check runs at head of EVERY retry — up to 3× ReadPIDFile + IdentifyDaemon on persistent mismatch; spec Do-list maps to single pre-check before loop; worth confirming with spec author OR hoisting above loop
31. **T4-7**: no integration test exercising real unlink+recreate race against real flock; seam-driven units prove logic but real-syscall race would catch seam/syscall drift
32. **T4-8**: plan's meta-test (synthetic mutation injection) not implemented — add `t.Run` parsing inline source with intruder statement, asserting diagnostic substring
33. **T4-8**: `TestAcquireDaemonLock_SingleProductionCallSite` scans only `cmd/` non-recursively; broaden via `filepath.WalkDir` from repo root or document scope limitation
34. **T4-10**: spec deviation "no orphan-PID written to daemon.pid" not cross-linked from spec; optional footnote
35. **T5-1**: binary version `"dev"` rather than tagged release; document re-measurement expectation in memo
36. **T5-1**: `measureAttachDetach`/`measureClientAttached` substitute `refresh-client`/`run-shell -b true` for true PTY attach; regression in real `client-attached` hook fire path would silently under-measure
37. **T5-2**: plan specified exported `SaverPanePID`; shipped `SaverPanePIDOrAbsent` — well-motivated; one-line planning memo note explaining divergence
38. **T5-8**: assertion D depends on `PORTAL_LOG_LEVEL=INFO` propagating test → tmux server → respawn-pane'd daemon; fragile chain; assert via positive log marker that log level is actually INFO
39. **T6-1**: add meta-test proving all three daemon PIDs dead after `t.Cleanup`
40. **T6-1**: add `ps -o ppid= -p <orphan PID>` to close literal parent-PID divergence acceptance criterion
41. **T6-5**: plan AC "fresh-process subprocess via `go build` ... NOT in-test call" intentionally deviated from; documented + precedent-cited; **discussion thread** — amend plan to permit in-process OR add subprocess form for belt-and-braces fd-isolation
42. **T6-5**: post-refusal `pgrep -fx` count not asserted — two-line addition would close gap
43. **T6-6**: plan's `ProcessState.Exited()` criterion structurally untestable when daemon owned by tmux; codify the surrogate triad (scrollback bytes-identical + retained daemon.pid + log marker) in spec § Component D
44. **T6-7**: 3×50ms retry ladder mentioned in plan not implemented; if integration-CI flake appears, retry ladder is documented mitigation
45. **T6-8**: plan named 5 test cases under "Tests"; only one implemented as `Test*` function (rest upheld by structural review); consider splitting audit-grep assertions into compile-time structural tests visible in `go test -v`
46. **T8-2**: drift resolved in downstream T10-2/T10-3 not directly under T8-2; consider annotating planning.md row with "superseded"
47. **T8-3**: small table-driven unit test for `state.PgrepPortalDaemons` would lock three-shape contract independently
48. **T9-2**: focused 3-case table unit test for `SaverPanePIDOrAbsent` (ErrNoSuchSession→absent, ErrEmptyPaneList→absent, generic→err passthrough)
49. **T11-2**: `composition_e2e_scrollback_stability_integration_test.go` line 138 `observation, _ := snapshotScrollbackPaths(...)` discards the new `dirExists` return. If the scrollback dir is deleted mid-window, `observation` is nil and `assertPathSetEqual` reports it as "removed paths" — correct behaviour, but explicit `if !dirExists { t.Fatalf("scrollback dir disappeared at observation %d", i) }` would yield a sharper diagnostic. Optional refinement.
50. **T11-2**: optional unit test pinning the new `snapshotScrollbackPaths` contract (missing-dir vs empty-existing-dir) would lock the bool-return discriminator against future drift. Plan marked it "recommended but not load-bearing"; not in acceptance criteria.
51. **T11-3**: Composite end-to-end verification step 9 (`specification.md:464`) asserts "`_portal-saver`'s pane process is `portal state daemon`" without acknowledging that in the lock-loser cascade the session disappears on tmux 3.6b. The composite scenario's preconditions implicitly scope step 9 to the lock-winner case, but a one-line footnote would close the last subtle gap between bullet 3's amendment and the composite verification.

### Bugs

(All cycle-1 bugs resolved — see Cycle-2 Remediation Coverage above. No new bugs found in cycle 2.)
