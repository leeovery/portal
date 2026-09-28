TASK: One Hand-Off, So the Marker Cannot Be Left Behind (lazy-resume-on-attach-4-8)

ACCEPTANCE CRITERIA:
- `resumeHandOff` and `execHandOff` are the only functions in `cmd`'s production sources that call an `ExecSelf`/`ExecShell` seam, and the guard fails when a third appears
- Each emits the `exec` INFO marker as the statement immediately before the exec, and only `execHandOff` carries `hook_present`
- Per-site marker attrs are unchanged: `target` and `args` everywhere, `hook_present` on exactly the five sites carrying it today and on neither chain hand-off
- A chain hand-off that cannot resolve the binary renders `resolve portal executable:` exactly once — for the empty-path sentinel and for an `os.Executable` failure alike
- The seven hand-copied "must stay the statement immediately before the exec" comments are gone; the rule is stated once, where it is enforced
- `go test ./...` and `go test -tags integration -p 1 ./...` pass

STATUS: issues_found

SPEC CONTEXT: The spec requires the draw to replace its process image with a minimal waiter rather than block in-process, which is the ExecSelf/ExecShell hand-off chain this task consolidates. The task itself is a structural hardening from analysis: the `exec` INFO marker (the chain's last forensic record in portal.log, written through an unbuffered writer) must be emitted immediately before the exec, and it should come from one place with a source guard enforcing it. Nothing further in the spec constrains this task.

IMPLEMENTATION:
- Status: Implemented. Later tasks legitimately moved the code on: task 8-1 added `handOffToHookOrShell`, which now fronts the five `hook_present` sites and the later discard hand-off.
- Location:
  - cmd/state_resume_chain.go:96-110: the `osExecutable` seam, plus `resumeChainExe`, which owns the single `resolve portal executable:` prefix on both failure branches (`%w` wrap at :104, sentinel at :107).
  - cmd/state_resume_chain.go:117-127: `resumeHandOff` resolves, returns the resolver's error unwrapped, composes via `resumeChainArgv`, and emits `logger.Info("exec", "target", exe, "args", …)` as the statement immediately before `execSelf(exe, argv)`. It carries no `hook_present`.
  - cmd/state_resume_chain.go:132-135: `execHandOff` emits the same marker plus `hook_present` immediately before `execShell(prog, args)`.
  - cmd/state_resume_chain.go:140-148: `handOffToHookOrShell` (8-1) routes both of its branches through `execHandOff`.
  - Routed sites:
    - cmd/state_resume_draw.go:72 (`runResumeDraw` → `resumeHandOff(..., resumeWaitSubcommand, shown)` after setting Width/Height)
    - cmd/state_resume_wait.go:397-400 (`resumeRedraw`: `cfg.restore()` then `resumeHandOff(..., resumeDrawSubcommand, ...)`)
    - cmd/state_resume_wait.go:324 (`resumeAnswerEnter`)
    - cmd/state_resume_wait.go:354 (`resumeAnswerDiscard`)
    - cmd/state_resume_recover.go:58 (`runResumeRecover`)
    - cmd/state_hydrate.go:251 (`execShellOrHookAndExit`)
    - cmd/state_hydrate.go:273 (`execResumeChainAndExit`)
  - cmd/state_hydrate.go:366-373: `markResumePending`'s `ResolveExe` defaults to `resumeChainExe` and returns its error unwrapped, so the WARN renders one prefix.
  - cmd/exec_handoff_guard_test.go:16-40: the source guard.
- Notes:
  - The only exec-seam calls left in cmd's production sources are cmd/state_resume_chain.go:125 and :134. Every other site passes `cfg.ExecSelf`/`cfg.ExecShell` by reference, and `defaultExecShell` is only ever assigned, never called.
  - The chain hand-off's error reaches Cobra unwrapped, from `runResumeDraw` (:72) and `resumeRedraw` (:399). A grep finds no other `resolve portal executable` text in production.
  - The seven ordering comments are gone. The rule is stated once, on `resumeHandOff` (cmd/state_resume_chain.go:112-116), with `execHandOff` pointing at it and the guard stating its rationale.

TESTS:
- Status: Adequate
- Coverage:
  - `TestResumeHandOff/"it emits the exec marker before it hands the process image over"` (cmd/state_resume_chain_handoff_test.go:14-47) reads the sink inside the exec fake, so a marker emitted after the exec (or not at all) fails the `len(atExec) != 1` fatal. It also pins `target`/`args` against the handed-over program and argv, the INFO level, the absence of `hook_present`, and the composed argv.
  - `TestExecHandOff` (:97-134) makes the same inside-the-fake assertion for `execHandOff`, plus `hook_present` true and false.
  - "renders one resolve portal executable clause":
    - through `resumeHandOff` for the empty path, with a no-exec assertion (:49-64)
    - through `resumeChainExe` for both the empty path and an `osExecutable` failure, with `errors.Is` preserved (:67-95)
  - The guard counts calls into each helper and refuses a vacuous pass (:35-39).
  - Task commit aa81f2812 left the existing draw/wait/recover/enter/hydrate-lazy suites unmodified, so they stand as the routing regression net as planned.
- Notes: The hand-off level test covers only the empty-path branch for an `os.Executable` failure. That is sufficient, because `resumeHandOff` returns the resolver's error verbatim and the wrap is pinned at `resumeChainExe`. No over-testing.

CODE QUALITY:
- Project conventions: Followed. `osExecutable` is a package function var, staged through `withFuncSeam`. The guard is built on `sourceguardtest` (`ParsePackageSources`, `ForEachFuncCall`, `CalleeName`, `Position`). No `t.Parallel`.
- SOLID principles: Good. Each helper has one responsibility and there is one declaration of the ordering rule.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: One guard-reach gap (see FINDINGS).

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/state_resume_chain.go:140 — `handOffToHookOrShell` receives the exec seam under the parameter name `exec`. The guard recognises a seam call only by callee identifier: `isExecSeamCallee` at cmd/exec_handoff_guard_test.go:42-44 matches `execSelf`/`execShell` case-insensitively. So a direct `exec(shell, []string{shell})` written inside this function would hand the process image over with no marker and the guard would stay green. This is the one function in `cmd` that currently holds the seam under a non-matching name, and it is where a future hook/shell hand-off change would land. Fix: rename the parameter to `execShell` (matching `execHandOff`'s own parameter) at :140 and at its two uses at :143 and :147. The guard's existing name match then covers any direct call there. — FAILS: a hand-off added inside `handOffToHookOrShell` can omit the `exec` marker, leaving the pane's last image unrecorded in portal.log, and the guard that exists to catch exactly that stays green. That breaks the criterion "the guard fails when a third appears" for a seam carried under another name.

UNSETTLED:
- "`go test ./...` and `go test -tags integration -p 1 ./...` pass" — run both lanes. Also run the planned mutation: add a bare `cfg.ExecShell(...)` to a production function outside the two helpers and confirm `TestExecSeamsAreCalledOnlyByTheHandOffHelpers` reddens.
