# Review Report: built-in-session-resurrection-12-9

**TASK**: Reconcile plan body wording for task 5-2 `Restoring.Clear` failure classification

**ACCEPTANCE CRITERIA**:
- Plan body for task 5-2 classifies `Restoring.Clear` failure as fatal (not soft/WARN)
- Cite spec § Fatal Bootstrap Errors
- Cite CLAUDE.md bootstrap step 6
- Add cycle-1 review-resolution note

**STATUS**: Complete

**SPEC CONTEXT**:
Spec § Fatal Bootstrap Errors at `specification.md:1395`: "**`@portal-restoring` clear (unset) fails at step 6**: same as set-option failure. The marker MUST NOT leak past bootstrap — a stuck `@portal-restoring=1` would suppress every subsequent daemon tick across this server's lifetime, silently breaking persistence."

CLAUDE.md bootstrap step 6 confirms: "Clear `@portal-restoring` — fatal on failure (the marker must not leak past bootstrap)."

Implementation at `cmd/bootstrap/bootstrap.go:225-229` matches: step 6 returns `o.fatalf("clear @portal-restoring marker", err)` producing a `*FatalError`.

**IMPLEMENTATION**:
- Status: Implemented (documentation-only change applied)
- Location: `/Users/leeovery/Code/portal/.workflows/built-in-session-resurrection/planning/built-in-session-resurrection/phase-5-tasks.md`
  - Do block, Step 6 (line 110): "fatal — wrap as `*FatalError(...)`, log ERROR via `ComponentBootstrap`, and return... Per spec § Fatal Bootstrap Errors and `CLAUDE.md` bootstrap step 6 ... _Cycle-1 review reconciliation: an earlier draft of this body characterised the failure as soft + WARN; spec, CLAUDE.md, and the `cmd/bootstrap/bootstrap.go` step-6 implementation all classify it as fatal — wording corrected here._"
  - Acceptance Criteria (line 141): fatal classification with spec citation.
  - Edge Cases (line 169): fatal classification with cycle-1 reconciliation note.
- All three required content elements present — fatal classification, both citations, explicit cycle-1 reconciliation note (in two locations).

**TESTS**:
- Status: N/A (documentation-only change)
- Implementation-side regression guard already exists in task 5-2's test `"it returns a FatalError when Restoring.Clear fails"` in `cmd/bootstrap/bootstrap_test.go`.

**CODE QUALITY**:
- Readability: Good — reconciliation notes marked with italics; citations name authoritative sources.
- Issues: None.

**BLOCKING ISSUES**:
- None

**NON-BLOCKING NOTES**:
- [idea] Two reconciliation notes (Do step 6 line 110 and Edge Cases line 169) with slightly different wording. Co-locating with the technical statement they correct is reader-friendly; consolidating to a single canonical note is a minor tidy-pass option.
