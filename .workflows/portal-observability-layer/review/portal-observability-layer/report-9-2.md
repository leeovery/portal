TASK: Resolve the signal-hydrate command's component attribution (decide-and-document) (portal-observability-layer-9-2)

ACCEPTANCE CRITERIA:
- A single explicit choice (option a or b); the command's three enumeration/per-FIFO WARNs no longer ambiguously attributed.
- If (a): runSignalHydrate's "list skeleton markers failed", "list panes for session failed", "write fifo failed" WARNs render under signal: matching EagerSignalHydrate; grep "signal:" covers the hook-driven signaling path.
- If (b): taxonomy signal row at spec:166 carries the boundary note + in-source comment at the binding site.
- No new component; change confined to a re-binding (a) or doc/comment note (b); no behavioural change to signaling.

STATUS: Complete

SPEC CONTEXT:
Spec closed 15-component space (145-153) + taxonomy table (155-173); signal row :166 amended with the cycle-3 decision enumerating EagerSignalHydrate, runSignalHydrate diagnostics, and the lower-level send/receive plumbing — all under signal, while process_role stays hydrate (argv-resolved binary), orthogonal to subsystem component. Hydrate helper's own exit-path lines (incl signal timeout) stay under hydrate per Hook-firing catalog.

IMPLEMENTATION:
- Status: Implemented (option a — re-attribute to signal)
- Location: cmd/state_signal_hydrate.go:55 (cfg.Logger = signalLoggerOrDefault(cfg.Logger), was hydrateLoggerOrDefault); :89-94 (new signalLoggerOrDefault defaulting to signalLogger); :131 (RunE wires Logger: signalLogger); :58/:64/:75 (three WARNs log through cfg.Logger now signal-bound); cmd/state_common.go:36 (signalLogger = log.For("signal") + doc block recording orthogonal component-vs-process_role rationale); spec:166 amended.
- Notes: Binding now matches sibling eager_signal_hydrate.go:23 + the internal/state plumbing. hydrateLoggerOrDefault/hydrateLogger remain only for actual hydrate-helper entry points (state_hydrate.go) — the carved-out exit-path lines. No stale hydrate-attributed enumeration diagnostic remains. Decision discoverable at binding site (:11-25/:48-53/:122-126), helper doc, state_common.go, spec. No behavioural change.

TESTS:
- Status: Adequate
- Location: cmd/state_signal_hydrate_test.go:328-394
- Coverage: TestSignalHydrate_WARNsRenderUnderSignalComponent (table over the three WARN sites; assertSignalComponentWARN asserts WARN + component=signal + NOT component=hydrate). Leaves cfg.Logger nil to exercise the production default (signalLoggerOrDefault → cached signalLogger) with log.SetTestHandler re-pointing at logtest.Sink — prevents a false pass from a pre-bound logger. Existing behavioural tests confirm signaling unchanged.
- Notes: Would fail if binding regressed to hydrate (negative assertion catches the pre-change state). Focused, proportionate. AC(b)'s test guard moot since (a) chosen.

CODE QUALITY:
- Project conventions: Followed (log.For factory at package init; no t.Parallel given signalHydrateRunFunc mutation).
- SOLID: Good — signalLoggerOrDefault mirrors hydrateLoggerOrDefault; component-vs-process_role separation preserved.
- Complexity: Low (localized re-binding + parallel helper).
- Modern idioms: Yes (slog; "error", err attrs).
- Readability: Exemplary — rationale documented at every landing site + spec row.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The signal-vs-hydrate orthogonality rationale is now restated in five places (state_signal_hydrate.go:19-25/:48-53/:83-88/:122-126; state_common.go:25-35; spec:166); correct/helpful but mild documentation duplication; a future cleanup could collapse the cmd-layer copies to one canonical comment + cross-references. Discoverability currently outweighs DRY cost.
