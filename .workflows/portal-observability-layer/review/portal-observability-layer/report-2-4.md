TASK: First-run migration guard deleting legacy regular-file portal.log / portal.log.old (portal-observability-layer-2-4)

ACCEPTANCE CRITERIA:
- Pre-existing regular-file portal.log removed; subsequent swing creates a symlink.
- Pre-existing portal.log.old removed alongside.
- portal.log already a symlink → guard no-ops (link + target untouched).
- portal.log.old absent → tolerated.
- portal.log absent entirely → no-op.
- Second reopen (after first swing) → guard deletes nothing.

STATUS: Complete

SPEC CONTEXT:
Spec § Log rotation mechanism step 2 (412-413) + Resolved operational edges (434). Before swing, if portal.log exists as regular file (lstat not-a-symlink), os.Remove it + any portal.log.old. After first swing portal.log is always a symlink → guard no-ops forever (no separate flag). Pre-migration history intentionally not preserved.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/log/symlink.go:52-71 (migrationGuard); invoked at sink.go:311 before swingSymlink (sink.go:328); legacyOldName const symlink.go:30.
- Notes: Matches spec precisely — os.Lstat (not Stat) detects symlink rather than follows; ENOENT → nil; symlink → no-op; regular-file → removes link + .old best-effort. "At most once by construction" documented, no flag. Guard return swallowed at call site (never aborts reopen). Defensive: non-ENOENT Lstat error also returns nil (consistent with never-abort).

TESTS:
- Status: Adequate
- Coverage: all six ACs in migration_guard_unit_test.go (removes regular file; removes .old alongside; no-op when symlink w/ untouched assertions; tolerates absent .old; no-op when absent; no fire on second run w/ re-seeded .old). Plus end-to-end TestRotatingSink_MigratesLegacyRegularFilePortalLogToSymlinkOnReopen exercises guard+swing through real Write→reopen→swingSymlink.
- Notes: Behaviour (filesystem end-state). Would fail if broken. No over-testing. (Separate migration_guard_test.go is the standing legacy-symbol guard, unrelated.)

CODE QUALITY:
- Project conventions: Followed (stateDir-as-string, no internal/state import; reuses symlinkPath/portalLogName; best-effort _= matches package; no t.Parallel).
- SOLID: Good — single responsibility, separated from swingSymlink.
- Complexity: Low.
- Modern idioms: Yes (os.IsNotExist, Mode()&ModeSymlink, filepath.Join).
- Readability: Good — doc explains at-most-once invariant + Lstat-vs-Stat.
- Issues: None of substance.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] Duplicated literal: prod legacyOldName = portalLogName + ".old" vs test const legacyOld = "portal.log.old"; test could reference the prod const (same package).
- [idea] migrationGuard returns error but all paths return nil and the sole caller discards it; the return is forward-looking (2-7 WARN). Defensible to leave for the upcoming task.
