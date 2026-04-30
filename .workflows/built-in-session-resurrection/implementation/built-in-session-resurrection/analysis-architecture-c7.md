---
agent: architecture
cycle: 7
findings_count: 2
status: issues_found
---
# Architecture Analysis (Cycle 7)

## Summary

Phase 13 architecture is sound. The new restoretest package fits cleanly alongside tmuxtest with a justified three-consumer base, build-tag gating is correctly scoped, env propagation in `DriveSignalHydrateBinary` correctly mirrors production, and the T13-4 revert correctly preserves the load-bearing spec invariant separating raw hookKey from FS-safe paneKey. Two low-severity findings — misleading godoc on open.go seam init() rationale, and a preemptively-exported FIFO primitive in restoretest.

---

## API Surface

The 8 exported helpers in `internal/restoretest/` form a coherent test-scaffolding API. Signatures are function-shape (no leaked types defined here); the only internal-package type in signatures is `*tmux.Client` (DI shared with consumers) and stdlib types. Acceptable here because every consumer of restoretest already constructs one via `tmuxtest.New(t).Client()` — there's no caller outside the work-unit (restoretest is `//go:build integration` and consumed only by three integration test files in the same repo).

`DriveSignalHydrateBinary` has a 6-positional-string parameter list. Borderline by code-quality.md's 4+ smell, but for a test helper with consistent positional ordering and 3 call sites this is readable. Not worth restructuring at current call-site count.

## Package Boundaries

`internal/restoretest/` is justified. Three independent test packages consume it. Placement alongside `internal/tmuxtest/` is correct — tmuxtest owns socket isolation, restoretest owns portal-binary build + restore-cycle drivers. No boundary overlap.

The `//go:build integration` gate is well-placed: every consumer file is also `//go:build integration`, so the package contributes zero compile cost to default `go test ./...` runs.

## Test Seam Pattern

Cmd-package function-typed test seams now total four: `openTUIFunc`, `openPathFunc` (Phase 13, init()-assigned) and `signalHydrateRunFunc`, `hydrateRunFunc` (older, direct `var x = fn`). At N=4 each occurrence has distinct semantics and tests genuinely need argv-shape capture without launching real subprocesses or replacing the test process via `syscall.Exec`. A registry mechanism would not improve clarity at this density — still organic.

## Production-Code Invariants

The T13-4 revert (rejecting cycle-6's S5 finding to unify hookKey via SanitizePaneKey) is correct, anchored in spec L352: hook structural keys use the **raw** session name; paneKey uses the FS-safe form. Two distinct address spaces with distinct correctness contracts. Collapsing them — even via a transform that happens to coincide for ASCII-clean session names — would silently corrupt hook lookup for any session containing `/`, `\`, null, or a leading `.`. **This is a load-bearing architectural invariant.**

Production code enforces the separation:
- `internal/restore/session.go:108` — hookKey via `tmux.PaneTarget(sess.Name, w.Index, p.Index)` (raw)
- `internal/state/panekey.go:27` — paneKey via `SanitizePaneKey(session, window, pane)` (FS-safe with collision suffix)

## Documentation Coherence

Spec L1395 enumerates "@portal-restoring clear (unset) fails at step 6" as fatal: *"the marker MUST NOT leak past bootstrap."* `cmd/bootstrap/bootstrap.go:225-229` correctly returns `o.fatalf("clear @portal-restoring marker", err)`. CLAUDE.md still describes step 6 as "fatal on failure" (unchanged in this phase, correctly). Spec, code, and CLAUDE.md aligned.

`DriveSignalHydrateBinary` env propagation is correct given portal's config resolution chain. `internal/state/paths.go:35` consults `PORTAL_STATE_DIR` first; `XDG_CONFIG_HOME` / `HOME` only when unset. With both env vars set, `XDG_CONFIG_HOME` is irrelevant and rightly omitted.

---

## Findings

### FINDING A1: openTUIFunc / openPathFunc init() rationale is misleading

- **SEVERITY:** low
- **FILES:** `cmd/open.go:21`, `cmd/open.go:28`, `cmd/open.go:428-432`
- **DESCRIPTION:** Both seams' godoc claims "Initialized in init() to break the openTUIFunc → openTUI → openCmd → openTUIFunc cycle." There is no such cycle — `openTUI` and `openPath` do not reference `openCmd`. The same package's `signalHydrateRunFunc = runSignalHydrate` (`cmd/state_signal_hydrate.go:127`) successfully uses direct package-level initialization in structurally identical circumstances. The init()-based assignment is defensive, not load-bearing; the comment will mislead future maintainers applying this pattern to a third seam.
- **RECOMMENDATION:** Either drop the init() assignment and convert to `var openTUIFunc = openTUI` / `var openPathFunc = openPath` (consistent with the existing in-package seams in state_signal_hydrate.go and state_hydrate.go), or correct the godoc to describe the actual reason. Low severity; harmless functionally.

### FINDING A2: OpenAndSignalFIFO is exported with no external callers

- **SEVERITY:** low
- **FILES:** `internal/restoretest/restoretest.go:262`
- **DESCRIPTION:** `OpenAndSignalFIFO` is exported and documented as needed by "integration round-trip tests across multiple layers", but the only caller is the sibling `DriveSignalHydrate` inside the same file. Both `internal/restore/integration_full_test.go` and the cmd-package integration tests reach FIFO writes via `DriveSignalHydrate` / `DriveSignalHydrateBinary`, not the lower primitive. Exporting preemptively widens restoretest's API surface and could let a future caller couple to the (delay, budget) tuple shape rather than the higher-level Drive helper.
- **RECOMMENDATION:** Unexport to `openAndSignalFIFO`. Re-export when a second caller materializes. Low severity; build-tag-gated package so the leak is contained.
