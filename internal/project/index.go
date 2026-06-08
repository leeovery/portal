package project

// Index is a pre-canonicalised lookup table mapping a directory's canonical key
// (CanonicalDirKey) to the Project stored at that directory. It exists to make a
// grouped TUI render cheap: the stored Project.Path values are canonicalised
// ONCE at construction (NewIndex), rather than re-running CanonicalDirKey — a
// filepath.EvalSymlinks syscall — over every stored project on every session,
// every render (the O(sessions × projects) syscall cost a per-call linear scan
// over the stored project set would incur).
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
// It returns (Project{}, key, false) when no project matches — the same
// canonical-key match semantics a linear scan over the stored project set would
// yield, but at O(1) amortised cost with no per-call EvalSymlinks over that set.
//
// The returned key is always CanonicalDirKey(dirPath) — the canonical
// (EvalSymlinks-resolved) form of the input — whether or not a project matched.
// It is returned so callers that need the canonical key (e.g. buildByProject's
// By-Project GroupKey) reuse the single computation performed here instead of
// paying a second identical EvalSymlinks syscall.
//
// An empty dirPath is canonicalised like any other path; since a real project's
// canonical key never collides with CanonicalDirKey("") (the process working
// directory), an empty dir reports not-found. Grouping callers additionally
// guard s.Dir == "" before calling.
func (idx Index) Match(dirPath string) (Project, string, bool) {
	key := CanonicalDirKey(dirPath)
	p, ok := idx.byKey[key]
	return p, key, ok
}
