---
status: in-progress
created: 2026-07-02
cycle: 1
phase: Input Review
topic: Skip Bootstrap When Warm
---

# Review Tracking: Skip Bootstrap When Warm - Input Review

## Findings

### 1. The known ~1s resurrection-race edge is dropped from the "lone warm bootstrap is already safe" claim

**Source**: discussion.md, "What 'skip' means" → Grounding — current warm-path reality (line 74)
**Category**: Enhancement to existing topic
**Affects**: Overview & Goals → "Explicit non-goal — single-command safety" (spec line 17); potentially also "Motivation" framing

**Details**:
The spec asserts flatly that "A lone warm bootstrap is *already* safe today" (line 17) and that the feature "is **not** about correctness of one warm bootstrap." The discussion arrives at that same conclusion but explicitly qualifies it: the "actively unsafe" framing was corrected, and *the only latent edge is a ~1s resurrection race if a session is killed outside the picker and `x` is run before the daemon's next tick captures the kill; pre-existing, rare, not this feature's concern.* This is a real, named boundary condition on the "already safe" claim. The spec presents the conclusion without the single acknowledged caveat, so a reader could over-read "already safe" as unconditional. Since this feature changes *when* warm bootstraps run, noting the one pre-existing edge (and that it is explicitly out of scope) closes the loop that the discussion deliberately left closed.

**Current**:
- **Explicit non-goal — single-command safety.** A lone warm bootstrap is *already* safe today: `Restore` skips already-live sessions (`internal/restore/restore.go`), so on a warm server it is a near-no-op. The feature is **not** about correctness of one warm bootstrap; it is about concurrency and redundancy.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 2. The confirming trace fact — the daemon today does NOT clean markers/FIFOs/hooks (only `gcOrphanScrollback`) — is missing from the hooks-cleanup section

**Source**: discussion.md, "Cleanup steps over a long-lived (weeks) server" → Context (line 143)
**Category**: Enhancement to existing topic
**Affects**: Daemon-Owned Hooks Cleanup → intro paragraph / "The trace" (spec lines 200-208)

**Details**:
The spec's "Daemon-Owned Hooks Cleanup" opens by stating the weeks-long-server worry and then traces each cleanup target to its producer. But it omits the load-bearing *confirming fact* the discussion established first: the daemon does **not** currently clean any of these — "the daemon's only GC is `gcOrphanScrollback`, scrollback `.bin` files, inside `Commit`; markers/FIFOs/hooks cleanup live only in bootstrap + `portal clean`." This fact is what makes the whole re-homing decision non-trivial (the daemon has to gain a genuinely new responsibility, not just keep doing something it already did) and grounds why cleanup would otherwise pile up for weeks if merely skipped. The spec's edge-case note references `gcOrphanScrollback` in passing (line 181, EnsureSaver context — actually the daemon section) but never states the pre-condition that the daemon has zero marker/FIFO/hooks GC today.

**Current**:
The weeks-long-server constraint raised a worry: cleanup steps (marker sweep, FIFO sweep, hooks `CleanStale`) are framed as once-per-lifetime, but if cruft *accrues* during a weeks-long lifetime, skipping them on abridged commands would let it pile up for weeks. Tracing each cleanup target to its producer resolves this.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 3. The `bootstrap-cleanstale-wipes-hooks-on-tmux-transient` bug interaction — a concrete benefit of removing step 11 from bootstrap — is not captured

**Source**: discussion.md, "Cleanup steps over a long-lived (weeks) server" → Options Considered (lines 156, 158)
**Category**: Enhancement to existing topic
**Affects**: Daemon-Owned Hooks Cleanup → "Decision — remove hooks cleanup from the orchestrator" and/or Operational contract (spec lines 214-226)

**Details**:
The discussion weighs the hooks-cleanup decision partly on a **named existing bug**: `bootstrap-cleanstale-wipes-hooks-on-tmux-transient`, which "only triggers inside a bootstrap when `list-panes -a` returns transiently-empty." Two concrete points fall out: (a) *skipping* step 11 on warm commands "reduces exposure" to this bug; (b) *keeping* it "re-introduces the hooks-wipe bug surface on every warm command." The spec preserves the mass-deletion hazard guard (`len(livePanes)==0 && hooks present` → skip) at line 226 but never names this specific bug nor states that removing the step from the per-command bootstrap path is itself a mitigation of a real, known failure mode. This is a substantive rationale/benefit of the removal decision, not just a stylistic detail — it strengthens the "remove from orchestrator entirely" argument beyond "it's inert anyway."

**Current**:
Rationale for full removal (not just skipping on abridged): a bootstrap-time cleanup would only *uniquely* help when a full bootstrap runs **and** EnsureSaver fails to start the daemon — a scenario already catastrophic (no daemon ⇒ no scrollback capture), where an inert stale-hook entry is noise. What it cleans is inert anyway. At cold boot the freshly-started daemon cleans on its first eligible tick (~10s) rather than during bootstrap — fine, since it's inert. (Trade-off acknowledged: slightly more surgery than leaving a harmless idempotent double-clean in place; the clean single-home model won.)

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 4. The one boundary condition on "a stale hook cannot misfire" — a user hand-recreating a session under an old nanoid name — is dropped

**Source**: discussion.md, "Can a stale hook *misfire*?" → Conclusion (line 171)
**Category**: Enhancement to existing topic
**Affects**: Daemon-Owned Hooks Cleanup → "A stale hook entry cannot misfire" (spec line 212)

**Details**:
The spec states the misfire conclusion absolutely: "a genuinely-stale hook entry cannot fire on the wrong session." The discussion reaches the same conclusion but explicitly carries the single caveat that bounds it: "(Confidence: high, **modulo a user manually recreating a session under an old nanoid name by hand — not a realistic path**.)" This is the only edge under which the "cannot misfire" invariant could theoretically break, and the discussion deliberately named and dismissed it. Since the entire "remove hooks cleanup from bootstrap" decision leans on the no-misfire guarantee, the acknowledged (if unrealistic) boundary belongs in the spec so the guarantee isn't read as stronger than the discussion concluded.

**Current**:
Within-session index reuse keeps the key **live** (never classed as stale). **Conclusion: a genuinely-stale hook entry cannot fire on the wrong session — the only cost of leaving it is inert JSON bloat.**

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---
