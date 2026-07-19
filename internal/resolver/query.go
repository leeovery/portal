package resolver

import (
	"fmt"
	"os"
	"slices"
)

// SessionLister returns the user-visible (leading-underscore-filtered) set of
// running tmux session names — the same view the picker and tab completion use.
// A nil/empty slice or an error is treated by the resolver as "no sessions".
type SessionLister interface {
	ListSessionNames() ([]string, error)
}

// AliasLookup retrieves the path for a given alias name.
type AliasLookup interface {
	Get(name string) (string, bool)
}

// ZoxideQuerier queries zoxide for a directory matching the given terms.
type ZoxideQuerier interface {
	Query(terms string) (string, error)
}

// DirValidator checks whether a directory exists on disk.
type DirValidator interface {
	Exists(path string) bool
}

// QueryResult is the interface for resolution outcomes.
type QueryResult interface {
	queryResult()
}

// PathResult indicates the query resolved to a directory path that should be
// minted as a fresh session (Axiom 2: directory-domain hits always mint).
// Domain records which arm produced it ("path" / "alias" / "zoxide") for the
// caller's resolution log line.
type PathResult struct {
	Path   string
	Domain string
}

func (*PathResult) queryResult() {}

// SessionResult indicates the query resolved to an existing running session in
// the session domain. Domain is "session" for an exact-name hit; a later task
// also produces SessionResult with Domain "glob" for glob expansion.
type SessionResult struct {
	Name   string
	Domain string
}

func (*SessionResult) queryResult() {}

// MissResult indicates the query resolved to nothing across every domain — a
// total miss. It carries the raw input so the caller can render the hard-fail
// escape-hatch error and emit the resolution log line (domain = miss). There is
// no implicit TUI-picker fallback (spec § Miss handling — total miss is a hard
// fail).
type MissResult struct {
	Target string
}

func (*MissResult) queryResult() {}

// DirNotFoundError indicates a resolved directory does not exist on disk.
type DirNotFoundError struct {
	Path string
}

// Error returns the user-facing error message.
func (e *DirNotFoundError) Error() string {
	return fmt.Sprintf("Directory not found: %s", e.Path)
}

// OSDirValidator checks directory existence using the real filesystem.
type OSDirValidator struct{}

// Exists reports whether path is an existing directory on disk.
func (v *OSDirValidator) Exists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// QueryResolver applies the resolution chain: exact session-name match, path
// detection, alias lookup, zoxide query, then a total miss (hard fail).
type QueryResolver struct {
	sessions     SessionLister
	aliases      AliasLookup
	zoxide       ZoxideQuerier
	dirValidator DirValidator
}

// NewQueryResolver creates a QueryResolver with the given dependencies.
func NewQueryResolver(sessions SessionLister, aliases AliasLookup, zoxide ZoxideQuerier, dirValidator DirValidator) *QueryResolver {
	return &QueryResolver{
		sessions:     sessions,
		aliases:      aliases,
		zoxide:       zoxide,
		dirValidator: dirValidator,
	}
}

// Resolve applies the resolution chain for the given query.
// A glob pre-check runs first: a target containing glob metacharacters is
// session-domain by construction and is expanded against the user-visible
// session set, never reaching the directory chain. Otherwise the session domain
// is checked: an exact match against the user-visible session set yields a
// SessionResult (attach). Otherwise, path-like arguments are resolved directly
// via ResolvePath, and non-path arguments are checked against aliases, then
// zoxide; a target that resolves nowhere yields a MissResult (the caller
// hard-fails, no TUI fallback). After alias or zoxide resolution, the directory
// is validated on disk.
func (qr *QueryResolver) Resolve(query string) (QueryResult, error) {
	// Glob pre-check (spec § Target resolution precedence step 1): a target
	// containing glob metacharacters is session-domain by construction —
	// expanded against the user-visible session set and never run through the
	// path/alias/zoxide chain. Unconditional, so a directory path whose name
	// contains glob metacharacters (e.g. ~/tmp/foo[1]) is captured here and
	// hard-fails as an unreachable bare positional rather than reaching
	// ResolvePath (reach it with -p). Zero matches is a total miss (hard fail);
	// at single-target arity a multi-match resolves to the first match — the
	// per-match window fan-out is the Phase 3 burst.
	if HasGlobMeta(query) {
		names, _ := qr.sessions.ListSessionNames()
		matches := MatchSessions(query, names)
		if len(matches) == 0 {
			return &MissResult{Target: query}, nil
		}
		return &SessionResult{Name: matches[0], Domain: "glob"}, nil
	}

	// Session domain first (spec § Target resolution precedence: exact session
	// name → path → alias → zoxide). Fetch the user-visible session set once; a
	// nil/empty list or a lister error collapses to "no sessions" — the tmux
	// client already returns ([]string{}, nil) when no server runs, and an
	// error is not surfaced here (treated as no match).
	if names, err := qr.sessions.ListSessionNames(); err == nil && slices.Contains(names, query) {
		return &SessionResult{Name: query, Domain: "session"}, nil
	}

	if IsPathArgument(query) {
		resolved, err := ResolvePath(query)
		if err != nil {
			return nil, err
		}
		return &PathResult{Path: resolved, Domain: "path"}, nil
	}

	// Alias lookup
	if path, ok := qr.aliases.Get(query); ok {
		return qr.validatedPath(path, "alias")
	}

	// Zoxide query. A zoxide error (not installed / no match) is swallowed here
	// so the bare-target chain continues to the miss tail — unlike the pinned
	// -z, which errors explicitly (spec § Domain-pinning flags).
	if path, err := qr.zoxide.Query(query); err == nil {
		return qr.validatedPath(path, "zoxide")
	}

	// No domain resolved the target: a total miss. The caller turns this into
	// the hard-fail escape-hatch error (spec § Miss handling); there is no
	// implicit TUI-picker fallback.
	return &MissResult{Target: query}, nil
}

// validatedPath returns a PathResult (tagged with the resolving domain) after
// verifying the directory exists on disk. A non-existent directory is a hard
// error (DirNotFoundError), distinct from a miss.
func (qr *QueryResolver) validatedPath(path, domain string) (QueryResult, error) {
	if !qr.dirValidator.Exists(path) {
		return nil, &DirNotFoundError{Path: path}
	}
	return &PathResult{Path: path, Domain: domain}, nil
}
