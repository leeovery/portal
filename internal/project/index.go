package project

// Index maps a directory's canonical key to the Project stored there,
// canonicalising each stored path once instead of paying an EvalSymlinks syscall
// per project per lookup. It is a derived cache: rebuild it whenever the project
// set changes. Construct via NewIndex or NewIndexBy; the zero value matches
// nothing and keys by CanonicalDirKey.
type Index struct {
	byKey map[string]Project
	key   func(string) string
}

// NewIndex builds an Index keyed by CanonicalDirKey. An empty slice yields a
// usable Index whose Match always reports not-found. Two projects reducing to
// the same canonical key collide last-write-wins.
func NewIndex(projects []Project) Index {
	return NewIndexBy(projects, CanonicalDirKey)
}

// NewIndexBy builds an Index whose stored paths and looked-up directories are
// both reduced by key, so the two sides always agree. A nil key is
// CanonicalDirKey.
func NewIndexBy(projects []Project, key func(string) string) Index {
	idx := Index{byKey: make(map[string]Project, len(projects)), key: key}
	for _, p := range projects {
		idx.byKey[idx.keyOf(p.Path)] = p
	}
	return idx
}

// Match finds the Project whose directory matches dirPath, returning
// dirPath's key whether or not one matched — so a caller needing the canonical
// key reuses it rather than paying a second EvalSymlinks.
func (idx Index) Match(dirPath string) (Project, string, bool) {
	key := idx.keyOf(dirPath)
	p, ok := idx.byKey[key]
	return p, key, ok
}

func (idx Index) keyOf(dirPath string) string {
	if idx.key == nil {
		return CanonicalDirKey(dirPath)
	}
	return idx.key(dirPath)
}
