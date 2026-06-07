package project

import (
	"path/filepath"

	"github.com/leeovery/portal/internal/resolver"
)

// CanonicalDirKey produces the canonical lookup key for a directory path. It is
// the single source of truth for the dir→project match key: both the stamped
// @portal-dir value and the fallback-derived git root must be reduced to this
// form before being compared against a stored Project.Path, otherwise a session
// would silently drop out of its group.
//
// The canonical form is: ~ expanded to the home directory, resolved to an
// absolute path, symlinks evaluated, then cleaned. When EvalSymlinks fails
// (most commonly because the directory does not exist on disk), it falls back
// to filepath.Clean(abs) so a missing directory still yields a stable key.
//
// Tilde expansion is delegated to resolver.ExpandTilde (the single source of
// truth) rather than re-implemented here. Note that resolver.NormalisePath
// deliberately does NOT evaluate symlinks; this helper must, so it is built
// from the lower-level primitives rather than reusing NormalisePath.
func CanonicalDirKey(path string) string {
	expanded := resolver.ExpandTilde(path)

	abs, err := filepath.Abs(expanded)
	if err != nil {
		// Abs only fails when the working directory cannot be determined; fall
		// back to a cleaned form of the expanded path so a key still results.
		return filepath.Clean(expanded)
	}

	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// The path (or a component) does not exist or cannot be resolved; fall
		// back to Clean(abs) so a missing directory still yields a stable key.
		return filepath.Clean(abs)
	}

	return filepath.Clean(resolved)
}

// MatchProjectByDir finds the Project whose directory matches dirPath by
// canonical key. The stored Project.Path is canonicalised on each comparison so
// the match is robust to symlink/trailing-slash/tilde differences between the
// stored path and the lookup path. It returns (Project{}, false) when no
// project matches — the caller routes such a path to the Unknown/Untagged
// bucket.
func MatchProjectByDir(projects []Project, dirPath string) (Project, bool) {
	want := CanonicalDirKey(dirPath)

	for _, p := range projects {
		if CanonicalDirKey(p.Path) == want {
			return p, true
		}
	}

	return Project{}, false
}
