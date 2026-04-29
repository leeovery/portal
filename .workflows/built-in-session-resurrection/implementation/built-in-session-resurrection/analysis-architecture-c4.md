---
agent: architecture
cycle: 4
findings_count: 2
---
# Architecture Analysis (Cycle 4)

## Summary

Cycle 3's nine-step restructure is structurally sound — interfaces compose cleanly, NoOp coverage is exhaustive for degradable steps, integration tests pin the new step-7 ordering invariant, and orphaned API surfaces from cycle-3 cleanup appear consistently used. Two residual issues remain: the FIFOSweeper adapter silently swallows its upstream marker-enumeration failure, and two doc-comments still describe the orchestrator as eight-step.

---

## Findings

### FINDING: FIFOSweeper adapter silently swallows ListSkeletonMarkers error
- **Severity**: medium
- **Files**: `internal/bootstrapadapter/adapters.go:114-120`
- **Description**: The cycle-3 FIFOSweeper extraction split sweep into two operations: enumerate live markers (`state.ListSkeletonMarkers`) and remove orphan FIFOs (`state.SweepOrphanFIFOs`). The adapter's `Sweep()` returns `nil` on a `ListSkeletonMarkers` failure with no log line (`return nil // soft-fail: sweep is best-effort.`). Per-FIFO failures inside `SweepOrphanFIFOs` are logged via `Logger.Warn` — but the upstream marker-enumeration failure (a transient tmux failure during step 7) produces zero observability. The orchestrator's contract for FIFOSweeper says "a non-nil err is logged via Warn and swallowed" — that is the natural place to surface this failure, but the adapter elects to return nil instead, breaking the symmetry and leaving operators with no signal in portal.log when orphan FIFOs accumulate due to recurring marker-enumeration failures.
- **Recommendation**: Either (a) return the error from `FIFOSweeper.Sweep()` and let the orchestrator's step 7 Warn log it — adapter's "best-effort: degrade to nil" semantics still hold because the orchestrator swallows after logging — or (b) log inline via `s.Logger.Warn` before returning nil. Option (a) is cleaner: it pushes log-and-swallow uniformly into the orchestrator's step site, removing the inline silent-swallow.

### FINDING: Step-count terminology drift in two doc-comments after cycle-3 restructure
- **Severity**: low
- **Files**: `cmd/root.go:92`, `cmd/bootstrap/phase5_integration_test.go:3`
- **Description**: Cycle 3 promoted FIFOSweeper to a discrete step, taking the orchestrator from "eight-step" to "nine-step." The authoritative package docstring (`cmd/bootstrap/bootstrap.go:1-15`) and `bootstrap_test.go:92,621` were updated; two outliers were missed. `cmd/root.go:92` still says "the canonical eight-step sequence (see cmd/bootstrap_production.go)" and `phase5_integration_test.go:3` says "Phase 5 integration tests exercise the eight-step bootstrap.Orchestrator". Both contradict the package docstring's load-bearing step-count claim.
- **Recommendation**: Update both comments to "nine-step" to match `cmd/bootstrap/bootstrap.go`'s authoritative package docstring.

(Note: The architecture finding overlaps with two of the standards findings about doc-comment step-count drift. Synthesizer should dedupe.)
