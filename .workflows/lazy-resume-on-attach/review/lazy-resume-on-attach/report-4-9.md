TASK: One Home for What a Missing Registration Means (lazy-resume-on-attach-4-9, tick-501ae4) — declare the miss / empty-command / read-error rule and its breadcrumbs once, and route both the reboot (hydrate) path and the panel's Enter path through it.

ACCEPTANCE CRITERIA:
- One function declares the miss / empty-command / read-error rule, and both resume paths call it
- A registration found carrying an empty command drops the pane to a bare shell on both paths and records `result=miss`; `sh -c "; exec $SHELL"` is unreachable
- Breadcrumbs are unchanged: same messages, same `hook_key`/`result`/`error` attrs, same levels, one DEBUG per lookup and a WARN only on a read error
- A nil hook store still records `result=miss` on the hydrate path
- The hydrate path still resolves the stored `Mode` — a lazy registration still waits, an eager one still fires at hydrate
- `go test ./...` and `go test -tags integration -p 1 ./...` pass

STATUS: complete

SPEC CONTEXT: The spec says a stored object carrying no command, or an empty one, is not a registration: the pane falls through to a plain shell as an unregistered pane does. On the panel, Enter reads the store again at the moment of the answer. An entry that has gone by then, or an unreadable store, drops the pane to a plain shell, and the marker clears either way. The existing degradation, where a lookup failure gives a bare shell so the pane stays usable, is kept unchanged. The per-registration mode resolves against the install-wide default at restore, so the hydrate path needs the whole registration (command and mode), not just the command.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_chain.go:150-165 — `resumeRegistrationOrLog(logger, lookup func(hookKey string) (hooks.OnResume, error), hookKey) hooks.OnResume`. It has three branches. A read error emits DEBUG `hook lookup` with `result=error` and `error`, then WARN `lookup on-resume hook failed` with `hook_key` and `error`, and returns the zero value. A result with `!Found || Command == ""` emits DEBUG `result=miss` and returns the zero value. A hit emits DEBUG `result=hit` and returns the whole registration. The empty-command case follows the wait path's rule, and the returned value is normalised.
  - cmd/state_hydrate.go:226-235 — `lookupOnResumeOrLog` now only calls the shared rule through a closure. A nil `HookStore` answers `(hooks.OnResume{}, nil)`, so it records `result=miss`. Otherwise the closure calls `LookupOnResume(hookKey, hooks.ViaHydrate)`.
  - cmd/state_hydrate.go:196-200 — `resolveResumeDecision` still reads `Mode` from the returned registration (`lookup.Found && resumemode.Resolve(...) == Lazy`). Normalisation means a found-but-empty registration can no longer choose to wait.
  - cmd/state_hydrate.go:239-252 — `execShellOrHookAndExit` hands `lookup.Command` to `handOffToHookOrShell` (cmd/state_resume_chain.go:140-148). That function execs a bare shell for an empty command, so `hookExecArgs("", shell)` cannot be reached from either branch.
  - cmd/state_resume_wait.go:315-326 — `resumeAnswerEnter` takes `.Command` from `resumeRegistrationOrLog(cfg.Logger, cfg.LookupResume, cfg.HookKey)` (line 320). `resumeCommandAtAnswer` no longer exists in the file.
- Notes: Neither path keeps a second copy of the rule. `state_resume_draw.go` and `state_resume_recover.go` do not read the store at all. The recover tail and the discard answer hand `""` to `handOffToHookOrShell`, so there is no third copy for later work to diverge from. `internal/hooks/lookup.go:38-40` still collapses an empty stored command to the zero result. The shared branch is defensive, which is what the plan describes.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/state_resume_registration_test.go:33-104 — new tests of the shared function through a fake lookup, named exactly as the plan asks:
    - "it reads a registration carrying an empty command as no hook" — zero value, `result=miss`, no WARN. Its comment at lines 34-37 says the production lookup cannot produce this value.
    - "it records a read failure as a DEBUG and a WARN and answers no hook" — DEBUG `result=error` with `error` matched by `errors.Is`; WARN with `hook_key` and `error`.
    - "it records a miss as a DEBUG alone"
    - A hit case asserting the whole registration, `Mode` included, and one DEBUG.
    If the normalisation were dropped, the empty-command case would fail on both the returned value and the `result` attr.
  - Wait path: cmd/state_resume_enter_test.go:189-200 (a registration found with an empty command gives a plain shell) and 202-221 (the read-error WARN and DEBUG `result=error`).
  - Hydrate path:
    - cmd/state_hydrate_exec_log_test.go:29-69 — nil store gives DEBUG `result=miss` with no `error` attr (AC4).
    - cmd/state_hydrate_exec_log_test.go:71-116 — `result=error`, `error` attr, exactly one WARN.
    - cmd/state_hydrate_lazy_test.go:344-360 — absent store records a miss.
    - cmd/state_hydrate_lazy_test.go:362-383 — unreadable store: `result=error`, one WARN, no wait.
    - cmd/state_hydrate_lazy_test.go:440-452 — empty stored command under a lazy pin gives a bare shell and no pending marker.
    - cmd/state_hydrate_lazy_test.go:141-182 — eager still fires and a pinned lazy still parks (AC5).
  - The mutation targets the plan names are real and each would fail if the WARN were dropped:
    - cmd/state_resume_enter_test.go:213 uses `Only` on the WARN.
    - cmd/state_hydrate_exec_log_test.go:105 and cmd/state_hydrate_lazy_test.go:380 count WARN lines equal to 1.
    Each path therefore observes the one shared declaration.
- Notes: None.

CODE QUALITY:
- Project conventions: Followed — the rule sits in cmd/state_resume_chain.go beside the chain's shared vocabulary. The new tests use `logtest.NewCaptureLogger` and the `Records()...Only` query chain, and no test uses `t.Parallel()`.
- SOLID principles: Good — the rule is independent of `hydrateConfig` and `resumeWaitConfig` and depends only on a lookup function and a logger.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good — the comments in the changed code are accurate against it (cmd/state_resume_chain.go:150-151, cmd/state_hydrate.go:226-227).
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` and `go test -tags integration -p 1 ./...` pass" — needs both lanes run on the delivered tree. That run also confirms that the new test helpers in cmd/state_resume_registration_test.go (`registrationHookKey`, `lookupReturning`, `assertLookupDebug`) do not collide with other declarations in the cmd test package.
