package project

// Index is a pre-canonicalised lookup table mapping a directory's canonical key
// (CanonicalDirKey) to the Project stored at that directory. It exists to make a
// grouped TUI render cheap: the stored Project.Path values are canonicalised
// ONCE at construction (NewIndex), rather than re-running CanonicalDirKey — a
// filepath.EvalSymlinks syscall — over every stored project on every session,
// every render (the O(sessions × projects) syscall cost that
// MatchProjectByDir incurs).
//
// An Index is a derived cache of a []Project: it must be rebuilt (via NewIndex)
// whenever the underlying project set changes, or lookups will return stale
// matches. The zero value is not usable; always construct via NewIndex.
type Index struct {
	byKey map[string]Project
}

// NewIndex builds an Index from projects, canonicalising each Project.Path via
// CanonicalDirKey exactly once. A nil or empty projects slice yields an empty —
// but fully usable, non-panicking — Index whose Match always reports not-found.
//
// Collision policy: last-write-wins. When two projects reduce to the same
// canonical key (e.g. one stored with a trailing slash, or two records for the
// same directory), the later project in the slice overwrites the earlier one.
func NewIndex(projects []Project) Index {
	byKey := make(map[string]Project, len(projects))
	for _, p := range projects {
		byKey[CanonicalDirKey(p.Path)] = p
	}
	return Index{byKey: byKey}
}

// Match finds the Project whose directory matches dirPath, canonicalising
// dirPath via CanonicalDirKey exactly once and performing a single map lookup.
// It returns (Project{}, false) when no project matches — semantically identical
// to MatchProjectByDir, but at O(1) amortised cost with no per-call EvalSymlinks
// over the stored project set.
//
// An empty dirPath is canonicalised like any other path; since a real project's
// canonical key never collides with CanonicalDirKey("") (the process working
// directory), an empty dir reports not-found, preserving MatchProjectByDir's
// semantics. Grouping callers additionally guard s.Dir == "" before calling.
func (idx Index) Match(dirPath string) (Project, bool) {
	p, ok := idx.byKey[CanonicalDirKey(dirPath)]
	return p, ok
}
