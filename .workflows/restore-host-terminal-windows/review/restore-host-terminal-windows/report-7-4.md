TASK: 7-4 — Remove (or unexport) the dead spawn.AttachCommand public API (tick-160e87)

ACCEPTANCE CRITERIA:
- spawn.AttachCommand is no longer exported (removed, or unexported only if a real caller remains).
- ExecutableResolver and composeAttachArgv are unchanged and remain the sole production composition path.
- The package builds and all spawn tests pass; no dangling reference to the removed symbol remains.
- (If unexported instead of removed) a unit test exercises the retained helper so it is no longer dead.

STATUS: Complete

SPEC CONTEXT: This is a Phase 7 analysis-cycle hardening task. AttachCommand was public spawn surface (resolve executable + compose attach argv) with no production caller — Burster.Run resolves os.Executable once up front and calls the pure composeAttachArgv per window (deliberately NOT per-window resolve). An untested public composition path can silently drift from composeAttachArgv. Removal chosen (preferred per tick) rather than unexport, since Burster.Run already resolves once up front.

IMPLEMENTATION:
- Status: Implemented (removal path chosen)
- Location: internal/spawn/command.go — now contains only ExecutableResolver (line 6) and composeAttachArgv (line 27); AttachCommand definition is gone.
- Verification:
  - Repo-wide grep for "AttachCommand" returns exactly one match: cmd/attach_test.go:65 `func TestAttachCommand` — the unit test for the unrelated `portal attach` command, NOT spawn.AttachCommand. No dangling reference to the removed symbol anywhere in spawn/ or spawntest/.
  - ExecutableResolver unchanged and still used by the burster (burst.go:77 field, :90 NewBurster param, :134 b.Exe() call).
  - composeAttachArgv unchanged and remains the sole production composition path (burst.go:161 is the only production call site).
  - Stale doc-comment updated: the former burst.go:113 "not AttachCommand" note is now phrased "Composing each argv from the once-resolved exePath via the pure composeAttachArgv builder" (burst.go:112-114) — no residual reference.
  - spawntest comment updated: spawntest/adapter.go:91 references "the exact wire format composeAttachArgv produces" — no residual AttachCommand reference.
- Notes: Clean removal; no leftover imports or orphaned helpers. Because removal (not unexport) was chosen, the conditional "unit test exercises the retained helper" acceptance clause does not apply.

TESTS:
- Status: Adequate
- Coverage: command_test.go retains TestComposeAttachArgv (5 focused sub-tests: full argv shape, PATH-only injection + TMUX/TMUX_PANE strip, spaced-session single-element, provided-exe-not-bare-lookup, --spawn-ack two-element tail) — the composition contract is fully covered on the surviving symbol. The fixedExe helper (an ExecutableResolver factory) remains exercised by 10 call sites in burst_test.go, so ExecutableResolver is not left dead. No test ever asserted AttachCommand behaviour beyond its existence, so removal drops no coverage.
- Notes: No under-testing (composition contract intact on composeAttachArgv), no over-testing (sub-tests each assert a distinct property, no redundancy). Test judged by reading only; not executed, per task constraints.

CODE QUALITY:
- Project conventions: Followed — reduces exported surface to what is actually reached (matches the codebase's leaf/seam discipline); ExecutableResolver seam preserved.
- SOLID principles: Good — removes an untested parallel composition path (single source of truth for argv is now composeAttachArgv).
- Complexity: Low — net deletion.
- Modern idioms: N/A (deletion).
- Readability: Good — command.go is now a tight two-declaration file with accurate doc-comments; downstream comments were corrected in lockstep so no comment references a symbol that no longer exists.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
