---
status: in-progress
created: 2026-07-01
cycle: 1
phase: Plan Integrity Review
topic: Session Rename Orphans Resume Hook
---

# Review Tracking: Session Rename Orphans Resume Hook - Integrity

## Summary

The plan is high quality overall. All 18 tasks follow the full canonical template (Problem / Solution / Outcome / Do / Acceptance Criteria / Tests / Edge Cases / Context / Spec Reference); acceptance criteria are concrete and pass/fail; tests include edge cases beyond the happy path; vertical slicing is clean (each task is a single verifiable TDD cycle); phase boundaries are logical (Phase 1 foundation primitives → Phase 2 live-key adoption → Phase 3 persistence + integration proof); and the sequencing/dependency reasoning in each phase's "Why this order" is sound. Grounding references were spot-checked against the live source and are accurate (line numbers approximate as stated): `renameAndRefresh` at `model.go:3220` → `sessionRenamer.RenameSession`, `collectArmInfos` baking `tmux.PaneTarget(sess.Name, w.Index, p.Index)` at `session.go:110`, `captureFormat` with 10 fields + `captureFieldCount = 10`, the `StructuralKeyResolver`/`AllPaneLister` seams, and all referenced `tmuxtest`/`restoretest` helper signatures.

Three findings follow: one Important (a test-coverage integrity gap where an acceptance criterion overstates what the test can prove given package boundaries), and two Minor (a factually-incorrect grounding rationale, and an under-specified test criterion). None block implementation.

## Findings

### 1. Task 3-5 Trigger B cannot genuinely exercise `renameAndRefresh` from `restore_test`; acceptance criterion overstates coverage [RESOLVED: Fixed]

**Severity**: Important
**Plan Reference**: Phase 3, task `session-rename-orphans-resume-hook-3-5` (Integration — rename-then-restore fires the registered hook for both triggers)
**Category**: Acceptance Criteria Quality / Task Self-Containment (no ambiguity)

**Details**:
The task's Trigger B ("in-TUI `renameAndRefresh`") is scoped to a new test file `internal/restore/rename_reboot_hook_integration_test.go` in package `restore_test`. But `renameAndRefresh` is an **unexported method on `tui.Model`** (`func (m Model) renameAndRefresh(...)` at `model.go:3220`), and there is no exported seam that invokes it directly — the only exported constructors are `tui.New(...)` / `tui.Build(Deps)`, which yield a `Model` whose rename flow is only reachable by driving the Bubble Tea `Update` loop with a rename-modal confirmation message. From a `restore_test` package the method is therefore genuinely unreachable without a full `Model` + message-driving harness.

The task anticipates this ("If constructing a full `Model` is impractical in this package, drive the exact production `RenameSession` call the TUI path makes... and note in a comment that this is the byte-equivalent of the in-TUI trigger"). In practice the implementer will take that escape hatch (the full-Model harness is impractical cross-package), which collapses Trigger B into a byte-identical `client.RenameSession(old, new)` — i.e. exactly Trigger A. The acceptance criterion "After the in-TUI `renameAndRefresh` rename (**same `RenameSession` production path**), capture+restore+signal-hydrate fires the hook" then passes trivially without `renameAndRefresh` ever executing, so the committed guard does not actually cover the in-TUI trigger it claims to. This is an integrity gap: the criterion asserts more than the (practical) test proves, and it forces the implementer to make an undocumented judgement call about which of two divergent harness shapes to write.

The fix is to make the criterion honest about what is verified and to route the genuine in-TUI coverage to the package that owns `renameAndRefresh` (a `tui`-package unit test that drives the rename seam), leaving the `restore_test` integration leg explicitly labelled as the `RenameSession`-equivalent path. This keeps the "both triggers" claim truthful without demanding an impractical cross-package Model harness.

**Current** (task `session-rename-orphans-resume-hook-3-5`, the "Do" bullet for Trigger B):
> - **Trigger B (in-TUI):** exercise `renameAndRefresh` (`internal/tui/model.go` ~line 3220) via its production seam — construct a `tui.Model` (or invoke the `SessionRenamer` seam it calls: `m.sessionRenamer.RenameSession(oldName, newName)`) so the rename goes through the SAME code path the `r` key drives. Do NOT modify `renameAndRefresh` — it stays a bare `RenameSession` + list refresh; this task only TESTS it. If constructing a full `Model` is impractical in this package, drive the exact production `RenameSession` call the TUI path makes against the socket client and note in a comment that this is the byte-equivalent of the in-TUI trigger (both reduce to `RenameSession(old, new)` with zero hook re-keying — the decisive property the fix relies on).

**Proposed** (replace that "Do" bullet with):
> - **Trigger B (in-TUI, `RenameSession`-equivalent leg):** `renameAndRefresh` (`internal/tui/model.go:3220`) is an UNEXPORTED `Model` method — it cannot be called from `restore_test`, and driving it via `tui.New`/`tui.Build` + the `Update` loop is out of scope for this restore-package integration test. In THIS file, exercise the byte-equivalent production call the in-TUI path makes against the socket client — `client.RenameSession(oldName, newName)` — and add a comment stating this is the `RenameSession`-equivalent of the in-TUI trigger (`renameAndRefresh` reduces to `m.sessionRenamer.RenameSession(old, new)` + a list refresh, with ZERO hook re-keying — the decisive property the fix relies on). This leg proves the fix survives the SAME `RenameSession` call the TUI issues; the genuine `renameAndRefresh` seam coverage lives in the `tui`-package unit test added below (so the in-TUI code path is actually executed somewhere, not merely asserted-equivalent). Do NOT modify `renameAndRefresh`.
> - **In-TUI seam unit test (package `tui`):** add a focused unit test in `internal/tui` (package `tui`, so the unexported method is reachable — or `tui_test` if an exported driver exists) that drives `renameAndRefresh(old, new)` (or the `r`-key rename-modal confirmation path that calls it) through a stubbed `SessionRenamer`, and asserts it issues exactly one `RenameSession(old, new)` with NO hook re-registration / re-keying. This is the actual execution of the in-TUI trigger; it is a pure-Go unit test (no tmux, no `//go:build integration`), so it runs everywhere. Combined with the `RenameSession`-equivalent integration leg above, the "both triggers" claim is genuinely covered: the integration leg proves reboot-survival through the shared `RenameSession` call, and this unit test proves the in-TUI path is that shared call with no re-keying.

**Also update the corresponding Acceptance Criterion** from:
> - [ ] After the in-TUI `renameAndRefresh` rename (same `RenameSession` production path), capture+restore+signal-hydrate fires the hook (side-effect observed).

to:
> - [ ] The `RenameSession`-equivalent leg (the exact `client.RenameSession(old, new)` call the in-TUI path issues) fires the hook after capture+restore+signal-hydrate (side-effect observed) — proving reboot-survival through the shared rename call.
> - [ ] A `tui`-package unit test drives `renameAndRefresh` (the in-TUI seam) and asserts it reduces to a single `RenameSession(old, new)` with NO hook re-keying — so the in-TUI code path is actually executed, not merely asserted byte-equivalent.

**And update the corresponding Tests entry** from:
> - `"it fires the resume hook after an in-TUI renameAndRefresh rename and reboot"` — same, but the rename goes through the `RenameSession` seam the TUI path uses; assert `HOOK_FIRED` once.

to:
> - `"it fires the resume hook after a RenameSession-equivalent (in-TUI path) rename and reboot"` — the rename goes through the exact `RenameSession` call the TUI path issues; assert `HOOK_FIRED` once.
> - (`internal/tui`, package `tui`, NO `t.Parallel()`) `"it reduces the in-TUI renameAndRefresh to a single RenameSession with no hook re-keying"` — drive `renameAndRefresh` (or the rename-modal confirm path) through a stub `SessionRenamer`; assert exactly one `RenameSession(old, new)` and zero hook mutations.

**Resolution**: Pending
**Notes**:

---

### 2. [RESOLVED: Fixed] Task 3-3 states an incorrect import-cycle rationale ("`internal/session` imports only `internal/tmux`")

**Severity**: Minor
**Plan Reference**: Phase 3, task `session-rename-orphans-resume-hook-3-3` (Re-stamp `@portal-id` in `createSkeleton`)
**Category**: Task Self-Containment (grounding accuracy)

**Details**:
Task 3-3 adds the first import of `internal/session` into `internal/restore/session.go` and justifies cycle-freedom by asserting, in three places (Solution, Do, Context), that "`internal/session` imports only `internal/tmux` (verified — it imports neither `internal/restore` nor `internal/state`)". This is factually wrong: `internal/session` imports `internal/project` and `internal/resolver` directly, and transitively pulls in `internal/state` (confirmed via `go list -deps ./internal/session`). The cycle-free **conclusion is correct** (I compile-verified that adding `internal/restore → internal/session` builds with no cycle), but for a different reason than stated: nothing in `internal/session`'s dependency closure imports `internal/restore` (and `internal/state` imports neither `session` nor `restore`), so the new edge is safe. Leaving the false rationale in place is a grounding hazard — an implementer or future maintainer trusting "session imports only tmux" could draw wrong conclusions if the dependency graph shifts, and it undermines confidence in an otherwise well-grounded plan. Correct the rationale to the accurate one; the code guidance and placement are unchanged.

**Current** (task `session-rename-orphans-resume-hook-3-3`, "Do" bullet 1):
> - In `internal/restore/session.go`, add `"github.com/leeovery/portal/internal/session"` to the import block (~lines 25-36). This is the FIRST import of `internal/session` into `internal/restore`; it is cycle-free because `internal/session` imports only `internal/tmux` (verified — it imports neither `internal/restore` nor `internal/state`). Use `session.PortalIDOption` for the option name so it stays byte-identical to the creation-time stamp and to the `@portal-id` literal in `tmux.HookKeyFormat`.

**Proposed** (replace that bullet with):
> - In `internal/restore/session.go`, add `"github.com/leeovery/portal/internal/session"` to the import block (~lines 25-36). This is the FIRST import of `internal/session` into `internal/restore`; it is cycle-free because nothing in `internal/session`'s dependency closure imports `internal/restore`. (Precisely: `internal/session` imports `internal/tmux`, `internal/project`, and `internal/resolver`, and transitively `internal/state` — but `internal/state` imports neither `internal/session` nor `internal/restore`, and `internal/session` does not import `internal/restore`, so the new `restore → session` edge closes no cycle. Confirmed with `go list -deps ./internal/session` and a compile check.) Use `session.PortalIDOption` for the option name so it stays byte-identical to the creation-time stamp and to the `@portal-id` literal in `tmux.HookKeyFormat`.

**Also correct the matching Context grounding note** from:
> Constant: the option name is the single shared `PortalIDOption = "@portal-id"` constant (Task 1-3, in `internal/session`), referenced by every set-option site (creation, restore re-stamp) and kept in sync with the literal `@portal-id` embedded in `tmux.HookKeyFormat`. Grounding note: `internal/restore/session.go` does not currently import `internal/session`; this task adds that import — cycle-free because `internal/session` imports only `internal/tmux`.

to:
> Constant: the option name is the single shared `PortalIDOption = "@portal-id"` constant (Task 1-3, in `internal/session`), referenced by every set-option site (creation, restore re-stamp) and kept in sync with the literal `@portal-id` embedded in `tmux.HookKeyFormat`. Grounding note: `internal/restore/session.go` does not currently import `internal/session`; this task adds that import — cycle-free because nothing in `internal/session`'s dependency closure imports `internal/restore` (session's closure includes `tmux`/`project`/`resolver`/`state`, none of which import `restore`).

**Resolution**: Pending
**Notes**:

---

### 3. Task 2-3's "non-nil empty slice" test criterion offers three unresolved fallbacks, leaving the implementer to choose

**Severity**: Minor
**Plan Reference**: Phase 2, task `session-rename-orphans-resume-hook-2-3` (Add `tmux.ListAllPaneHookKeys`), the Tests entry for the empty-slice case
**Category**: Acceptance Criteria Quality (no criteria an implementer must interpret)

**Details**:
The empty-slice test entry concedes the condition cannot be produced against the live harness ("the harness always has the anchor session"), then lists three mutually-exclusive fallbacks — (a) "assert against a server whose only session was killed", (b) "a unit-level assertion on `parsePaneOutput`", or (c) "inspecting the return type contract rather than a live empty read" — without deciding which one the implementer should write. Options (a) and (b) test materially different things (a live enumeration returning `[]string{}` vs. the underlying parser), and (c) ("inspecting the return type contract") is not an executable assertion at all. This is an under-specified criterion that forces a design decision at implementation time. The cleanest resolution: since `ListAllPaneHookKeys` delegates to `parsePaneOutput` (whose `[]string{}`-on-empty contract is already unit-covered), assert the empty-output behaviour at the parser level and drop the live-empty ambiguity — matching how the existing `ListAllPanes` empty contract is established.

**Current** (task `session-rename-orphans-resume-hook-2-3`, Tests, the empty-slice bullet):
> - `"it returns a non-nil empty slice when there are no panes"` — covered structurally by `parsePaneOutput("")` returning `[]string{}`; assert against a server whose only session was killed, or via a unit-level assertion on `parsePaneOutput` if a live empty enumeration is impractical (the harness always has the anchor session, so this may be asserted by inspecting the return type contract rather than a live empty read).

**Proposed** (replace that bullet with):
> - `"it returns a non-nil empty slice when parsePaneOutput sees no rows"` — a live empty enumeration is impractical (the `tmuxtest` harness always has an anchor session), so assert the empty contract directly at the parser level: `parsePaneOutput("")` returns a non-nil `[]string{}` (len 0), which `ListAllPaneHookKeys` returns verbatim as `([]string{}, nil)` on empty output. This is a pure-Go assertion needing no live server; it pins the inherited empty-output contract without a fabricated live-empty read.

**Resolution**: Pending
**Notes**:

---
