TASK: Lifecycle-marker level-filter bypass in the handler (portal-observability-layer-2-10)

ACCEPTANCE CRITERIA:
- At WARN, process-component record with msg start/exit/exec/log-level resolved still emitted.
- At ERROR, same lifecycle records still emitted.
- At WARN, a non-process INFO record (component=daemon msg "tick complete") filtered.
- A process-component INFO whose msg is NOT in the closed set filtered at WARN.
- At INFO/DEBUG, all records behave normally per level.

STATUS: Complete

SPEC CONTEXT:
Spec § Defensive invariants → Lifecycle markers bypass the level filter (593-595): closed process-component set {start, exit, exec, panic, log-level resolved} emitted unconditionally regardless of PORTAL_LOG_LEVEL, semantically INFO (no ERROR pollution). § Log-level propagation verification (637): log-level resolved must bypass so the test anchor holds at warn/error. [needs-info] (component-only vs component+msg) resolved as component+msg by the four-way classification.

IMPLEMENTATION:
- Status: Implemented (faithful, incl explicit Enabled/Handle filter-authority design)
- Location: internal/log/handler.go — lifecycleBypassMsgs closed set (38-52, exactly the five, single source of truth); processComponent const (30); Enabled (118-121, coarse INFO-floor pre-gate via min(level, LevelInfo)); Handle bypass (142-145: bypass := component==processComponent && lifecycleBypassMsgs[r.Message]; if !bypass && r.Level < level → return nil).
- Notes: Bypass in Handle (not Enabled); Enabled admits INFO floor; handler is filter authority; identification by component+msg; markers stay at original level (no bump). [needs-info] resolved correctly. Negligible-cost note documented per task instruction.

TESTS:
- Status: Adequate
- Location: internal/log/handler_test.go
- Coverage: EmitsProcessLifecycleMarkersAtWarn (all five msgs, asserts level stays INFO — no bump); EmitsProcessLifecycleMarkersAtError; FiltersNonProcessInfoAtWarn (daemon tick complete dropped); FiltersArbitraryProcessInfoNotInLifecycleSetAtWarn (process "doing work" dropped); BehavesNormallyAtInfoAndDebug (6-case table); DropsDebugWhenConfiguredLevelIsInfo (guards Task 1-3 contract under the Enabled INFO-floor change).
- Notes: Behaviour (emitted vs filtered), would fail if broken (remove bypass / widen to component-only / bump level all fail an assertion). Each test distinct AC/edge. All five message keys covered individually.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; handler swap helpers).
- SOLID: Good — single source of truth for closed set; Handle owns filter authority; Enabled/Handle responsibilities cleanly separated.
- Complexity: Low (single boolean guard).
- Modern idioms: Yes (min builtin, map[string]bool set, slog.Leveler).
- Readability: Good — closed-set/Enabled-pre-gate/Handle-authority documented.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] panic emitted at ERROR so it passes the gate at WARN/ERROR without the bypass; bypass only materially matters above ERROR. Correct/harmless asymmetry.
- [idea] lifecycleBypassMsgs string literals duplicate the emission-site message strings (init.go/close_exit.go); a shared const set would remove drift risk (cross-task, touches 2-11/2-12).
