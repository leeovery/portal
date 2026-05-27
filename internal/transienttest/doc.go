// Package transienttest provides shared test scaffolding for portal's
// `list-panes -a` tmux-transient integration tests.
//
// The package consolidates the Commander-injection primitive, hooks.json
// seeding, and socket-anchored Commander pass-through that were previously
// duplicated between `package bootstrap_test`
// (cmd/bootstrap/transient_listpanes_helpers_integration_test.go) and
// `package cmd`
// (cmd/cleanstale_transient_listpanes_integration_test.go and
// cmd/cleanstale_transient_listpanes_clean_integration_test.go).
//
// Exports:
//
//   - FailureMode + PassThrough / FailExitNonZero / FailEmptyStdout — the
//     per-test policy applied by Commander when it observes a
//     `list-panes -a` invocation.
//   - Commander — wraps an inner tmux.Commander and intercepts only
//     `list-panes -a` calls based on the per-test policy.
//   - SocketCommander — pass-through tmux.Commander targeting an isolated
//     tmux socket via `tmux -S <path> -f /dev/null`.
//   - SeedHooksJSON / HooksJSONBytes / ResolveHooksFilePathFromEnv —
//     hooks.json seeding and byte-level read helpers that consume the env
//     slice returned by portaltest.IsolateStateForTest.
//
// Files in this package live outside `_test.go` so they are importable
// from any other package's test files. The package depends only on
// internal/hooks, internal/tmux, and stdlib + testing — no import cycles
// with cmd or cmd/bootstrap.
//
// Test-only: production code MUST NOT import this package. Enforcement is
// contributor discipline (matches the precedent for tmuxtest /
// restoretest / portalbintest).
package transienttest
