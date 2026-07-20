TASK: cli-verb-surface-redesign-4-2 — Runtime tmux checks (saver up, hooks no duplicates) + distinct down-server report

ACCEPTANCE CRITERIA (from plan row 4-2 + spec Exit-code contract / check catalog):
- server down → saver/hooks/daemon fail with the distinct "Portal runtime not running — run `portal open` to start" message, NOT corruption
- ≥2 Portal-fingerprint entries on any managed event → duplicate → fail, using per-event ShowGlobalHooksForEvent + managedEvents fingerprints, NOT the tmux-3.6b-blind no-arg show-hooks
- `_portal-bootstrap` present but saver absent still fails the saver check
- transient tmux read reported honestly (not-evaluable), not as a crash

STATUS: Complete

SPEC CONTEXT:
Spec §"doctor" check catalog: daemon alive; global hooks registered without duplicates (exactly one Portal entry per managed event); `_portal-saver` up. Spec §Exit-code contract: a down server counts as unhealthy → non-zero, reported "honestly and distinctly — 'Portal runtime not running — run `portal open` to start' vs. actual corruption — not a crash." Host-terminal check is informational and never drives exit (that is task 4-4). The hooks check must use the per-event read because tmux 3.6b's no-arg show-hooks -g is blind to the pane-*/geometry window-* events on which duplicate hooks stack (CLAUDE.md bootstrap step 2 + hooks_register.go).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/doctor.go:33 — `doctorRuntimeNotRunning` const (byte-exact distinct down-server detail, em-dash U+2014, no backticks).
  - cmd/doctor.go:530-544 `checkSaverUp` — down server → checkFail w/ not-running detail; else probes injected SaverPresent seam (production wraps tmux.SaverPanePIDOrAbsent over tmux.PortalSaverName): present→pass, absent→fail, transient err→checkNotEvaluable.
  - cmd/doctor.go:561-588 `checkHooksRegistered` — down server → checkFail w/ not-running detail; read error → checkNotEvaluable; any event count ≥2 → duplicate checkFail (first offending event in sorted order for determinism); any event count 0 → not-registered checkFail; else pass. Duplicates reported ahead of missing.
  - cmd/doctor.go:502-523 `checkDaemonAlive` — down-server branch also emits the same distinct not-running detail (the "all three runtime checks" down-server contract).
  - cmd/doctor.go:352 — server gate read ONCE (deps.ServerRunning) and threaded into daemon/saver/hooks; state-dir & sessions.json checks stay server-independent.
  - internal/tmux/hooks_count.go:23 `PortalHookCountsByEvent` — iterates managedEvents, reads each via ShowGlobalHooksForEvent (NOT the no-arg global), classifies with the SAME containsAny(entry.Command, me.fingerprints) predicate convergeEvent uses; returns a map carrying every managed event (0-count included) so caller can distinguish registered-once / duplicated / not-registered. Per-event read failure → nil map + wrapped error (the transient not-evaluable path).
  - internal/tmux/tmux.go:954 ShowGlobalHooksForEvent — pre-existing per-event `show-hooks -g <event>` seam, reused (not modified) by this task.
- Notes:
  - `_portal-bootstrap`-independence is structural: the saver probe is keyed to tmux.PortalSaverName specifically, so a live `_portal-bootstrap` (which keeps the server up → serverUp=true) cannot rescue the saver check. The "server up + saver absent → checkFail" path is exactly the `_portal-bootstrap`-present/saver-absent scenario.
  - Down-server message: task/spec render `portal open` in markdown backticks; the literal rendered detail is `run portal open to start` (no backticks). Implementation and its independent test constant agree byte-for-byte and match the spec's plain-text intent. Not drift.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/doctor_test.go:407 TestDoctorServerDownReportsRuntimeNotRunning — down server → daemon/saver/hooks ALL checkFail with the byte-exact not-running detail (asserted against an independently-declared const, not the production one); healthy probe returns are supplied so a wrongly-bypassed gate would fail loudly; doctorUnhealthy==true (unhealthy → non-zero, distinct from corruption); state dir + sessions.json stay checkPass (server-independent).
  - cmd/doctor_test.go:451 TestDoctorHooksCheck — one-per-event pass; a duplicated event (count 3) → "duplicate hook entries on pane-focus-out (3)"; duplicate reports first-in-sorted-order; zero-count → not-registered fail; duplicate takes precedence over zero-count; transient read (err) → checkNotEvaluable and does not drive exit.
  - cmd/doctor_test.go:562 TestDoctorSaverCheck — present→pass, absent→checkFail "_portal-saver not running", transient err→checkNotEvaluable and does not drive exit.
  - internal/tmux/hooks_count_test.go:18 TestPortalHookCountsByEvent_CountsOnlyPortalFingerprintEntries — foreign (non-Portal-fingerprint) entry ignored; stacked Portal duplicate → count 2; every managed event present in the map; runs over per-event dispatch.
  - internal/tmux/hooks_count_test.go:67 TestPortalHookCountsByEvent_PerEventReadFailurePropagates — per-event read failure → nil map + wrapped error naming the failed event (errors.Is recovers sentinel).
  - internal/tmux/hooks_register_test.go:128 perEventDispatch(WithFaults) mock carries a `t.Fatalf` guard that fails if the no-arg global `show-hooks -g` is ever dispatched — structurally proving PortalHookCountsByEvent uses per-event reads, directly pinning the "not the tmux-3.6b-blind no-arg show-hooks" clause.
- Notes:
  - The task's "≥2 → duplicate" bound is verified at both layers: the counter (count==2 stacked dup) and the check (count==3 fail; sorted-order determinism). Would fail if the feature broke.
  - No dedicated named subtest for "_portal-bootstrap present, saver absent", but the scenario is behaviourally identical to TestDoctorSaverCheck/"absent fails" (serverUp=true via SaverPresent-independent gate, saverPresent=false) and the probe is structurally keyed to PortalSaverName. Coverage is adequate; a rename/comment would only clarify intent.
  - Not over-tested: each subtest pins a distinct classification (pass/dup/first-sorted/zero/precedence/transient); no redundant happy-path duplication.

CODE QUALITY:
- Project conventions: Followed. DI via *DoctorDeps seam with per-field nil-check fall-through (matches commitNowDeps/bootstrapDeps idiom); managedEvents/fingerprint knowledge stays inside internal/tmux (PortalHookCountsByEvent) and never leaks to cmd; read-only (show-hooks only, no set-hook). Heavy doc-comments are consistent with house style.
- SOLID: Good. checkSaverUp / checkHooksRegistered / PortalHookCountsByEvent each single-responsibility; count classification (tmux) cleanly separated from health verdict (cmd).
- DRY: Good. doctorRuntimeNotRunning is one shared constant across the three runtime checks; the count predicate reuses convergeEvent's containsAny(entry.Command, fingerprints) rather than re-deriving classification. The test-side duplicate of the message const is a deliberate, documented independent contract pin.
- Complexity: Low. Two ordered linear passes over the sorted event set (dup-first, then missing); clear branches.
- Modern idioms: Yes (errors.Is/%w wrap, sorted determinism, map-presence for tri-state).
- Readability: Good; intent is documented at each decision point.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] cmd/doctor_test.go:588 — the "_portal-bootstrap present but saver absent" edge from the plan row is only implicitly covered by the "absent fails" subtest (serverUp=true, present=false). Consider renaming/commenting that subtest to name the `_portal-bootstrap`-independence intent, or adding a one-line subtest, so the acceptance edge is explicit in the suite. Coverage is already adequate (the probe is keyed to PortalSaverName), so this is clarity-only.
