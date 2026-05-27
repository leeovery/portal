---
status: complete
created: 2026-05-27
cycle: 2
phase: Plan Integrity Review
topic: Bootstrap CleanStale Wipes Hooks On Tmux Transient
---

# Review Tracking: Bootstrap CleanStale Wipes Hooks On Tmux Transient - Integrity

Cycle 2 — post-fix follow-up after cycle 1 findings were applied. All eight cycle-1 fixes verified in place. One new minor finding identified during fresh integrity scan.

## Findings

### 1. Task 3-2 destructures `orch.Run(ctx)` with the wrong arity

**Severity**: Minor
**Plan Reference**: Phase 3, Task `bootstrap-cleanstale-wipes-hooks-on-tmux-transient-3-2`, Do step 6 (the commander-factory-seam code sample landed by cycle 1).
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The cycle-1 fix to Task 3-2 added a concrete code sample showing the test invoking the orchestrator: `warnings, err := orch.Run(ctx)`. But `(*bootstrap.Orchestrator).Run` at `cmd/bootstrap/bootstrap.go:248` returns three values, not two:

```go
func (o *Orchestrator) Run(ctx context.Context) (bool, []Warning, error) {
```

The first return value (`bool`) signals whether the tmux server was just started in this bootstrap (consumed by callers to decide whether to drain warnings via the TUI loading page vs the bare-CLI path — see CLAUDE.md §"Server bootstrap"). Following the task literally produces a compile error. An implementer would have to pause, audit the signature, and either rename the bool or `_`-discard it — exactly the "force the implementer to make design decisions" failure mode this review catches.

The fix is mechanical: update the destructure to three values. While the snippet is being touched, two related polish items in the same sample are worth tightening:

1. The sample creates `ctx` implicitly — better to spell it `ctx := context.Background()` so the import surface is unambiguous.
2. The assertion sentence after the sample says "assert against the returned warnings slice and `err`" — update to also state that the new first return (`serverStarted bool`) is `_`-discarded in this test (it carries no assertion value for the hooks-preservation property).

**Current** (Do step 6, the test-side override snippet plus the trailing sentence):

```
     In the test, override the factory before invoking `buildProductionOrchestrator`:
     ```go
     base := commanderFactory()
     stub := &transientListPanesCommander{inner: base, mode: <policy>, sticky: true}
     prev := commanderFactory
     commanderFactory = func() tmux.Commander { return stub }
     t.Cleanup(func() { commanderFactory = prev })
     orch, _ := buildProductionOrchestrator()
     warnings, err := orch.Run(ctx)
     ```
     Assert against the returned warnings slice and `err`. Wiring caveat: this task introduces the seam in addition to consuming it — add the new package-level `var commanderFactory` and update the one call site inside `buildProductionOrchestrator` in the same PR so the test compiles. The production surface widens by one unexported variable — acceptable per the `cleanDeps` precedent.
```

**Proposed** (Do step 6):

```
     In the test, override the factory before invoking `buildProductionOrchestrator`:
     ```go
     base := commanderFactory()
     stub := &transientListPanesCommander{inner: base, mode: <policy>, sticky: true}
     prev := commanderFactory
     commanderFactory = func() tmux.Commander { return stub }
     t.Cleanup(func() { commanderFactory = prev })

     orch, _ := buildProductionOrchestrator()
     ctx := context.Background()
     _, warnings, err := orch.Run(ctx)
     ```
     Note the three-value destructure — `(*bootstrap.Orchestrator).Run` at `cmd/bootstrap/bootstrap.go:248` returns `(serverStarted bool, warnings []Warning, err error)`. The `serverStarted` bool is `_`-discarded in this test (it signals whether the tmux server was just started, consumed elsewhere for TUI-vs-bare-CLI warning drain ordering — carries no assertion value for the hooks-preservation property under test here). Assert against the returned `warnings` slice and `err`. Wiring caveat: this task introduces the seam in addition to consuming it — add the new package-level `var commanderFactory` and update the one call site inside `buildProductionOrchestrator` in the same PR so the test compiles. The production surface widens by one unexported variable — acceptable per the `cleanDeps` precedent.
```

**Resolution**: Fixed
**Notes**:

---
