# Duplication Analysis — Cycle 2 (independent re-scan)

STATUS: findings
FINDINGS_COUNT: 5

## Finding 1: Fingerprint diff/format helper suite re-duplicated in composition_e2e_self_eject_integration_test.go

SEVERITY: high

FILES:
- `cmd/bootstrap/composition_e2e_self_eject_integration_test.go:442-577` (`assertScrollbackSnapshotsEqualSelfEjectComposite`, `fingerprintFieldDeltasSelfEjectComposite`, `formatFingerprintSelfEjectComposite`, `sortedSnapshotKeysSelfEjectComposite`, `joinDiagLinesSelfEjectComposite`)
- `internal/portaltest/fingerprint.go:190-290` (canonical `DiffFingerprints` / `FormatFingerprint` / `FormatDelta`)
- `cmd/bootstrap/composition_abc_integration_test.go:272-284` (clean reference call site already migrated)

DESCRIPTION: T7-1 consolidated the per-(path, field) diff helpers into `internal/portaltest` and migrated three call sites. T6-6 (composition_e2e_self_eject) was authored concurrently and independently reintroduced the full five-helper suite (~135 LOC) with renamed identifiers. Every field branch is replicated; format strings are byte-identical to `portaltest.FormatDelta`.

RECOMMENDATION: Replace `assertScrollbackSnapshotsEqualSelfEjectComposite` body with the `portaltest.DiffFingerprints` + `portaltest.FormatDelta` loop. Delete the four sibling local helpers. Net: ~-135 LOC.

EFFORT: small

## Finding 2: pgrepPortalDaemons re-implemented in test file alongside the production adapter

SEVERITY: medium

FILES:
- `internal/bootstrapadapter/orphan_sweep.go:60-110` (`pgrepDaemonPattern` const + `pgrepPortalDaemons()`)
- `cmd/bootstrap/orphan_sweep_integration_test.go:92-96, 515-549` (`pgrepCandidatePattern` const + `pgrepPortalDaemonCount` + `pgrepPortalDaemonPIDs`)
- `internal/state/daemon_identity.go:38` (third declaration of the same regex as `daemonArgvPattern`)

DESCRIPTION: Both `orphan_sweep.go` and the test file declare the canonical regex `"^portal state daemon( |$)"` as a package-level constant and both implement the same `exec.Command("pgrep", "-fx", <pattern>)` enumeration with identical exit-1+empty-stdout handling. The regex declaration spans three sites total pinned to the same spec sentence with nothing enforcing the coupling.

RECOMMENDATION: Promote a single exported pattern constant (most naturally `state.PortalDaemonArgvPattern`, so `IdentifyDaemon`'s `regexp.MustCompile` reuses it). Have the test's helpers call `internal/bootstrapadapter.pgrepPortalDaemons()` directly — or, if test cannot import production, promote the enumeration into `internal/portaltest`.

EFFORT: small

## Finding 3: tmux.SaverPanePID and Client.FirstPanePIDInSession do the same list-panes-and-parse

SEVERITY: medium

FILES:
- `internal/tmux/saver_pane_pid.go:44-64` (`SaverPanePID(c, sessionName)` — added T5-2)
- `internal/tmux/tmux.go:575-592` (`Client.FirstPanePIDInSession(session)` — added T4-4)

DESCRIPTION: Both helpers run `list-panes -t =<session> -F '#{pane_pid}'`, split stdout, take the first non-empty line, `strconv.Atoi`. Differences:
- `FirstPanePIDInSession` adds `-s` (session-wide); `SaverPanePID` omits it (active window only — load-bearing for Component D mismatch detection).
- `FirstPanePIDInSession` returns `(0, nil)` on empty pane list and raw wrapped error otherwise. `SaverPanePID` returns the richer sentinels (`ErrNoSuchSession`, `ErrEmptyPaneList`, `ErrPanePIDParse`).

Both authored in this work unit by different task executors. Both consumers treat any error as "absent" — neither actually needs the divergent error contracts.

RECOMMENDATION: Drop `FirstPanePIDInSession`; have `bootstrapadapter/orphan_sweep.go` call `tmux.SaverPanePID(client, tmux.PortalSaverName)` directly, treating `ErrNoSuchSession`/`ErrEmptyPaneList` as "legitimate set empty" via `errors.Is`. The `-s` distinction must be preserved (active-window-only is load-bearing for the daemon probe).

EFFORT: medium

## Finding 4: captureLogger and recordingLogger are redundant Logger fakes in the same package

SEVERITY: low

FILES:
- `cmd/bootstrap/bootstrap_test.go:98-127` (`recordingLogger`)
- `cmd/bootstrap/orphan_sweep_integration_test.go:359-404` (`captureLogger`)

DESCRIPTION: Both fakes live in `package bootstrap_test` and satisfy `bootstrap.Logger`. The orphan_sweep file's docstring claims the duplication exists because "this _test package does not need to reach across the bootstrap / bootstrap_test package boundary" — but both files are in `bootstrap_test`.

RECOMMENDATION: Delete `captureLogger`; extend `recordingLogger` with `allEntries()` drawing from all four level slices. Migrate the `captureLogger` call sites. Net: ~-45 LOC.

EFFORT: small

## Finding 5: sortedSnapKeys / sortedKeys / sortedSnapshotKeys triplicate the sorted-map-keys pattern

SEVERITY: low

FILES:
- `cmd/state_daemon_self_supervision_integration_test.go:878-885`
- `cmd/bootstrap/composition_e2e_scrollback_stability_integration_test.go:224-231`
- `internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go:362`

DESCRIPTION: Three trivial three-line "collect keys, sort.Strings, return" helpers over differently-typed value maps. `slices.Sorted(maps.Keys(m))` (Go 1.21+) removes the need for any of them.

RECOMMENDATION: Replace each call site inline with `slices.Sorted(maps.Keys(m))`. Resolving Finding 1 automatically collapses one of the four duplicates.

EFFORT: small
