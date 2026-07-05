package state

// SandboxRegistryEnv names the env var that carries the daemon-pgrep
// sandbox's cross-process ownership registry: the absolute path of a
// file listing test-owned state directories, one per line.
//
// Purpose — extend the test-isolation sandbox (pgrep_sandbox.go,
// //go:build integration) across the process boundary. The in-process
// registration API (EnableDaemonSandbox / RegisterSandboxStateDir /
// RegisterSandboxDaemon) protects the test binary's own
// PgrepPortalDaemons calls, but a test that execs the built `portal`
// binary (e.g. `portal list` driving a full bootstrap) runs the
// orphan-daemon sweep in a *separate process* where none of those
// in-process registrations exist. When that subprocess binary is built
// with `-tags integration` (portalbintest always does this) and this
// env var is present, its PgrepPortalDaemons enumerations are filtered
// to daemons whose <stateDir>/daemon.pid lives in a registered dir —
// making the developer's live daemon structurally invisible to
// subprocess sweeps, exactly as it is to in-process ones.
//
// The registry is a FILE (not an inline dir list) so ownership stays
// dynamic: portaltest.SpawnIsolatedDaemon appends each orphan's state
// dir after subprocess env slices have already been constructed, and
// every enumeration re-reads the file.
//
// This const is declared in an untagged file so internal/portaltest
// (compiled in both build modes) can reference it; in a production
// build it is a dead string — the read side exists only under
// //go:build integration, and no production code sets the env var.
const SandboxRegistryEnv = "PORTAL_TEST_SANDBOX_REGISTRY"
