AGENT: duplication
FINDINGS:

- FINDING: `setupTransientCleanStaleEnv` and `setupCleanTransientEnv` are near-duplicate test-harness env builders
  SEVERITY: low
  FILES: cmd/cleanstale_transient_listpanes_integration_test.go:97-124, cmd/cleanstale_transient_listpanes_clean_integration_test.go:106-113
  DESCRIPTION: Both sibling files (same `package cmd`, both `//go:build integration`) declare a per-subtest env-builder helper that does the same four things: `portaltest.IsolateStateForTest(t)`, `t.Setenv("PORTAL_STATE_DIR", stateDir)`, `t.Setenv("PORTAL_LOG_LEVEL", "debug")`, re-push `XDG_CONFIG_HOME`. The bootstrap-tail variant additionally `tmuxtest.SkipIfNoTmux(t)` + stages portal binary + returns a `tmuxtest.Socket`. The four invariant Setenv/Isolate steps are byte-identical; the XDG re-push rationale block is duplicated in both docstrings.
  RECOMMENDATION: Extract a single `isolateCleanStaleTestEnv(t)` helper into a new `cmd/cleanstale_transient_listpanes_shared_test.go`. Both callsite helpers become thin wrappers — one place to amend if isolation contracts evolve.

- FINDING: mode_a / mode_b integration-test bodies duplicated row-for-row across bootstrap and clean files
  SEVERITY: low
  FILES: cmd/cleanstale_transient_listpanes_integration_test.go:210-298, cmd/cleanstale_transient_listpanes_clean_integration_test.go:164-269
  DESCRIPTION: Mode_a and mode_b subtest bodies run the same six-step shape at both callsites (setup → seed → snapshot → install commander → invoke entry point → assert byte-identity + log fingerprint). Seed maps and needle strings are duplicated verbatim. Needles substring-match the format strings that now live exactly once in `runHookStaleCleanup` — drift between subtests partially undoes cycle 1's single-source-of-truth win.
  RECOMMENDATION: Extract `runTransientCleanStaleModeSubtest(t, spec transientModeSpec)` table-driven helper. `transientModeSpec` carries the failure mode + entry-point invoker + post-run extra-assert closure. The four subtests collapse to four three-line driver calls listing only the deltas.

- FINDING: `liveFormat` literal duplicated between `cmd/bootstrap/stale_marker_cleanup.go` and `internal/tmux/tmux.go:705`
  SEVERITY: low
  FILES: cmd/bootstrap/stale_marker_cleanup.go:39, internal/tmux/tmux.go:705
  DESCRIPTION: After Change 1, `ListAllPanes` wraps `ListAllPanesWithFormat` with the literal `"#{session_name}:#{window_index}.#{pane_index}"`. The same literal is declared as `liveFormat` constant in `stale_marker_cleanup.go:39`. Spec § Change 1 ("Format-string alignment") explicitly hinges on these matching exactly. Drift would silently desync the two cleanup paths' interpretation of "what is a paneKey" — exactly the class of silent data corruption this work unit closes.
  RECOMMENDATION: Promote the literal to a single exported constant `tmux.StructuralKeyFormat`. Update the new `ListAllPanes` call site and `stale_marker_cleanup.go`'s `liveFormat` to reference it. Format-string-pinning tests assert against the constant directly.

SUMMARY: Three residual extraction candidates, all low-severity and mechanically collapsible. The format-literal extraction is the highest-leverage: it pins a load-bearing structural invariant at the type system instead of by convention.
