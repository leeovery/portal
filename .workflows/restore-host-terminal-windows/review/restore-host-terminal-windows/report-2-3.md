TASK: restore-host-terminal-windows-2-3 — Env-self-sufficient attach command composition

ACCEPTANCE CRITERIA (from plan/tick):
1. AttachCommand("proj-abc123", exe→"/abs/portal", getenv{PATH:"/opt/homebrew/bin:/usr/bin"}) returns
   [/usr/bin/env, -u, TMUX, -u, TMUX_PANE, PATH=/opt/homebrew/bin:/usr/bin, /abs/portal, attach, proj-abc123].
2. Exactly one PATH= assignment and NO TMUX=/TMUX_PANE= assignment (only the two -u unsets), even when
   getenv reports a live TMUX/TMUX_PANE.
3. A session name containing a space stays a single, unquoted argv element (no shell quoting anywhere).
4. The argv uses the resolved absolute executable path, not the literal string "portal".
5. When exe() returns an error, composition returns nil + a non-nil error wrapping it (errors.Is reaches
   the injected sentinel).
6. The argv does NOT contain --spawn-ack (Phase-2 scope; deferred to Phase 3).

STATUS: Complete

SPEC CONTEXT:
Spec §"Command composition" and §"Spawned-window environment (PATH injection)": the picker builds each N-1
spawned command as an env-self-sufficient argv — `/usr/bin/env PATH=<picker's full PATH>` prefixed ahead of
`<os.Executable()> attach <session> [--spawn-ack <batch>:<token>]`. Inject the MINIMAL set (only PATH), never
a whole-env snapshot. Load-bearing invariant: TMUX/TMUX_PANE MUST NOT propagate (spawned windows must run OUT
of tmux so their `portal attach` takes the exec-attach path, not switch-client), so a picker composing from
inside tmux explicitly strips them. Single uniform mechanism for both native and config adapters — the composed
command is a real argv, never shell syntax. Using the picker's own binary keeps the version-gated warm-command
latch satisfied so each attach takes the abridged fast-path.

IMPLEMENTATION:
- Status: Implemented (correctly evolved by later phases)
- Location: internal/spawn/command.go:27 (composeAttachArgv); exe-resolution seam consumed at
  internal/spawn/burst.go:134; ack value formatter at internal/spawn/ackid.go:77 (FormatSpawnAckFlag).
- Notes: Task 2-3 landed at commit a19a9924. Two later, in-scope tasks evolved this file:
    * Phase 3 (d05b21c6, task 3-5) appended `--spawn-ack <batch>:<token>` — the Phase-3 work that criterion 6
      explicitly deferred. So criterion 6 ("must NOT contain --spawn-ack") was correct AT Phase 2 and is now
      correctly superseded; the current argv legitimately carries the flag.
    * Phase 7 (752a3024, task 7-4) removed the exported `spawn.AttachCommand` wrapper as dead code and moved
      exe-resolution + error surfacing into Burster.Run (burst.go:134-137), leaving composeAttachArgv as the
      pure unexported builder — its sole consumer is the Burster within the same package.
  Task 2-3's core contribution — the env-self-sufficient argv shape — is intact and correct:
    composeAttachArgv returns exactly
    []string{"/usr/bin/env","-u","TMUX","-u","TMUX_PANE","PATH="+path, exePath,"attach", session,
             "--spawn-ack", FormatSpawnAckFlag(batch,token)}.
  Criteria met against the CURRENT codebase:
    1 ✓ (argv shape correct; --spawn-ack tail is the legitimate Phase-3 addition)
    2 ✓ builder reads no env at all — it only receives `path`; the Burster reads solely Getenv("PATH")
        (burst.go:138), so a whole-env snapshot / TMUX leak is now STRUCTURALLY impossible (stronger than the
        original getenv-taking AttachCommand).
    3 ✓ session is a discrete element after "attach"; no quoting added.
    4 ✓ exePath is the passed absolute path; no bare "portal" element.
    5 ✓ exe() error surfaced — Burster.Run returns "", nil, err on b.Exe() failure before any window opens.
    6 ✓ (Phase-2 deferral honoured at Phase 2; correctly resolved in Phase 3.)

TESTS:
- Status: Adequate
- Coverage: internal/spawn/command_test.go drives composeAttachArgv with 5 focused subtests:
    * full wire-format equality (slices.Equal against the exact expected argv);
    * PATH-only / TMUX-strip: counts assignments — asserts exactly one PATH=, zero TMUX=, zero TMUX_PANE=,
      and presence of a -u unset (criterion 2);
    * session-with-space stays a single element immediately after "attach", plus a no-shell-quote sweep
      across all elements (criterion 3);
    * uses the provided absolute exe path, asserts no bare "portal" element (criterion 4);
    * --spawn-ack appended as two discrete tail elements, never a joined --spawn-ack=value (Phase-3 shape).
  The exe-error surfacing (criterion 5), which moved out of the removed AttachCommand, is covered in
  internal/spawn/burst_test.go:351 ("it aborts before opening any window when the executable cannot be
  resolved"): errors.Is(err, sentinel), batch=="" , results==nil, and zero OpenWindow calls — a complete
  assertion of the surface-not-swallow contract.
- Notes: No over-testing — the full-equality subtest pins the exact wire format while the other four assert
  order-independent invariants; the overlap is intentional and each subtest guards a distinct property. No
  under-testing gap: the pure builder cannot error, so the error case rightly lives at the seam layer where it
  is tested. The composed-from-inside-tmux "live TMUX in getenv" scenario is no longer expressible against the
  builder (it takes no getenv) and is redundant given the structural guarantee, so its absence is not a gap.

CODE QUALITY:
- Project conventions: Followed. Injectable seam (ExecutableResolver) over os.Executable per the DI pattern;
  pure unexported builder with a single in-package consumer; test lives in the unit lane (no tmux/daemon/binary,
  no t.Parallel). Matches golang-design-patterns / golang-testing guidance.
- SOLID principles: Good. composeAttachArgv has a single responsibility (argv assembly, no I/O, no error path);
  error handling is correctly located at the resolution seam (Burster.Run), not smeared into the builder.
- Complexity: Low. Straight-line slice literal; no branching.
- Modern idioms: Yes. slices.Equal/Index/Contains in tests; string-prefix assignment counting.
- Readability: Good — the doc comment enumerates every argv fragment's rationale (why the -u strip is
  load-bearing, why only PATH, why the picker's own binary, why session/ack are discrete elements).
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/spawn/burst.go:135-137 — Burster.Run returns the raw exe-resolution error unwrapped
  (`return "", nil, err`). Task 2-3 originally specified wrapping this as
  `fmt.Errorf("spawn: resolve executable path: %w", err)`; that context was lost when Phase 7 removed the
  AttachCommand wrapper. errors.Is still reaches the sentinel (test passes), so this is a minor debuggability
  loss, not a correctness bug — and it lives in burst.go (Phase 3/7 scope), just outside this task's file. Add
  the context wrap if the burst abort path should be self-identifying in logs.
