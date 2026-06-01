package log

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// symlinkFunc is the test-only seam over os.Symlink so the swing-failure path
// can be exercised deterministically (forcing a natural os.Symlink failure is
// awkward — it would require an unwritable parent that also can't host the prior
// symlink we assert is preserved). It is unexported and minimal; tests swap it
// and restore via t.Cleanup. Production always uses os.Symlink.
var symlinkFunc = os.Symlink

// pidSymlinkTmp is the pid-scoped temp link name used by the atomic swing:
// ${stateDir}/portal.log.<pid>.symlink.tmp. Embedding the caller's pid (the
// running process's os.Getpid() in production) means two portal processes
// swinging concurrently can never collide on the temp name. A single process
// performs at most one swing at a time (the sink holds its mutex across reopen),
// so no per-swing counter is needed.
func pidSymlinkTmp(stateDir string, pid int) string {
	return filepath.Join(stateDir, portalLogName+"."+strconv.Itoa(pid)+".symlink.tmp")
}

// legacyOldName is the basename of the single rotated file the pre-migration
// logger left alongside a regular-file portal.log. The new symlink-based scheme
// never writes it, so the first-run migration guard deletes it as legacy debris.
const legacyOldName = portalLogName + ".old"

// migrationGuard clears the pre-migration legacy slate so the portal.log name is
// free to become a symlink on the first reopen under the new scheme. It is
// invoked from the sink's reopen path BEFORE swingSymlink.
//
// Steps: lstat ${stateDir}/portal.log (Lstat, NOT Stat, so a symlink is detected
// as a symlink rather than followed to its target).
//   - ENOENT: nothing to clear — the name is already free. Return.
//   - A symlink: the steady state after the first swing. The guard no-ops,
//     leaving the link and its target untouched. Return.
//   - A regular file (the pre-migration legacy log): os.Remove it, and os.Remove
//     any portal.log.old. Both removals are ENOENT-tolerant best-effort.
//
// The guard fires AT MOST ONCE per file lifetime BY CONSTRUCTION: the very next
// swingSymlink converts portal.log into a symlink, so every subsequent reopen's
// lstat sees a symlink and the guard no-ops forever after. No "already migrated"
// flag is needed — the symlink itself is the marker.
//
// All removals are best-effort. A removal failure is swallowed (consistent with
// how swingSymlink's error is swallowed in the reopen path); the WARN emission on
// such a failure is Task 2-7's territory. The guard never aborts the reopen.
func migrationGuard(stateDir string) error {
	link := symlinkPath(stateDir)

	info, err := os.Lstat(link)
	if os.IsNotExist(err) {
		return nil // Absent entirely: the portal.log name is already free.
	}
	if err != nil {
		return nil // Lstat error (best-effort): do not abort the reopen.
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil // Already a symlink: no-op on every run after the first.
	}

	// A regular file — pre-migration legacy debris. Remove it and any .old sibling
	// best-effort so the swing can claim the portal.log name as a symlink.
	_ = os.Remove(link)
	_ = os.Remove(filepath.Join(stateDir, legacyOldName))
	return nil
}

// swingSymlink atomically re-points ${stateDir}/portal.log at target, where
// target is the BARE day-file filename (e.g. portal.log.<today> or, on size-cap
// rotation, portal.log.<today>.<N>). The target is stored RELATIVE — just the
// basename, not an absolute path — so the link stays valid if the state dir is
// moved; the inode-identity check in the sink follows the link regardless.
//
// The swing is: os.Remove the pid-scoped temp (best-effort — reclaims a leftover
// from a prior crash of THIS pid between Symlink and Rename; ENOENT is ignored),
// os.Symlink(target, pidTmp), then os.Rename(pidTmp, link). Rename is atomic on
// Unix and last-writer-wins. Because every concurrent swinger's target is
// identical (the same day file for the same day), a racing swing is benign.
//
// On any Symlink/Rename error the wrapped error is returned and the prior symlink
// is left untouched — the caller (best-effort write path, Task 2-7) treats a
// swing failure as WARN-and-continue and keeps writing to the already-open fd.
//
// A temp leaked by a crash between Symlink and Rename is reclaimed best-effort on
// the next swing (the os.Remove first step) and by `portal clean` (which sweeps
// portal.log.* siblings — out of scope here).
func swingSymlink(stateDir, target string) error {
	return swingSymlinkAs(stateDir, target, os.Getpid())
}

// swingSymlinkAs is swingSymlink with an explicit pid, factored so the
// cross-process concurrency test can model genuinely distinct processes (each
// with its own pid-scoped temp). Production always calls through swingSymlink
// with os.Getpid(); the single-swing-per-pid invariant (the sink holds its mutex
// across reopen) means a given pid's temp is never contended in production.
func swingSymlinkAs(stateDir, target string, pid int) error {
	link := symlinkPath(stateDir)
	pidTmp := pidSymlinkTmp(stateDir, pid)

	// Reclaim a same-pid temp leaked by a prior crash; ignore ENOENT.
	if err := os.Remove(pidTmp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale symlink temp %s: %w", pidTmp, err)
	}

	if err := symlinkFunc(target, pidTmp); err != nil {
		return fmt.Errorf("create symlink temp %s -> %s: %w", pidTmp, target, err)
	}

	if err := os.Rename(pidTmp, link); err != nil {
		return fmt.Errorf("rename symlink temp %s -> %s: %w", pidTmp, link, err)
	}
	return nil
}
