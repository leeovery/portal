package resolver

// Domain is the typed resolution domain that discriminates every open target and
// resolver result across the cmd↔resolver routing boundary. The concrete set is
// fully known at design time, so it is a closed string-backed constant set rather
// than a bare string: the routing switches (globExpandableDomain, resolveOpenSurfaces)
// become exhaustive-checkable and a domain added to one vocabulary but not another
// is a compile-time mismatch, not a silent misroute.
//
// The underlying string of DomainSession/DomainPath/DomainAlias/DomainZoxide (and
// DomainMiss) is the EXACT spec-governed `resolve` decision-log `domain` attr value
// (closed taxonomy: session/path/alias/zoxide, or miss), so String() feeds that log
// line byte-identically. DomainBare and DomainGlob are routing-only — deterministic
// targets emit no resolve line — but belong to the same closed set.
type Domain string

// The full domain vocabulary. bare is a Target-only positional running the full
// precedence chain; session/path/alias/zoxide/glob/miss are resolver-result domains.
const (
	// DomainBare is a positional target running the full precedence chain.
	DomainBare Domain = "bare"
	// DomainSession is an existing-session (attach) hit.
	DomainSession Domain = "session"
	// DomainPath is a directory-path (mint) hit.
	DomainPath Domain = "path"
	// DomainAlias is an alias-key (mint) hit.
	DomainAlias Domain = "alias"
	// DomainZoxide is a zoxide-query (mint) hit.
	DomainZoxide Domain = "zoxide"
	// DomainGlob is a session-glob expansion match (attach).
	DomainGlob Domain = "glob"
	// DomainMiss is a total miss across every domain.
	DomainMiss Domain = "miss"
)

// String returns the domain's spec-governed string form — for the session/path/
// alias/zoxide/miss domains this is the exact `resolve` log-attr value, so the
// decision log line stays byte-identical.
func (d Domain) String() string {
	return string(d)
}
