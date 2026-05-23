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

// fileFingerprint captures everything needed to detect mutation of
// a single filesystem entry under the developer's state directory.
//
// All stats are gathered via os.Lstat so symlink mutations (target
// change, file-to-symlink swap) are visible at the snapshot layer.
//
// hashed reports whether sha256 was populated; for files >hashSizeCap
// and for non-regular entries (symlinks, directories) it is false and
// the snapshot relies on size/mtime/ctime (and symlinkTarget for
// symlinks) to detect change.
type fileFingerprint struct {
	exists        bool
	size          int64
	mtimeNanos    int64
	ctimeNanos    int64
	sha256        [32]byte
	hashed        bool
	isSymlink     bool
	symlinkTarget string
}

// snapshotStateDir walks root and returns a map keyed by path
// relative to root. A non-existent root yields an empty map and
// nil error — any post-test content then counts as "created".
//
// Walk uses os.Lstat (NOT Stat) so symlinks at any depth report
// their own inode metadata, not the target's. WalkDir does not
// follow symlinks by default. Sub-symlinks pointing into the
// directory are recorded as entries but not descended into.
func snapshotStateDir(root string) (map[string]fileFingerprint, error) {
	out := make(map[string]fileFingerprint)

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

// fingerprintEntry stats path via Lstat, fills size/mtime/ctime/
// symlinkTarget/sha256, and returns the populated fingerprint.
func fingerprintEntry(path string) (fileFingerprint, error) {
	fp := fileFingerprint{exists: true}

	info, err := os.Lstat(path)
	if err != nil {
		return fileFingerprint{}, err
	}
	fp.size = info.Size()
	fp.mtimeNanos, fp.ctimeNanos = statNanos(info)

	mode := info.Mode()
	switch {
	case mode&os.ModeSymlink != 0:
		fp.isSymlink = true
		target, readErr := os.Readlink(path)
		if readErr != nil {
			return fileFingerprint{}, readErr
		}
		fp.symlinkTarget = target
	case mode.IsRegular() && info.Size() <= hashSizeCap:
		sum, hashErr := hashFile(path)
		if hashErr != nil {
			return fileFingerprint{}, hashErr
		}
		fp.sha256 = sum
		fp.hashed = true
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
func reportStateDirDelta(report errorReporter, root string, pre map[string]fileFingerprint) {
	post, err := snapshotStateDir(root)
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
func emitFieldDeltas(report errorReporter, path string, before, after fileFingerprint) {
	if before.isSymlink != after.isSymlink {
		report(deltaFmt, path, "became-symlink")
		// Other field-level deltas on a type swap are noise;
		// surfacing one clear cause is sufficient.
		return
	}
	if before.isSymlink && after.isSymlink && before.symlinkTarget != after.symlinkTarget {
		report(deltaFmt, path, "symlink-target-changed")
		return
	}
	if before.size != after.size {
		report(deltaFmt, path, "size-changed")
	}
	if before.mtimeNanos != after.mtimeNanos {
		report(deltaFmt, path, "mtime-changed")
	}
	if before.ctimeNanos != after.ctimeNanos {
		report(deltaFmt, path, "ctime-changed")
	}
	if before.hashed && after.hashed && before.sha256 != after.sha256 {
		report(deltaFmt, path, "content-changed")
	}
}

// unionPaths returns the sorted union of map keys.
func unionPaths(a, b map[string]fileFingerprint) []string {
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
