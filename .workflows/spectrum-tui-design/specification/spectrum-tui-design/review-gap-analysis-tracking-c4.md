---
status: in-progress
created: 2026-06-18
cycle: 4
phase: Gap Analysis
topic: spectrum-tui-design
---

# Review Tracking: Spectrum TUI Design - Gap Analysis

> **Cycle-4 scope: follow-through verification of the cycle-3 edits.** Verified clean:
> (a) §8.1 help-modal exception clause and §8.5 `esc close` are now mutually consistent and cross-referenced — both state the exception, both use `esc close` with no "to", and §8.1's `esc <verb>` enumeration covers help's `esc close`. The c3-#1 contradiction is fully resolved with no new contradiction introduced.
> (b) §8.3/§8.6 kill/delete reworded to "Confirm logic preserved; rendering + keymap changed … drops `n`"; the modal bodies, "Keys:" lines, §12.1 modal keymaps, and footers all agree on `y`/`Esc`. No stray `n`, no "parity" leftover claiming an unchanged flow. The c3-#2 contradiction is fully resolved.
> (c) §15.5 (new Paper-reference comparison) is consistent with §15.1 (frame map), §15.2 (no pixel-diff gate), §15.4 (distinct artifact paths: vhs capture at `testdata/vhs/<name>.png` vs reference export at `testdata/vhs/reference/<frame>.png` — no collision), and the verification-mandate callout (§15.1/§15.5 cross-refs accurate).
> (d) The one "esc *to*" hit (§7.3 `esc to clear the filter`) is a centred empty-state prose hint, not a modal dismiss key, so the §8.1 modal-wording rule does not govern it; it is pre-existing (untouched by cycle 3) and out of this cycle's scope.
>
> One genuinely NEW, Minor follow-through drift introduced by the §15.5 addition is recorded below.

## Findings

### 1. §15.4 step 3 ("human opens the latest screenshot") not updated when §15.5 added the committed Paper reference ("the human gate opens both")

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §15.4 (Verification responsibilities in the task loop — step 3, Human gate), §15.5 (Comparing the capture against the Paper reference)

**Priority**: Minor

**Details**:
The cycle-3 addition of §15.5 introduced a second, committed comparison artifact — the **Paper reference export** (`testdata/vhs/reference/<frame>.png`) — to sit beside the task's `vhs` capture, and explicitly states (line 577): "Both the implementer (self-check) and the reviewer (gate) place the task's `vhs` capture **beside the committed Paper reference** … **the human gate opens both**."

§15.4 step 3 (line 570 — the Human-gate bullet) predates §15.5 and was not updated to match. It still reads: "the human opens the task's **latest screenshot** (and inspects the live TUI) before approving" — singular, naming only the `vhs` capture, with no mention of the committed Paper reference that §15.5 now says the human opens alongside it.

This is a minor follow-through drift, not a contradiction that blocks anything: §15.5 is the more specific, authoritative statement and a planner/implementer would follow "opens both." But within §15.4 itself, the Human-gate step now under-describes the human's comparison (it reads as a one-image glance at the capture, when §15.5 establishes it is a two-image side-by-side against the committed reference). Tightening step 3 to name both images keeps §15.4 and §15.5 in lockstep and removes the "is the human comparing against the frame or just eyeballing the capture?" ambiguity for whoever builds the task loop.

Note for context (no action needed): §15.4 steps 1 and 2 say "comparing it to the named Paper frame" / "matches the frame" — these read fine because §15.5 cross-references §15.4 and is clearly the detail layer defining *how* the frame comparison is materialised (against the committed export). Only step 3's "latest screenshot" (singular, no reference) is the actual drift.

**Current**:
§15.4 step 3: "3. **Human gate** — the human opens the task's **latest screenshot** (and inspects the live TUI) before approving."

§15.5 (line 577): "Both the **implementer** (self-check, §15.4) and the **reviewer** (gate, §15.4) place the task's **`vhs` capture beside the committed Paper reference** and judge **layout / structure / colour-role match**; the human gate opens both."

**Proposed Addition**:
*(leave blank until discussed)* — reconcile §15.4 step 3 with §15.5: update the Human-gate bullet to say the human opens **both** the task's latest `vhs` capture **and** its committed Paper reference (§15.5) side-by-side before approving (and inspects the live TUI). Keeps §15.4's three-step loop consistent with §15.5's "opens both."

**Resolution**: Pending
**Notes**:

---
