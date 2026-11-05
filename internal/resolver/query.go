package resolver

import (
	"errors"
	"fmt"
	"os"
	"slices"
)

// SessionLister returns the names of the user's live sessions, Portal's own
// left out. An error is treated as "no sessions".
type SessionLister interface {
	ListSessionNames() ([]string, error)
}

// KeptLookup reports whether the saved session of a name no live session
// holds is kept: one the next restore would bring back. An error is treated
// as "not kept".
type KeptLookup interface {
	IsKept(name string) (bool, error)
}

type AliasLookup interface {
	Get(name string) (string, bool)
	Keys() []string
}

type ZoxideQuerier interface {
	Query(terms string) (string, error)
}

type DirValidator interface {
	Exists(path string) bool
}

type QueryResult interface {
	queryResult()
}

type PathResult struct {
	Path   string
	Domain Domain
}

func (*PathResult) queryResult() {}

type SessionResult struct {
	Name   string
	Domain Domain
}

func (*SessionResult) queryResult() {}

// KeptResult is an exact name whose saved session is kept and not live: the
// caller brings it back, then attaches it as it does a SessionResult.
type KeptResult struct {
	Name string
}

func (*KeptResult) queryResult() {}

// MissResult is a total miss: the caller hard-fails on it, with no implicit
// picker fallback.
type MissResult struct {
	Target string
}

func (*MissResult) queryResult() {}

type DirNotFoundError struct {
	Path string
}

func (e *DirNotFoundError) Error() string {
	return fmt.Sprintf("Directory not found: %s", e.Path)
}

type OSDirValidator struct{}

func (v *OSDirValidator) Exists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

type QueryResolver struct {
	sessions     SessionLister
	kept         KeptLookup
	aliases      AliasLookup
	zoxide       ZoxideQuerier
	dirValidator DirValidator
}

// NewQueryResolver returns a resolver that keeps no saved session: WithKept
// gives one that does.
func NewQueryResolver(sessions SessionLister, aliases AliasLookup, zoxide ZoxideQuerier, dirValidator DirValidator) *QueryResolver {
	return &QueryResolver{
		sessions:     sessions,
		aliases:      aliases,
		zoxide:       zoxide,
		dirValidator: dirValidator,
	}
}

// WithKept is a copy of qr that resolves an exact name with a kept saved
// session, as kept judges it, to a *KeptResult; nil keeps none.
func (qr *QueryResolver) WithKept(kept KeptLookup) *QueryResolver {
	c := *qr
	c.kept = kept
	return &c
}

func (qr *QueryResolver) isExactSession(query string) bool {
	names, err := qr.sessions.ListSessionNames()
	if err != nil {
		return false
	}
	return slices.Contains(names, query)
}

func (qr *QueryResolver) isKept(query string) bool {
	if qr.kept == nil {
		return false
	}
	kept, err := qr.kept.IsKept(query)
	return err == nil && kept
}

// exactName resolves query as an exact name: a live session's, then a kept
// saved session's. It is nil for neither.
func (qr *QueryResolver) exactName(query string) QueryResult {
	switch {
	case qr.isExactSession(query):
		return &SessionResult{Name: query, Domain: DomainSession}
	case qr.isKept(query):
		return &KeptResult{Name: query}
	}
	return nil
}

// Resolve applies the single-target chain: exact session name → kept saved
// session → path → alias → zoxide → miss. It does not expand globs — a glob
// matches nothing in the chain and falls through to a miss rather than
// silently taking a first match.
func (qr *QueryResolver) Resolve(query string) (QueryResult, error) {
	if r := qr.exactName(query); r != nil {
		return r, nil
	}

	if IsPathArgument(query) {
		resolved, err := ResolvePath(query)
		if err != nil {
			return nil, err
		}
		return &PathResult{Path: resolved, Domain: DomainPath}, nil
	}

	if path, ok := qr.aliases.Get(query); ok {
		return qr.validatedPath(path, DomainAlias)
	}

	// A zoxide error (not installed, no match) falls through to the miss tail;
	// only the -z pin surfaces it.
	if path, err := qr.zoxide.Query(query); err == nil {
		return qr.validatedPath(path, DomainZoxide)
	}

	return &MissResult{Target: query}, nil
}

func expandSessionGlobAll(pattern string, names []string) []QueryResult {
	matches := MatchGlob(pattern, names)
	if len(matches) == 0 {
		return []QueryResult{&MissResult{Target: pattern}}
	}
	results := make([]QueryResult, 0, len(matches))
	for _, m := range matches {
		results = append(results, &SessionResult{Name: m, Domain: DomainGlob})
	}
	return results
}

// ResolveBareAll is the multi-target Resolve: a glob expands to one result per
// matching live session and every failure is collected as a *MissResult so
// pre-flight reports all of them at once. The returned error is always nil.
func (qr *QueryResolver) ResolveBareAll(query string) ([]QueryResult, error) {
	if HasGlobMeta(query) {
		names, _ := qr.sessions.ListSessionNames()
		return expandSessionGlobAll(query, names), nil
	}

	r, err := qr.Resolve(query)
	if err != nil {
		return []QueryResult{&MissResult{Target: query}}, nil
	}
	return []QueryResult{r}, nil
}

// ResolveSessionPinAll is the multi-target ResolveSessionPin: a glob expands over
// the live session set alone and a miss is collected as a *MissResult instead of
// erroring. The returned error is always nil.
func (qr *QueryResolver) ResolveSessionPinAll(query string) ([]QueryResult, error) {
	if HasGlobMeta(query) {
		names, _ := qr.sessions.ListSessionNames()
		return expandSessionGlobAll(query, names), nil
	}

	if r := qr.exactName(query); r != nil {
		return []QueryResult{r}, nil
	}
	return []QueryResult{&MissResult{Target: query}}, nil
}

// ResolveAliasPinAll is the multi-target ResolveAliasPin: a glob expands over the
// alias-key namespace, and a failure is collected as a *MissResult under the
// matched key so surviving keys still resolve. The returned error is always nil.
func (qr *QueryResolver) ResolveAliasPinAll(value string) ([]QueryResult, error) {
	if HasGlobMeta(value) {
		matches := MatchGlob(value, qr.aliases.Keys())
		if len(matches) == 0 {
			return []QueryResult{&MissResult{Target: value}}, nil
		}
		results := make([]QueryResult, 0, len(matches))
		for _, k := range matches {
			// matches are drawn from Keys(), so Get always finds the key.
			path, _ := qr.aliases.Get(k)
			r, err := qr.validatedPath(path, DomainAlias)
			if err != nil {
				results = append(results, &MissResult{Target: k})
				continue
			}
			results = append(results, r)
		}
		return results, nil
	}

	path, ok := qr.aliases.Get(value)
	if !ok {
		return []QueryResult{&MissResult{Target: value}}, nil
	}
	r, err := qr.validatedPath(path, DomainAlias)
	if err != nil {
		return []QueryResult{&MissResult{Target: value}}, nil
	}
	return []QueryResult{r}, nil
}

// ResolveSessionPin resolves query by exact session name only, a live session's
// or a kept saved session's — no other domain, no minting, no picker fallback.
// It does not expand globs, so a glob hard-fails rather than taking a first
// match.
func (qr *QueryResolver) ResolveSessionPin(query string) (QueryResult, error) {
	if r := qr.exactName(query); r != nil {
		return r, nil
	}

	return nil, fmt.Errorf("No session found: %s", query) //nolint:staticcheck // user-facing message per spec
}

// ResolvePathPin resolves dir in the path domain only, and always mints. It
// stats the literal path, so a directory whose name contains glob
// metacharacters is reachable here, unlike the same value as a bare positional.
func (qr *QueryResolver) ResolvePathPin(dir string) (QueryResult, error) {
	resolved, err := ResolvePath(dir)
	if err != nil {
		return nil, err
	}
	return &PathResult{Path: resolved, Domain: DomainPath}, nil
}

// ResolveAliasPin resolves value in the alias domain only, by exact key, so an
// alias shadowed by a same-named session stays reachable. It does not expand
// globs. An unknown key hard-fails, a gone directory with *DirNotFoundError.
func (qr *QueryResolver) ResolveAliasPin(value string) (QueryResult, error) {
	path, ok := qr.aliases.Get(value)
	if !ok {
		return nil, unknownAliasError(value)
	}
	return qr.validatedPath(path, DomainAlias)
}

// ResolveZoxidePin resolves query in the zoxide domain only. Unlike the bare
// chain, which swallows zoxide failures, it returns ErrZoxideNotInstalled
// verbatim; any other failure hard-fails.
func (qr *QueryResolver) ResolveZoxidePin(query string) (QueryResult, error) {
	path, err := qr.zoxide.Query(query)
	if err != nil {
		if errors.Is(err, ErrZoxideNotInstalled) {
			return nil, ErrZoxideNotInstalled
		}
		return nil, fmt.Errorf("No zoxide match for: %s", query) //nolint:staticcheck // user-facing message per spec
	}
	return qr.validatedPath(path, DomainZoxide)
}

func unknownAliasError(value string) error {
	return fmt.Errorf("No alias found: %s", value) //nolint:staticcheck // user-facing message per spec
}

func (qr *QueryResolver) validatedPath(path string, domain Domain) (QueryResult, error) {
	if !qr.dirValidator.Exists(path) {
		return nil, &DirNotFoundError{Path: path}
	}
	return &PathResult{Path: path, Domain: domain}, nil
}
