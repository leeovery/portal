---
status: in-progress
created: 2026-07-21
cycle: 3
phase: Gap Analysis
topic: spawned-window-dead-ends-on-session-exit
---

# Review Tracking: spawned-window-dead-ends-on-session-exit - Gap Analysis

## Findings

### 1. Unit-test requirement doesn't explicitly exercise the highest-risk quote-nesting case (mint passthrough / special characters)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Testing Requirements → Unit coverage (automated); Acceptance Criteria 7

**Details**:
The spec repeatedly names the mint-surface `-- <command…>` passthrough as the argv element that "must survive the same quote nesting as any other argv element" (Where the change lives; Constraints; AC3), and the quoting constraint explicitly warns that "Naive concatenation of the illustrative form would emit a corrupted command that fails at Ghostty launch — the exact failure class this fix removes."

However, the unit-coverage bullets and AC7 phrase the nesting assertion generically ("Assert quoting nests correctly — the embedded argv is not corrupted by the added `bash -lc '…'` layer" / "round-trips uncorrupted through the added `bash -lc` layer") without requiring a test fixture that actually contains quote-sensitive characters — a single quote, `;`, `$`, `"`. That is precisely the input where the double single-quote nesting (`'\''`) can break.

An attach-surface argv (session name, batch:token, PATH, exePath) typically carries no shell-special characters, so an implementer could satisfy the generic assertion with an attach-only fixture and still leave the actual double-escaping path — a passthrough command like `-- sh -c 'echo hi'` — unexercised. Making a quote-sensitive fixture (ideally the mint passthrough) an explicit test-coverage requirement closes the exact failure class the fix's own quoting constraint identifies, and directly backs the "round-trips uncorrupted" clause of AC7.

This is a strengthening of an already-present coverage bullet rather than a wholly missing requirement, hence Minor.

**Proposed Addition**:
{Leave blank until discussed.}

**Resolution**: Pending
**Notes**:

---
