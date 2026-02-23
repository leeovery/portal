package resolver

import (
	"fmt"
	"os"
)

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

// QueryResolver applies the resolution chain: path detection, alias lookup,
// zoxide query, then TUI fallback.
type QueryResolver struct {
	aliases      AliasLookup
	zoxide       ZoxideQuerier
	dirValidator DirValidator
}

// NewQueryResolver creates a QueryResolver with the given dependencies.
func NewQueryResolver(aliases AliasLookup, zoxide ZoxideQuerier, dirValidator DirValidator) *QueryResolver {
	return &QueryResolver{
		aliases:      aliases,
		zoxide:       zoxide,
		dirValidator: dirValidator,
	}
}

// Resolve applies the resolution chain for the given query.
// Path-like arguments are resolved directly via ResolvePath.
// Non-path arguments are checked against aliases, then zoxide, then fall back to TUI.
// After alias or zoxide resolution, the directory is validated on disk.
func (qr *QueryResolver) Resolve(query string) (QueryResult, error) {
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
