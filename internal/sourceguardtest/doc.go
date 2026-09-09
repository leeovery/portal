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
//
// Wherever a guard's scan takes a root or a directory, showing that it bites is
// a rule test of its own: stage the violation in a temp tree and drive the same
// scan over it — a repo-wide scan through the Rooted option, a directory-taking
// enumeration by handing it the staged directory — so the biting half is a
// permanent test rather than a ceremony someone repeats by hand. The fallback is
// for the shape that cannot be re-pointed, a guard anchored at its own package
// directory: copy the tree to a scratch location, introduce the violation there,
// run the guard against the copy and discard it — never editing the tree back. A
// `go test -overlay` probe cannot serve in its place: an overlay substitutes the
// go command's build inputs, while ParsePackageSources reads each file from disk
// (parser.ParseFile with a nil src) and the test binary's working directory is
// the real package directory, so the guard reads the original bytes and passes
// over a violation only the compiler ever saw. That proves the guard raises no
// false positive and nothing about whether it bites; and where a guard compares
// parsed literals against sibling values taken from the compiled package, the two
// halves are then reading different sources and the result says nothing in either
// direction.
package sourceguardtest
