package resolver

import (
	"errors"
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

// AliasLookup retrieves the path for a given alias name and enumerates the
// finite alias-key namespace. Keys backs the -a/--alias pin's key-glob
// expansion (spec § Glob targets — alias keys are a finite Portal-owned
// namespace). Both methods are satisfied by *alias.Store.
type AliasLookup interface {
	Get(name string) (string, bool)
	Keys() []string
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

// expandSessionGlobAll expands a session-domain glob pattern against names into
// the K-surface result slice shared by ResolveBareAll and ResolveSessionPinAll:
// zero matches is a single collected *MissResult carrying the pattern, otherwise
// each match (in MatchSessions order) becomes a *SessionResult{Domain:"glob"}.
// names is passed in because the two callers source it differently (ResolveBareAll
// fetches it inside its glob branch; ResolveSessionPinAll reuses an earlier fetch).
func expandSessionGlobAll(pattern string, names []string) []QueryResult {
	matches := MatchSessions(pattern, names)
	if len(matches) == 0 {
		return []QueryResult{&MissResult{Target: pattern}}
	}
	results := make([]QueryResult, 0, len(matches))
	for _, m := range matches {
		results = append(results, &SessionResult{Name: m, Domain: "glob"})
	}
	return results
}

// ResolveBareAll adapts the single-result bare Resolve chain to the K-surface
// multi-target context (Phase 3 burst pre-flight). CRITICAL divergence from
// Resolve: a not-found is a COLLECTED MISS (a *MissResult in the returned slice),
// NOT a hard error — the aggregated pre-flight reports EVERY unresolvable target,
// not just the first (spec § Atomic pre-flight & partial failure). ANY bare-chain
// hard error (a bad path, a gone alias/zoxide dir) therefore collapses to a single
// *MissResult carrying the raw target.
//
// A glob value is session-domain by construction (spec § Glob targets): it expands
// against the user-visible session set to K *SessionResult{Domain:"glob"} — the
// per-match window fan-out this whole task exists to produce — and zero matches is
// a single collected miss. A non-glob value defers to Resolve for the full
// session→path→alias→zoxide→miss chain and wraps the outcome as a single result.
//
// The returned error is ALWAYS nil: the bare chain never surfaces
// ErrZoxideNotInstalled (Resolve swallows it internally), so there is no
// environment-fault to propagate — every failure is a collected miss.
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

// ResolveSessionPinAll adapts ResolveSessionPin to the K-surface multi-target
// context. CRITICAL divergence from ResolveSessionPin: a not-found (exact miss,
// zero-match glob, or empty session set) is a COLLECTED MISS (a *MissResult), NOT
// the "No session found" hard error — multi-target pre-flight collects every miss
// (spec § Atomic pre-flight & partial failure). A glob expands to K
// *SessionResult{Domain:"glob"} over the user-visible set; an exact hit is a single
// *SessionResult{Domain:"session"}. The returned error is always nil.
func (qr *QueryResolver) ResolveSessionPinAll(query string) ([]QueryResult, error) {
	names, _ := qr.sessions.ListSessionNames()

	if HasGlobMeta(query) {
		return expandSessionGlobAll(query, names), nil
	}

	if slices.Contains(names, query) {
		return []QueryResult{&SessionResult{Name: query, Domain: "session"}}, nil
	}
	return []QueryResult{&MissResult{Target: query}}, nil
}

// ResolveAliasPinAll adapts ResolveAliasPin to the K-surface multi-target context.
// CRITICAL divergence from ResolveAliasPin: a not-found (unknown key, zero-match
// glob) or a gone directory is a COLLECTED MISS (a *MissResult), NOT the "No alias
// found" / *DirNotFoundError hard error — multi-target pre-flight collects every
// miss (spec § Atomic pre-flight & partial failure).
//
// A glob value expands against the enumerated alias-key namespace (Keys +
// MatchSessions); each matched key's directory is validated on disk and reduced to
// a *PathResult{Domain:"alias"}, but a gone directory for one matched key becomes a
// *MissResult carrying THAT KEY (the surviving keys still resolve) — the parent
// reduces every mint to a literal existing dir at resolve time (spec § Burst
// exec-argv & mint responsibility). An exact key resolves the same way, collecting
// the miss under the raw value on an unknown key or a gone dir. The returned error
// is always nil.
func (qr *QueryResolver) ResolveAliasPinAll(value string) ([]QueryResult, error) {
	if HasGlobMeta(value) {
		matches := MatchSessions(value, qr.aliases.Keys())
		if len(matches) == 0 {
			return []QueryResult{&MissResult{Target: value}}, nil
		}
		results := make([]QueryResult, 0, len(matches))
		for _, k := range matches {
			// matches are drawn from Keys(), so Get always finds the key.
			path, _ := qr.aliases.Get(k)
			r, err := qr.validatedPath(path, "alias")
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
	r, err := qr.validatedPath(path, "alias")
	if err != nil {
		return []QueryResult{&MissResult{Target: value}}, nil
	}
	return []QueryResult{r}, nil
}

// ResolveSessionPin resolves query in the session domain ONLY — the -s/--session
// pin (spec § Domain-pinning flags). It matches the value against the
// user-visible session set (an exact name, or a filepath.Match glob) and never
// consults aliases / zoxide / the filesystem: a pin names its domain explicitly.
// A hit yields a SessionResult — Domain "session" for an exact name, "glob" for a
// glob expansion (the first match at single-target arity; per-match window
// fan-out is the Phase 3 burst). The pin never mints and never falls back to the
// picker (spec § Pinned-domain contract).
func (qr *QueryResolver) ResolveSessionPin(query string) (QueryResult, error) {
	// Fetch the user-visible session set once. A nil/empty slice or a lister error
	// collapses to "no sessions" — the same tolerance the bare-target session
	// pre-check (Resolve) applies.
	names, _ := qr.sessions.ListSessionNames()

	if HasGlobMeta(query) {
		if matches := MatchSessions(query, names); len(matches) > 0 {
			return &SessionResult{Name: matches[0], Domain: "glob"}, nil
		}
	} else if slices.Contains(names, query) {
		return &SessionResult{Name: query, Domain: "session"}, nil
	}

	// Miss: no exact match, zero glob matches, or an empty session set. Hard-fail
	// with the VERBATIM string the retired attach command used, so `open --session`
	// is byte-identical to the former `attach` on the miss path (planner decision).
	// A plain error (not a UsageError) → runtime failure → exit 1. The capitalised
	// leading word trips staticcheck ST1005, silenced per the directive attach.go
	// carried.
	return nil, fmt.Errorf("No session found: %s", query) //nolint:staticcheck // user-facing message per spec
}

// ResolvePathPin resolves dir in the path domain ONLY — the -p/--path pin (spec §
// Domain-pinning flags). It reuses ResolvePath (tilde/relative expansion +
// existence + is-directory validation) and never consults the glob pre-check, the
// session set, aliases, or zoxide: a pin names its domain explicitly. A hit always
// mints (Axiom 2: a directory-domain hit → PathResult with Domain "path"). Because
// ResolvePath stats the LITERAL path, a directory whose name contains glob
// metacharacters (e.g. ~/tmp/foo[1]) is reachable here — the metacharacters are
// never expanded — whereas the same value as a bare positional hard-fails via the
// glob pre-check (spec § Glob targets). A non-existent directory or a
// non-directory file hard-fails (exit 1) and the pin never falls back to the
// picker (spec § Pinned-domain contract). It does NOT go through validatedPath:
// ResolvePath already validates existence and rejects a non-directory file.
func (qr *QueryResolver) ResolvePathPin(dir string) (QueryResult, error) {
	resolved, err := ResolvePath(dir)
	if err != nil {
		return nil, err
	}
	return &PathResult{Path: resolved, Domain: "path"}, nil
}

// ResolveAliasPin resolves value in the alias domain ONLY — the -a/--alias pin
// (spec § Domain-pinning flags). It looks the key up directly in the alias store,
// bypassing the session→path→alias→zoxide precedence, so it is the ONLY way to
// reach an alias key SHADOWED by a same-named session. When value is a glob it is
// expanded against the enumerated alias-key namespace (Keys + filepath.Match via
// MatchSessions — a finite Portal-owned namespace, spec § Glob targets); at
// single-target arity the first match mints (per-match window fan-out is the
// Phase 3 burst). The resolved key's directory is validated on disk via
// validatedPath: a hit always mints (Axiom 2 — PathResult with Domain "alias"), a
// gone directory hard-fails with *DirNotFoundError (distinct from the unknown-key
// miss), and an unknown key — or a glob matching zero keys — hard-fails with a
// plain "No alias found" error. It never consults qr.sessions or qr.zoxide, and
// the pin never falls back to the picker (spec § Pinned-domain contract).
func (qr *QueryResolver) ResolveAliasPin(value string) (QueryResult, error) {
	if HasGlobMeta(value) {
		matches := MatchSessions(value, qr.aliases.Keys())
		if len(matches) == 0 {
			return nil, unknownAliasError(value)
		}
		// matches are drawn from Keys(), so the first is always a real key.
		path, _ := qr.aliases.Get(matches[0])
		return qr.validatedPath(path, "alias")
	}

	path, ok := qr.aliases.Get(value)
	if !ok {
		return nil, unknownAliasError(value)
	}
	return qr.validatedPath(path, "alias")
}

// ResolveZoxidePin resolves query in the zoxide domain ONLY — the -z/--zoxide pin
// (spec § Domain-pinning flags). It queries zoxide and, UNLIKE the bare-target
// chain (which swallows any zoxide error and silently falls through to the miss
// tail), makes the outcome explicit: zoxide-not-installed surfaces
// ErrZoxideNotInstalled verbatim (returned directly so a caller can errors.Is it
// — a script sees WHY, distinct from the silent fall-through), and any other query
// failure (a no-match) hard-fails with a plain "No zoxide match" error (exit 1). On
// a hit the best-match directory is validated on disk via validatedPath: a hit
// always mints (Axiom 2 — PathResult with Domain "zoxide"), and a gone best-match
// dir hard-fails with *DirNotFoundError (distinct from the no-match). It never
// consults qr.sessions or qr.aliases — zoxide-domain only — and never falls back to
// the picker (spec § Pinned-domain contract).
func (qr *QueryResolver) ResolveZoxidePin(query string) (QueryResult, error) {
	path, err := qr.zoxide.Query(query)
	if err != nil {
		if errors.Is(err, ErrZoxideNotInstalled) {
			return nil, ErrZoxideNotInstalled
		}
		// Any other query failure is a no-match: a distinct hard-fail with the
		// house-style capitalised message. The capitalised leading word matches the
		// sibling pins (cf. "No session found" / "No alias found") and trips
		// staticcheck ST1005, silenced per the directive.
		return nil, fmt.Errorf("No zoxide match for: %s", query) //nolint:staticcheck // user-facing message per spec
	}
	return qr.validatedPath(path, "zoxide")
}

// unknownAliasError is the alias-pin hard-fail for an unknown key or a glob
// matching zero keys (spec § Domain-pinning flags: -a hard-fails on an unknown
// key). Single-sourced so both the exact-miss and zero-match-glob paths produce a
// byte-identical message. A plain error (not a UsageError) → runtime failure →
// exit 1. The capitalised leading word matches the house style (cf. "No session
// found") and trips staticcheck ST1005, silenced per the directive.
func unknownAliasError(value string) error {
	return fmt.Errorf("No alias found: %s", value) //nolint:staticcheck // user-facing message per spec
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
