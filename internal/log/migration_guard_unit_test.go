package log

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// legacyOld is the basename of the pre-migration single rotated file the old
// logger left alongside a regular-file portal.log.
const legacyOld = "portal.log.old"

func TestMigrationGuard_RemovesLegacyRegularFilePortalLog(t *testing.T) {
	dir := t.TempDir()

	// Seed a pre-migration regular-file portal.log.
	if err := os.WriteFile(symlinkPath(dir), []byte("legacy log\n"), 0o600); err != nil {
		t.Fatalf("seed regular-file portal.log: %v", err)
	}

	if err := migrationGuard(dir); err != nil {
		t.Fatalf("migrationGuard: %v", err)
	}

	// The regular file must be gone, freeing the portal.log name for the swing.
	if _, err := os.Lstat(symlinkPath(dir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("portal.log still present after guard (lstat err = %v); want removed", err)
	}
}

func TestMigrationGuard_RemovesPortalLogOldAlongsideRegularFile(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(symlinkPath(dir), []byte("legacy log\n"), 0o600); err != nil {
		t.Fatalf("seed regular-file portal.log: %v", err)
	}
	oldPath := filepath.Join(dir, legacyOld)
	if err := os.WriteFile(oldPath, []byte("legacy old\n"), 0o600); err != nil {
		t.Fatalf("seed portal.log.old: %v", err)
	}

	if err := migrationGuard(dir); err != nil {
		t.Fatalf("migrationGuard: %v", err)
	}

	if _, err := os.Lstat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("portal.log.old still present after guard (lstat err = %v); want removed", err)
	}
}

func TestMigrationGuard_NoOpsWhenPortalLogAlreadySymlink(t *testing.T) {
	dir := t.TempDir()

	// portal.log is already a symlink (steady state after the first swing). The
	// guard must leave both the link AND its target untouched.
	const targetName = "portal.log.2026-05-30"
	targetPath := filepath.Join(dir, targetName)
	if err := os.WriteFile(targetPath, []byte("day file\n"), 0o600); err != nil {
		t.Fatalf("seed day file: %v", err)
	}
	if err := os.Symlink(targetName, symlinkPath(dir)); err != nil {
		t.Fatalf("seed symlink: %v", err)
	}

	if err := migrationGuard(dir); err != nil {
		t.Fatalf("migrationGuard: %v", err)
	}

	// The symlink survives and still points at the same target.
	got, err := os.Readlink(symlinkPath(dir))
	if err != nil {
		t.Fatalf("readlink after guard: %v", err)
	}
	if got != targetName {
		t.Errorf("symlink target = %q after guard, want %q (untouched)", got, targetName)
	}
	// The target file survives with its contents intact.
	b, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target after guard: %v", err)
	}
	if string(b) != "day file\n" {
		t.Errorf("target contents = %q after guard, want %q (untouched)", string(b), "day file\n")
	}
}

func TestMigrationGuard_ToleratesAbsentPortalLogOld(t *testing.T) {
	dir := t.TempDir()

	// Regular-file portal.log present, but NO portal.log.old to remove.
	if err := os.WriteFile(symlinkPath(dir), []byte("legacy log\n"), 0o600); err != nil {
		t.Fatalf("seed regular-file portal.log: %v", err)
	}

	if err := migrationGuard(dir); err != nil {
		t.Fatalf("migrationGuard with absent portal.log.old: %v", err)
	}

	if _, err := os.Lstat(symlinkPath(dir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("portal.log still present after guard (lstat err = %v); want removed", err)
	}
}

func TestMigrationGuard_NoOpsWhenPortalLogAbsentEntirely(t *testing.T) {
	dir := t.TempDir()

	// Empty state dir: nothing to clear.
	if err := migrationGuard(dir); err != nil {
		t.Fatalf("migrationGuard on empty dir: %v", err)
	}

	if _, err := os.Lstat(symlinkPath(dir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("portal.log unexpectedly present after guard on empty dir (lstat err = %v)", err)
	}
}

func TestMigrationGuard_DoesNotFireOnSecondRunAfterSymlinkExists(t *testing.T) {
	dir := t.TempDir()

	// First run: a legacy regular-file portal.log plus a portal.log.old.
	if err := os.WriteFile(symlinkPath(dir), []byte("legacy log\n"), 0o600); err != nil {
		t.Fatalf("seed regular-file portal.log: %v", err)
	}
	oldPath := filepath.Join(dir, legacyOld)
	if err := os.WriteFile(oldPath, []byte("legacy old\n"), 0o600); err != nil {
		t.Fatalf("seed portal.log.old: %v", err)
	}

	if err := migrationGuard(dir); err != nil {
		t.Fatalf("first migrationGuard: %v", err)
	}

	// Simulate the swing that immediately follows the first guard run: portal.log
	// becomes a symlink to today's file.
	const targetName = "portal.log.2026-05-30"
	targetPath := filepath.Join(dir, targetName)
	if err := os.WriteFile(targetPath, []byte("day file\n"), 0o600); err != nil {
		t.Fatalf("seed day file: %v", err)
	}
	if err := os.Symlink(targetName, symlinkPath(dir)); err != nil {
		t.Fatalf("seed symlink after first guard: %v", err)
	}
	// Re-seed a portal.log.old to prove the second run leaves it ALONE.
	if err := os.WriteFile(oldPath, []byte("new old\n"), 0o600); err != nil {
		t.Fatalf("re-seed portal.log.old: %v", err)
	}

	// Second run: guard sees a symlink and must delete nothing.
	if err := migrationGuard(dir); err != nil {
		t.Fatalf("second migrationGuard: %v", err)
	}

	// Symlink untouched.
	got, err := os.Readlink(symlinkPath(dir))
	if err != nil {
		t.Fatalf("readlink after second guard: %v", err)
	}
	if got != targetName {
		t.Errorf("symlink target = %q after second guard, want %q (untouched)", got, targetName)
	}
	// The re-seeded portal.log.old must survive the no-op second run.
	if _, err := os.Lstat(oldPath); err != nil {
		t.Errorf("portal.log.old removed by second-run guard (lstat err = %v); want left intact", err)
	}
}
