---
status: in-progress
created: 2026-07-23
cycle: 1
phase: Gap Analysis
topic: remote-trigger-spawns-on-local-terminal
---

# Review Tracking: remote-trigger-spawns-on-local-terminal - Gap Analysis

## Findings

### 1. Outcome contract omits the `ListClients` enumeration-failure case

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: "Behavioural outcomes by scenario" table; "Edge Contracts to Pin"; "Testing Requirements → Existing invariants that must stay green"

**Details**:
The spec presents its outcome set as a complete contract — a scenario table (7 rows), an "Edge Contracts to Pin" list, and an enumerated "existing invariants that must stay green" list. All three omit one distinct outcome the code produces today and must continue to produce: `lister.ListClients(session)` returning an error → `(NULL, ErrDetectTransient-wrapped error)`. This is a separate branch from the four post-selection outcomes (local drive / clean NULL / transient winner walk / empty list) and is guarded by an existing subtest, "it returns a transient error when list-clients fails" (`detect_inside_test.go:151`), which the spec never references.

The change (compute the winner in Go from the already-fetched slice, then walk only the winner) does not touch this branch, so it stays green — but a contract that claims to pin all edges leaves a real outcome unstated. An implementer rewriting the function purely from the spec's enumerated outcomes could plausibly reshape the early-return structure without realising the list-clients-error path (and its test) is load-bearing.

Note the adjacent single-client-walk-failure subtest (`:171`) is, by contrast, covered — it collapses into the "winner's walk transient-fails → NULL + WARN" row — so only the enumeration-failure outcome is genuinely missing from the contract.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 2. "New tests to add" overlaps the invert/reframe transforms (same scenarios listed twice)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: "Testing Requirements" (the two transformed tests + the three new tests)

**Details**:
Two of the three "New tests to add" describe the identical scenario as one of the "invert/reframe" transforms, without the spec noting the overlap:

- **New test "Remote most-active, local idle → NULL"** is the same scenario as the inverted `:133` subtest ("Invert the codified-bug test": high-activity remote + low-activity local, expectation flipped from local-wins to NULL). Both seed remote-most-active + local-idle and assert NULL.
- **New test "Fail-safe on transient winner walk (most-active client's walk transient-fails + a lower-activity resolvable local present → NULL + transient)"** is the same scenario as the reframed `:196` subtest ("Reframe the resilience test": high-activity walk-fail client + lower-activity resolvable local, expectation reframed to NULL + `ErrDetectTransient`). Both seed a transient-failing winner over a resolvable lower-activity local and assert NULL + transient.

Only the third new test — "Local most-active, remote idle bystander → local drives" — is genuinely novel (no existing subtest covers it).

Because the spec lists these as two separate obligations (transform an existing test AND add a new one) without stating they land on the same scenario, an implementer planning the suite is left to guess whether the reframed test *satisfies* the new-test requirement or whether a second, near-duplicate test is expected. Impact is low (worst case a redundant test), but it is a real planning-readiness ambiguity: the "New tests to add" list overstates how much net-new coverage is required.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---
