---
status: in-progress
created: 2026-05-13
cycle: 2
phase: Input Review
topic: distinguish-transport-errors-in-getserveroption
---

# Review Tracking: distinguish-transport-errors-in-getserveroption - Input Review

## Findings

### 1. Data-integrity consequence of the permissive-on-error flip

**Source**: investigation §Analysis → Code Trace, paragraph ending "...commit potentially-corrupt state." (line ~114)
**Category**: Enhancement to existing topic
**Affects**: Problem & Goal → Problem section

**Details**:
The investigation spells out the concrete consequence of the conservative-→-permissive flip: the daemon's two read sites "proceed as if restoration is NOT in progress and commit potentially-corrupt state." The current spec captures the mechanism ("silently flips them from conservative-on-error to permissive-on-error in the presence of any transient tmux failure during the restoration window") but stops short of naming the data-integrity outcome — that the per-tick capture / shutdown-flush would commit state derived from a half-restored skeleton. Naming the consequence sharpens why the bug matters even though it is latent.

**Current**:
> The bug is latent — no user-visible incident has been reported. The two production consumers (`cmd/state_daemon.go` `tick()` at L95-99 and `defaultShutdownFlush()` at L187-201) read `@portal-restoring` defensively and already want conservative-on-error behaviour ("skip the tick / skip the flush"). The conflation silently flips them from conservative-on-error to permissive-on-error in the presence of any transient tmux failure during the restoration window.

**Proposed Addition**:
Append a sentence to the existing paragraph naming the data-integrity consequence: under the flip, both consumers proceed as if restoration is not in progress and would commit (or flush) state derived from a half-restored skeleton.

**Resolution**: Pending
**Notes**:

---

### 2. Latency reasoning: tmux runs against a local socket

**Source**: investigation §Analysis → "Why It Wasn't Caught" (third bullet, line ~156)
**Category**: Enhancement to existing topic
**Affects**: Problem & Goal → Problem section (latency framing) or Risk & Rollout

**Details**:
The investigation explains *why* the bug has stayed latent: "Production has not surfaced the bug because tmux runs against a local socket; transient transport failures are vanishingly rare. The bug is structural, not observed." The current spec asserts latency without explaining the underlying reason. Including the local-socket reasoning gives a future reader the rationale for why regression risk is low and why no incident has surfaced — material that anchors both the Problem framing and the Risk & Rollout "Regression risk: Low" claim.

**Current**:
> The bug is latent — no user-visible incident has been reported.

**Proposed Addition**:
Tighten the latency statement to include the local-socket causal reasoning (one short clause). Could equivalently appear under Risk & Rollout's regression-risk bullet — either site is reasonable.

**Resolution**: Pending
**Notes**:

---

### 3. Contributing factor: "default to ErrOptionNotFound" pattern was tempting because not-found is by far the most common case

**Source**: investigation §Analysis → "Contributing Factors" (second bullet, line ~149)
**Category**: Enhancement to existing topic
**Affects**: Problem & Goal → Problem section, or a "Why it happened" subsection of Design

**Details**:
The investigation names a contributing factor: the conflation was tempting because "the legitimate 'not found' case is by far the most common — every other use of `GetServerOption` was for an existence check that happily treats failure as absence." This explains the historical introduction of the bug and reinforces the design choice (typed `CommandError`) by showing why a simpler default-to-sentinel pattern was attractive at the time. The spec currently records the "first wrapper added with a contract the underlying primitive could not deliver" angle (via the Documentation section's framing) but does not mention the existence-check usage pattern that made the original shape feel safe.

**Proposed Addition**:
Add a short note — either to the Problem section or as a closing line in the Design "Why this layer" subsection — that the original conflation was tempting because every prior caller of `GetServerOption` was an existence check that happily mapped failure to absence; the contract drift surfaced only when the first wrapper (`TryGetServerOption`) asserted distinguishability.

**Resolution**: Pending
**Notes**:

---
