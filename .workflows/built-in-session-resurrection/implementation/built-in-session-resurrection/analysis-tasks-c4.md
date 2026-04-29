---
topic: built-in-session-resurrection
cycle: 4
total_proposed: 3
---
# Analysis Tasks: built-in-session-resurrection (Cycle 4)

## Task 1: Propagate nine-step bootstrap terminology across spec, CLAUDE.md, and doc-comments
status: pending
severity: high
sources: standards, architecture

**Problem**: Cycle 3 promoted FIFO sweep to a discrete bootstrap step, taking the orchestrator from eight to nine steps. The authoritative package docstring (`cmd/bootstrap/bootstrap.go:1,125`) and `bootstrap_test.go` were updated, but the spec, CLAUDE.md, `cmd/root.go`, and `cmd/bootstrap/phase5_integration_test.go` were not. The contractual specification still describes an eight-step sequence with CleanStale at step 7 and contains a line (772) that says the per-pane defensive pattern "eliminates the need for a separate stale-FIFO sweep step" — directly contradicting the implemented FIFOSweeper step. CLAUDE.md tells new contributors the bootstrap is eight-step. Two doc-comments contradict adjacent files in the same import graph.

**Solution**: Update every site so the documented step-count and step-numbering match the implemented nine-step orchestrator (1.EnsureServer, 2.RegisterPortalHooks, 3.Set@portal-restoring, 4.EnsureSaver, 5.Restore, 6.Clear@portal-restoring, 7.SweepOrphanFIFOs, 8.CleanStale, 9.Return).

**Outcome**: Spec, CLAUDE.md, and all doc-comments uniformly describe a nine-step bootstrap with FIFOSweeper at step 7, CleanStale at step 8, and Return at step 9. No reader-facing or contract-facing source contradicts the package docstring.

**Do**:
1. In `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md`:
   - Update § "Bootstrap Flow (Integrated) → PersistentPreRunE Sequence" (around lines 1007–1048) to insert a new step `7. SweepOrphanFIFOs — best-effort cleanup of orphan hydrate-*.fifo files whose paneKey is no longer represented by a live @portal-skeleton-* marker`. Renumber CleanStale → 8 and Return → 9.
   - Line 1048: change "returns immediately after step 7" → "returns immediately after step 9" (or whatever phrasing reflects the final step under the new numbering).
   - Lines 1161, 1165 in § "CleanStale Behavior": change "step 7 of `PersistentPreRunE`" → "step 8 of `PersistentPreRunE`".
   - Line 772: rewrite "This defensive pattern eliminates the need for a separate stale-FIFO sweep step" to acknowledge the bootstrap-step-7 sweep as a defensive complement to per-pane CreateFIFO.
2. In `CLAUDE.md`:
   - Line 66: change "**eight-step** `bootstrap.Orchestrator`" → "**nine-step** `bootstrap.Orchestrator`".
   - In the numbered list (lines 67–75) insert a new entry between current items 6 and 7: `7. **SweepOrphanFIFOs** — best-effort cleanup of orphan hydrate-*.fifo files whose paneKey is no longer represented by a live @portal-skeleton-* marker.`
   - Renumber CleanStale → 8 and Return → 9.
3. In `cmd/root.go:92`: change "canonical **eight-step** sequence" → "canonical **nine-step** sequence" in the `buildBootstrapDeps` doc-comment.
4. In `cmd/bootstrap/phase5_integration_test.go`:
   - Line 3: change "exercise the **eight-step** bootstrap.Orchestrator" → "exercise the **nine-step** bootstrap.Orchestrator".
   - Line 39 (markerProbeStub comment): change "absent during step 7 (CleanStale)" → "absent during step 8 (CleanStale)".
   - Line 74: same change as line 39 — "step 7 (CleanStale)" → "step 8 (CleanStale)".
5. Cross-check no other references to "eight-step" remain via a repo-wide grep for `eight-step` and `step 7 (CleanStale)`; resolve any further hits the same way.

**Acceptance Criteria**:
- `grep -R "eight-step" .` returns no hits in spec, CLAUDE.md, cmd/, or internal/ source/test files.
- Spec § Bootstrap Flow lists nine numbered steps with SweepOrphanFIFOs at 7, CleanStale at 8, Return at 9.
- Spec § CleanStale Behavior describes CleanStale as step 8.
- Spec line 772 no longer claims FIFO sweep is unneeded; instead acknowledges step 7 as a defensive complement.
- CLAUDE.md preface line says "nine-step" and the numbered list contains nine items in the order above.
- `cmd/root.go:92` doc-comment says "canonical nine-step sequence".
- `phase5_integration_test.go` file-level comment says "nine-step" and both markerProbeStub comments reference "step 8 (CleanStale)".

**Tests**:
- `go build ./...` succeeds (no behavioral change; comment-only / spec-only edits).
- `go test ./...` continues to pass.

---

## Task 2: Surface FIFOSweeper marker-enumeration failures via orchestrator step-7 logging
status: pending
severity: medium
sources: architecture

**Problem**: `internal/bootstrapadapter/adapters.go:114-120` implements `FIFOSweeper.Sweep()` as a two-stage operation: enumerate live markers via `state.ListSkeletonMarkers`, then remove orphan FIFOs via `state.SweepOrphanFIFOs`. When `ListSkeletonMarkers` fails, the adapter returns `nil` with the comment "soft-fail: sweep is best-effort" — producing zero log output. Per-FIFO failures inside `SweepOrphanFIFOs` are logged via `Logger.Warn`, so the asymmetry is jarring: a transient tmux failure during marker enumeration silently disables the entire sweep with no signal in `portal.log`. Operators investigating accumulating orphan FIFOs would have no breadcrumb. The orchestrator's documented contract for FIFOSweeper says "a non-nil err is logged via Warn and swallowed".

**Solution**: Return the `ListSkeletonMarkers` error from `FIFOSweeper.Sweep()` so the orchestrator's step-7 Warn-and-swallow path logs it uniformly. The orchestrator already swallows after logging, so the adapter's "best-effort: degrade to nil" semantics from the operator's perspective are preserved — the only observable change is that `portal.log` now contains the enumeration failure when it occurs.

**Outcome**: Marker-enumeration failures during bootstrap step 7 produce a Warn-level log line in `portal.log`, matching the existing observability for per-FIFO failures.

**Do**:
1. In `internal/bootstrapadapter/adapters.go` (around lines 114–120 in `FIFOSweeper.Sweep`): replace the `return nil // soft-fail: sweep is best-effort.` branch on `ListSkeletonMarkers` failure with `return fmt.Errorf("list skeleton markers: %w", err)`.
2. Verify the orchestrator's step-7 site (in `cmd/bootstrap/bootstrap.go`) follows the documented "Warn-and-swallow" pattern. If it does, no orchestrator change is needed; if it doesn't, add a `Logger.Warn(...)` call before swallowing.
3. Confirm the `state.SweepOrphanFIFOs` per-FIFO Warn logging path is unaffected.

**Acceptance Criteria**:
- `FIFOSweeper.Sweep()` returns a non-nil wrapped error when `state.ListSkeletonMarkers` fails.
- The orchestrator's step-7 site logs returned errors via `Logger.Warn` before swallowing — bootstrap continues to step 8 regardless.
- No new fatal abort path is introduced.
- Per-FIFO inline `Logger.Warn` calls inside `SweepOrphanFIFOs` remain unchanged.

**Tests**:
- Add or extend a unit test in `internal/bootstrapadapter` that injects a stub `state.ListSkeletonMarkers` returning an error and asserts `Sweep()` returns a non-nil error wrapping the underlying cause.
- Add or extend an orchestrator-level test verifying that a `FIFOSweeper.Sweep` failure is logged via the test logger and bootstrap progresses to CleanStale (step 8).

---

## Task 3: Consolidate BootstrapWarning struct and emission across cmd and tui packages
status: pending
severity: medium
sources: duplication

**Problem**: Two structural twins were introduced together by T6-9 / T6-10 and now drift in lock-step:
1. `cmd/bootstrap_warnings.go:55-61` (`EmitTo`) and `internal/tui/bootstrap_warnings.go:38-44` (`WriteBootstrapWarnings`) are byte-identical nested loops. Both carry mirror comments noting the CLI and TUI paths must produce identical stderr output. A single Fprintln→Fprintf change would have to be applied in both bodies.
2. `cmd/bootstrap.Warning` (`cmd/bootstrap/errors.go:49`) and `tui.BootstrapWarning` (`internal/tui/bootstrap_warnings.go:19`) are byte-identical struct shapes (`{Lines []string}`) sitting either side of the cmd→tui import boundary. `drainBootstrapWarningsForTUI` (`cmd/bootstrap_warnings.go:78-88`) is a pure O(n) field-copy whose only job is to bridge them.

**Solution**: Hoist the warning shape and the writer helper into a small leaf package (e.g. `internal/warning`) holding the canonical `Warning` struct and a single `WriteLines(io.Writer, []Warning)` helper. `cmd/bootstrap` aliases the type for its existing callers; both `cmd/bootstrap_warnings.go` and `internal/tui/bootstrap_warnings.go` consume the shared writer. The `drainBootstrapWarningsForTUI` conversion copy disappears.

**Outcome**: Single source of truth for the bootstrap warning shape and the stderr emission loop.

**Do**:
1. Create `internal/warning/warning.go` containing:
   - `type Warning struct { Lines []string }` with the existing godoc semantic preserved.
   - `func WriteLines(w io.Writer, ws []Warning) error` (or matching the existing return signature) implementing the byte-identical nested-loop emission.
2. In `cmd/bootstrap/errors.go:49`: replace the local `Warning` type with a type alias to `warning.Warning` (`type Warning = warning.Warning`).
3. In `cmd/bootstrap_warnings.go`:
   - Replace `EmitTo`'s body with a call to `warning.WriteLines`.
   - Delete `drainBootstrapWarningsForTUI` (lines 78–88) and update its call sites to pass `[]warning.Warning` directly.
4. In `internal/tui/bootstrap_warnings.go`:
   - Replace `BootstrapWarning` (lines 19–21) with a type alias to `warning.Warning`, OR change the TUI's external surface to consume `warning.Warning` directly. Pick whichever produces the smaller diff.
   - Replace `WriteBootstrapWarnings`'s body with a call to `warning.WriteLines`.
   - Remove the mirror comment.
5. Verify no caller anywhere else in the repo constructs `cmd/bootstrap.Warning{...}` or `tui.BootstrapWarning{...}` literals that would break under the alias.
6. Run `go build ./...` and `go test ./...`.

**Acceptance Criteria**:
- A new leaf package (e.g. `internal/warning`) exists with a single `Warning` struct and a single `WriteLines` helper.
- `cmd/bootstrap.Warning` and `tui.BootstrapWarning` are either type aliases to `warning.Warning` or removed (callers reference `warning.Warning`).
- `drainBootstrapWarningsForTUI` no longer exists.
- The byte-identical `for _, warn := range …; for _, line := range warn.Lines; fmt.Fprintln(w, line)` loop appears exactly once in the codebase, inside the new helper.
- `go build ./...` and `go test ./...` pass.

**Tests**:
- Add a unit test in the new `internal/warning` package covering `WriteLines` with: zero warnings → empty output; one warning with multiple lines → newline-separated output; multiple warnings → concatenated newline-separated output.
- Confirm existing tests for the cmd path (BootstrapWarningsSink) and the TUI path (loading-page warning drain) still pass without modification.
