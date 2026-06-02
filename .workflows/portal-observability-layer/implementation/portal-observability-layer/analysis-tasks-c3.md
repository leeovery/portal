---
topic: portal-observability-layer
cycle: 3
total_proposed: 2
---
# Analysis Tasks: Portal Observability Layer (Cycle 3)

## Task 1: Extract the shared test capture-handler ("captureSink") into one leaf test-helper package
status: pending
severity: medium
sources: duplication

**Problem**: A near-identical in-process log-capturing `slog.Handler` ("captureSink") plus its `newCaptureLogger`/`newCaptureLoggerForComponent` constructor is independently re-authored in three packages — `cmd/logging_capture_test.go:51-115`, `internal/state/logging_capture_test.go:21-58`, and `internal/restore/logging_capture_test.go:18-79`. The shared ~40-line core is byte-for-byte identical across all three: a `struct{ mu sync.Mutex; lines []string }`, `Enabled` returning true unconditionally, `WithGroup` passthrough, a `Handle` that renders `<LEVEL> <msg> key=value...` (writes `r.Level.String()` + `" "` + `r.Message`, then ranges `r.Attrs` with `fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())`), a `body()` returning `strings.Join(s.lines, "\n")` under the mutex, and a `newCaptureLogger(t)` returning `slog.New(sink), sink`. The import block (`context`, `fmt`, `log/slog`, `strings`, `sync`, `testing`) is identical too. This is a canonical Rule-of-Three: the rendered body shape `<LEVEL> <msg> key=value` is the contract every consumer's substring assertions key on (state: 3+ test files, restore: 5+, cmd: 9+), yet it is hand-maintained in three places. A change to attr rendering (quoting, ordering, value formatting) must be applied three times or the three packages' tests silently diverge in what they accept.

**Solution**: Extract the common base — the capturing handler + `newCaptureLogger(t) (*slog.Logger, *Sink)` + `Sink.body()` — into one shared leaf test-helper package and have all three packages consume it. Leave the two genuine per-package divergences as thin wrappers/embedded fields layered on top of the shared base, not folded into it.

**Outcome**: One canonical declaration of the capture-handler base; the `<LEVEL> <msg> key=value` rendering contract changes in exactly one place. All existing substring/record assertions in cmd, state, and restore test surfaces continue to pass against the shared base, with the two package-specific extensions preserved as thin layers.

**Do**:
1. Create a new leaf test-helper package that imports nothing portal-internal — e.g. `internal/logtest` — OR a test-only file in `internal/log` (which every package already depends on and which owns `SetTestHandler`). Do NOT place it in `internal/portaltest`: that package already imports `internal/state`, so hosting the helper there would create an import cycle with `internal/state`'s own test surface.
2. Move the common base into the new package: the `struct{ mu sync.Mutex; lines []string }` sink, `Enabled` (returns true), `WithGroup` (passthrough), `Handle` (renders `<LEVEL> <msg> key=value...` exactly as today — `r.Level.String()` + `" "` + `r.Message`, then range `r.Attrs` with `fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())`), `body()` (`strings.Join(s.lines, "\n")` under the mutex), and `newCaptureLogger(t) (*slog.Logger, *Sink)`.
3. Repoint `internal/state`'s `state_test` external test package to import the shared helper, deleting its local copy in `internal/state/logging_capture_test.go`.
4. Repoint `internal/restore`'s `restore_test` external test package to import the shared helper. Preserve restore's genuine extension — the `records []capturedRecord` + `recordsWithMessage` (exact attr-key-set assertions) — as a thin wrapper or embedded field over the shared sink, NOT folded into the base.
5. Repoint the in-package `cmd` test surface (`package cmd` `*_test.go`) to import the shared helper, deleting its local copy in `cmd/logging_capture_test.go`. Preserve cmd's genuine extension — the `WithAttrs`/shared/bound component-rendering so the `.With("component", ...)` binding appears on every line (`newCaptureLoggerForComponent`) — as a thin wrapper or embedded field over the shared sink, NOT folded into the base.
6. Remove the now-redundant per-package "substring assertions keep working after the observability migration retyped every seam to *slog.Logger" justification comments where the base machinery used to live; carry a single statement of that rationale into the shared helper's doc comment.

**Acceptance Criteria**:
- The capture-handler base (sink struct, `Enabled`/`WithGroup`/`Handle`/`body`, `newCaptureLogger`) is declared in exactly one place.
- The new helper package (or test-only `internal/log` file) imports nothing portal-internal beyond `internal/log` itself, introducing no import cycle (verify `go build ./...` and `go test ./...` both succeed).
- The rendered body contract `<LEVEL> <msg> key=value` is byte-identical to the current output, so every existing substring assertion in cmd, state, and restore passes unchanged.
- cmd's component-binding extension (`newCaptureLoggerForComponent` / per-line `component=` rendering) and restore's `records`/`recordsWithMessage` exact-key-set extension both remain in their owning packages as thin wrappers/embedded fields over the shared sink.
- No production (non-`*_test.go`) code imports the new helper.

**Tests**:
- Run `go test ./cmd/... ./internal/state/... ./internal/restore/...` — all existing log-assertion tests pass against the shared base without modification to their assertion expectations.
- Verify no import cycle: `go vet ./...` / `go test ./...` compiles cleanly across the full tree.
- If a new helper package is added, include a minimal test exercising `newCaptureLogger` + `Handle` to lock the `<LEVEL> <msg> key=value` rendering contract in the helper's own package.

## Task 2: Resolve the signal-hydrate command's component attribution (decide-and-document)
status: pending
severity: low
sources: standards

**Problem**: The `portal state signal-hydrate` command's FIFO-signal diagnostics still render under the `hydrate` component, not `signal`, and the boundary is still undocumented (unresolved cycle-2 finding, carried into cycle 3 unchanged). `runSignalHydrate` wires `Logger: hydrateLogger` (`cmd/state_signal_hydrate.go:98`) and normalizes via `hydrateLoggerOrDefault` (`:41`), so its marker-enumeration WARN ("list skeleton markers failed", `:44`), per-session WARN ("list panes for session failed", `:50`), and per-FIFO-write WARN ("write fifo failed", `:61`) all render under `hydrate:`. Its structural sibling EagerSignalHydrate — same enumerate-markers-then-write-FIFO shape — correctly binds `log.For("signal")` (`cmd/bootstrap/eager_signal_hydrate.go:23`), as does the lower-level plumbing (`internal/state/signal_hydrate.go:26`). So `grep "signal:"` misses the hook-driven signaling path. A literal reading of the taxonomy is defensible (it scopes signal's plumbing to "in internal/state"; `runSignalHydrate` lives in `cmd/` with process_role `hydrate`) — which is why this is low and not a clear violation — but the cycle-2 recommendation to decide-and-document the boundary was not actioned: the taxonomy table (`specification.md:166`) was not amended (its existing parenthetical covers only the hydrate helper's exit-path lines, not the command's enumeration diagnostics) and no in-source note records the decision.

**Solution**: Pick one of the two options and make it explicit. This is a bounded decide-and-document item — keep it small. EITHER (a) re-attribute: route `runSignalHydrate`'s three enumeration/per-FIFO WARNs through `signal` so `grep "signal:"` is complete and the command matches its EagerSignalHydrate sibling; OR (b) keep `hydrate` (matching the command's process_role) and record the boundary as a deliberate decision via a one-line taxonomy note plus a brief in-source comment.

**Outcome**: The signal-vs-hydrate boundary for the hook-driven signal-hydrate command is a recorded, intentional decision rather than incidental drift — either `grep "signal:"` covers the hook-driven signaling path, or the documented reason it does not is discoverable in both the taxonomy and at the binding site.

**Do**:

If choosing option (a) — re-attribute to `signal`:
1. In `cmd/state_signal_hydrate.go`, change the command body wiring (`:98`) from `Logger: hydrateLogger` to the `signal` component logger (`log.For("signal")`, matching `cmd/bootstrap/eager_signal_hydrate.go:23`).
2. Update the `hydrateLoggerOrDefault` normalization at `:41` (and the surrounding doc comments at `:11-18`, `:27-39`) so the default falls back to the `signal` logger, keeping the field/comment language consistent with the new component.
3. Confirm the three WARN sites (`:44`, `:50`, `:61`) now render under `signal:`.

If choosing option (b) — keep `hydrate`, document the boundary:
1. Amend the `signal` row of the taxonomy table at `specification.md:166` with a one-line note: the `signal-hydrate` command's own enumeration diagnostics render under `hydrate` (matching its process_role) while only the lower-level FIFO send/receive plumbing in `internal/state` (and EagerSignalHydrate) renders under `signal`.
2. Add a brief in-source comment at the `Logger: hydrateLogger` binding site (`cmd/state_signal_hydrate.go:98`, or above `runSignalHydrate` at `:40`) recording that the `hydrate` attribution is a deliberate, taxonomy-recorded decision (not drift) so the boundary is discoverable at the binding site.

**Acceptance Criteria**:
- A single, explicit choice is made (option a or option b) — the command's three enumeration/per-FIFO WARNs are no longer ambiguously attributed.
- If (a): `runSignalHydrate`'s "list skeleton markers failed", "list panes for session failed", and "write fifo failed" WARNs render under `signal:`, matching EagerSignalHydrate; `grep "signal:"` now covers the hook-driven signaling path.
- If (b): the taxonomy `signal` row at `specification.md:166` carries the one-line boundary note AND an in-source comment at the binding site records the decision.
- No new component is introduced; the change is confined to a re-binding (a) or a doc/comment note (b) — no behavioural change to signaling itself.

**Tests**:
- If (a): update/extend the existing `runSignalHydrate` test(s) that assert component attribution to expect `signal:` on the three WARN sites; confirm via the shared capture-handler that the rendered lines carry the `signal` component binding.
- If (b): no behavioural test change required; verify the spec note and in-source comment land via review. Confirm `go test ./cmd/...` still passes (no attribution assertion regressed).
