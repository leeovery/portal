// Package restoretest provides shared test scaffolding for portal's
// reboot round-trip integration tests.
//
// The package has a mixed build-tag layout:
//
//	Always-built (no build tag):
//	  - SeedSessionsJSON / SeedSessionsJSONWithSavedAt — sessions.json
//	    fixture builders, defined in sessions_json.go.
//	  - WaitForFileExists — sentinel-file polling helper, defined in
//	    waitfor_file_exists.go.
//	  - OpenTestLogger — silent *slog.Logger factory for adapter tests,
//	    defined in logger.go.
//	  These run under default `go test ./...` and have no dependency on
//	  tmux fixtures.
//
//	Integration-only (`//go:build integration`):
//	  - BuildPortalBinaryDir / BuildPortalBinaryStable — thin wrappers
//	    over portalbintest.BuildPortalBinary that adapt the
//	    error-returning helper to *testing.T-driven (Fatal) and
//	    sync.Once-cached (stable os.MkdirTemp) lifetimes respectively.
//	  - PrependPATH — t.Setenv-based PATH manipulation.
//	  - DriveSignalHydrate / DriveSignalHydrateBinary — direct FIFO-byte
//	    writer (DriveSignalHydrate) and its `portal state signal-hydrate`
//	    subprocess equivalent (DriveSignalHydrateBinary) for cmd-side
//	    integration tests that drive signal-hydrate manually.
//	  - WaitForSkeletonMarkersCleared — long-budget marker-clear poller
//	    used by reboot round-trip tests.
//	  - SortedKeySet — deterministic key-set formatter used by the
//	    above helpers' failure diagnostics.
//	  These live in restoretest.go (`//go:build integration`) and exist
//	  only to share scaffolding across the integration-tagged consumer
//	  files in cmd/bootstrap, internal/restore, and cmd.
//
// General-purpose `go build` plumbing (BuildPortalBinary,
// StagePortalBinary, ProjectRoot) lives in the sibling
// internal/portalbintest package — it has no semantic tie to restore
// and is consumed by daemon and saver integration tests too.
//
// Convention: integration-only helpers live in `//go:build integration`-
// tagged files; general-purpose seed primitives omit the tag. Adding a
// new helper means choosing the file/tag that matches its dependency
// surface — if it needs tmux, it goes in a tagged file; if it is a pure
// stdlib + testing helper, it goes in an untagged file.
//
// The package depends only on internal/tmux + internal/state +
// internal/portalbintest + stdlib + testing — no import cycles with
// internal/restore or cmd/bootstrap.
package restoretest
