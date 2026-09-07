// Package sourceguardtest provides the Go-source scanning primitives shared by
// the guards that police a structural rule by reading the repository's own .go
// files rather than by executing them. Beyond the shared *testing.T stand-in its
// helpers report through and portalbintest, whose module-root resolution the
// repo-wide scan is anchored at, it depends on stdlib alone, and it carries no
// build tag, so every guard it serves runs in the unit lane.
//
// Test-only: production code must not import it. The trailing "test" in the name
// carries that boundary at the import line, as it does for every sibling helper
// package.
//
// These primitives stay here rather than folding back into portalbintest, whose
// subject is building and staging the portal binary: source scanning is a
// separate concern that guards across several packages share.
package sourceguardtest
