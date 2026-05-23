# Duplication Analysis — Cycle 1

STATUS: findings
FINDINGS_COUNT: 4

## Finding 1: Fingerprint diff / format / sort helpers duplicated across three integration-test files

SEVERITY: high

FILES:
- `internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go:359-475` (assertSnapshotsEqual, fingerprintFieldDeltas, formatFP, sortedKeys, joinLines)
- `cmd/state_daemon_self_supervision_integration_test.go:888-1026` (assertScrollbackSnapshotsEqual, fingerprintFieldDeltasSelfEject, formatFingerprint, sortedSnapKeys, joinSnapLines)
- `cmd/bootstrap/composition_abc_integration_test.go:300-446` (assertSnapshotsEqualOrFail, fingerprintDeltas, formatFingerprint, joinFingerprintDiags, unionFingerprintKeys, sortStrings)

DESCRIPTION: Three independent test files re-implement the same five-helper diff suite over `portaltest.Fingerprint` maps. Combined ~400 LOC must be kept in lockstep — any change to `Fingerprint`'s shape requires three coordinated edits with no compile-time guard. Same diff logic appears a fourth time as `emitFieldDeltas` in `internal/portaltest/fingerprint.go:203-226`.

RECOMMENDATION: Promote a single consolidated helper into `internal/portaltest`. Suggested API: `DiffFingerprints(pre, post map[string]Fingerprint) []FingerprintDelta` returning structured deltas, plus `FormatFingerprint(fp) string` and `FormatDelta(d FingerprintDelta) string`. Callers wrap with their own `t.Fatalf(...)` template. Package-internal `emitFieldDeltas`/`reportStateDirDelta` should reuse the new helper.

EFFORT: medium

## Finding 2: spawnOrphanDaemonIsolated and spawnOrphanDaemonIsolatedNamed are a strict superset pair

SEVERITY: medium

FILES:
- `cmd/bootstrap/orphan_sweep_integration_test.go:455-467` (spawnOrphanDaemonIsolated)
- `cmd/bootstrap/composition_e2e_harness_integration_test.go:383-395` (spawnOrphanDaemonIsolatedNamed)

DESCRIPTION: Both helpers live in the same `package bootstrap_test` and are line-for-line identical except for return signature: `*exec.Cmd` vs `(*exec.Cmd, string)`.

RECOMMENDATION: Delete `spawnOrphanDaemonIsolated`; have call sites use the Named variant with `_` for the second return. Drop the "Named" suffix. Net: -13 LOC.

EFFORT: small

## Finding 3: applyHostNoiseMitigation inlined in three packages with triplicated rationale

SEVERITY: medium

FILES:
- `cmd/bootstrap/orphan_sweep_integration_test.go:412-427`
- `cmd/state_daemon_self_supervision_integration_test.go` (local twin)
- `internal/tmux/portal_saver_endstate_integration_test.go` (local twin)

DESCRIPTION: The two-line helper `t.Setenv("HOME", t.TempDir()); t.Setenv("XDG_CONFIG_HOME", "")` is inlined in each test package because the canonical implementation in sibling `_test` packages cannot be cross-imported. Function body is trivial (2 lines) but rationale comment is ~12 lines, currently triplicated.

RECOMMENDATION: Promote `ApplyHostNoiseMitigation(t *testing.T)` into `internal/portaltest`. Consolidating the rationale to one location lets each test file replace ~14 lines with a single call.

EFFORT: small

## Finding 4: pgrep / waitForPgrepCount helpers shared via package-level visibility only

SEVERITY: low

FILES:
- `cmd/bootstrap/orphan_sweep_integration_test.go:546-578` (definitions, callers across `cmd/bootstrap/composition_e2e_*.go`)

DESCRIPTION: Correctly shared within `package bootstrap_test` today. No duplication yet. Flagged as a future-vigilance item.

RECOMMENDATION: No change for this work unit. Future test additions outside `cmd/bootstrap` should promote into `internal/portaltest` rather than re-inlining.

EFFORT: small (only if/when needed)
