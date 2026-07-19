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

// PathResult indicates the query resolved to a directory path.
type PathResult struct {
	Path string
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

// FallbackResult indicates no resolution was found; the TUI should be
// launched with the query pre-filled as filter text.
type FallbackResult struct {
	Query string
}

func (*FallbackResult) queryResult() {}

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
// detection, alias lookup, zoxide query, then TUI fallback.
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
// The session domain is checked first: an exact match against the user-visible
// session set yields a SessionResult (attach). Otherwise, path-like arguments
// are resolved directly via ResolvePath, and non-path arguments are checked
// against aliases, then zoxide, then fall back to TUI. After alias or zoxide
// resolution, the directory is validated on disk.
func (qr *QueryResolver) Resolve(query string) (QueryResult, error) {
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
		return &PathResult{Path: resolved}, nil
	}

	// Alias lookup
	if path, ok := qr.aliases.Get(query); ok {
		return qr.validatedPath(path)
	}

	// Zoxide query
	if path, err := qr.zoxide.Query(query); err == nil {
		return qr.validatedPath(path)
	}

	// Zoxide not installed or no match: fall through to TUI
	return &FallbackResult{Query: query}, nil
}

// validatedPath returns a PathResult after verifying the directory exists on disk.
func (qr *QueryResolver) validatedPath(path string) (QueryResult, error) {
	if !qr.dirValidator.Exists(path) {
		return nil, &DirNotFoundError{Path: path}
	}
	return &PathResult{Path: path}, nil
}
