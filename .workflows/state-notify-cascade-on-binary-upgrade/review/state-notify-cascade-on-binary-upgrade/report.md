# Implementation Review: State Notify Cascade on Binary Upgrade

**Plan**: state-notify-cascade-on-binary-upgrade
**QA Verdict**: Approve

## Summary

The bugfix is implemented faithfully and completely. The root cause — tmux 3.6b's no-arg `show-hooks -g` being blind to `pane-*` and geometry/rename `window-*` events, which let Portal's idempotency oracle stack unbounded duplicate hooks on `pane-focus-out` and `window-layout-changed` — is fixed by the single uniform architectural shift the spec prescribed: every Portal hook read now goes through a per-event `ShowGlobalHooksForEvent(event)` seam, registration is rebuilt as declarative "ensure exactly one" convergence, teardown reads per-event, and the defective no-arg `ShowGlobalHooks` method is deleted with no remaining caller. The two migration helpers (`migrateHydrationHooks`, `migrateSessionClosedHook`) are folded into the unified path as planned. All 20 tasks across the 5 phases were independently verified Complete with zero blocking issues. Crucially, the fix is proven by real-tmux integration tests running against tmux 3.6b — the actual affected version — not mock commanders, exactly as the spec's Testing Requirements mandate; the orchestrator confirmed those tests run (not skipped) and pass. `go build`, `go vet`, and the full `go test ./...` suite are green. The four analysis-cycle phases (2–5) consolidated duplicated test-harness and production helpers, made `managedEvents` the single source of truth for both the event-set and teardown fingerprint-set, and — in phase 4 — closed a genuine latent seam (AC #5): teardown previously omitted the `portal state commit-now` fingerprint, so the converged `session-closed` hook would have survived `portal hooks reset`; that is now derived from the `managedEvents` union and covered by a non-vacuous real-tmux teardown-at-depth test.

## QA Verification

### Specification Compliance

Implementation aligns with the specification. All 8 spec Acceptance Criteria are satisfied:

1. **No growth across bootstraps** — `TestRegisterPortalHooks_NoGrowthAcrossBootstraps` runs registration N=3× on real tmux, asserts every managed event stays at exactly one Portal entry, naming `pane-focus-out` and `window-layout-changed` explicitly, reading per-event so the assertion is itself non-blind.
2. **Existing stacks self-collapse** — `TestRegisterPortalHooks_SelfHealsKDeepStackLeavingUserHookIntact` seeds a K-deep stack and asserts collapse to one (non-vacuous pre-condition).
3. **Stale bodies migrate in place** — covered for un-separated `signal-hydrate` and pre-fix `notifyCommand` on `session-closed`.
4. **User hooks survive** — co-resident non-Portal hooks on managed events (incl. the two blind ones) are untouched by both registration and teardown.
5. **Teardown reaps at depth** — `TestUnregisterPortalHooks_ReapsAtDepthOnBlindEventsLeavingUserHookIntact`; the `session-closed` commit-now fingerprint seam was closed in phase 4 (task 4-1) and is now reaped to zero under a non-vacuous seeded stack.
6. **Global read removed** — `grep "ShowGlobalHooks\b"` returns nothing across `internal/` and `cmd/`; `ShowGlobalHooksForEvent` is the sole survivor (one intentional raw-socket `show-hooks -g` remains only in the blind-spot regression test).
7. **Idempotent and churn-free** — `TestRegisterPortalHooks_SecondRegistrationIsChurnFree` proves index stability + no eviction INFO + no WARN on a second registration.
8. **Cascade eliminated** — structurally guaranteed by the no-growth convergence guard (per spec, no separate process-count test required).

The deliberate registration-vs-teardown fingerprint asymmetry (registration omits `portal state migrate-rename`; teardown retains it for old-binary cleanup) is preserved and guarded by parity tests. No new log component or attr key was introduced (closed taxonomy honoured). `ParseShowHooks` is byte-for-byte unchanged.

### Plan Completion

- [x] Phase 1 (Per-Event Hook Convergence) — all 11 acceptance criteria met
- [x] Phase 2 (Analysis Cycle 1) — all tasks completed
- [x] Phase 3 (Analysis Cycle 2) — all tasks completed
- [x] Phase 4 (Analysis Cycle 3) — all tasks completed, incl. the AC #5 fingerprint-seam fix
- [x] Phase 5 (Analysis Cycle 4) — all tasks completed
- [x] All 20 tasks completed and independently verified
- [x] No scope creep — the migrate-rename v2 work, the CPU-peg lead, and the hooks-and-saver-vanish cross-link remained out of scope as the spec specified

### Code Quality

No issues found. The convergence engine reuses single-responsibility shared primitives (`parseEventEntries`, `evictPortalEntries`, `warnShowHooksFailure`) across registration and teardown while deliberately keeping their divergent error contracts (registration best-effort/not-counted vs teardown `errors.Join` naming `event[index]`) and fingerprint sets at the call site rather than homogenising them. `managedEvents` is the single source of truth: `managedEventNames()`, `managedEventFingerprintUnion()`, and `teardownFingerprints()` all derive from it, so a future hook category automatically widens teardown coverage. The exported `UnregisterPortalHooks(*Client) error` signature is preserved for the `cmd/state_cleanup.go` function-value consumer; an injected-logger inner variant closes the teardown-WARN testability gap. Idiomatic Go throughout (`errors.Join`, `sort.Reverse`, `%w` wrapping, `log.OrDiscard` nil-tolerance); doc comments are accurate and traceable to spec sections.

### Test Quality

Tests adequately verify requirements, neither under- nor over-tested. The defect is a tmux-output-shape issue invisible to mock commanders, and the implementation correctly uses real-tmux socket fixtures (`internal/tmuxtest`) for the five spec-mandated integration tests, all running against tmux 3.6b. The blind-spot regression guard locks the exact tmux 3.6b reality the fix depends on (no-arg omits the pane/geometry events while per-event includes them). Mock-commander unit tests cover the pure convergence/teardown logic, fault injection (read failure, per-index unset failure, `CommandError`), and the no-double-log invariant on both paths. The phase-4 teardown-at-depth extension uses a non-vacuous pre-condition assert so a green pass cannot mask "nothing was there." The no-arg-global-read `t.Fatalf` tripwire is shared by both register and teardown dispatch paths, so a regression to the blind read fails loudly. The analysis-cycle test refactors (phases 2/3/5) are behaviour-preserving with assertions unchanged in meaning.

### Required Changes (if any)

None.

## Recommendations

### Quick-fixes

1. **(task 4-3)** `internal/tmux/hooks_register_six_event_routing_test.go:102` — the re-anchored spec-section pointer in the `t.Errorf` reads `Registration Redesign — Ensure Exactly One`, but the actual spec heading carries internal double-quotes: `Registration Redesign — "Ensure Exactly One"` (specification.md:54). Add the quotes for exact fidelity. Cosmetic; unambiguous as-is.

### Ideas

2. **(task 4-1)** `internal/tmux/hooks_register_realtmux_test.go:450-455` — the post-teardown zero-assertion hand-authors a literal teardown-fingerprint slice rather than deriving from `tmux.PortalTeardownFingerprints()`. This is a deliberate independent oracle (it would catch a derivation bug that also corrupts the production list). A one-line comment marking the intentional duplication would stop a future contributor "DRY-ing" it back to the production helper and silently weakening the guard.
3. **(task 1-7)** The self-heal test exercises depth-collapse only on `pane-focus-out`; `window-layout-changed` depth-collapse is covered only indirectly (by 1-6's depth-1 no-growth across N runs). Optionally parameterise the self-heal test over both blind events for symmetry. Not required by the AC.
4. **(task 4-2)** No executable negative test deliberately fires the no-arg-global-read `t.Fatalf` on the teardown path — it is a shared structural tripwire that only triggers on regression. Optionally add a tiny sub-test invoking the dispatcher with `("show-hooks","-g")` under a recovered `*testing.T` to make the fatal path observable proof of AC #2.
5. **(task 3-2)** `TestRegisterPortalHooks_SelfHealsKDeepStackLeavingUserHookIntact` still hand-rolls a read→parse→fingerprint-filter loop that is now cleanly expressible via the new `portalEntryCommandsForEvent` primitive. A residual, not a regression; routing it through would complete the single-source-of-truth intent.
6. **(tasks 1-2 / 3-1)** The two convergence/teardown WARNs ("show-hooks failed" vs "failed to evict portal hook") are distinguished only by free-form message text. If a future log-grep consumer needs to discriminate them programmatically, a structured attr would be cleaner — constrained today by the intentionally closed attr taxonomy. Also, per-event eviction detail is not emitted at DEBUG on the successful-eviction path (spec marks this optional, "may"). Both are enhancements, not gaps.
7. **(task 4-2)** After task 5-1 routed the read-fault teardown sites directly through `perEventDispatchWithFaults`, `dispatchUnregisterHooks` (which now only carries unset faults) is slightly asymmetric with the read-fault sites that bypass it. A one-line cross-reference comment explaining why would aid future readers.
