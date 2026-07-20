package resolver_test

import (
	"testing"

	"github.com/leeovery/portal/internal/resolver"
)

// TestDomain_String pins the typed Domain constant set to the exact spec-governed
// strings. The session/path/alias/zoxide/miss values are the closed taxonomy the
// `resolve` decision log line's `domain` attr emits, so this test is the
// byte-identical contract at the source of truth (String() must not drift).
func TestDomain_String(t *testing.T) {
	cases := []struct {
		domain   resolver.Domain
		expected string
	}{
		{resolver.DomainBare, "bare"},
		{resolver.DomainSession, "session"},
		{resolver.DomainPath, "path"},
		{resolver.DomainAlias, "alias"},
		{resolver.DomainZoxide, "zoxide"},
		{resolver.DomainGlob, "glob"},
		{resolver.DomainMiss, "miss"},
	}
	for _, c := range cases {
		if got := c.domain.String(); got != c.expected {
			t.Errorf("Domain(%q).String() = %q, want %q", string(c.domain), got, c.expected)
		}
	}
}
