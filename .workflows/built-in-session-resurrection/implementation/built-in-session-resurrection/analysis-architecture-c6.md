---
agent: architecture
cycle: 6
findings_count: 5
status: issues_found
---
# Architecture Analysis (Cycle 6)

## Summary

Phase 12 composes cleanly — Logger widening is non-breaking with all three implementors updated; ErrCorruptIndex semantics are consistent across all consumers; seams are healthy. Five low-severity issues, all docs/wording.

---

## API Surface

### FINDING A1: `bootstrap.Logger` widening is non-breaking and well-contained

- **SEVERITY:** low
- **FILES:** `cmd/bootstrap/bootstrap.go:112-132`, `cmd/bootstrap/bootstrap_test.go:71-96`, `internal/state/logger.go:222-224`
- **DESCRIPTION:** Adding `Debug(component, format string, args ...any)` is technically an interface widening, but every concrete implementor in scope is updated coherently. `*state.Logger` already had `Debug` natively; `noopLogger` adds a one-line no-op (bootstrap.go:126); `recordingLogger` adds a parallel `debugs` slice. No external implementors exist. The four-method shape matches `*state.Logger`'s native shape so the bootstrap interface is now a clean subset rather than a hand-curated three-method projection that would risk drifting. Minor asymmetry: `*state.Logger` exposes Debug/Info/Warn/Error while `bootstrap.Logger` exposes Debug/Warn/Error (no Info) — intentional since bootstrap step events are entry-debug, soft-warn, or fatal-error only. Documentation nit, not worth changing.
- **RECOMMENDATION:** None.

## Error-Classification Semantics

### FINDING A2: Permission-denied wrap of `ErrCorruptIndex` is correct across all consumers but the user-facing warning text is misleading in the permission case

- **SEVERITY:** low
- **FILES:** `internal/state/index_reader.go:10-19,39-54`; `internal/restore/restore.go:76-85`; `cmd/bootstrap/errors.go:50-55`; `cmd/bootstrap/bootstrap.go:213-222`; `cmd/state_daemon.go:239-249`; `internal/state/status.go:114-127`
- **DESCRIPTION:** Traced every `errors.Is(..., state.ErrCorruptIndex)` matcher. Two production matchers, both behave correctly under the widened classification:
  1. `internal/restore/restore.go:81` — matches and returns `(true, wrapped)`. Pre-T12-8, permission-denied fell through to `(false, nil)` (silent). Now it surfaces as a `CorruptSessionsJSONWarning` to the operator. The fallthrough is correctly demoted to "defensive" since `ReadIndex` now wraps every non-nil error.
  2. `cmd/bootstrap/bootstrap.go:213-222` — branches on the `corrupt bool`, not `errors.Is`. Unaffected by the wrap change.

  Two non-matching consumers also unaffected: `cmd/state_daemon.go:245-248` only logs `err.Error()`; `internal/state/status.go:118-122` treats `skip || err != nil` as a single bucket.

  Weak link: `CorruptSessionsJSONWarning()` says `"Portal state file is corrupt — restoration skipped."` (cmd/bootstrap/errors.go:52). For a chmod-locked-but-otherwise-valid sessions.json, this is technically wrong — the file is not corrupt. The redirect to `portal state status` and `portal.log` does point operators at the truth, but the headline misleads.
- **RECOMMENDATION:** Either (a) accept the trade-off and note in `errors.go` that the wording covers permission-denied as well, or (b) generalise the headline ("Portal state file unusable — restoration skipped."). Option (a) is the lighter touch.

## Symlink-Protection Boundary

### FINDING A3: Lstat-only leaf protection is sound but the godoc's last sentence is misleading about how `os.RemoveAll` handles intermediate symlinks

- **SEVERITY:** low
- **FILES:** `cmd/state_cleanup.go:118-152`
- **DESCRIPTION:** With `PORTAL_STATE_DIR=~/state-link/inner` where `state-link → ~/Documents` and `inner` is a regular dir, `Lstat` reports a regular dir at the leaf and `os.RemoveAll` traverses through `state-link` to delete `~/Documents/inner`. This is intended `RemoveAll` semantics. Since `PORTAL_STATE_DIR` is opt-in, the user owns whatever they aim at — same trust-the-env-var contract every CLI uses. The Lstat-leaf guard correctly catches the "user accidentally points PORTAL_STATE_DIR at a symlink leaf" case (e.g. `PORTAL_STATE_DIR=~/portal-state` where that itself is a symlink to `$HOME`).

  The docstring (cmd/state_cleanup.go:127-128) ends with: *"The Lstat check below is sufficient — RemoveAll follows the leaf inode and never traverses through a symlink target."* The "never traverses through a symlink target" sentence is wrong as written — `os.RemoveAll` DOES traverse intermediate symlinked path components. Only the *leaf*-symlink case is what this Lstat guard covers. A future maintainer reading the doc could assume broader protection than exists.
- **RECOMMENDATION:** Tighten the docstring to scope the protection precisely. Suggested wording: *"The Lstat check below covers the leaf-symlink case (PORTAL_STATE_DIR resolves directly to a symlink). RemoveAll DOES traverse intermediate symlinked components — by design, since users may legitimately have `~/.config` symlinked to a different volume. Whatever leaf directory PORTAL_STATE_DIR points at (after intermediate resolution) is what gets purged."* The new regression test `TestStateCleanup_PurgeAllowsSymlinkedIntermediatePathComponents` (cmd/state_cleanup_test.go:533-587) already exercises this scope correctly — the doc just needs to match the test's assertions.

## Test Seam Quality

### FINDING A4: Phase 12 integration tests inject seams cleanly via existing adapter and Orchestrator interfaces; minor primitive duplication in `reboot_roundtrip_test.go` is justified by package-cycle constraints

- **SEVERITY:** low
- **FILES:** `cmd/bootstrap/reboot_roundtrip_test.go:413-449,511-573`; `cmd/bootstrap/phase5_marker_suppression_integration_test.go`; `cmd/reattach_integration_test.go`; `cmd/state_cleanup_test.go:34-63`
- **DESCRIPTION:** Test seams are healthy. The `bootstrap.Orchestrator` exposes every step (Server/Hooks/Restoring/Saver/Restore/Sweeper/Clean/Logger), so the new integration tests wire production adapters for the steps under test and `NoOp*` shims for the rest. No test reaches into private functions to drive scenarios. Two minor reach-overs in `reboot_roundtrip_test.go`, both justified and noted in godoc:
  1. `driveSignalHydrate`/`openAndSignalFIFO` (lines 511-573) duplicates `cmd/state_signal_hydrate.go::writeFIFOSignal` because the production primitive lives in package `cmd` and `cmd/bootstrap` cannot import `cmd` (would be a cycle). Exporting it solely for one test would leak an internal seam — the duplication is the right structural call.
  2. `captureAndCommit` (lines 413-449) duplicates `daemonDeps.captureAndCommit` for the same reason — the production method is on a `cmd`-package private struct.

  Both are noted in godoc and would only become a problem if the production primitives drifted in ways the duplicates didn't track. The `recordingCommander` in `state_cleanup_test.go:34-63` (sub-package mock duplicating `internal/tmux/MockCommander` shape) is also justified — comment at lines 33-36 explains it lets cmd-level tests drive a real `*tmux.Client` end-to-end.
- **RECOMMENDATION:** None — the trade-offs are clearly documented and the duplications are minimal. (See duplication agent finding D1 for the cross-package duplication concern; the architectural justification holds, but a shared `internal/restoretest/` package would still let both copies share a home.)

## Documentation Coherence

### FINDING A5: `review-integrity-tracking-c2.md` retains pre-T12-9 "log WARN and continue" wording for `Restoring.Clear`, now contradicting the reconciled fatal-on-failure rule

- **SEVERITY:** low
- **FILES:** `.workflows/built-in-session-resurrection/planning/built-in-session-resurrection/review-integrity-tracking-c2.md` (L251 / L259)
- **DESCRIPTION:** T12-9 reconciled spec/CLAUDE.md/plan/impl/test on the rule that step 6 (Clear `@portal-restoring`) is fatal on failure. cmd/bootstrap/bootstrap.go:227-229 confirms: `return ... o.fatalf("clear @portal-restoring marker", err)`. The historical document `review-integrity-tracking-c2.md` L251/L259 still say "log WARN and continue." That document is a historical review-integrity record (cycle 2) capturing the disagreement state at that time — not a normative document, and not in T12-9's reconciliation scope.
- **RECOMMENDATION:** Optionally annotate L251/L259 with an in-place note ("Resolved by T12-9: step 6 is fatal on failure") so a future reader cross-referencing the historical record is not misled. If the doc is treated as a strict historical artifact (not edited post-cycle), leave as-is.

## Status

**STATUS:** issues_found
**FINDINGS_COUNT:** 5

Phase 12 architecture is sound. The `bootstrap.Logger` widening, the permission-error wrap of `ErrCorruptIndex`, and the Lstat-only symlink protection all compose cleanly across consumers — every `errors.Is` matcher behaves correctly under the new classification, and seam quality in the new integration tests is high. The five low-severity flags are all documentation precision issues — none affect correctness or shape.
