package resolver

// Domain discriminates an open target and a resolver result. The underlying
// strings of the result domains are the decision-log domain attr values verbatim;
// DomainBare and DomainGlob are routing-only and emit no log line.
type Domain string

const (
	DomainBare    Domain = "bare"
	DomainSession Domain = "session"
	DomainPath    Domain = "path"
	DomainAlias   Domain = "alias"
	DomainZoxide  Domain = "zoxide"
	DomainKept    Domain = "kept"
	DomainGlob    Domain = "glob"
	DomainMiss    Domain = "miss"
)

func (d Domain) String() string {
	return string(d)
}
