---
status: in-progress
created: 2026-07-16
cycle: 2
phase: Gap Analysis
topic: ghostty-spawn-zero-windows
---

# Review Tracking: ghostty-spawn-zero-windows - Gap Analysis

## Findings

### 1. Fix 1 silently breaks an existing unit test the spec requires to stay green

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 1 (Correct the Ghostty AppleScript template); Testing & Validation Requirements ("Existing lanes stay green")

**Details**:
Fix 1 replaces the template with the single-statement form:

```
tell application "Ghostty"
	new window with configuration {command:"%s", wait after command:true}
end tell
```

This corrected template does **not** contain the substring `surface configuration` — that keyword only existed in the old, invalid `make new surface configuration` form (the record literal is now `{command:…, wait after command:true}`, and the parameter is `with configuration`, not `surface configuration`).

The existing unit test `internal/spawn/ghostty_command_test.go` → `TestGhosttyOpenScript` → subtest "it builds a surface configuration with a command property and new window" asserts `strings.Contains(script, "surface configuration")` among its `wants`. After Fix 1 that assertion fails, so the subtest fails on the **unit lane** (`go test ./...`).

The spec's Testing & Validation Requirements section explicitly requires "Existing lanes stay green. `go test ./...` (unit) … must both pass," and enumerates the tests to add/update (Fix 4 compile-check, Rider #1 logemit, Rider #2 parity) — but it never lists updating `TestGhosttyOpenScript`'s stale `surface configuration` assertion. This is an internal contradiction (a required-green lane that the change breaks) plus a missing lockstep task: an implementer following the spec would change the template, run the unit lane, and hit an unexplained failure with no guidance on which assertion is now stale versus which is a real regression.

A secondary instance of the same omission: Fix 2 (Rider #1) moves an `AckTimeout` window from DEBUG to WARN, which invalidates the existing `internal/spawn/logemit_test.go` assertion that expects `DEBUG external window … ack=timeout …`. That breakage is at least *implied* by the described new WARN behavior, but the spec still frames the Rider #1 testing note as "assert [new behaviors]" (additive) rather than "update the existing DEBUG-timeout assertion," leaving the lockstep edit implicit.

The corrective note is small: the Testing section should call out that existing assertions invalidated by the template/level changes must be updated in lockstep — concretely, `TestGhosttyOpenScript` should drop/replace the `surface configuration` expectation with the corrected terminology (e.g. `new window`, `with configuration`, and the still-present `command:` / `wait after command`), and the `logemit_test` timeout case must move to WARN. Without this, the "lanes stay green" acceptance criterion is unachievable as written.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---

### 2. Fix 4's compile-guard rests on an unstated assumption about `osacompile` terminology resolution

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix 4 (Compile-check regression guard); Testing & Validation Requirements

**Details**:
Fix 4 asserts that feeding `ghosttyOpenScript(...)` through `osacompile` "resolves the `tell application "Ghostty"` terminology against the installed Ghostty scripting dictionary and **opens no window / runs nothing**," and gates the test only on Ghostty **presence** (`t.Skip` when not macOS or when `Ghostty.app` is not present).

The guard's whole value — a non-disruptive, reliable tripwire — depends on two operational facts the spec states as settled but never actually pins down:

1. **Running state.** Resolving app-specific terminology (`new window with configuration`, the `surface configuration` record vocabulary) can, on macOS, require the target app to be *running* (AppleScript fetching the app's terminology). The skip condition keys on Ghostty being *installed*, not *running*. If resolution needs Ghostty running, a machine with Ghostty installed-but-not-running produces a compile failure unrelated to the template — a false-positive test failure indistinguishable from a genuine regression.
2. **Side effect.** If `osacompile` launches Ghostty (or its helper) to fetch terminology, the "opens no window / runs nothing" property is not strictly true, and the guard has an observable side effect on the developer's machine that the spec does not acknowledge.

The spec gives no fallback for either case (e.g. "ensure Ghostty is running first," or "skip if terminology cannot be resolved," or an alternative resolution path). This matters because the entire topic exists precisely because a scripting-tool behavior was assumed correct without live validation; asserting `osacompile`'s resolution semantics as fact — the same class of unvalidated assumption — reintroduces that risk into the very guard meant to prevent it. The gap is that the spec presents the mechanism as verified when it is an assumption an implementer must confirm before the guard can be trusted, and it does not state what the test should do if the assumption does not hold.

**Proposed Addition**:
{leave blank until discussed}

**Resolution**: Pending
**Notes**:

---
