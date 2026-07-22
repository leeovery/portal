---
status: in-progress
created: 2026-07-22
cycle: 2
phase: Input Review
topic: Persistent No Host Terminal Banner
---

# Review Tracking: Persistent No Host Terminal Banner - Input Review

## Findings

### 1. Named block-flash copy constraint — "add the multi-select-unavailable intent, do not repeat the banner"

**Source**: Investigation — "Fix Direction" § "Chosen Approach" point 4, "Copy-shape constraint (from fix validation)" (lines 177): *"The banner already says 'unsupported terminal'; the flash must ADD the 'multi-select unavailable' intent, not repeat the banner."* Related: the spec-fork note (line 227) had suggested the named blocked-entry copy would *"likely nam[e] the identity + a `terminals.json`/docs pointer for the named case"* — a suggestion the spec resolved *against* (the named block flash carries neither), and the missing constraint is exactly the rationale for that resolution.

**Category**: Enhancement to existing topic
**Affects**: §5 (Unsupported-Terminal Copy — "Notes & decisions", the Blocked-entry flash bullet)

**Details**:
The spec captures the *mechanism* of the named two-row co-render (§5 line 129 and §7 line 177: banner on the header row, block flash on the notice-band row) but drops the validation-derived *copy constraint* that governs what the named block flash may say: it must **add** the "multi-select unavailable" intent and must **not repeat** the banner's already-shown "unsupported terminal" text or its identity/`see docs` content.

This constraint is load-bearing and explains a non-obvious copy choice. The investigation's spec-fork (line 227) proposed that the named blocked-entry copy would name the identity and carry a `terminals.json`/docs pointer. The spec resolved the opposite — the named block flash is the bare `multi-select isn't available on this terminal` (no bundle id, no `see docs`) — precisely *because* the co-rendered persistent banner already carries the bundle id and `see docs`, so the flash need only add the multi-select-unavailable intent without duplicating the banner. Without recording this constraint, a future copy edit could "helpfully" re-add the identity/docs pointer to the named block flash (matching the discarded spec-fork suggestion), producing a redundant two-row state where both rows repeat "unsupported terminal … <bundleID>". The spec currently gives the final string but not the rule that keeps the two co-rendered rows non-redundant.

(Note the NULL counterpart of this fork *is* already reasoned in the spec: §6 explains dropping the "no host-local terminal" jargon and §5's "Accepted minor imprecision" note covers the "over a remote connection" wording — so only the named-shape non-repetition constraint is unrecorded.)

**Current**:
(from §5, "Notes & decisions", the Blocked-entry flash behaviour bullet)
> - **Blocked-entry flash behaviour** (settled): distinct from the reactive no-op (a pre-emptive block attempts nothing, so no `— nothing opened`); uses the existing §11 notice-band flash slot and self-clears on the **next actionable key** (the authoritative trigger — matching the existing `setFlash` / `isActionableKey` lifecycle; "next keypress" elsewhere is shorthand for this); on a named terminal it co-renders two-row with the persistent banner (banner on the header row, flash on the notice-band row); repeated `m` while the flash shows clears then re-blocks + re-flashes (intentional). Reusing the §11 flash slot also inherits its existing auto-clear timer — that is expected and not forbidden; the "self-clears on the next actionable key" acceptance wording is the *key-driven* clear path, not a prohibition on the timer.

**Proposed Addition**:
{leave blank until discussed — likely a clause appended to the named co-render note in §5 stating that, in the named two-row state, the block flash carries only the "multi-select isn't available" intent and must not repeat the banner's "unsupported terminal"/bundle-id/`see docs` content, since the co-rendered banner already supplies the identity and remedy}

**Resolution**: Pending
**Notes**:

---
