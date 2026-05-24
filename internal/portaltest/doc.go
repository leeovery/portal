// Package portaltest provides test-only helpers for spawning the
// portal CLI as a subprocess under per-test state-directory
// isolation.
//
// The flagship helper, IsolateStateForTest, builds an env slice
// rooted in a per-test t.TempDir() and scrubbed of any inherited
// XDG_CONFIG_HOME. Callers assign the returned slice to
// exec.Cmd.Env when launching `portal state daemon` (or any other
// portal subcommand whose paths depend on XDG_CONFIG_HOME) so the
// spawned subprocess writes only to the isolated state dir — never
// to the developer's real ~/.config/portal/state/.
//
// This package is sibling to (not part of) internal/portalbintest:
// env isolation is orthogonal to binary building. A daemon-spawning
// integration test typically composes both — portalbintest.StagePortalBinary
// to put `portal` on PATH, and portaltest.IsolateStateForTest to
// scope where it writes.
//
// Test-only. Importing this package from non-*_test.go files is
// prohibited — the *testing.T parameter on the exported helpers
// enforces this structurally (the testing import would fail
// production builds).
package portaltest
