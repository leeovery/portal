TASK: process: panic emission in the main recover block (portal-observability-layer-2-13)

ACCEPTANCE CRITERIA:
- Panic during Execute() recovered → exactly one ERROR process: panic reason=<r>, code=2, Close skipped.
- On panic path no process: exit (mutually exclusive with panic).
- Non-panic path unchanged (clean → exit code=0 via Close; error → exit code=N).
- reason attr carries recovered panic value.
- os.Exit(2) on panic path (preserves Phase-1 mapping).

STATUS: Complete

SPEC CONTEXT:
Spec § Mechanical rule process: exit + main exit shape (518-561). recover block emits log.For("process").Error("panic","reason",r), sets code=2/panicked=true; Close gated behind !panicked so exactly one terminal marker per run. Four-way classification mutually exclusive. reason cross-listed in Process group (not a new key).

IMPLEMENTATION:
- Status: Implemented (verbatim match to canonical shape)
- Location: main.go:51-72 (run() recover block, :61 emits marker); :41-44 (!panicked Close gate).
- Notes: recover emits Error("panic","reason",r) first, then code=2/panicked=true (spec 542-545). Close gated behind !panicked. os.Exit(code) with code=2. Factored into run()+main() per Task 1-7 seam (structurally equivalent). "panic" in lifecycleBypassMsgs (handler.go:50). Component string "process" hardcoded in main (processComponent unexported) — byte-verified against the constant.

TESTS:
- Status: Adequate
- Location: main_panic_test.go (TestRunPanicEmission) + main_test.go:118-133
- Coverage: ERROR process: panic with reason on recovered panic (exactly 1, Level=ERROR, reason="kaboom"); skips Close on panic (0 exit records via mainEmitClose model); clean run → exactly 1 exit code=0; error run → exit code=1; four-way mutual exclusivity (panics+exits==1 across clean/error/panic). main_test verifies code=2/panicked=true.
- Notes: Asserts off captured slog.Records. Mutual-exclusivity is the strongest invariant. mainEmitClose models the gate without real os.Exit. No t.Parallel. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (executeFunc/errOut + withSeams + t.Cleanup; log.SetTestHandler; no t.Parallel).
- SOLID: Good — run() owns recovery+classification, main() owns os.Exit + Close gate.
- Complexity: Low.
- Modern idioms: Yes (recover-in-defer-closure, named returns, errors.As).
- Readability: Good — comment explains why marker emitted before panicked set + why Close skipped.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] main.go:61 hardcodes "process" because internal/log.processComponent is unexported; an exported alias (log.ComponentProcess) would compile-time-link main's literal to the canonical taxonomy and prevent silent drift. Out of scope for this task. (Recurring with 2-14.)
