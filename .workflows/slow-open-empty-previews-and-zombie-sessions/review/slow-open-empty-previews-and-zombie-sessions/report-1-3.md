TASK: 1-3 — Add fingerprint-diff t.Cleanup backstop to isolation helper

ACCEPTANCE CRITERIA:
- Clean: no Errorf when state dir untouched
- "created" on new file
- "size-changed" / "content-changed" detection
- "mtime-changed" / "ctime-changed" detection
- "created" / "became-symlink" / "symlink-target-changed" for symlinks
- Files >1 MiB: size detected, no content hash
- Non-existent state dir → empty snapshot → creations flagged
- Siblings out of scope
- All deltas reported (not just first)

STATUS: Complete (with one documented design deviation; non-blocking)

SPEC CONTEXT: Spec § Component G item 1 sub-bullet 5 — `t.Cleanup` snapshots `~/.config/portal/state/` pre-test (lstat), re-snapshots post-test, `t.Errorf`s any delta. Fingerprint: exists, size, mtimeNanos, ctimeNanos, SHA-256 ≤ 1 MiB. Missing dir → empty snapshot. Symlinks via lstat. Siblings out of scope.

IMPLEMENTATION:
- Status: Implemented (with one intentional documented deviation)
- Location:
  - `internal/portaltest/isolated_env.go:56-109` — `IsolateStateForTest` wires backstop
  - `internal/portaltest/isolated_env.go:117-130` — `backstopT` seam + `installBackstopCleanup`
  - `internal/portaltest/fingerprint.go:42-51` — `Fingerprint` struct (exact spec shape)
  - `internal/portaltest/fingerprint.go:66-104` — `SnapshotStateDir` (lstat, non-existent → empty)
  - `internal/portaltest/fingerprint.go:190-244` — `DiffFingerprints` with canonical Field constants and type-swap short-circuit
  - `internal/portaltest/fingerprint.go:299-331` — `reportStateDirDelta` + `backstopFieldLabels`
  - `internal/portaltest/fingerprint.go:362-370` — `resolveDevStateDir`
  - `internal/portaltest/fingerprint_darwin.go`, `fingerprint_linux.go` — build-tagged `statTimeNanos`
- Notes:
  1. INTENTIONAL DEVIATION: spec says capture state-dir BEFORE env modification. Implementation scrubs HOME and XDG_CONFIG_HOME FIRST, then snapshots HOME-rooted path. Documented in `IsolateStateForTest` docstring (lines 26-35 "Host-noise mitigation"). Trade-off: backstop no longer protects against tests hardcoding `/Users/leeovery/.config/...` paths — only env-flow tests covered. Defensible given production paths all flow through xdg env resolution
  2. Renamed `IsolateStateForTest` (consistent across CLAUDE.md and 40+ callsites)

TESTS:
- Status: Adequate
- Coverage:
  - SnapshotStateDir: nonexistent root, regular file, symlink-via-lstat, >1 MiB skips hash, content-mutation (fingerprint_test.go:74-225)
  - reportStateDirDelta: clean, created, deleted, size-changed, content-changed (mtime-pinned), mtime-bumped, became-symlink, symlink-target-changed, large-file-no-hash, all-deltas-reported, siblings-out-of-scope, nonexistent-with-post-creation (lines 229-497)
  - DiffFingerprints: empty, identical, additions/removals-only, every field channel, type-swap short-circuit, stable sort (fingerprint_diff_test.go:16-295)
  - Wiring: `TestBackstopCleanupFiresOnExternalMutation` + `TestBackstopCleanupSilentOnClean` via `fakeBackstopT`
  - Host-noise mitigation: `TestNeutralizesHomeAndXDGConfigHome`
- White-box `package portaltest` placement for tests of unexported `reportStateDirDelta` is right call
- ctime-changed not end-to-end OS-tested (cannot portably force via Chtimes); diff channel itself covered

CODE QUALITY:
- Project conventions: Followed; test-only via doc.go; build-tagged platform files
- SOLID: Good; `backstopT` interface narrows `*testing.T` surface (clean ISP)
- Complexity: Low; most complex `fieldDeltas` is ~15 lines
- Modern idioms: `errors.Is`, `filepath.WalkDir`, `sort.SliceStable`
- Readability: Excellent; every exported symbol has docstring; rationale captured inline

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Intentional deviation from spec's "snapshot BEFORE env mutation" — materially changes backstop's threat model (env-flow tests only). An updated spec note would close the loop
- [idea] Implementation adds `hashed` field-flip delta (`fingerprint.go:174` + `237-239`) translated to `hashed-changed` — useful signal not anticipated in plan
- [quickfix] `fingerprint.go` says "test-only" but exported `SnapshotStateDir`/`DiffFingerprints`/`FormatDelta`/`Fingerprint` are consumed from out-of-package integration tests and don't take `*testing.T`; tighten docstring
