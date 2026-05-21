# Analysis Tasks: esc-after-preview-hides-session-list (Cycle 1)

## Task 1: Rename drainRefilterCmd to drainCmdThroughUpdate
status: pending
severity: low
sources: architecture

**Problem**: The test helper `drainRefilterCmd` (`internal/tui/pagepreview_refetch_test.go:130-141`) is named as if it were refilter-specific, but its body is a domain-agnostic single-step "invoke cmd; feed message back through Update" round-trip. Its own unit test (`TestDrainRefilterCmdInvokesCmdAndFeedsResultThroughUpdate`) explicitly probes it with a non-refilter `WindowSizeMsg`, and a second consumer already exists at `internal/tui/kill_refresh_filter_test.go:127`. The misleading name creates friction for future consumers (rename-refresh, ProjectsLoadedMsg propagation) that need the same generic round-trip.

**Solution**: Rename the helper and its dedicated unit test to reflect the general contract, and rewrite the doc comment to lead with the domain-neutral behaviour. Scope is test-package-internal only — no production code changes.

**Outcome**: Helper name and doc comment accurately describe the single-step cmd-drain contract. Future test authors discover and reuse it without being misled by refilter-specific framing. All existing call sites pass.

**Do**:
1. In `internal/tui/pagepreview_refetch_test.go`, rename the function `drainRefilterCmd` → `drainCmdThroughUpdate` (declaration around line 130).
2. Rewrite the doc comment so the first sentence states the general contract. Keep the existing paragraph about the SetItems/filter scenario as a follow-on "Typical use" note documenting the canonical refilter case.
3. Update the call site within `pagepreview_refetch_test.go`.
4. Update the call site at `internal/tui/kill_refresh_filter_test.go:127`.
5. Rename the dedicated unit test `TestDrainRefilterCmdInvokesCmdAndFeedsResultThroughUpdate` → `TestDrainCmdThroughUpdateInvokesCmdAndFeedsResultThroughUpdate` and update any inline string references.
6. Run `go build ./...` then `go test ./internal/tui/...` to confirm green.

**Acceptance Criteria**:
- No symbol named `drainRefilterCmd` remains anywhere under `internal/tui/`.
- Both existing consumers call the renamed helper and pass.
- Doc comment leads with the general contract; refilter usage appears as illustrative follow-on.
- The dedicated unit test still exists under the new name and continues to exercise both the nil-cmd short-circuit and the non-refilter `WindowSizeMsg` round-trip.
- `go test ./internal/tui/...` is green.

**Tests**:
- Renamed helper unit test — asserts nil-cmd short-circuit and `WindowSizeMsg` round-trip.
- `TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh` — remains green.
- `TestKillRefreshUnderFilterPreservesFilteredList` — remains green.

---

## Discarded findings (deferred per Rule-of-Three / spec)

- killerStub vs mockSessionKiller structural twin (duplication, standards) — spec-acknowledged package-boundary divergence; only a cross-reference comment was suggested.
- Filter-commit setup block duplicated between two new tests (duplication) — N=2, defer.
- WithInsideTmux panic-on-non-nil stylistically anomalous (architecture) — spec mandates this exact panic.
- ProjectsLoadedMsg handler diverges from applySessions encapsulation (architecture) — defer until second projects-list mutator appears.
- visibleSessionNames colocated as if file-local (architecture) — defer until a third consumer arrives.
