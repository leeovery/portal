---
status: in-progress
created: 2026-07-21
cycle: 2
phase: Input Review
topic: Spawned Window Dead-Ends On Session Exit
---

# Review Tracking: Spawned Window Dead-Ends On Session Exit - Input Review

## Findings

### 1. Custom-terminal residual dead-end is a known accepted consequence, not stated as one

**Source**: investigation §Blast Radius — "Potentially affected (verify during fix)" (lines 278-282); §Fix Direction "Options Explored" (custom-terminal scoping)
**Category**: Enhancement to existing topic
**Affects**: Scope & Non-Goals ("No change for custom `terminals.json` terminals")

**Details**:
The investigation explicitly flags config-`terminals.json` adapters as *potentially affected*: they share `composeOpenArgv`/`renderCommandString`, and "Whether they dead-end depends on the user's terminal's post-command behaviour. A fix placed in composition (or in portal itself) would cover them; a Ghostty-adapter-only fix would not." The scope decision was made (Ghostty-adapter-only), which the spec captures — but the spec frames custom-terminal end-behaviour purely as the user's *deliberate* choice ("including a deliberate close-on-exit if they chose it").

That framing under-represents the source: per the investigation, a custom terminal whose recipe/terminal exhibits wait-after-command-style behaviour can dead-end the *same way the Ghostty adapter did pre-fix* — i.e. unintentionally, not by the user's design — and Portal is deliberately choosing not to cover that case. Naming this residual as a known, accepted consequence of the Ghostty-only scope (rather than implying all custom-terminal end-behaviour is intentional) makes the scope boundary honest and gives planning/implementation the same picture the investigation had at the scope-decision point. It also reinforces *why* a shared-composition fix was rejected (it would have covered these terminals but broke the `{command}` contract).

**Current**:
- **No change for custom `terminals.json` terminals.** How a custom terminal's window ends is the user's own command/recipe's business. Custom-terminal users keep full control, including a deliberate close-on-exit if they chose it. Portal does not impose a shell fallback on them.

**Proposed Addition**:
_(leave blank until discussed)_

**Resolution**: Pending
**Notes**:

---
