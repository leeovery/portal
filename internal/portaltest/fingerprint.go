// Test-only. Importing this package from non-*_test.go files is
// prohibited — the *testing.T parameter on the exported helpers
// enforces this structurally.

package portaltest

import (
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

// hashSizeCap is the inclusive upper bound on file size eligible for
// SHA-256 content hashing. Files larger than this are still
// fingerprinted (size + mtime + ctime) but not hashed — content
// changes that do not move size/mtime/ctime would slip past the
// backstop for >1 MiB files, which is an acceptable trade-off
// against the cost of hashing arbitrarily large files in every
// test cleanup.
const hashSizeCap = 1 << 20 // 1 MiB

// Fingerprint captures everything needed to detect mutation of a
// single filesystem entry under a directory tree.
//
// All stats are gathered via os.Lstat so symlink mutations (target
// change, file-to-symlink swap) are visible at the snapshot layer.
//
// Hashed reports whether Sha256 was populated; for files >hashSizeCap
// and for non-regular entries (symlinks, directories) it is false and
// the snapshot relies on Size/MtimeNanos/CtimeNanos (and SymlinkTarget
// for symlinks) to detect change.
//
// Exported (with exported fields) so out-of-package integration tests
// can take a directory snapshot at a caller-chosen point in time and
// compare two snapshots field-by-field. The shared shape keeps the
// in-package backstop and out-of-package callers from drifting.
type Fingerprint struct {
	Exists        bool
	Size          int64
	MtimeNanos    int64
	CtimeNanos    int64
	Sha256        [32]byte
	Hashed        bool
	IsSymlink     bool
	SymlinkTarget string
}

// SnapshotStateDir walks root and returns a map keyed by path
// relative to root. A non-existent root yields an empty map and
// nil error — any post-snapshot content then counts as "created".
//
// Walk uses os.Lstat (NOT Stat) so symlinks at any depth report
// their own inode metadata, not the target's. WalkDir does not
// follow symlinks by default. Sub-symlinks pointing into the
// directory are recorded as entries but not descended into.
//
// Exported for integration tests that need to compare two
// snapshots at caller-chosen points (e.g. before/after a SIGKILL).
// The in-package backstop in isolated_env.go consumes the same
// function so the two paths cannot drift.
func SnapshotStateDir(root string) (map[string]Fingerprint, error) {
	out := make(map[string]Fingerprint)

	if _, err := os.Lstat(root); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, nil
		}
		return nil, err
	}

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Surface walk errors at lower layers; one unreadable
			// entry should not silently drop the rest.
			return err
		}
		if path == root {
			// Skip the root itself; the snapshot tracks entries
			// inside the state dir, not the dir's own metadata.
			return nil
		}

		fp, fpErr := fingerprintEntry(path)
		if fpErr != nil {
			return fpErr
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		out[rel] = fp
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return out, nil
}

// fingerprintEntry stats path via Lstat, fills Size/MtimeNanos/
// CtimeNanos/SymlinkTarget/Sha256, and returns the populated
// Fingerprint.
func fingerprintEntry(path string) (Fingerprint, error) {
	fp := Fingerprint{Exists: true}

	info, err := os.Lstat(path)
	if err != nil {
		return Fingerprint{}, err
	}
	fp.Size = info.Size()
	fp.MtimeNanos, fp.CtimeNanos = statNanos(info)

	mode := info.Mode()
	switch {
	case mode&os.ModeSymlink != 0:
		fp.IsSymlink = true
		target, readErr := os.Readlink(path)
		if readErr != nil {
			return Fingerprint{}, readErr
		}
		fp.SymlinkTarget = target
	case mode.IsRegular() && info.Size() <= hashSizeCap:
		sum, hashErr := hashFile(path)
		if hashErr != nil {
			return Fingerprint{}, hashErr
		}
		fp.Sha256 = sum
		fp.Hashed = true
	}
	return fp, nil
}

// hashFile returns SHA-256 of path's contents.
func hashFile(path string) ([32]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(data), nil
}

// errorReporter narrows the surface area to t.Errorf. Production
// callers pass *testing.T.Errorf; meta-tests pass a recorder so the
// cleanup logic can be exercised without polluting the host
// testing.T with intentional failures.
type errorReporter func(format string, args ...any)

// reportStateDirDelta snapshots root and reports any delta against
// pre via report. Every changed path generates exactly one report
// call; the function does not stop after the first delta — the
// caller receives the full picture so a single test surfacing
// multiple violations is debuggable in one run.
//
// Delta types reported:
//   - "created" — path is absent from pre but present now
//   - "deleted" — path is present in pre but absent now
//   - "size-changed"
//   - "mtime-changed"
//   - "ctime-changed"
//   - "content-changed"
//   - "became-symlink" — non-symlink → symlink (or vice versa)
//   - "symlink-target-changed"
//
// All deltas surface against the same path with the format
// "portaltest backstop: developer state dir mutated at %s: %s".
func reportStateDirDelta(report errorReporter, root string, pre map[string]Fingerprint) {
	post, err := SnapshotStateDir(root)
	if err != nil {
		report("portaltest backstop: post-test snapshot of %s failed: %v", root, err)
		return
	}

	// Build the union of paths and walk in deterministic order so
	// failures are reproducible across runs.
	paths := unionPaths(pre, post)

	for _, path := range paths {
		before, hadBefore := pre[path]
		after, hasAfter := post[path]

		switch {
		case !hadBefore && hasAfter:
			report(deltaFmt, path, "created")
		case hadBefore && !hasAfter:
			report(deltaFmt, path, "deleted")
		case hadBefore && hasAfter:
			emitFieldDeltas(report, path, before, after)
		}
	}
}

const deltaFmt = "portaltest backstop: developer state dir mutated at %s: %s"

// emitFieldDeltas reports every field-level difference between
// before and after for a path that exists in both snapshots.
// Order is fixed (became-symlink first, then symlink target, then
// size/mtime/ctime/content) so multi-field failures read cleanly.
func emitFieldDeltas(report errorReporter, path string, before, after Fingerprint) {
	if before.IsSymlink != after.IsSymlink {
		report(deltaFmt, path, "became-symlink")
		// Other field-level deltas on a type swap are noise;
		// surfacing one clear cause is sufficient.
		return
	}
	if before.IsSymlink && after.IsSymlink && before.SymlinkTarget != after.SymlinkTarget {
		report(deltaFmt, path, "symlink-target-changed")
		return
	}
	if before.Size != after.Size {
		report(deltaFmt, path, "size-changed")
	}
	if before.MtimeNanos != after.MtimeNanos {
		report(deltaFmt, path, "mtime-changed")
	}
	if before.CtimeNanos != after.CtimeNanos {
		report(deltaFmt, path, "ctime-changed")
	}
	if before.Hashed && after.Hashed && before.Sha256 != after.Sha256 {
		report(deltaFmt, path, "content-changed")
	}
}

// unionPaths returns the sorted union of map keys.
func unionPaths(a, b map[string]Fingerprint) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		seen[k] = struct{}{}
	}
	for k := range b {
		seen[k] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// resolveDevStateDir resolves the developer's real state directory
// using PRE-OVERRIDE env semantics. It MUST be called before the
// helper mutates the spawned subprocess's XDG_CONFIG_HOME so the
// snapshot targets the developer's live install, not the per-test
// temp dir.
//
// Resolution precedence mirrors internal/xdg.ConfigBase but is
// inlined here to avoid a dependency cycle and to make the
// pre-override capture explicit at the call site:
//  1. $XDG_CONFIG_HOME (verbatim) if non-empty
//  2. $HOME/.config
//  3. "" — caller is expected to skip the backstop if HOME is unset
func resolveDevStateDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "portal", "state")
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".config", "portal", "state")
	}
	return ""
}

// statNanos extracts (mtime, ctime) nanoseconds from a FileInfo.
// Returns (0, 0) when the underlying syscall.Stat_t is not
// available — on supported platforms (linux, darwin) this never
// happens; the zero return is a graceful degradation that keeps
// the backstop functional via size + content-hash even if the
// stat call returns an unexpected type.
func statNanos(info os.FileInfo) (mtime, ctime int64) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0
	}
	return statTimeNanos(st)
}
