---
status: in-progress
created: 2026-07-01
cycle: 2
phase: Plan Integrity Review
topic: Session Rename Orphans Resume Hook
---

# Review Tracking: Session Rename Orphans Resume Hook - Integrity

## Cycle-1 fix verification (no findings — consistency confirmed)

The three cycle-1 fixes were re-checked and are internally consistent, introducing no new scoping/sequencing/self-containment defects on their own terms:

- **Task 3-3 import-cycle rationale** — Verified against the live codebase with `go list -deps ./internal/session`: `internal/session` transitively imports `internal/state`, but `internal/state` imports neither `session` nor `restore`, and `session` does not import `restore`. The new `restore → session` edge closes no cycle. The corrected rationale in Task 3-3 (`Do` + `Context`) matches ground truth exactly.
- **Task 2-3 empty-slice test** — The parser-level pin (`parsePaneOutput("")` returns a non-nil `[]string{}`, which `ListAllPaneHookKeys` returns verbatim) is a sound, non-fabricated assertion. `parsePaneOutput` (tmux.go:519) and `ListAllPanesWithFormat` (tmux.go:747) exist as referenced and support the delegation contract.
- **Task 3-5 RenameSession-equivalent integration leg + new tui-package unit test** — The integration leg (`client.RenameSession(old,new)`) is byte-accurate to the production path (`renameAndRefresh`, model.go:3220, reduces to `m.sessionRenamer.RenameSession` + list refresh). The *integration* half is sound. However, the *new tui-package unit test* half the fix added carries a new self-containment/accuracy problem — see Finding 1 below.

## Findings

### 1. Task 3-5's new `tui`-package unit test conflicts with the existing `tui_test` rename scaffolding and specifies an unobservable assertion

**Severity**: Important
**Plan Reference**: Phase 3, Task session-rename-orphans-resume-hook-3-5 (the in-TUI seam unit test added by the cycle-1 split) — `Do` bullet "In-TUI seam unit test (package `tui`)", the third `Tests` bullet, and Acceptance Criterion 3.
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:

The cycle-1 split added, to Task 3-5, an instruction to write a *new* unit test that "drives `renameAndRefresh(old, new)` … through a stubbed `SessionRenamer`, and asserts it issues exactly one `RenameSession(old, new)` with NO hook re-registration / re-keying," placed in "package `tui`, so the unexported method is reachable — or `tui_test` if an exported driver exists."

Checking the ground truth surfaces two problems that force the implementer to guess:

1. **The parenthetical hedge is already resolved on the ground, opposite to the task's leading recommendation.** The existing rename tests live in **`package tui_test`** (external test package — `internal/tui/model_test.go:1`), and they already drive the exact in-TUI path through the **exported** `tui.New` + `Update` loop with an established `mockSessionRenamer` (`model_test.go:1608-1619`) and `newModelWithRenamer` helper (`model_test.go:1621-1623`). `TestRenameSession`'s subtest `"enter in rename modal renames session and refreshes"` (`model_test.go:1660-1701`) already: presses `r` → types a new name → presses Enter (invoking `renameAndRefresh` via `updateRenameModal`, model.go:3206) → executes the returned `tea.Cmd` → **asserts `renamer.renamedOld`/`renamer.renamedNew` equal the expected old/new names**. That is precisely the "exactly one `RenameSession(old, new)`" assertion the new test is asked to make. The task's leading recommendation ("package `tui`, so the unexported method is reachable") steers the implementer *away* from this working `tui_test` scaffolding toward an internal-package test that would have to duplicate `mockSessionRenamer` (an internal `package tui` test cannot see `tui_test`'s type) — a needless fork of the mock, or an awkward move of it.

2. **The distinguishing assertion — "NO hook re-registration / re-keying" / "zero hook mutations" — is unobservable in the `tui` package.** The TUI model has **zero** hook collaborators: `grep -c "hooks\|HookKey\|re-key\|rekey"` in `internal/tui/model.go` returns 0, and `renameAndRefresh` (model.go:3220-3228) calls only `sessionRenamer.RenameSession` then `sessionLister.ListSessions`. There is no hook seam, store, or re-keying dependency wired into the model, so "assert zero hook mutations" has no concrete object to observe — the property holds vacuously by construction. The task presents this as a positive assertion the test must make, but it is a structural fact about the package (no hook seam exists), not something a stub can witness. An implementer will not know what to instantiate or assert against.

The net effect: the new test as specified is either (a) redundant with the existing `tui_test` `TestRenameSession` positive assertion, or (b) asks for an assertion with no observable seam. The fix's genuine intent — "prove the in-TUI code path is actually executed (not merely asserted byte-equivalent to `RenameSession`)" — is legitimate, but it is *already satisfied* by the existing `tui_test` subtest that drives the full modal → `renameAndRefresh` path and asserts the resulting `RenameSession` call. The task should point at (adapt/rename) that existing subtest and drop the misleading "package `tui` / unexported method / zero hook mutations" framing, rephrasing the property as the structural fact it is.

This does not touch the integration leg (Trigger A external rename + the `RenameSession`-equivalent leg), which remains correct and self-contained.

**Current** (Task 3-5 `Do` — the "In-TUI seam unit test" bullet):

> - **In-TUI seam unit test (package `tui`):** add a focused unit test in `internal/tui` (package `tui`, so the unexported method is reachable — or `tui_test` if an exported driver exists) that drives `renameAndRefresh(old, new)` (or the `r`-key rename-modal confirmation path that calls it) through a stubbed `SessionRenamer`, and asserts it issues exactly one `RenameSession(old, new)` with NO hook re-registration / re-keying. This is the actual execution of the in-TUI trigger; it is a pure-Go unit test (no tmux, no `//go:build integration`), so it runs everywhere. Combined with the `RenameSession`-equivalent integration leg above, the "both triggers" claim is genuinely covered: the integration leg proves reboot-survival through the shared `RenameSession` call, and this unit test proves the in-TUI path is that shared call with no re-keying.

**Proposed** (Task 3-5 `Do` — the "In-TUI seam unit test" bullet):

> - **In-TUI seam unit test (package `tui_test`):** adapt the existing rename-flow unit test rather than adding a fresh internal-package one. The in-TUI rename path is already driven end-to-end in `internal/tui/model_test.go` (package `tui_test`) by `TestRenameSession`'s `"enter in rename modal renames session and refreshes"` subtest (~lines 1660-1701): it builds a model via the exported `tui.New` + `tui.WithRenamer(mockSessionRenamer)` (`newModelWithRenamer`, ~line 1621), presses `r` → types a new name → presses `Enter` (which routes through `updateRenameModal` → `renameAndRefresh`, `model.go:3206`), executes the returned `tea.Cmd`, and asserts `renamer.renamedOld`/`renamer.renamedNew` equal the expected old/new names — i.e. exactly one `RenameSession(old, new)` from the genuine in-TUI code path. Reuse this exported driver and the existing `mockSessionRenamer`; do NOT add a duplicate `package tui` (internal) test or a second copy of the mock. Add (or extend) an assertion/comment making explicit that the rename path performs a bare `RenameSession` + list refresh with NO hook re-keying — framed as the structural fact it is: the `tui` model wires no hook seam at all (`internal/tui/model.go` references nothing hook-related), so `renameAndRefresh` cannot and does not re-key hooks; that structural absence is the decisive property the fix relies on. This is the actual execution of the in-TUI trigger (pure-Go, no tmux, no `//go:build integration`, runs everywhere). Combined with the `RenameSession`-equivalent integration leg above, the "both triggers" claim is genuinely covered: the integration leg proves reboot-survival through the shared `RenameSession` call, and this `tui_test` subtest proves the in-TUI path IS that shared call and carries no hook-re-keying seam.

**Also update Task 3-5 Acceptance Criterion 3:**

**Current**:

> - [ ] A `tui`-package unit test drives `renameAndRefresh` (the in-TUI seam) and asserts it reduces to a single `RenameSession(old, new)` with NO hook re-keying — so the in-TUI code path is actually executed, not merely asserted byte-equivalent.

**Proposed**:

> - [ ] A `tui_test`-package unit test (the existing `TestRenameSession` "enter in rename modal renames session and refreshes" subtest, reused/extended) drives the in-TUI rename path through the exported `tui.New` + `Update` loop with `mockSessionRenamer` and asserts it reduces to a single `RenameSession(old, new)`; the test (or an accompanying comment/assertion) makes explicit that the `tui` model wires no hook seam, so the rename path cannot re-key hooks — the in-TUI code path is actually executed, not merely asserted byte-equivalent, and no duplicate internal-package test or forked mock is introduced.

**Also update the corresponding `Tests` bullet:**

**Current**:

> - (`internal/tui`, package `tui`, NO `t.Parallel()`) `"it reduces the in-TUI renameAndRefresh to a single RenameSession with no hook re-keying"` — drive `renameAndRefresh` (or the rename-modal confirm path) through a stub `SessionRenamer`; assert exactly one `RenameSession(old, new)` and zero hook mutations.

**Proposed**:

> - (`internal/tui/model_test.go`, package `tui_test`, NO `t.Parallel()`) `"it reduces the in-TUI rename path to a single RenameSession with no hook re-keying"` — reuse/extend `TestRenameSession`'s `"enter in rename modal renames session and refreshes"` subtest: drive the `r`-key → rename-modal → Enter confirmation path via the exported `tui.New` + `WithRenamer(mockSessionRenamer)` driver; assert exactly one `RenameSession(old, new)` with the expected args, and assert/comment that the `tui` model wires no hook seam (so no hook re-keying is possible on this path).

**Resolution**: Pending
**Notes**:

---
