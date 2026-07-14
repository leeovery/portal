---
topic: restore-host-terminal-windows
cycle: 3
total_proposed: 8
---
# Analysis Tasks: restore-host-terminal-windows (Cycle 3)

## Task 1: Extract the section-header line-0 splice into one shared helper
status: pending
severity: medium
sources: duplication, architecture

**Problem**: The four-line idiom that swaps a freshly-rendered header line in for the first line of the `bubbles/list` view — `idx := strings.IndexByte(listView, '\n'); if idx < 0 { return header }; return header + listView[idx:]` — is repeated verbatim EIGHT times across the two section-header appliers: six in `applySectionHeader` (internal/tui/model.go:4707-4711 Opening band, 4732-4736 pre-flight abort banner, 4750-4754 multi-select `N selected` banner, 4771-4775 unsupported banner, 4784-4788 filter-query header, 4799-4803 standard header) and two in `applyProjectsSectionHeader` (model.go:4436-4440 filter-query, 4448-4452 standard). This work unit authored FOUR of the eight copies as it added new section-header claimants across separate tasks (Opening band 6-5, pre-flight abort 6-7, multi-select banner 5-3, proactive unsupported banner 6-2), each independently re-copying the snippet rather than reusing the two pre-existing copies — so the count crossed the Rule-of-Three three times over without extraction. Every branch now hand-maintains the "no-newline degenerate returns header bare, otherwise splice header onto listView from the first newline" contract — the one-row-per-delegate pagination invariant all the branches depend on — so a future tweak (a `\r\n` listView, a different degenerate fallback) must be applied in eight places or it silently diverges. (The architecture agent flagged the same repeated line-0 surgery while reviewing the sessions-page banner stack; the `strings.IndexByte` at model.go:4845 in `replaceListBodyWithNoMatches` is a DIFFERENT operation — keep-first-line, replace-body — and is NOT part of this cluster.)

**Solution**: Extract one small package-private helper — `func replaceHeaderLine(listView, header string) string` holding the `IndexByte` + degenerate-guard + splice — and have all eight branches return `replaceHeaderLine(listView, render…(…))`. Each claimant branch then collapses to its render call plus the shared splice, so the "swap, don't insert" invariant lives in exactly one place.

**Outcome**: The section-header row's swap-don't-insert contract is single-sourced; each of the eight claimant branches is its render call plus one shared splice; a future change to the splice (e.g. `\r\n` handling) is a single edit that cannot drift between the burst banners, the standard headers, and the Projects appliers.

**Do**:
1. In `internal/tui` (alongside `applySectionHeader` in model.go, or a small render-helper file) add `func replaceHeaderLine(listView, header string) string` reproducing the current idiom exactly: `idx := strings.IndexByte(listView, '\n'); if idx < 0 { return header }; return header + listView[idx:]`.
2. Rewrite the six `applySectionHeader` branches (Opening band, pre-flight abort, multi-select, unsupported, filter-query, standard) to `return replaceHeaderLine(listView, <the branch's render… call>)`, preserving each branch's existing guard/condition and render arguments unchanged.
3. Rewrite the two `applyProjectsSectionHeader` branches (filter-query, standard) the same way.
4. Leave `replaceListBodyWithNoMatches` (model.go:4845) untouched — it is a different keep-first-line/replace-body operation, not part of this cluster.

**Acceptance Criteria**:
- The `idx := strings.IndexByte(listView, '\n')` + degenerate-guard + splice appears exactly once (inside `replaceHeaderLine`); no `applySectionHeader` / `applyProjectsSectionHeader` branch hand-rolls it.
- All eight branches preserve their existing claimant conditions, precedence order, and render arguments — the rendered section-header output for every branch is byte-identical to today.
- `replaceListBodyWithNoMatches` is unchanged.
- The one-row-per-delegate pagination invariant holds (existing grouped/filter/banner render tests remain green).

**Tests**:
- Unit (tui): `replaceHeaderLine` over a multi-line `listView` (splices header + tail from first newline), a single-line `listView` with no `\n` (returns header bare), and an empty `listView`.
- Regression: the existing Opening-band / abort-banner / multi-select-banner / unsupported-banner / filter-query / standard-header and Projects section-header render assertions pass unchanged (byte-identical output).

## Task 2: Delete the four dead burst-outcome fields on Model
status: pending
severity: medium
sources: architecture

**Problem**: Four of the ~13 burst-lifecycle fields added to `Model` are dead (internal/tui/model.go:502-505). `burstBatch` and `burstResults` are only ever assigned their zero values in `resetBurstState` (`m.burstResults = nil`, `m.burstBatch = ""` at burst_progress.go:264-265) — never a real value anywhere. `burstIdentity` and `burstResolution` are written once in `dispatchBurst` (burst_progress.go:477-478) but never read: the terminal-outcome path (the `spawnCompleteMsg` arm, `burstAllConfirmed`, `emitBurstSummary`) reads the outcome from the `tea.Msg` (`msg.Batch/Results/Identity/Resolution`) and from `len(m.burstExternal)`, never from these model fields. So the burst outcome has two candidate homes — the message and the model fields — and only the message is live. The model doc comment even describes them as active, which worsens the trap: a future reader wiring logic to `m.burstResolution` would read stale/empty state that is only ever set at dispatch and never maintained. This is latent state on a god-object `Model` that already gained ~20 fields for this feature.

**Solution**: Delete the four unused fields (`burstBatch`, `burstResults`, `burstIdentity`, `burstResolution`), their two reset lines in `resetBurstState`, and their two write lines in `dispatchBurst`; the outcome already travels correctly on `spawnCompleteMsg`. Correct the model doc comment so it no longer describes the removed fields as active.

**Outcome**: The burst outcome has a single home (the `spawnCompleteMsg`); the `Model` sheds four dead fields and the misleading "active" doc comment, so "which burst fields are live" is answered by the code, not by grep against stale state.

**Do**:
1. Grep the repo for `burstBatch`, `burstResults`, `burstIdentity`, `burstResolution` to confirm the only references are the declarations (model.go:502-505), the two `resetBurstState` zeroing lines (burst_progress.go:264-265), the two `dispatchBurst` write lines (burst_progress.go:477-478), and the doc comment — no reader consumes them.
2. Remove the four field declarations and update the adjacent doc comment so it no longer lists `burstBatch`/`burstResults`/`burstIdentity`/`burstResolution` as active state.
3. Remove the `m.burstResults = nil` and `m.burstBatch = ""` lines from `resetBurstState`, and the `m.burstIdentity = …` / `m.burstResolution = …` lines from `dispatchBurst`.
4. Confirm `go build ./...` and the tui package compile with no dangling references; the live burst fields (`burstPending`/`burstPipe`/`burstCancel`/`burstTrigger`/`burstExternal`/`burstTotal`/`burstDone`/`burstCancelled`/`pendingBurstEnter`) and the `spawnCompleteMsg`-driven terminal path are untouched.
5. (Optional, not required for acceptance) consider grouping the remaining ~9 cohesive burst fields into a single `burstState` struct on `Model` so the sub-state-machine is one addressable unit — defer if it widens the change beyond dead-field removal.

**Acceptance Criteria**:
- `burstBatch`, `burstResults`, `burstIdentity`, and `burstResolution` are removed from `Model`, along with their `resetBurstState` and `dispatchBurst` assignments.
- No reference to any of the four symbols remains; `go build ./...` and `go test ./internal/tui/...` are green.
- The burst terminal-outcome behaviour (full-success self-attach, partial-failure flash, permission arm, cancellation) is unchanged — it already reads the outcome from `spawnCompleteMsg`.
- The `Model` doc comment no longer describes the removed fields as active.

**Tests**:
- Regression: the existing burst self-attach, partial-failure, permission, cancel, and input-lock suites pass unchanged (they already read outcome from the message, not the deleted fields).
- Build/compile: `go build ./...` green after removal.

## Task 3: Extract the left-bar single-glyph column renderer
status: pending
severity: low
sources: duplication

**Problem**: `renderMarkedLeftBarColumn` (internal/tui/session_item.go:387-390) and `renderGoneLeftBarColumn` (session_item.go:402-405) are byte-identical except for the glyph constant they render: `markerStyle.Render(<glyph>) + bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(<glyph>)))` (multiSelectMarker `●` vs flashWarningGlyph `⚠`). The selected branch of the pre-existing `renderLeftBarColumn` (session_item.go:370-376) renders the SAME shape for the `▌` selectorBar. This work unit added the two new copies across separate tasks — the marked-row `●` column (task 5-2) and the gone-row `⚠` column (task 6-7) — each hand-rolling the "render one glyph in the fixed 2-cell left-bar column and pad the remainder" logic that already existed for the selector bar, taking the pattern to three near-identical instances (Rule of Three met). The shared invariant is the `leftBarColumnWidth` (2-cell) geometry that keeps the name's left edge fixed regardless of which glyph occupies col 0; three independent copies mean a change to that column width or the pad rule must be mirrored three ways or the marked/gone/selected rows drift out of column alignment.

**Solution**: Extract a single glyph-column helper — `func renderLeftBarGlyphColumn(glyph string, glyphStyle, bg lipgloss.Style) string` returning `glyphStyle.Render(glyph) + bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(glyph)))` — and have `renderMarkedLeftBarColumn` (`●`), `renderGoneLeftBarColumn` (`⚠`), and `renderLeftBarColumn`'s selected branch (`▌`) all delegate to it.

**Outcome**: The 2-cell left-bar column geometry lives in one place; the marked/gone/selected columns share one pad rule and cannot drift out of alignment; the precedence switch in `renderSessionRow` (gone → marked → selector) is unchanged.

**Do**:
1. In `internal/tui/session_item.go` add `func renderLeftBarGlyphColumn(glyph string, glyphStyle, bg lipgloss.Style) string` returning `glyphStyle.Render(glyph) + bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(glyph)))`.
2. Rewrite `renderMarkedLeftBarColumn` to `return renderLeftBarGlyphColumn(multiSelectMarker, markerStyle, bg)` and `renderGoneLeftBarColumn` to `return renderLeftBarGlyphColumn(flashWarningGlyph, markerStyle, bg)`.
3. Rewrite `renderLeftBarColumn`'s selected branch to `return renderLeftBarGlyphColumn(selectorBar, selectorStyle, bg)`; leave the unselected branch (`bg.Render(padTo("", leftBarColumnWidth))`) as-is.
4. Keep each caller's existing style argument (marker/selector) and the gone → marked → selector precedence in `renderSessionRow` unchanged.

**Acceptance Criteria**:
- The `glyphStyle.Render(glyph) + bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(glyph)))` shape appears exactly once (inside `renderLeftBarGlyphColumn`); the marked, gone, and selected-selector columns all delegate to it.
- The rendered left-bar column for a marked (`●`), gone (`⚠`), and selected (`▌`) row is byte-identical to today, including the fixed 2-cell width and the name's left edge.
- The gone → marked → selector precedence in `renderSessionRow` is unchanged.

**Tests**:
- Unit (tui): `renderLeftBarGlyphColumn` for `●`, `⚠`, and `▌` produces the same 2-cell column (glyph + correct pad width) as the three originals for representative styles.
- Regression: existing session-row render assertions for marked, gone, and selected rows pass unchanged.

## Task 4: Unify the footer narrow-degrade fitter across the standard and multi-select footers
status: pending
severity: low
sources: duplication

**Problem**: `fitFilterCluster` (internal/tui/footer.go:186-222, added by this work unit task 5-4 for the multi-select footer) is a near-line-for-line copy of the pre-existing `fitLeftCluster` (footer.go:322-369) narrow-degrade algorithm: try the full cluster first and return it if it fits; otherwise greedily grow a leading prefix in a `for n := 1; n <= len(entries); n++` loop appending a `<cluster> · …` separator+ellipsis, breaking when the candidate width exceeds the budget; then fall back to the bare ellipsis if it fits, else an empty cluster. The two differ only in (a) the entry type ranged over (`filterFooterEntry` vs `keymapEntry`), (b) the cluster renderer called (`renderFilterCluster` vs `renderFooterCluster`), and (c) whether a right-anchor budget is reserved (`fitFilterCluster` uses the full width; `fitLeftCluster` subtracts the right anchor + one spacer). `fitFilterCluster`'s own doc-comment flags the relationship ("It mirrors fitLeftCluster … for the per-glyph filterFooterEntry cluster path") — the parallel is acknowledged and hand-kept. Only two instances (borderline against the Rule of Three), but each is a ~25-line block and the §2.7 narrow-degrade behaviour is exactly the layout invariant that should not be able to drift between the standard/Projects footers and the multi-select footer.

**Solution**: Parameterise the shared narrow-degrade loop over the cluster renderer and the budget — a package-private helper taking a `renderCluster func(n int) (string, int)` (render the first `n` entries, return cluster + width) plus the separator/ellipsis widths and a budget int, returning the fitted cluster + width — and have both `fitLeftCluster` and `fitFilterCluster` call it with their own renderer and budget. The per-type cluster renderers (`renderFilterCluster` / `renderFooterCluster`) stay separate; only the try-full-then-greedy-prefix-with-ellipsis algorithm is unified.

**Outcome**: The §2.7 narrow-degrade truncation rule lives in one place; the multi-select footer and the standard/Projects footers cannot drift on how they degrade on one line; the per-type cluster renderers and the right-anchor budget difference remain caller-supplied.

**Do**:
1. In `internal/tui/footer.go` add a package-private narrow-degrade helper — e.g. `func fitClusterToWidth(count int, w int, renderCluster func(n int) (string, int), sep, ellipsis string) (string, int)` — reproducing the current shared algorithm (full-first fast path via `renderCluster(count)`; the `for n := 1; n <= count; n++` greedy-prefix loop appending `sep`+`ellipsis`; the bare-ellipsis fallback; the empty-cluster final fallback), computing `sepWidth`/`ellipsisWidth` from the passed strings.
2. Rewrite `fitFilterCluster` to compute its budget (full width) and delegate to the helper, passing a closure that renders `entries[:n]` via `renderFilterCluster` and the `renderFooterDetail(footerEntrySeparator …)` / `renderFooterDetail(footerEllipsis …)` strings.
3. Rewrite `fitLeftCluster` to delegate the same way, passing its budget (full width minus the right anchor + spacer) and a closure over `renderFooterCluster` with its own separator/ellipsis strings.
4. Keep `renderFilterCluster` and `renderFooterCluster` separate; keep each fitter's budget computation (full vs right-anchor-reserved) at the caller.

**Acceptance Criteria**:
- The try-full-then-greedy-prefix-with-ellipsis loop exists exactly once (inside the shared helper); both `fitLeftCluster` and `fitFilterCluster` delegate to it.
- The per-type cluster renderers (`renderFilterCluster` / `renderFooterCluster`) and each fitter's budget computation (full width vs right-anchor-reserved) remain caller-owned and unchanged in behaviour.
- Fitted output (cluster + exact rendered width, always ≤ budget) for both footers is byte-identical to today across wide, narrow-degrade, single-entry-plus-ellipsis, and extreme-narrow cases.

**Tests**:
- Unit (tui): drive the shared helper (via both `fitLeftCluster` and `fitFilterCluster`) across a wide budget (full cluster), a narrow budget (prefix + `· …`), a budget fitting only the ellipsis, and a sub-ellipsis budget (empty) — asserting the returned width ≤ budget and matches the pre-refactor output.
- Regression: existing standard-footer, Projects-footer, and multi-select-footer narrow-degrade assertions pass unchanged.

## Task 5: Give Outcome a zero sentinel so a zero-value Result is not silently a success
status: pending
severity: low
sources: architecture

**Problem**: `OutcomeSuccess Outcome = iota` (internal/spawn/adapter.go:35-46) makes `0 == success`, so `Result{}.OK()` returns true (adapter.go:79-81) and would gate a self-attach as if a window opened. This is inconsistent with the SAME package's `RecipeKind` (`RecipeArgv RecipeKind = iota + 1`, zero = explicit invalid sentinel) and with the project's stated enum convention (start at 1 / zero = unset). Today every `Result` is built through the `Success`/`SpawnFailed`/`PermissionRequired` constructors and the "opened" partition keys off `Ack` (not `Result.OK`), so the practical blast radius is small — but `Burster.Run` does branch self-attach-await on `result.OK()`, so any future path yielding a zero `Result` (a map miss, a not-yet-populated field, a partially-built struct) would be classified as a clean success rather than a failure. Making "success" the accidental default of an outcome that gates an irreversible exec is the wrong-way-round default.

**Solution**: Introduce a zero sentinel — `OutcomeUnknown Outcome = iota` then `OutcomeSuccess`/`OutcomeSpawnFailed`/`OutcomePermissionRequired` (now 1/2/3) — so a zero-value `Result` fails `OK()` and is never mistaken for success, matching `RecipeKind`'s treatment in the same file. `OK()` stays `Outcome == OutcomeSuccess`.

**Outcome**: A zero-value `Result` reads as unknown, not success, so a map miss / partially-built struct can never silently gate a self-attach; the `Outcome` enum matches `RecipeKind`'s zero-is-invalid convention in the same package.

**Do**:
1. In `internal/spawn/adapter.go` insert `OutcomeUnknown Outcome = iota` as the first `const` member (zero value), with a doc comment stating it is the invalid/unset sentinel and that `OpenWindow` must never return it — mirroring `RecipeKind`'s zero-invalid treatment.
2. Renumber the existing members implicitly by placing `OutcomeSuccess`/`OutcomeSpawnFailed`/`OutcomePermissionRequired` after it (they become 1/2/3); keep the `Success`/`SpawnFailed`/`PermissionRequired` constructors and `OK()` (`Outcome == OutcomeSuccess`) unchanged.
3. Grep for any code depending on the numeric value of `OutcomeSuccess == 0` (there should be none — classification goes through `OK()` / `Confirmed()` / `FirstPermission`); confirm the burster, classify.go, and the adapters still compile and behave identically.
4. Confirm `Result{}.OK()` now returns false and no production path constructs a bare `Result{}` that relied on the old success-at-zero.

**Acceptance Criteria**:
- `Outcome`'s zero value is `OutcomeUnknown`; `Result{}.OK()` returns false.
- `OutcomeSuccess`/`OutcomeSpawnFailed`/`OutcomePermissionRequired` and their constructors behave identically; `OK()` is still `Outcome == OutcomeSuccess`.
- No code depends on the prior numeric value of any `Outcome` member; `go build ./...` and `go test ./internal/spawn/...` are green.
- The self-attach gate (`Burster.Run` branching on `result.OK()`) is unchanged for all constructed results.

**Tests**:
- Unit (spawn): `Result{}.OK()` is false (zero-value = unknown, not success); `Success(...).OK()` true; `SpawnFailed(...)`/`PermissionRequired(...)` `.OK()` false.
- Regression: existing adapter, classify, and burster tests pass unchanged.

## Task 6: Derive burstAllConfirmed from the shared PartitionResults chokepoint
status: pending
severity: low
sources: architecture

**Problem**: `classify.go` is explicitly framed as "the single count-semantics chokepoint" and documents that "A batch is all-confirmed precisely when the returned failed slice is empty." `burstAllConfirmed` (internal/tui/burst_progress.go:240-250) instead loops over `msg.Results` applying `!r.Confirmed()` itself. It reuses the shared `Confirmed()` predicate (so it is not fully independent) and adds a legitimate extra guard (`len(msg.Results) != len(m.burstExternal)`), but it does not derive all-confirmed from `PartitionResults`' `failed == empty` relationship the way the CLI path does. Given the package's stated obsession with a single drift-proof chokepoint, this is a small inconsistency: a future change to what "all confirmed" means would update `PartitionResults`' contract but silently leave this parallel loop behind. (This also captures the one concrete, not-already-shipped residual of the broader "orchestration sequence reimplemented per caller" observation — the picker's success gate should rest on the same chokepoint the CLI's does, verified by a cross-path parity test.)

**Solution**: Derive it from the chokepoint: `_, failed := spawn.PartitionResults(msg.Results); return msg.Err == nil && len(msg.Results) == len(m.burstExternal) && len(failed) == 0`, so the picker's success gate rests on the same `PartitionResults` relationship the CLI's does. Keep the existing `msg.Err` and length guards.

**Outcome**: The picker's all-confirmed gate is derived from the single count-semantics chokepoint, not a parallel `Confirmed()` loop; a future change to the "all confirmed" contract in `PartitionResults` is honoured on both the CLI and picker paths.

**Do**:
1. In `internal/tui/burst_progress.go`, rewrite `burstAllConfirmed`'s body to compute `_, failed := spawn.PartitionResults(msg.Results)` and return `msg.Err == nil && len(msg.Results) == len(m.burstExternal) && len(failed) == 0`, dropping the hand-rolled `for … if !r.Confirmed()` loop.
2. Preserve the existing early-return semantics (a non-nil `msg.Err` or a length mismatch yields false) — the rewritten single expression already covers both.
3. Leave the doc comment's N=1 reasoning intact (the length check still guards the vacuous case).

**Acceptance Criteria**:
- `burstAllConfirmed` derives all-confirmed from `spawn.PartitionResults(...)`'s `failed == empty` relationship; no residual `for … !r.Confirmed()` loop remains.
- The `msg.Err == nil` and `len(msg.Results) == len(m.burstExternal)` guards are preserved; the function's truth table is unchanged for all-confirmed, partial-failure, permission, error, and length-mismatch inputs.

**Tests**:
- Unit (tui): `burstAllConfirmed` returns true only for an error-free, full-length, all-`AckConfirmed` `spawnCompleteMsg`; false for any `AckTimeout`/`AckFailed` present, a `msg.Err`, or a length mismatch.
- Cross-caller parity: a shared fixture table of `[]spawn.WindowResult` reaches the same terminal classification (all-confirmed vs partial vs permission) on both the CLI (`cmd/spawn.go`) and picker paths, asserted against `spawn.PartitionResults` / `spawn.FirstPermission` so the two orchestrations cannot drift.
- Regression: existing burst full-success self-attach and partial-failure suites pass unchanged.

## Task 7: Route the spawn-ack write-failure DEBUG through the enumerated `detail` attr
status: pending
severity: low
sources: standards

**Problem**: The best-effort spawn-ack marker-write failure logs `spawnLogger.Debug("spawn-ack marker write failed", "session", name, "batch", ackBatch, "error", err)` (cmd/attach.go:64-68). The spec (§Observability → Attr keys) enumerates the closed `spawn` attr set as batch / terminal / bundle_id / resolution / session / ack / opened / total / detail, designating `detail` as the opaque OS-specific payload attr. This DEBUG line's `error` key is not in the spawn-specific enumeration, and CLAUDE.md states "never invent at call-site" for the closed taxonomy. Mitigating: `error` is an established cross-component attr used pervasively (internal/state, internal/log), so it is arguably drawn from the accepted vocabulary rather than invented — hence low impact — but for strict conformance the write failure should ride the spec-designated `detail` attr, matching how detect.go / logemit.go route opaque payloads.

**Solution**: Carry the write-failure payload via the spec-designated opaque `detail` attr (`"detail", err.Error()`) instead of `"error", err`, keeping the `spawn` component within its enumerated attr set. No behavioural change.

**Outcome**: The spawn-ack write-failure DEBUG stays within the closed `spawn` attr enumeration (`detail` for the opaque payload), consistent with detect.go / logemit.go; no out-of-catalog attr key on a `spawn`-component line.

**Do**:
1. In `cmd/attach.go` (~64-68) change the failed-write DEBUG's `"error", err` attr to `"detail", err.Error()`, keeping the `"session"` and `"batch"` attrs (both in the enumerated `spawn` set) and the message string unchanged.
2. Confirm the emission remains DEBUG-level and best-effort (still falls through to `Connect`, does not return) — behaviour is unchanged; only the attr key changes.
3. Verify no test asserts the old `error` attr key on this line; update any that do to `detail`.

**Acceptance Criteria**:
- The spawn-ack write-failure DEBUG emits only enumerated `spawn` attr keys (`session`, `batch`, `detail`); no `error` key on this `spawn`-component line.
- The line remains DEBUG, best-effort, and non-fatal (the write failure still falls through to `Connect`).
- The message string is unchanged.

**Tests**:
- Unit (cmd): induce an ack-write failure (fake `AckWriter` returning an error) and assert the DEBUG record carries `detail` (= the error text) and `session`/`batch`, with no `error` attr; the connector is still invoked.
- Regression: existing attach ack-path tests pass unchanged.

## Task 8: Fix the `--spawn-ack` flag help text delimiter label
status: pending
severity: low
sources: standards

**Problem**: The `--spawn-ack` flag help reads "internal: write the @portal-spawn-<batch>:<token> ack marker before attaching" (cmd/attach.go:93), using a colon between batch and token. The written server-option name is `@portal-spawn-<batch>-<token>` (hyphen — `SpawnMarkerName` in internal/spawn/ackid.go); the colon is only the flag-VALUE delimiter (`FormatSpawnAckFlag` → `"<batch>:<token>"`). The help text conflates the two delimiters. Cosmetic `--help` inaccuracy only; no functional impact.

**Solution**: Reword the help so it reflects the actual marker name (hyphen) and, if desired, the flag-value form (colon) — e.g. "internal: <batch>:<token> — write the @portal-spawn-<batch>-<token> ack marker before attaching".

**Outcome**: The `--spawn-ack` help text accurately labels the marker name (`@portal-spawn-<batch>-<token>`, hyphen) distinctly from the flag-value delimiter (`<batch>:<token>`, colon); `portal attach --help` no longer conflates the two.

**Do**:
1. In `cmd/attach.go:93` change the flag help string so the marker name uses the hyphen form `@portal-spawn-<batch>-<token>` (matching `SpawnMarkerName`), keeping the internal-only framing — e.g. `"internal: <batch>:<token> — write the @portal-spawn-<batch>-<token> ack marker before attaching"`.
2. Confirm the flag name, default (`""`), and parsing (`FormatSpawnAckFlag` colon-delimited value) are unchanged — text only.

**Acceptance Criteria**:
- The `--spawn-ack` help text names the marker `@portal-spawn-<batch>-<token>` (hyphen) and no longer implies a colon in the marker name.
- The flag name, default value, and value parsing are unchanged.

**Tests**:
- None required (help-text copy change; no behavioural assertion). Existing `cmd/attach` tests remain green.
