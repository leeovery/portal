package portaltest

import (
	"crypto/sha256"
	"errors"
	"fmt"
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

// FingerprintDelta captures a single per-(path, field) change between
// two snapshots. Path is the relative path within the snapshot root;
// Field is one of the canonical change classes (see DiffFingerprints).
// Pre is the zero value when Field == "created"; Post is the zero
// value when Field == "deleted".
type FingerprintDelta struct {
	Path  string
	Field string
	Pre   Fingerprint
	Post  Fingerprint
}

// Canonical Field values returned by DiffFingerprints.
const (
	fieldCreated       = "created"
	fieldDeleted       = "deleted"
	fieldSize          = "size"
	fieldMtime         = "mtime"
	fieldCtime         = "ctime"
	fieldContent       = "content"
	fieldHashed        = "hashed"
	fieldSymlinkTarget = "symlink-target"
	fieldBecameSymlink = "became-symlink"
)

// DiffFingerprints returns the set of deltas between pre and post.
// The returned slice is sorted by (Path, Field) so diagnostics built
// on top of it are reproducible across re-runs.
//
// Per-path semantics: a path missing from one side yields a single
// "created" or "deleted" delta; an IsSymlink flip yields a single
// "became-symlink" delta (other channels are noise on a type swap);
// otherwise zero or more field deltas (size, mtime, ctime, content,
// hashed, symlink-target). "hashed" fires when the Hashed flag
// flipped (file crossed hashSizeCap); "content" only when both
// sides were Hashed and Sha256 differs.
func DiffFingerprints(pre, post map[string]Fingerprint) []FingerprintDelta {
	paths := unionPaths(pre, post)
	out := make([]FingerprintDelta, 0, len(paths))
	for _, path := range paths {
		before, hadBefore := pre[path]
		after, hasAfter := post[path]
		switch {
		case !hadBefore && hasAfter:
			out = append(out, FingerprintDelta{Path: path, Field: fieldCreated, Post: after})
		case hadBefore && !hasAfter:
			out = append(out, FingerprintDelta{Path: path, Field: fieldDeleted, Pre: before})
		case hadBefore && hasAfter:
			out = append(out, fieldDeltas(path, before, after)...)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Field < out[j].Field
	})
	return out
}

// fieldDeltas returns per-field deltas for a path in both snapshots.
// A type swap short-circuits to the became-symlink signal alone.
func fieldDeltas(path string, before, after Fingerprint) []FingerprintDelta {
	mk := func(field string) FingerprintDelta {
		return FingerprintDelta{Path: path, Field: field, Pre: before, Post: after}
	}
	if before.IsSymlink != after.IsSymlink {
		return []FingerprintDelta{mk(fieldBecameSymlink)}
	}
	if before.IsSymlink && after.IsSymlink && before.SymlinkTarget != after.SymlinkTarget {
		return []FingerprintDelta{mk(fieldSymlinkTarget)}
	}

	var out []FingerprintDelta
	if before.Size != after.Size {
		out = append(out, mk(fieldSize))
	}
	if before.MtimeNanos != after.MtimeNanos {
		out = append(out, mk(fieldMtime))
	}
	if before.CtimeNanos != after.CtimeNanos {
		out = append(out, mk(fieldCtime))
	}
	if before.Hashed != after.Hashed {
		out = append(out, mk(fieldHashed))
	}
	if before.Hashed && after.Hashed && before.Sha256 != after.Sha256 {
		out = append(out, mk(fieldContent))
	}
	return out
}

// FormatFingerprint renders fp compactly for error diagnostics where
// the full fingerprint must be shown (created / deleted deltas).
func FormatFingerprint(fp Fingerprint) string {
	if fp.IsSymlink {
		return fmt.Sprintf("symlink(target=%q, mtime=%d ns, ctime=%d ns)",
			fp.SymlinkTarget, fp.MtimeNanos, fp.CtimeNanos)
	}
	if fp.Hashed {
		return fmt.Sprintf("file(size=%d, mtime=%d ns, ctime=%d ns, sha256=%x)",
			fp.Size, fp.MtimeNanos, fp.CtimeNanos, fp.Sha256)
	}
	return fmt.Sprintf("entry(size=%d, mtime=%d ns, ctime=%d ns, hashed=false)",
		fp.Size, fp.MtimeNanos, fp.CtimeNanos)
}

// FormatDelta renders d as a single line for t.Errorf / t.Fatalf.
// Created / deleted variants embed FormatFingerprint of the surviving
// side; field-mutation variants embed pre and post values for the field.
func FormatDelta(d FingerprintDelta) string {
	switch d.Field {
	case fieldCreated:
		return fmt.Sprintf("%s: created (post=%s)", d.Path, FormatFingerprint(d.Post))
	case fieldDeleted:
		return fmt.Sprintf("%s: deleted (pre=%s)", d.Path, FormatFingerprint(d.Pre))
	case fieldBecameSymlink:
		return fmt.Sprintf("%s.IsSymlink: pre=%v post=%v", d.Path, d.Pre.IsSymlink, d.Post.IsSymlink)
	case fieldSymlinkTarget:
		return fmt.Sprintf("%s.SymlinkTarget: pre=%q post=%q",
			d.Path, d.Pre.SymlinkTarget, d.Post.SymlinkTarget)
	case fieldSize:
		return fmt.Sprintf("%s.Size: pre=%d post=%d", d.Path, d.Pre.Size, d.Post.Size)
	case fieldMtime:
		return fmt.Sprintf("%s.MtimeNanos: pre=%d post=%d (Δ=%d ns)",
			d.Path, d.Pre.MtimeNanos, d.Post.MtimeNanos, d.Post.MtimeNanos-d.Pre.MtimeNanos)
	case fieldCtime:
		return fmt.Sprintf("%s.CtimeNanos: pre=%d post=%d (Δ=%d ns)",
			d.Path, d.Pre.CtimeNanos, d.Post.CtimeNanos, d.Post.CtimeNanos-d.Pre.CtimeNanos)
	case fieldHashed:
		return fmt.Sprintf("%s.Hashed: pre=%v post=%v", d.Path, d.Pre.Hashed, d.Post.Hashed)
	case fieldContent:
		return fmt.Sprintf("%s.Sha256: pre=%x post=%x", d.Path, d.Pre.Sha256, d.Post.Sha256)
	default:
		return fmt.Sprintf("%s.%s: pre=%+v post=%+v", d.Path, d.Field, d.Pre, d.Post)
	}
}

// reportStateDirDelta snapshots root and reports every delta against
// pre via report (one call per delta — the caller receives the full
// picture). Delegates to DiffFingerprints and translates the canonical
// Field names to the legacy backstop strings the in-package consumers
// (isolated_env.go t.Cleanup hooks) already match on.
//
// Format: "portaltest backstop: developer state dir mutated at %s: %s"
func reportStateDirDelta(report errorReporter, root string, pre map[string]Fingerprint) {
	post, err := SnapshotStateDir(root)
	if err != nil {
		report("portaltest backstop: post-test snapshot of %s failed: %v", root, err)
		return
	}
	for _, d := range DiffFingerprints(pre, post) {
		report(deltaFmt, d.Path, backstopFieldLabel(d.Field))
	}
}

const deltaFmt = "portaltest backstop: developer state dir mutated at %s: %s"

// backstopFieldLabels maps a canonical DiffFingerprints Field to the
// legacy backstop string. Existing meta-tests assert exact equality
// against these, so the legacy "-changed" suffix must be preserved.
// created / deleted / became-symlink pass through verbatim and are
// absent from the map.
var backstopFieldLabels = map[string]string{
	fieldSize:          "size-changed",
	fieldMtime:         "mtime-changed",
	fieldCtime:         "ctime-changed",
	fieldContent:       "content-changed",
	fieldHashed:        "hashed-changed",
	fieldSymlinkTarget: "symlink-target-changed",
}

func backstopFieldLabel(field string) string {
	if label, ok := backstopFieldLabels[field]; ok {
		return label
	}
	return field
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
