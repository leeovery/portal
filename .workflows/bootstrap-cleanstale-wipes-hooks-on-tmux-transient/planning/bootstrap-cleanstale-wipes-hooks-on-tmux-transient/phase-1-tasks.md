---
phase: 1
phase_name: Repurpose `ListAllPanes` to Propagate Errors
total: 2
---

## bootstrap-cleanstale-wipes-hooks-on-tmux-transient-1-1 | approved

### Task 1.1: Propagate tmux errors from `ListAllPanes`

**Problem**: `(*tmux.Client).ListAllPanes` at `internal/tmux/tmux.go:687-693` currently swallows every error class from the underlying `Commander` (transient transport failures, exit ≠ 0 from a saver-respawn race, server-gone, and legitimate empty) into the same `([]string{}, nil)` return shape. This conflation is the root cause Layer 1 of the silent-`hooks.json`-wipe defect: callers cannot distinguish "tmux failed" from "no panes exist", so bootstrap step 11 (`CleanStale`) and `portal clean` treat a transient list-panes failure as an authoritative empty live set and proceed to delete every entry from `hooks.json`. The peer helper `ListAllPanesWithFormat` (same file, lines 655-665) already propagates errors correctly; the divergence is the documented, intentional behaviour whose cost is exactly this bug.

**Solution**: Repurpose (not delete, not deprecate) `ListAllPanes` into a thin wrapper around `ListAllPanesWithFormat("#{session_name}:#{window_index}.#{pane_index}")` so it inherits the error-propagating contract while keeping all existing call sites compiling unchanged. Reuse the existing `parsePaneOutput` helper at `internal/tmux/tmux.go:452-466` for parsing the raw output — no new parser is required. Rewrite the docstring to describe the new error-propagating contract and remove the "no tmux server" convenience framing.

**Outcome**: `ListAllPanes` returns `(nil, err)` whenever `ListAllPanesWithFormat` returns a non-nil error (mode (a) closed at the source), and continues to return `(parsePaneOutput(raw), nil)` on the success path. The docstring at `internal/tmux/tmux.go:683-686` no longer describes the swallow behaviour. Downstream callers in Phase 2 will consume the new contract; non-empty live-set tests in `cmd/clean_test.go` remain green because the success-path shape is unchanged.

**Do**:
1. Open `internal/tmux/tmux.go` and locate `ListAllPanes` at lines 683-693.
2. Replace the body so it delegates:
   ```go
   func (c *Client) ListAllPanes() ([]string, error) {
       raw, err := c.ListAllPanesWithFormat("#{session_name}:#{window_index}.#{pane_index}")
       if err != nil {
           return nil, err
       }
       return parsePaneOutput(raw), nil
   }
   ```
3. Rewrite the docstring (lines 683-686). Remove the existing "Returns an empty slice and nil error when no tmux server is running." sentence. Replace the docstring with prose describing: (a) the helper enumerates live panes via the error-propagating `ListAllPanesWithFormat` helper using the canonical structural-key format `"#{session_name}:#{window_index}.#{pane_index}"` (matching `(*tmux.Client).ResolveStructuralKey` output and `hooks.json` keys); (b) on `tmux` failure it returns `(nil, err)`; (c) on success it returns the parsed structural-key slice; (d) callers decide policy for empty/error results (this helper does not paper over them).
4. Update the existing `TestListAllPanes` subtest `"returns empty slice when no tmux server running"` at `internal/tmux/tmux_test.go:1461-1473`. The subtest currently asserts `(empty, nil)` on `Commander` error. Flip the assertion: the test should now assert that `err` is non-nil and the returned slice is `nil`, and the test name should be updated to reflect the new contract (e.g. `"returns error when underlying commander fails"`). Preserve the `MockCommander{Err: ...}` setup shape so the structural coverage of "what happens on commander error" is retained — only the asserted outcome flips, mirroring the structural-preserve-flip-assert pattern referenced in the spec (commit `7e33c04b`).
5. Confirm the remaining `TestListAllPanes` subtests at `internal/tmux/tmux_test.go:1378-1504` still pass — the multi-session, multi-window, colons-in-name, dots-in-name, empty-output, and call-args subtests should be unaffected because they exercise the success path.

**Acceptance Criteria**:
- [ ] `ListAllPanes` body delegates to `ListAllPanesWithFormat("#{session_name}:#{window_index}.#{pane_index}")` and returns `(nil, err)` on non-nil helper error, `(parsePaneOutput(raw), nil)` otherwise
- [ ] Docstring rewritten: no mention of "no tmux server" convenience; describes the error-propagating contract and the canonical structural-key format
- [ ] Existing `TestListAllPanes` subtest at lines 1461-1473 is inverted to assert `(nil, non-nil err)` on commander error; renamed appropriately
- [ ] `go test ./internal/tmux/...` is green
- [ ] `go test ./...` is green (no downstream breakage — the two production consumers in `cmd/bootstrap_production.go` and `cmd/clean.go` already check `err` and treat `nil`/empty slices identically for range/len, per the spec's audit)

**Tests**:
- `"it returns (nil, err) when the underlying Commander returns exit ≠ 0"` — `MockCommander{Err: fmt.Errorf("...")}` produces non-nil error and nil slice (inverted from the existing `"returns empty slice when no tmux server running"` subtest)
- `"it returns structural keys across multiple sessions"` — preserved success-path subtest at line 1379 still passes
- `"it handles session names with colons"` — preserved subtest at line 1421 still passes (regression coverage for `parsePaneOutput` behaviour through the new delegation)
- `"it handles session names with dots"` — preserved subtest at line 1442 still passes
- `"it calls list-panes with -a flag and structural key format"` — preserved subtest at line 1489 still passes; verifies the format string is unchanged after delegation

**Edge Cases**:
- Underlying `Commander` returns exit ≠ 0 — must surface as `(nil, non-nil err)` not `(empty, nil)`.
- Wrapped tmux transport error from `ListAllPanesWithFormat` (which already wraps with `"failed to list panes: %w"`) — propagates through unchanged so callers can `errors.Is`/`errors.As` against any sentinel in the chain.
- Empty-stdout legitimate-no-panes case is **out of scope for this task** — Task 1.2 pins it explicitly.
- Docstring framing no longer mentions the swallow; future readers cannot rely on the prior convenience contract.

**Context**:
> From the specification (Change 1, lines 125-150 and Layer 1, lines 64-76):
>
> The conflation in `ListAllPanes` is a documented, intentional behavioural divergence. The cost of that divergence is exactly this bug.
>
> Disposition rationale (locked): repurpose, not delete or deprecate.
> - Deletion forces every call site (production and test) to be touched in this work unit; high blast radius for a contract-narrowing change.
> - Deprecation with `// Deprecated:` keeps the footgun alive — the compiler does not enforce the tag.
> - Repurpose structurally eliminates the swallow contract while keeping every existing call site compiling unchanged (the slice consumers compile through; only the error-path runtime shape narrows). New consumers inherit the safe behaviour by default.
>
> Format-string alignment: `"#{session_name}:#{window_index}.#{pane_index}"` matches the canonical structural-key form produced by `(*tmux.Client).ResolveStructuralKey` (used at hook-registration time). Hook entries in `hooks.json` are keyed by structural key, so the comparison in `hooks.Store.CleanStale` operates on identical-format strings on both sides.
>
> Helper docstring rewrite: describe what the helper actually does now — enumerate live panes via the error-propagating `ListAllPanesWithFormat` helper, return `(nil, err)` on tmux failure, and let the caller decide policy for empty/error results.
>
> Return-value contract change: pre-fix returns `([]string{}, nil)` on error; post-fix returns `(nil, err)`. The Defect Class Scope audit confirms only two production consumers (`cmd/bootstrap_production.go:76-83` and `cmd/clean.go:75-91`) exist, both modified by this work unit (Phase 2); both check `err` and treat `nil`/empty slices identically for range/len operations, so the contract shift is safe across the audited set.

**Spec Reference**: `.workflows/bootstrap-cleanstale-wipes-hooks-on-tmux-transient/specification/bootstrap-cleanstale-wipes-hooks-on-tmux-transient/specification.md` (Change 1 — Repurpose `ListAllPanes` to Wrap the Error-Propagating Helper; Layer 1 — Helper Swallow)

## bootstrap-cleanstale-wipes-hooks-on-tmux-transient-1-2 | approved

### Task 1.2: Pin the legitimate-empty contract for `ListAllPanes`

**Problem**: After Task 1.1, `ListAllPanes` propagates commander errors. But the second post-fix contract — "exit 0 with empty stdout still means an authoritative empty live set" — is not explicitly pinned by a unit test. Without it, a future change to `parsePaneOutput` or to `ListAllPanesWithFormat` could silently regress the distinguishability between "tmux failed" (mode (a)) and "no panes exist" (mode (b)) that the entire defense-in-depth fix depends on. Phase 2's hazard guard at the consumer layer is the only thing that closes mode (b); for the guard to be reachable, `ListAllPanes` must still return `([]string{}, nil)` on legitimate-empty so the consumer sees `len(livePanes) == 0` with no error to short-circuit on.

**Solution**: Add a focused unit subtest in `internal/tmux/tmux_test.go` (inside the existing `TestListAllPanes` block) that exercises the legitimate-empty path explicitly: when the underlying `Commander` returns exit 0 with empty stdout (or whitespace-only stdout), `ListAllPanes` must return `([]string{}, nil)`. This locks the contract that distinguishes mode (a) from mode (b) at the helper boundary so Phase 2's hazard guard remains the unambiguous closer of mode (b).

**Outcome**: A subtest in `TestListAllPanes` asserts the legitimate-empty contract for both empty stdout and whitespace-only stdout. The test passes against Task 1.1's implementation (the success-path delegation through `parsePaneOutput` already produces this shape). Future regressions to either helper or parser are caught at the unit boundary.

**Do**:
1. Confirm the pre-existing subtest `"returns empty slice when output is empty"` at `internal/tmux/tmux_test.go:1475-1487` already covers the empty-stdout case after Task 1.1's delegation. If it does (it should — `MockCommander{Output: ""}` causes `ListAllPanesWithFormat` to return `("", nil)` and `parsePaneOutput("")` returns `[]string{}`), preserve it and proceed to step 2.
2. Add a new subtest `"returns empty slice when output is whitespace-only"` inside `TestListAllPanes` (insert near line 1488, after the empty-output subtest and before `"calls list-panes with -a flag..."`):
   - `MockCommander{Output: "  \n\n\t\n "}` — exit 0, whitespace-only stdout.
   - Assert `err == nil`.
   - Assert `got` is non-nil but `len(got) == 0` (the slice should coerce to empty via `parsePaneOutput`'s `strings.TrimSpace` + skip-empty logic at `internal/tmux/tmux.go:458-465`).
3. Add an explicit pin assertion alongside the existing empty-output subtest comment block: a short comment (1-2 lines) above the subtest stating that this contract — "exit 0 + empty stdout ⇒ `([]string{}, nil)`" — is the distinguishability boundary that lets Phase 2's hazard guard (in `cleanStaleAdapter` / `portal clean`) close failure mode (b). This is the only place in the codebase where the contract is asserted; the comment self-documents why the subtest must not be deleted.
4. Run `go test ./internal/tmux/... -run TestListAllPanes -v` and confirm both empty-stdout and whitespace-only subtests pass.

**Acceptance Criteria**:
- [ ] Subtest `"returns empty slice when output is empty"` at `internal/tmux/tmux_test.go:1475-1487` continues to pass after Task 1.1's delegation and is annotated with a short comment naming the mode-(a)-vs-mode-(b) distinguishability contract
- [ ] New subtest `"returns empty slice when output is whitespace-only"` exists, exercises `MockCommander{Output: "  \n\n\t\n "}`, and asserts `(non-nil-but-empty-slice, nil)`
- [ ] `go test ./internal/tmux/...` is green
- [ ] `go test ./...` is green

**Tests**:
- `"it returns ([]string{}, nil) when list-panes -a exits 0 with empty stdout"` — the pre-existing subtest, preserved and annotated
- `"it returns ([]string{}, nil) when list-panes -a exits 0 with whitespace-only stdout"` — new subtest covering the trim-and-skip parser path
- (regression) `"it returns (nil, err) when the underlying commander fails"` — must remain distinguishable from the legitimate-empty subtests above; if it does not, the mode-(a)/mode-(b) boundary has regressed

**Edge Cases**:
- `list-panes -a` exit 0 with empty stdout — explicitly pinned as the legitimate-empty contract.
- Whitespace-only stdout — coerces to empty slice via `parsePaneOutput` trim/skip logic; pinned to prevent regressions if `parsePaneOutput` is ever rewritten.
- Single trailing newline (`"\n"`) — already covered transitively by the whitespace-only assertion; no separate subtest required.

**Context**:
> From the specification (Failure Modes Covered, lines 26-30; Closing Both Failure Modes, lines 222-229):
>
> The fix must close both failure modes that produce the destructive end-state:
> - **(a)** `list-panes -a` exit ≠ 0 (transient tmux failure during saver-respawn or under load) — evidenced by the observed Component B WARN.
> - **(b)** `list-panes -a` exit 0 with empty stdout (saver mid-respawn momentary "no panes" reply) — plausible but unobserved; precautionary coverage.
>
> | Failure mode | Closed by |
> |---|---|
> | (a) `list-panes -a` exit ≠ 0 | Change 1 (helper propagates error → adapter returns it as soft warning) |
> | (b) `list-panes -a` exit 0 with empty stdout | Change 3 (hazard guard refuses wipe when `len(live)==0 && len(persisted)>0`) |
>
> Either change alone leaves one failure mode open. Both are required.
>
> Task 1.2's pinned-contract test is the helper-boundary half of this: Phase 2's hazard guard (Change 3) cannot close mode (b) unless `ListAllPanes` reliably returns `([]string{}, nil)` on exit 0 with empty stdout. The unit subtest locks that contract so future helper refactors cannot silently regress the boundary.

**Spec Reference**: `.workflows/bootstrap-cleanstale-wipes-hooks-on-tmux-transient/specification/bootstrap-cleanstale-wipes-hooks-on-tmux-transient/specification.md` (Failure Modes Covered; Closing Both Failure Modes; Change 1 — return-value contract change)
