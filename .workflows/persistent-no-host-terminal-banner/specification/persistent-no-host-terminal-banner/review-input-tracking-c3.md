---
status: complete
created: 2026-07-22
cycle: 3
phase: Input Review
topic: Persistent No Host Terminal Banner
---

# Review Tracking: Persistent No Host Terminal Banner - Input Review

## Findings

### 1. Fork B / B2 rejection rationale — why `m` on NULL flashes rather than silently no-ops

**Source**: Investigation — "Fix Direction" § "Chosen Approach", **Fork B → B1** (line 181): *"Pressing `m` on NULL flashes an explanatory line … so a silent `m` doesn't read as broken; it self-clears on the next key."* And "Options Explored" (line 188): *"**Fork B2 (silent no-op for NULL).** Quieter, but a silent `m` reads as broken. Not chosen."*

**Category**: Enhancement to existing topic
**Affects**: §3 (Proactive Multi-Select Entry Block) — and/or §5 (Unsupported-Terminal Copy)

**Details**:
The investigation surfaced two behavioural forks and resolved both with recorded rationale. The spec captures **Fork A** and its rejected alternative in full — §3 states "A2 (eject on resolve) was explored and rejected for that reason." But the spec never records **Fork B**: the choice between a visible flash and a *silent* no-op when `m` is pressed on a NULL/remote terminal. The spec resolves Fork B implicitly (a flash fires — §6: "Pressing `m` does nothing but show the transient flash …") but drops the reason the flash exists at all: **Fork B2 (silent no-op) was explored and rejected because a silent `m` reads as broken.**

This is the sole justification for the pre-emptive entry block surfacing *any* visible feedback — especially on NULL/remote, where there is no `terminals.json` remedy, so a reader could reasonably ask "why not just silently swallow the keypress?" The investigation answers that; the spec does not. Recording it also gives the spec parity with its own treatment of A2 (a rejected fork kept because it clarifies a non-obvious behaviour choice) and protects the flash from being "simplified away" to a silent no-op by a later editor who sees no stated reason for it.

Note the *adjacent* NULL reasoning **is** already in the spec — §5's "Accepted minor imprecision" (the "over a remote connection" wording) and §6's "explains rather than guides" end-state cover the flash's *copy*; §5's "NULL/remote has no remedy, so it gets no pointer" covers *why it doesn't guide*. What's missing is one level up: why there is a flash rather than silence.

**Current**:
(from §3, "### Change")
> The fix adds a proactive check: when `DetectUnsupported()` is true, pressing `m` does **not** open the mode — it sets a transient blocked-entry flash instead (copy defined in Topic 5) and returns.

**Proposed Addition**:
New §3 subsection "### Visible flash, not a silent no-op (Fork B → B1)": records that Fork B resolved to B1 (visible flash) and B2 (silent no-op) was rejected because a silent `m` reads as broken; notes the flash must fire even on NULL/remote (no remedy) so the keypress isn't silently swallowed; warns against "simplifying" to silence — parity with §3's A2 treatment.

**Resolution**: Approved
**Notes**: Auto-applied. Source-derived rationale (not a new decision). Logged to §3.

---
