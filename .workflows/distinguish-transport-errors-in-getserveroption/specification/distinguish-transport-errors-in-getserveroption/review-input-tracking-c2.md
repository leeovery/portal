---
status: complete
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
Naming the concrete data-integrity outcome of the flip: both consumers proceed as if restoration is not in progress and would commit/flush state derived from a half-restored skeleton.

**Proposed Addition**:
Appended the consequence sentence to the existing "permissive-on-error" paragraph.

**Resolution**: Approved
**Notes**: Folded into same edit as Finding 2.

---

### 2. Latency reasoning: tmux runs against a local socket

**Source**: investigation §Analysis → "Why It Wasn't Caught" (third bullet, line ~156)
**Category**: Enhancement to existing topic
**Affects**: Problem & Goal → Problem section (latency framing) or Risk & Rollout

**Details**:
The local-socket reason transient failures are vanishingly rare was implicit.

**Proposed Addition**:
Inlined into the latency statement: "no user-visible incident has been reported, because tmux runs against a local Unix-domain socket where transient transport failures are vanishingly rare. The bug is structural, not observed."

**Resolution**: Approved
**Notes**: Folded into same edit as Finding 1.

---

### 3. Contributing factor: "default to ErrOptionNotFound" pattern was tempting because not-found is by far the most common case

**Source**: investigation §Analysis → "Contributing Factors" (second bullet, line ~149)
**Category**: Enhancement to existing topic
**Affects**: Problem & Goal → Problem section, or a "Why it happened" subsection of Design

**Details**:
Recorded the historical reason the original conflation felt safe.

**Proposed Addition**:
Added a paragraph after "Why this layer" naming the existence-check usage pattern that made the original shape feel safe, and noting that the fix preserves common-case ergonomics.

**Resolution**: Approved
**Notes**:

---
