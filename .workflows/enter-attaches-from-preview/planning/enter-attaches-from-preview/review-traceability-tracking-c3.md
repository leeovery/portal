---
status: in-progress
created: 2026-05-15
cycle: 3
phase: Traceability Review
topic: Enter Attaches From Preview
---

# Review Tracking: Enter Attaches From Preview - Traceability

## Cycle 1 + Cycle 2 Fix Verification

**Cycle 1 fix** (task 1-6 unconditional viewport-content-state):

- Acceptance criterion at `phase-1-tasks.md` line 336 — exact wording from cycle 1's Proposed block.
- Three viewport-state dispatch tests at lines 345-347 — exact wording from cycle 1's Proposed block.
- Task 1-8 retains independent chrome-wording invariance tests (real bytes / `(nil, nil)` / OS error). The two tasks pin different surfaces (dispatch vs chrome) of the same spec invariant — no duplication.

**Cycle 2 fix** (task 1-4 no-structural-enumeration):

- Acceptance criterion at `phase-1-tasks.md` line 206 — exact wording from cycle 2's Proposed block, listing the four spec-pinned commands and forbidding `list-*` / `display-message`.
- Two enumeration-absence tests at lines 217-218 — exact wording (success path + bail path).

Both fixes are cleanly integrated. No regressions introduced — all prior acceptance criteria and tests remain in place; the new bullets are additive.

---

## Findings

No findings.

A fresh, full re-scan of the specification against the plan in both directions surfaces no untraced plan content and no missing spec coverage.

### Direction 1 (Spec → Plan completeness) — verified

Every spec section maps to one or more tasks with adequate depth:

| Spec section | Plan coverage |
|---|---|
| Overview / additive scope | Phase 1 + Phase 2 split |
| Enter binding behaviour (intercept, no viewport leak) | Task 1-6 |
| What Enter commits to (session captured by name; window/pane walked-then-defaulted) | Task 1-6 (raw-indices ACs + tests; `m.session` dispatch in Do) |
| Transition mechanics (single Update, no intermediate render) | Task 1-4 (pipeline as one tea.Cmd) + Task 1-6 (single Update return) |
| Pre-select step 1 has-session | Tasks 1-3 (discriminator), 1-4 (pipeline branch) |
| Exact-match `=` target syntax (uniform across 4 calls) | Task 1-2 (HasSession, SelectWindow, SelectPane, SwitchClient, AttachConnector) |
| Step 2 select-window (best-effort, WARN, swallow) | Tasks 1-1 (method), 1-4 (WARN-swallow) |
| Step 3 select-pane (best-effort, WARN, swallow) | Tasks 1-2 (target shape), 1-4 (WARN-swallow) |
| Step 4 connector handoff (session-only target) | Tasks 1-2 (-A flag, =prefix), 1-5 (wiring), 1-4 (handoff) |
| No re-enumeration on Enter | Task 1-4 (cycle 2 fix — explicit AC + tests) |
| Captured coordinate values — raw tmux indices | Task 1-6 (AC + non-contiguous test) |
| Hook firing (no change) | Task 1-4 Context (no hook orchestration) |
| Session-killed-externally bail (transition + refresh) | Tasks 1-7 (placeholder), 2-5 (replace with flash) |
| Bail flash exact wording `session "<name>" no longer exists` | Task 2-5 (`formatSessionGoneFlash` helper, string-equality tests) |
| Render-frame ordering (flash not gated on refresh) | Task 2-5 (tea.Batch not Sequence + dedicated test) |
| Inline flash state | Task 2-1 (text + generation) |
| Inline flash render (conditional row, no reservation) | Task 2-2 (push/pop tests) |
| Tick auto-clear (build-chosen ~3s, principle pinned) | Task 2-3 (`flashAutoClearDuration`) |
| Replacement on rapid bails (latest wins, prior tick harmless) | Tasks 2-1, 2-3 (mechanism), 2-6 (integration tests) |
| Flash clear conditions (next actionable KeyMsg, exclude modifier/resize/focus) | Task 2-4 |
| Flash + filter "one key one intent" | Task 2-4 (filter input lands assertion) |
| TOCTOU residual | Spec explicitly not designed for; task 2-5 Edge Cases acknowledges scope boundary |
| Mid-load / placeholder preview content (Enter unconditional) | Task 1-6 (cycle 1 fix — AC + 3 viewport-state tests) |
| Stale row, filter dynamics, in-flight filter | Spec marks as no-op or impossible-by-construction; task 1-6 dispatches via captured `m.session` |
| Discoverability — new chrome line | Task 1-8 (exact format string) |
| Token wording unconditional | Task 1-8 (3 viewport-state chrome tests) |
| Sessions-page help bar unaffected | Task 1-8 (regression assertion) |
| Keymap expansion policy | Policy/architectural; no code task required (preview only adds Enter; other keys not bound) |
| Out of scope items | Correctly excluded from plan |

### Direction 2 (Plan → Spec fidelity) — verified

Every plan task and acceptance criterion traces to a spec section. Spot-checks:

- Task 1-2 `-A` flag addition for `attach-session` → spec § step 4 connector argv `tmux attach-session -A -t '=<session>'`.
- Task 1-3 `HasSessionProbe` shape → spec § step 1 discriminator contract.
- Task 1-4 `previewAttachPipeline` design → spec § Transition mechanics (single logical unit) + four-call ordering.
- Task 1-4 `ComponentPreview` log component → spec § step 2 log shape (build picks; spec suggests `ComponentPreview` or `ComponentTUI`).
- Task 1-7 placeholder bail (transition + refresh, no flash) → phase split rationale (Phase 1 shippable; Phase 2 owns flash).
- Task 2-1 monotonic `flashGen` uint64 → spec § Replacement on rapid successive bails (build picks mechanism; counter is one of the spec's example shapes).
- Task 2-3 generation-guarded ticks (no cancellation) → spec § Replacement (cancel-or-prevent; this work unit chooses self-discriminate).
- Task 2-5 `tea.Batch` (not `Sequence`) → spec § Render-frame ordering (must not gate flash on refresh).
- Task 2-6 integration-test-only task → spec § Replacement (end-to-end contract verification across pieces).

No content invented. No edge cases beyond what the spec enumerates. No acceptance criteria testing un-spec'd behaviour.

---
