---
status: in-progress
created: 2026-05-27
cycle: 1
phase: Plan Integrity Review
topic: Bootstrap CleanStale Wipes Hooks On Tmux Transient
---

# Review Tracking: Bootstrap CleanStale Wipes Hooks On Tmux Transient - Integrity

## Findings

### 1. Task 3-2 leaves Commander-injection mechanism unresolved

**Severity**: Important
**Plan Reference**: Phase 3, Task `bootstrap-cleanstale-wipes-hooks-on-tmux-transient-3-2` ("Bootstrap end-to-end integration test for tmux transient `list-panes` failure"), Do step 6 ("Drive a bootstrap-triggering command").
**Category**: Task Self-Containment / Scope and Granularity
**Change Type**: update-task

**Details**:
Task 3-2's Do step 6 leaves the most load-bearing implementation decision unresolved: how to inject the `transientListPanesCommander` into the orchestrator's `*tmux.Client`. The current text reads "call `buildProductionOrchestrator()` after overriding the package-level Commander factory (if such a seam exists) or by constructing the orchestrator inline with the stub client" — but `buildProductionOrchestrator()` is a parameter-less function at `cmd/bootstrap_production.go:99` that hard-codes `client := tmux.DefaultClient()`, and `tmux.DefaultClient()` itself instantiates a `*RealCommander`. No package-level Commander factory seam exists today. An implementer following the task as written has to invent a seam or hand-roll an inline orchestrator construction without guidance — which is exactly the "force the implementer to make design decisions" failure mode this review checks for.

Both call sites of `buildProductionOrchestrator()` (in `buildBootstrapDeps`) currently consume both return values; the function builds the client *inside* itself. Three concrete options actually exist; the plan should name the chosen one so the implementer does not re-derive this:

(a) Add a small test-only seam in `cmd/bootstrap_production.go` — package-level `var commanderFactory = func() tmux.Commander { return &tmux.RealCommander{} }` invoked inside `buildProductionOrchestrator`; the test overrides it via `t.Cleanup`-guarded mutation. Mirrors the `cleanDeps` / `bootstrapDeps` package-level-mutable-state pattern that the rest of `cmd/` uses (per CLAUDE.md § "DI / testing pattern").
(b) Refactor `buildProductionOrchestrator` to accept an injected `*tmux.Client` (production callers pass `tmux.DefaultClient()`). Higher-fidelity but widens the production surface for one test.
(c) Skip `buildProductionOrchestrator` entirely in the test — construct an `*Orchestrator` inline by replicating the field-population pattern from `bootstrap_production.go:122-160`. Highest fidelity drift risk but zero production change.

The plan should pick one. Option (a) is the closest match to existing convention; option (c) is least invasive but the inline replication is itself a drift-risk. Either way, picking one removes the unresolved design decision from the task.

**Current**:

```
  6. Drive a bootstrap-triggering command. Two options — pick the one that matches existing conventions in `cmd/bootstrap/composition_e2e_*_integration_test.go`:
     - **Option A (direct orchestrator)**: call `buildProductionOrchestrator()` after overriding the package-level Commander factory (if such a seam exists) or by constructing the orchestrator inline with the stub client. Invoke `orch.Bootstrap(ctx)` and collect the returned warnings. Lower friction for assertion access.
     - **Option B (subprocess)**: use `portalbintest.BuildPortalBinary` to build a `portal` binary, then run `portal open <path>` or equivalent as an `exec.Cmd` with `cmd.Env = env`. Higher fidelity but harder to inject the Commander stub — requires an env-var-driven hook in production code, which does not exist. Prefer Option A.
```

**Proposed**:

```
  6. Drive a bootstrap-triggering command via the **commander-factory seam approach** (Option A is the only viable approach end-to-end; Option B is rejected because no env-var-driven Commander seam exists in production code and adding one is out of scope).

     **Add a small test-only seam in `cmd/bootstrap_production.go`** following the `cleanDeps` / `bootstrapDeps` package-level-mutable-state pattern documented in CLAUDE.md § "DI / testing pattern":

     ```go
     // commanderFactory is the indirection seam tests use to inject a
     // wrapping Commander (e.g. cmd/bootstrap_production_test.go's
     // transient-listpanes stub). Production code leaves it at the
     // default factory; tests override and restore via t.Cleanup.
     var commanderFactory = func() tmux.Commander { return &tmux.RealCommander{} }
     ```

     Inside `buildProductionOrchestrator`, replace the first line `client := tmux.DefaultClient()` with `client := tmux.NewClient(commanderFactory())`. This is a one-line change; the production call path is unchanged because the default factory returns the same `*RealCommander` `DefaultClient` uses today.

     In the test, override the factory before invoking `buildProductionOrchestrator`:

     ```go
     base := commanderFactory()       // the production *RealCommander
     stub := &transientListPanesCommander{inner: base, mode: <policy>, sticky: true}
     prev := commanderFactory
     commanderFactory = func() tmux.Commander { return stub }
     t.Cleanup(func() { commanderFactory = prev })

     orch, _ := buildProductionOrchestrator()
     warnings, err := orch.Run(ctx)
     ```

     Then assert against the returned warnings slice and `err`. Use the orchestrator's exposed `Run` (or whichever method is canonical at `cmd/bootstrap/bootstrap.go`) for invocation, matching the call shape already used in `cmd/bootstrap/composition_e2e_*_integration_test.go`.

     **Wiring caveat for `cmd/bootstrap_production.go`**: this task introduces the seam in addition to consuming it. Add the new package-level `var commanderFactory` declaration and update the one call site inside `buildProductionOrchestrator` in the same PR so the test compiles. The production surface widens by one unexported variable — acceptable per the `cleanDeps` precedent.
```

**Resolution**: Pending
**Notes**:

---

### 2. Task 2-2 docstring acceptance criterion miscounts branches

**Severity**: Minor
**Plan Reference**: Phase 2, Task `bootstrap-cleanstale-wipes-hooks-on-tmux-transient-2-2`, Acceptance Criteria item 3.
**Category**: Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The AC says the docstring must describe "the four-branch contract", but the docstring drafted in Do step 5 enumerates 5 branches (ListAllPanes error, Load error, hazard guard, both-sides-empty, normal path) and the algorithm itself has 6 branches (counting the normal-path Save-error branch). An implementer satisfying "four-branch contract" literally would write an incomplete docstring; a reviewer checking the AC would notice the count mismatch and ask which is correct.

The intended count is 5 docstring bullets (Save-error rolls into "Normal path" since the docstring already says the Debug fires "after `hookStore.CleanStale` succeeds"). Align the AC wording to the actual draft.

**Current**:

```
- Docstring at the method declaration is rewritten — no longer claims "degrades to no-op"; instead describes the four-branch contract.
```

**Proposed**:

```
- Docstring at the method declaration is rewritten — no longer claims "degrades to no-op"; instead describes the five-branch contract enumerated in Do step 5 (ListAllPanes error, Load error, hazard guard, both-sides-empty silent no-op, normal path with completion Debug on success).
```

**Resolution**: Pending
**Notes**:

---

### 3. Task 2-2 comment-lift line range references the wrong span

**Severity**: Minor
**Plan Reference**: Phase 2, Task `bootstrap-cleanstale-wipes-hooks-on-tmux-transient-2-2`, Do step 4.
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Do step 4 says "Lift the comment block from `cmd/bootstrap/stale_marker_cleanup.go:80-92`". The actual content at lines 80-92 is items 2-6 of the `CleanStaleMarkers` algorithm-step comment block (a numbered list spanning `Enumerate live panes` → `For each marker paneKey absent...`). Only items 4-5 (the hazard-guard portion) describe the "deferral is a successful soft outcome" framing the task wants to preserve. An implementer following the literal range would copy unrelated algorithm steps into the wrong place.

The correct source span is roughly lines 81-92 (items 4 and 5 of the original comment, which contain the "Mass-unset hazard guard" framing and the "empty markers + empty live → return nil" branch).

**Current**:

```
4. Lift the comment block from `cmd/bootstrap/stale_marker_cleanup.go:80-92` above the hazard-guard branch. Adapt: replace "marker" with "hook entry" / "hooks.json entry", "unset every marker" with "delete every hooks.json entry", "markers protecting legitimate hydrate-in-progress panes" with "hooks.json entries for legitimate live panes whose enumeration momentarily failed". Preserve the "deferral is a successful soft outcome ('skip this run; next bootstrap retries'), not a failure" framing verbatim. The protected-data noun and the soft-outcome framing are both load-bearing per spec §Change 3.
```

**Proposed**:

```
4. Lift the hazard-guard comment block from `cmd/bootstrap/stale_marker_cleanup.go` — the source span is items 4 and 5 of the `CleanStaleMarkers` algorithm-step comment (the "Mass-unset hazard guard" paragraph and the "empty markers + empty live" paragraph that follows it, located around lines 81-92 — confirm before lifting since line numbers may shift). Adapt: replace "marker" with "hook entry" / "hooks.json entry", "unset every marker" with "delete every hooks.json entry", "markers protecting legitimate hydrate-in-progress panes" with "hooks.json entries for legitimate live panes whose enumeration momentarily failed". Preserve the "deferral is a successful soft outcome ('skip this run; next bootstrap retries'), not a failure" framing verbatim. Do **not** lift the surrounding algorithm-step bullets (items 2, 3, 6) — they describe `CleanStaleMarkers`'s flow, not the hazard-guard rationale. The protected-data noun and the soft-outcome framing are both load-bearing per spec §Change 3.
```

**Resolution**: Pending
**Notes**:

---

### 4. Phase 1 acceptance criteria omits the `tmux_test.go` subtest inversion

**Severity**: Minor
**Plan Reference**: Phase 1 acceptance criteria.
**Category**: Phase Structure / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
Phase 1's task-level detail in Task 1-1 (Do step 4) inverts the `TestListAllPanes` subtest at `internal/tmux/tmux_test.go:1461-1473`, but the phase-level acceptance criteria list does not mention this inversion — it only references the new unit test and the existing `cmd/clean_test.go` tests staying green. A reader skimming the phase AC could close out Phase 1 having missed the subtest inversion. Add the AC bullet so phase-level review surfaces it.

**Current**:

```
**Acceptance**:
- [ ] `(*tmux.Client).ListAllPanes` delegates to `ListAllPanesWithFormat("#{session_name}:#{window_index}.#{pane_index}")` and returns `(nil, err)` on non-nil helper error
- [ ] Helper docstring rewritten — removes the "no tmux server convenience" framing, describes the error-propagating contract
- [ ] New unit test asserts `(nil, err)` is returned when the underlying `Commander` returns exit ≠ 0 on `list-panes -a`
- [ ] New unit test asserts `([]string{}, nil)` is returned only when `list-panes -a` legitimately returns exit 0 with empty stdout
- [ ] Existing non-empty-live-set tests in `cmd/clean_test.go` pass unchanged
- [ ] `go test ./...` is green
```

**Proposed**:

```
**Acceptance**:
- [ ] `(*tmux.Client).ListAllPanes` delegates to `ListAllPanesWithFormat("#{session_name}:#{window_index}.#{pane_index}")` and returns `(nil, err)` on non-nil helper error
- [ ] Helper docstring rewritten — removes the "no tmux server convenience" framing, describes the error-propagating contract
- [ ] Existing subtest at `internal/tmux/tmux_test.go:1461-1473` (`"returns empty slice when no tmux server running"`) is inverted to assert `(nil, non-nil err)` on commander error and renamed to reflect the new contract
- [ ] New unit test asserts `(nil, err)` is returned when the underlying `Commander` returns exit ≠ 0 on `list-panes -a`
- [ ] New unit test asserts `([]string{}, nil)` is returned only when `list-panes -a` legitimately returns exit 0 with empty stdout
- [ ] Existing non-empty-live-set tests in `cmd/clean_test.go` pass unchanged
- [ ] `go test ./...` is green
```

**Resolution**: Pending
**Notes**:

---

### 5. Task 2-1 imprecise about `bootstrap.Logger` vs `*state.Logger` field type

**Severity**: Minor
**Plan Reference**: Phase 2, Task `bootstrap-cleanstale-wipes-hooks-on-tmux-transient-2-1`, Do step 1 and Acceptance Criteria.
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Task 2-1 specifies adding a `Logger bootstrap.Logger` field. But the orchestrator-scope `logger` at `cmd/bootstrap_production.go:109` is `*state.Logger`, not `bootstrap.Logger` — assignment works because `*state.Logger` satisfies the `bootstrap.Logger` interface. Reading the task in isolation, an implementer might trip over the type mismatch and either (a) try to convert the value awkwardly or (b) decide to use `*state.Logger` instead. Both are surmountable, but adding one sentence eliminates the ambiguity.

Also worth noting that the `MarkerCleanupCore.Logger` field at `cmd/bootstrap/stale_marker_cleanup.go` is declared as `bootstrap.Logger` interface type and is populated from the same `*state.Logger` value at `bootstrap_production.go:151` — the established convention is "declare as interface type, populate with concrete `*state.Logger`". Codify this in the task.

**Current** (Do step 1):

```
1. Open `cmd/bootstrap_production.go`. Edit the `cleanStaleAdapter` struct at lines 66-69 to add a third field: `Logger bootstrap.Logger`.
```

**Proposed** (Do step 1):

```
1. Open `cmd/bootstrap_production.go`. Edit the `cleanStaleAdapter` struct at lines 66-69 to add a third field: `Logger bootstrap.Logger`. This is the interface type (declared at `cmd/bootstrap/bootstrap.go:178-183`); the orchestrator-scope `logger` at line 109 is `*state.Logger`, whose method set satisfies the interface. This mirrors `MarkerCleanupCore.Logger` at `cmd/bootstrap/stale_marker_cleanup.go`, which is also declared as `bootstrap.Logger` and populated from the same `*state.Logger` value at `bootstrap_production.go:151`.
```

**Resolution**: Pending
**Notes**:

---

### 6. Task 2-2 step 6 misdescribes `hooksFile` as an unexported map type

**Severity**: Minor
**Plan Reference**: Phase 2, Task `bootstrap-cleanstale-wipes-hooks-on-tmux-transient-2-2`, Do step 6.
**Category**: Task Self-Containment / Context accuracy
**Change Type**: update-task

**Details**:
Do step 6 says: "Sanity-check the `hooksFile` type is exported enough to be the return shape of `Store.Load()` — per `internal/hooks/store.go:36` it returns `(hooksFile, error)`. `len(persisted)` works on the unexported map type returned from the same package boundary because we only consume `len()` here, not index into it; no API change is required."

This is imprecise: `hooksFile` is a `type hooksFile = map[string]map[string]string` **alias** (line 22 of `internal/hooks/store.go`), not a named unexported type. Type aliases are transparent — callers outside the `hooks` package see the underlying map type and can use it freely (declare local vars of the alias-expanded type, range over it, etc.). The footnote about "only consume `len()`" is therefore unnecessary caution. Tighten the language so an implementer doesn't waste time worrying about a type-visibility issue that does not exist.

**Current** (Do step 6):

```
6. Sanity-check the `hooksFile` type is exported enough to be the return shape of `Store.Load()` — per `internal/hooks/store.go:36` it returns `(hooksFile, error)`. `len(persisted)` works on the unexported map type returned from the same package boundary because we only consume `len()` here, not index into it; no API change is required.
```

**Proposed** (Do step 6):

```
6. Note on `Store.Load()` return type: `internal/hooks/store.go:22` declares `type hooksFile = map[string]map[string]string` as a type alias (note the `=`). Aliases are transparent across package boundaries, so the local `persisted` declared via `:=` is the underlying `map[string]map[string]string` and `len(persisted)` works directly. No API change required, no type-visibility workaround needed.
```

**Resolution**: Pending
**Notes**:

---

### 7. Task 3-1 hooks-path resolution leans on `PORTAL_HOOKS_FILE` without confirming `IsolateStateForTest` sets it

**Severity**: Minor
**Plan Reference**: Phase 3, Task `bootstrap-cleanstale-wipes-hooks-on-tmux-transient-3-1`, Do step 4 and Edge Cases.
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
The task says the `seedHooksJSON` helper should "resolve via `os.Getenv("PORTAL_HOOKS_FILE")` first, then `$XDG_CONFIG_HOME/portal/hooks.json`" and notes that "The `IsolateStateForTest` helper scrubs `XDG_CONFIG_HOME` to a per-test tempdir". This implies `PORTAL_HOOKS_FILE` is also overridden by `IsolateStateForTest` — but per CLAUDE.md the helper "scrubs developer `XDG_CONFIG_HOME`" without explicit mention of `PORTAL_HOOKS_FILE`. The Edge Case bullet says the helper "must respect `PORTAL_HOOKS_FILE` env override (set by `IsolateStateForTest`)". An implementer cannot verify this without reading the helper source.

Two options: (a) explicitly inspect `internal/portaltest/isolated_env.go` and confirm which env vars are in the returned env, then update the task with the confirmed list; or (b) make the resolution path robust to either case (use the helper's returned env to construct the path inline rather than re-deriving from the global env). Option (b) is the safer pattern for the integration tests in this work unit.

**Current** (Do step 4):

```
- Implement `seedHooksJSON(t *testing.T, stateDir string, entries map[string]string)` — write a valid `hooks.json` to the resolved config path (NB: hooks live under `~/.config/portal/hooks.json`, not under `stateDir`; resolve via `os.Getenv("PORTAL_HOOKS_FILE")` first, then `$XDG_CONFIG_HOME/portal/hooks.json`, matching `cmd/config.go`'s `configFilePath` resolution). Use the production `internal/hooks` package to construct the file so the on-disk shape stays canonical. The `IsolateStateForTest` helper scrubs `XDG_CONFIG_HOME` to a per-test tempdir — verify the path the helper writes to is under the isolated tree.
```

**Proposed** (Do step 4):

```
- Implement `seedHooksJSON(t *testing.T, env []string, entries map[string]string)` — write a valid `hooks.json` to the resolved config path. **Resolve from the `env` slice returned by `IsolateStateForTest(t)`, not from the global process env**, so the seed lands under the isolated tree regardless of which env vars the helper actually overrides today. Concretely: scan `env` for `PORTAL_HOOKS_FILE=...` first (use it verbatim if present); otherwise scan for `XDG_CONFIG_HOME=...` and join with `portal/hooks.json`; otherwise `t.Fatalf` (signals that isolation has regressed and the test would write to the developer's tree — preferable to silently corrupting). This mirrors `cmd/config.go`'s `configFilePath` resolution but consumes the test-isolated env rather than `os.Getenv`. Use the production `internal/hooks` package to construct the file so the on-disk shape stays canonical. Verify with a `t.Logf` of the resolved path that the seed lands under the isolated tree.
```

Also update **Edge Cases** entry:

```
- Hooks file path resolution: resolve from the `env` slice returned by `IsolateStateForTest(t)` rather than `os.Getenv`, so the helper is robust to future changes in which env vars `IsolateStateForTest` overrides
```

**Resolution**: Pending
**Notes**:

---

### 8. Phase 1 phase-level acceptance criterion conflates docstring rewrite scope

**Severity**: Minor
**Plan Reference**: Phase 1 phase-level acceptance criteria.
**Category**: Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The phase-level AC says "Helper docstring rewritten — removes the 'no tmux server convenience' framing, describes the error-propagating contract". Task 1-1 actually mandates richer content: it must (i) reference the canonical structural-key format string, (ii) describe `(nil, err)` on tmux failure, (iii) describe `parsePaneOutput(raw)` on success, (iv) state that callers decide policy for empty/error results. The phase-level AC is a strict subset and could be checked off without points (iii) and (iv). Tighten the phase-level AC to match the task content.

**Current**:

```
- [ ] Helper docstring rewritten — removes the "no tmux server convenience" framing, describes the error-propagating contract
```

**Proposed**:

```
- [ ] Helper docstring rewritten — removes the "no tmux server convenience" framing; describes (a) enumeration via the error-propagating `ListAllPanesWithFormat` using the canonical `"#{session_name}:#{window_index}.#{pane_index}"` format, (b) `(nil, err)` on tmux failure, (c) `(parsePaneOutput(raw), nil)` on success, (d) that callers decide policy for empty/error results
```

**Resolution**: Pending
**Notes**:

---
