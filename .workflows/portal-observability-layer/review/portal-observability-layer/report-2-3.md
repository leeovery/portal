TASK: Pid-scoped atomic symlink swing with crash-leftover reclamation (portal-observability-layer-2-3)

ACCEPTANCE CRITERIA:
- After swingSymlink(dir, "portal.log.2026-05-30"), portal.log is a symlink reading back as that target.
- Pre-existing portal.log.<pid>.symlink.tmp (prior-crash leftover) removed and swing succeeds.
- Two concurrent same-target swings both succeed (last-writer-wins), one valid symlink, no orphaned temp.
- Temp name pid-scoped — pid A never collides with pid B.
- A swing failure (simulated) returns an error and leaves the prior symlink in place.

STATUS: Complete

SPEC CONTEXT:
Spec § Log rotation mechanism step 2c (pid-scoped temp portal.log.<pid>.symlink.tmp, no counter, prior-crash os.Remove reclamation, os.Symlink+os.Rename atomic last-writer-wins, benign concurrent same-target) + § Resolved operational edges (failed swing leaves prior symlink in place). Strict-date-parse-rejecting temp name so sweeps never seal/delete it.

IMPLEMENTATION:
- Status: Implemented (no drift)
- Location: internal/log/symlink.go:23-25 (pidSymlinkTmp), :92-94 (swingSymlink), :101-118 (swingSymlinkAs); names.go:54-56 (symlinkPath). Call sites: sink.go:327-328 (reopen, relative bare filename, best-effort) and sink.go:181 (size-cap rotation, filepath.Base, best-effort).
- Notes: Relative bare-filename target (stable if state dir moves). Prior-crash reclamation distinguishes real os.Remove error from ENOENT (wraps/returns non-ENOENT). Both call sites swallow swing error (never aborts handler) but error is returned wrapped for a future WARN. swingSymlinkAs is a narrowly-scoped test helper for distinct pids. migrationGuard/legacyOldName co-located but belong to 2-4.

TESTS:
- Status: Adequate
- Location: internal/log/symlink_test.go
- Coverage: 1:1 with task test list + all ACs — PointsLinkAtTargetAtomically (+ no lingering temp); ReclaimsStaleSamePidTmpFromPriorCrash; ConcurrentSameTargetConvergesToOneLinkNoOrphan (8 goroutines distinct pids, -race); PidSymlinkTmp_EmbedsPidAndCannotCollideAcrossPids; LeavesPriorSymlinkInPlaceOnFailure (symlinkFunc seam).
- Notes: Behaviour-focused, would fail if broken. Single symlinkFunc seam is minimal/justified. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; package-var seam pattern; import-cycle guard).
- SOLID: Good — single-responsibility helpers; swingSymlinkAs(pid) cleanly separates testable core.
- Complexity: Low.
- Modern idioms: Yes (errors.Is os.ErrNotExist, %w wrapping).
- Readability: Good — intent-rich doc comments.
- Security: last-writer-wins is intended; pid-namespaced temp; relative target.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] swingSymlinkAs — add an inline note that it exists solely for the cross-process test, to deter production callers passing arbitrary pids.
- [idea] Both call sites swallow the swing error with no WARN; spec permits (not requires) a log-rotate WARN on swing failure; deferred to 2-7 per in-source comment. Informational.
