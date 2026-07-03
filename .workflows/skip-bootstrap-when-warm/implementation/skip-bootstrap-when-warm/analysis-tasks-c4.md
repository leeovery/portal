---
topic: skip-bootstrap-when-warm
cycle: 4
total_proposed: 1
---
# Analysis Tasks: Skip Bootstrap When Warm (Cycle 4)

## Task 1: Drop the non-vocabulary `marker` attr from the latch-write-failure WARN
status: approved
severity: low
sources: standards

**Problem**: `cmd/bootstrap/bootstrap.go:499` emits `o.Logger.Warn("latch write failed", "marker", state.BootstrappedMarkerName, "error", err)`, which mints `"marker"` as a slog attr key. A whole-tree search confirms `"marker"` appears as a log attr on no other production line. CLAUDE.md's logging contract declares the attr-key vocabulary closed — "New components/attrs require amending the spec — never invent at call-site" — and this feature's spec authorises only "a pure log line (WARN under the bootstrap component)", not a new attr key. The attr value is `state.BootstrappedMarkerName`, a compile-time constant (`"@portal-bootstrapped"`), so it carries no runtime information the message and the existing `"error"` attr do not already convey. This is the sole call-site-minted attr outlier in the feature; left in place it is a template a future contributor could copy, eroding the closed vocabulary.

**Solution**: Fold the latch marker name into the WARN message text and drop the `"marker"` attr, keeping `"error"`. For example: `o.Logger.Warn("latch write failed for "+state.BootstrappedMarkerName, "error", err)` (or a constant-composed equivalent). No new attr key is introduced; the only change is the shape of this one emitted line.

**Outcome**: The latch-write-failure WARN uses only closed-vocabulary attr keys (`error`, plus the handler-injected baselines `pid`/`version`/`process_role`). The `marker` key no longer appears anywhere in the production tree, removing the copy-template. The human-readable log line still names the specific latch. The path stays best-effort — failure is still swallowed and still logged at WARN under the bootstrap component.

**Do**:
1. Open `cmd/bootstrap/bootstrap.go` at the latch-write-failure WARN (~line 499).
2. Remove the `"marker", state.BootstrappedMarkerName` attr pair.
3. Incorporate the latch name into the message string so the specific latch is still identified — e.g. `"latch write failed for " + state.BootstrappedMarkerName`.
4. Leave the `"error", err` attr and the surrounding best-effort control flow unchanged.
5. Run a whole-tree search for `"marker"` used as an slog attr key in production code and confirm the count is now zero.

**Acceptance Criteria**:
- The WARN line emits no `marker` attr; the `"error"` attr is retained.
- A whole-tree search for `"marker"` as an slog attr key in production (non-test) code returns zero results.
- The emitted message still identifies the specific latch (`@portal-bootstrapped`).
- No behaviour change on any path beyond the shape of this single log line; the latch write remains best-effort (failure swallowed, WARN under the bootstrap component).

**Tests**:
- If an existing test asserts on the latch-write-failure WARN (message text or attr set), update it to the new message and the attr set without `marker`; otherwise no new test is required — this is a pure log-line-shape change on a best-effort path.
- All existing bootstrap / latch tests must remain green.
