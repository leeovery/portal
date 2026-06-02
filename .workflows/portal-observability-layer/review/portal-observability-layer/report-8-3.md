TASK: Mark the hydrate exec-failure fallback before its bare os.Exit(1) (portal-observability-layer-8-3)

ACCEPTANCE CRITERIA:
- The exec-failure fall-through in defaultExecShell no longer terminates via a bare os.Exit(1); termination paired with a terminal marker (process: exit with non-zero code via log.Close(1)).
- Process still exits non-zero on exec failure.
- Happy path (successful exec handoff) unchanged.
- No new bare os.Exit outside main; daemon self-eject remains the only sanctioned bare exit.

STATUS: Complete

SPEC CONTEXT:
Spec § Defensive invariants (486-587). Invariant 3 mandates per-process lifecycle markers (four-way classification; nothing = alarming). "Bare os.Exit prohibited outside main" (557), one sanctioned exception — daemon self-eject (558: daemon: self-eject → log.Close(0) → os.Exit(0)). The hydrate exec-failure fall-through was a second un-sanctioned bare exit: on it the just-emitted hydrate: exec INFO becomes a phantom handoff and the process vanishes unmarked (the alarming shape). Fix mirrors the self-eject's Close-before-exit discipline.

IMPLEMENTATION:
- Status: Implemented (approach (a) — local change)
- Location: cmd/state_hydrate.go:453-459 (defaultExecShell); seam doc :430-452; osExit seam state_daemon.go:101-112; log.Close init.go:151-153; production wiring state_hydrate.go:500.
- Notes: On syscall.Exec error fall-through: WARN("exec handoff failed","target","args","error",err) → log.Close(1) (emits process: exit code=1, no control flow) → osExit(1) (via the package-level seam shared with daemon self-eject, not a bare os.Exit). Happy path untouched (syscall.Exec replaces image; lines below unreachable on success; hydrate: exec INFO emitted by caller immediately before handoff). Only production os.Exit refs: main.go:44 + osExit=os.Exit seam (state_daemon.go:112); both self-eject and this path go through the seam → "no bare os.Exit outside main" holds.

TESTS:
- Status: Adequate
- Location: cmd/state_hydrate_exec_failure_test.go
- Coverage: TestDefaultExecShell_ExecFailure_MarksTerminationBeforeExit drives failure with non-existent absolute path (real ENOENT, not mocked exec); osExit stubbed (panic-to-unwind); shared logtest.Sink. Asserts exactly one osExit, code==1, WARN under component=hydrate, paired process: exit code=1 (component=process), ordering WARN-before-exit (snapshotted at osExit instant).
- Notes: Would fail if reverted to bare os.Exit (test process terminates) or log.Close(1) dropped (processExitIdx assertion). Happy-path hydrate: exec INFO independently covered by existing exec-handoff tests. Single focused test, not over-tested.

CODE QUALITY:
- Project conventions: Followed (osExit seam same pattern as daemon self-eject; t.Cleanup-restored package state; no t.Parallel; slog component logger + log.Close discipline).
- SOLID: Good — seam keeps exit path observable (DI).
- Complexity: Low (linear three-statement fall-through).
- Modern idioms: Yes (no naked exits, observable seam).
- Readability: Good — 23-line doc comment explains why dead on happy path, why bare exit violates spec, why it mirrors self-eject.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] defaultExecShell ignores the syscall.Exec return error for the exit-code decision (always Close(1)/osExit(1)); correct for the contract, but if future callers need to distinguish exec-failure causes for the exit code, approach (b) (return the error to the caller) is the cleaner extension. Not needed now — WARN captures error for forensics.
