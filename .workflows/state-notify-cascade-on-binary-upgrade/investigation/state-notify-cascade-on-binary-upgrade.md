# Investigation: State Notify Cascade On Binary Upgrade

## Symptoms

### Problem Description

**Expected behavior:**
`portal state notify` should fire only on a genuine state-change event (e.g. when a marker like `save.requested` needs writing). An idle tmux server with no real churn should produce few-to-zero `state notify` invocations.

**Actual behavior:**
On 2026-06-02, between 20:35:29 and 20:42:24 BST (a ~7-minute observation window), `portal state notify` was invoked **2315 times**. Each invocation is a full short-lived process (~500µs): boot → three lifecycle markers (`process: start`, `process: log-level resolved`, `process: exit`) → exit. Rate profile:
- Initial ~53s burst at ~23/sec (first 1224 invocations)
- Then ~2–14/sec sustained for ~6 more minutes
- Ceased completely at **20:42:24.218**

Total window impact: ~6945 log lines, ~933 KB written to `portal.log.2026-06-02`. Average ~5.5/sec, peak ~23/sec.

### Manifestation

- Log spam: ~6945 lifecycle-marker lines from short-lived `state notify` processes.
- Write amplification on `portal.log` (~933 KB in 7 min).
- fork+exec overhead at multi-Hz; each invocation presumably touches tmux state → tmux server load.
- Suspected write amplification on whatever marker `state notify` writes (`save.requested`?), pressuring the daemon capture loop.

### The diagnostic key: the cascade STOPS at saver-recycle

At 20:42:21 the user ran `portal open` for the first time since the brew upgrade 0.5.12 → 0.6.0. The new 0.6.0 binary's bootstrap step 5 detected the saver-pane daemon still running 0.5.12 (PID 36689), executed Component A's kill-barrier, and Component F respawned the daemon as PID 36776 on 0.6.0.

- Final `state notify` invocation: **20:42:24.218** — 8ms *before* `daemon: lock acquired tmux_pane=%144 pid=36776`.
- From that instant on: **zero** `state notify` invocations, despite the new daemon running normal capture ticks every ~32s (`sessions=21 panes=23 natural_churn=0`).

**Interpretation:** the cascade is NOT tied to the upgrade moment or the recycle itself — the recycle is what *stops* it. The cascade was driven by something the OLD 0.5.12 daemon was doing, or by stacked tmux hooks the new bootstrap reset. It went silent the instant the 0.5.12 daemon died.

### Reproduction Steps

Not yet reproduced. Two candidate mechanisms imply two different repro strategies (see Initial Hypotheses). The old daemon's actual lifetime is unestablished — brew upgrade preceded `portal open` by ~5–10 min, so all that's claimed is the cascade was visible for ~7 min in the window. Pre-0.6.0 binaries had no `process:` markers, so earlier `state notify` spawns left no per-invocation trace; "running invisibly for hours" is plausible but not evidenced.

**Reproducibility:** Unknown / not yet reproduced.

### Environment

- **Affected environments:** local (developer machine), real tmux.
- **Binary versions:** cascade observed under 0.5.12 daemon; 0.6.0 daemon appears cascade-free in the ~3 min post-recycle idle window.
- **Platform:** macOS (darwin).

### Impact

- **Severity:** High. Sustained multi-Hz fork+exec + tmux load + log/marker write amplification. May correlate with the recurring `hooks-and-saver-vanish-after-recent-fixes` defect — if a high-rate notify cascade drives `save.requested` writes, the capture loop is under sustained pressure during the windows when wipes were observed.
- **Scope:** Any install where an old-binary daemon survives a long time before a recycle.
- **Whether it runs continuously while a 0.5.12 daemon is alive, or only in certain windows, cannot be determined from one 7-min observation.**

### References

- `portal.log.2026-06-02` — pins both the 2315-invocation window and post-recycle silence.

---

## Analysis

### Initial Hypotheses

Two structurally different root causes, both consistent with the post-recycle silence:

1. **Daemon internal loop/feedback (version-specific):** the 0.5.12 daemon itself generated the notify invocations via some internal loop or feedback; the 0.6.0 daemon does not. Repro: run 0.5.12 in isolation.
2. **Stacked tmux global hooks:** tmux global hooks were stacked across prior binary lifetimes such that a single tmux event fired many `state notify` invocations. The new bootstrap's step-2 `RegisterPortalHooks` reset the stack to one entry per hook. Repro: `tmux show-options -g` before/after a forced saver recycle.

Both are consistent with the evidence on hand; it cannot currently tell them apart.

### Code Trace

_TBD — to be filled during Step 5 code analysis._

**Key files to examine:**
- `cmd/state_notify.go` (or wherever `state notify` subcommand is declared) — what it does, what it writes.
- `cmd/bootstrap/bootstrap.go` step 2 `RegisterPortalHooks` — hook registration idempotency.
- `internal/bootstrapadapter/hook_registrar.go` — production hook registrar, cross-binary-lifetime idempotency.
- `internal/tmux/portal_saver.go` — Component A kill-barrier + Component F respawn (the path that stops the cascade).
- `internal/state/capture.go` — daemon `save.requested` polling consumer.

### Root Cause

_TBD._

---

## Fix Direction

_TBD._

---

## Notes

- Discriminating the two hypotheses is the first analytical goal. Hook-stacking (hypothesis 2) is testable statically by reading `RegisterPortalHooks` for append-vs-replace semantics and checking whether any registered hook invokes `portal state notify`.
- The `0.6.0` binary itself appears cascade-free post-recycle (~3 min idle → only ~32s capture-tick INFO lines).
