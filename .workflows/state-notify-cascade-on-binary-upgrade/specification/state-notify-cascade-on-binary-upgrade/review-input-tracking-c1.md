---
status: complete
created: 2026-06-02
cycle: 1
phase: Input Review
topic: state-notify-cascade-on-binary-upgrade
---

# Review Tracking: state-notify-cascade-on-binary-upgrade - Input Review

## Findings

### 1. Concrete hook body shapes (the `run-shell` wrapper + `command -v portal` guard)

**Source**: investigation §"Code trace" (line 80) — `notifyCommand = run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`; §"Fix Direction → Concrete shape" (line 158) — parser output verified live as `pane-focus-out[0] run-shell "…"`.
**Category**: Enhancement to existing topic
**Affects**: "Registration Redesign — Ensure Exactly One" (the per-event parameter table) / "Concrete mechanism"

**Details**:
The spec's central new logic is the eviction-fingerprint table, which matches Portal-authored entries by substring (`portal state notify`, `portal state commit-now`, `portal state signal-hydrate`) against each entry's command body. But the spec never records what those command bodies actually look like. The investigation pins it: the desired body is not the bare subcommand — it is a `run-shell`-wrapped, guarded form: `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`. The eviction fingerprint `portal state notify` is therefore a substring *inside* that wrapper, and `ParseShowHooks` returns the entry as `pane-focus-out[0] run-shell "…"`.

This matters because the substring-match eviction predicate (and the "idempotent fast path: body already equals the desired body" check) operates on the full wrapped string, not the bare subcommand. An implementer reading only the spec could reasonably assume the desired body / fingerprint is the bare `portal state notify` and get the equality check (fast path) wrong. Recording the concrete `run-shell "command -v portal … && <subcmd>"` shape removes that ambiguity and is the exact form the live parser output (`run-shell "…"`) confirmed.

**Current**:
> - **New tmux client seam:** `ShowGlobalHooksForEvent(event)` → runs `show-hooks -g <event>`. Output format is byte-identical to the global form (`pane-focus-out[0] run-shell "…"`), so the existing `ParseShowHooks` parser needs **zero changes**.

**Proposed Addition**:
Added a "**Hook body shapes**" note after the per-event parameters table in "Registration Redesign", recording the `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"` wrapper shape, the guard rationale, the `ParseShowHooks` rendering, and that fingerprints/fast-path equality operate on the full wrapped body.

**Resolution**: Approved
**Notes**: Auto-approved (user selected `a`).

---

### 2. `state notify` is not self-amplifying (zero tmux calls, skips bootstrap)

**Source**: investigation §"Code trace" (line 87) — "`state notify` itself only touches the `save.requested` marker file and does **zero** tmux calls (`cmd/state_notify.go`), and `state` is in `skipTmuxCheck` (`cmd/root.go`) so notify does **not** run bootstrap — the cascade is therefore **not self-amplifying** through notify."
**Category**: Enhancement to existing topic
**Affects**: "Problem Statement" / "Root Cause" (the growth-vs-fire distinction)

**Details**:
The spec correctly records the load-bearing distinction "the stack only ever grows (every bootstrap +1 per blind event); switching fires but never grows it." But it omits the supporting mechanism the investigation established: the reason firing the cascade can never grow the stack is that each spawned `state notify` does zero tmux calls and is in `skipTmuxCheck`, so it never runs bootstrap step 2 — there is no feedback loop. This bounds the failure mode (it is linearly driven by `portal open` count, not self-reinforcing) and is the reason the fix needs no anti-amplification safeguard inside `state notify` itself — the spec's "no change to what the hooks do" non-goal implicitly relies on this but never states it. A one-line note closes the gap.

**Current**:
> - The stack only ever **grows** (every bootstrap +1 per blind event); switching fires but never grows it.

**Proposed Addition**:
Extended the "stack only ever grows" bullet in "Problem Statement" with the not-self-amplifying mechanism: each `state notify` does zero tmux calls, `state` is in `skipTmuxCheck` so notify never runs bootstrap; growth is linear in open/attach count, never self-reinforcing; fix needs no anti-amplification safeguard inside `state notify`.

**Resolution**: Approved
**Notes**: Auto-approved (user selected `a`).

---
