TASK: Resolve Component F Spec/Impl Mismatch — _portal-saver Session Persists After Daemon Exits (T11-3)

ACCEPTANCE CRITERIA:
- Spec Component F bullet 3 matches implementation assertion shape (log-noise-absence with tmux 3.6b rationale).
- No outstanding 'spec/impl mismatch' note remains.
- Add a paragraph note about `remain-on-exit on` as future opt-in.

STATUS: Complete

SPEC CONTEXT:
Component F closes the race between `new-session` and `set-option destroy-unattached=off` via placeholder-then-respawn ordering. Pre-amendment, bullet 3 asserted literal "_portal-saver session persists after daemon exits" — unachievable on tmux 3.6b without `remain-on-exit on`. The implementation (task 3-5) instead asserts log-noise-absence, the actual contract Component F provides on the supported tmux version.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md:387-391` (amended bullet 3)
  - `…/specification.md:394` (new Note on `remain-on-exit on` as future opt-in)
  - `…/specification.md:365` (design-rationale step 3 prose reconciled by T12-1)
- Notes: Bullet 3 now reads "Lock-loser cascade is quiet — no `no such session` log noise" with a labeled "Rationale for log-noise-absence over literal session-persistence" sub-paragraph documenting tmux 3.6b behaviour explicitly. The Note (line 394) defers `remain-on-exit on` with rationale (recovery cascade converges, restore-semantics interactions). T12-1 reconciliation at line 365 cross-references both bullet 3 and the Note.

TESTS:
- Status: Adequate (spec-only change; verification is in companion test)
- Coverage: Amended assertion shape exercised by `TestBootstrapPortalSaver_LockLoser_NoNoSuchSessionLogNoise` at `internal/tmux/portal_saver_endstate_integration_test.go:279`, which scrapes portal.log for zero "no such session: _portal-saver" entries — directly mirrors bullet 3's verification clause.
- Notes: Test file preamble (lines 4-32) documents the same re-framing rationale present in the spec amendment. Implementation and spec are narratively aligned.

CODE QUALITY:
- Project conventions: N/A (spec-only amendment)
- SOLID / Complexity / Modern idioms: N/A
- Readability: Good — bullet 3 has clear assertion sentence + labeled Rationale; Note paragraph properly scoped as "future opt-in"; cross-references between line 365 / 391 / 394 are explicit and bidirectional.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Composite end-to-end verification step 9 (`…/specification.md:464`) asserts "`_portal-saver`'s pane process is `portal state daemon`" without acknowledging that in the lock-loser cascade the session disappears on tmux 3.6b. The composite scenario's pre-conditions implicitly scope step 9 to the lock-winner case, but a one-line footnote would close the last subtle gap between bullet 3's amendment and the composite verification.
