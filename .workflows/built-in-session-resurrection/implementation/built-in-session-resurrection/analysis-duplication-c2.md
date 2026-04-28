---
agent: duplication
cycle: 2
findings_count: 6
---
# Duplication Analysis (Cycle 2)

## Summary

Six remaining duplication candidates after cycle 1's consolidation pass. The high-impact item is the verbatim `skipIfNoTmux` helper duplicated across two integration test files (natural fit for the existing `internal/tmuxtest` package). Three medium-severity items consolidate within-file or within-package patterns. Two lower-severity items are flagged as borderline rule-of-three.

---

## Findings

### FINDING: skipIfNoTmux duplicated verbatim across two integration test files
- **Severity**: high
- **Files**: `internal/restore/integration_test.go:35-40`, `cmd/bootstrap/phase5_integration_test.go:39-44`
- **Description**: Both files define an identical 5-line helper `func skipIfNoTmux(t *testing.T)` calling `exec.LookPath("tmux")` + `t.Skip` when absent. The phase5 file even comments "Mirrors internal/restore/integration_test.go's helper of the same name" — acknowledged drift-bait. Both files already import `internal/tmuxtest` (the home for cross-package integration-test scaffolding from T7-7). A future change to skip semantics (TMUX_VERSION env override, OS gate) would have to be applied in two places.
- **Recommendation**: Promote `skipIfNoTmux` to `internal/tmuxtest` as `tmuxtest.SkipIfNoTmux(t)`. Delete both local copies; replace each call site with the package call.

### FINDING: AtomicWrite + os.Chmod(0600) doublet duplicated in two state writers
- **Severity**: medium
- **Files**: `internal/state/commit.go:47-50`, `internal/state/scrollback.go:107-110`
- **Description**: Two production writers in the same package perform the identical pair: `fileutil.AtomicWrite(path, data)` immediately followed by `_ = os.Chmod(path, 0o600)`, both with comments justifying chmod as "defensive against umask". The intent — "write atomically and force 0600 regardless of umask" — is one logical operation; encoding it as two statements at every call site means a future writer that forgets the chmod silently regresses file permissions. `internal/state/daemon_state.go`'s WritePIDFile / WriteVersionFile achieve the same 0600 mode through fileutil.AtomicWrite's documented temp-file mode and intentionally skip the explicit chmod, so two inconsistent shapes coexist within one package.
- **Recommendation**: Add a small helper such as `fileutil.AtomicWrite0600(path string, data []byte) error` (or a private `internal/state` helper) wrapping AtomicWrite + chmod. Centralises the umask-defence comment.

### FINDING: Skeleton-marker unset-then-log idiom duplicated in two hydrate paths
- **Severity**: medium
- **Files**: `cmd/state_hydrate.go:178-180`, `cmd/state_hydrate.go:302-305`
- **Description**: The signal-arrived path in runHydrate and the file-missing recovery path in handleHydrateFileMissing each end with the same three-line block: derive `livePaneKey` from `cfg.FIFO`, call `state.UnsetSkeletonMarker(cfg.Client, livePaneKey)`, and on error log `cfg.Logger.Warn(...)`. Both sites compute the live paneKey from the same FIFO; both swallow the error after warning. Two of the three log-format arguments duplicate the prefix/key concatenation.
- **Recommendation**: Extract a private helper `unsetSkeletonMarkerOrLog(cfg hydrateConfig, livePaneKey string)` (or, since both sites already start by computing livePaneKey from cfg.FIFO, accept just `cfg` and derive internally) that performs the unset and WARN-on-error. Replace both blocks with a single call.

### FINDING: Isolated-socket tmux argument prefix repeated three times
- **Severity**: medium
- **Files**: `internal/tmuxtest/socket.go:70-73,113-119,124-130`
- **Description**: `Socket.cmd`, `socketCommander.Run`, and `socketCommander.RunRaw` each rebuild the same `[]string{"-S", socketPath, "-f", "/dev/null"}` prefix before appending the call's args. The string `/dev/null` and the order `-S … -f /dev/null` are repeated three times within one file. Any future change (adding `-2` for forced colour, honouring a TMUX_TMPDIR override) must touch three blocks. Single-file, single-package duplication — pure copy-paste from the original promotion in T7-7.
- **Recommendation**: Add a small private helper inside socket.go such as `func socketArgs(socketPath string, args ...string) []string` returning `append([]string{"-S", socketPath, "-f", "/dev/null"}, args...)`. Delegate all three call sites.

### FINDING: ReadPIDFile and ReadVersionFile share an identical "absent vs. error" decode shape
- **Severity**: low
- **Files**: `internal/state/daemon_state.go:36-50,101-110`
- **Description**: Both readers follow the same template: `os.ReadFile(path)` → on error return `(zero, ErrXxxAbsent)` for fs.ErrNotExist or wrap with a "read <name>" prefix; on success do a single trim/parse step. The body diverges only in the path accessor, the sentinel error, and the parse step. Adding a third daemon-state file (e.g., daemon.startedAt) would invite a third copy.
- **Recommendation**: Optional given rule-of-three threshold. A small generic helper `readDaemonFile(path string, absentSentinel error) ([]byte, error)` collapsing the open + ENOENT classification would let the two readers be three lines apiece. Defer if no third reader is on the horizon.

### FINDING: bootstrap.Orchestrator fatal-message construction repeats "Portal failed to <verb>" prefix
- **Severity**: low
- **Files**: `cmd/bootstrap/bootstrap.go:151,156,161,193`
- **Description**: Four step sites all build their fatal user message the same way: `o.fatal("Portal failed to <verb> ...: "+err.Error(), err)`. The prefix and trailing `": "+err.Error()` are mechanical wrapping; only the verb phrase varies. The spec mandates this format ("Portal failed to ...: <err>"), which makes drift particularly costly — a future step that omits the prefix or drops err.Error() will diverge silently.
- **Recommendation**: Push the prefix boilerplate into the fatal helper, e.g. `o.fatalf(verb, err)` → builds `"Portal failed to " + verb + ": " + err.Error()`. The four call sites collapse to a single string each.
