# Specification: Ghostty Spawn Zero Windows

## Specification

## Context: Defect & Root Cause

**Defect.** Multi-window spawn via the **native Ghostty adapter** is entirely non-functional. Selecting ≥2 sessions in the picker and pressing Enter (or running `portal spawn <sessions…>`) opens **zero** host-terminal windows: the log reports `spawn: opened 0/N`, no `portal attach --spawn-ack` process ever starts, no permission-required line appears, and the trigger window never self-attaches. Reproduces 100% on native Ghostty 1.3.1 (the path taken when no `terminals.json` override exists).

**Root cause.** The adapter's AppleScript template (`internal/spawn/ghostty.go`) is written against a **non-existent scripting API**:

```applescript
tell application "Ghostty"
	set surfaceConfig to make new surface configuration with properties {command:"%s", wait after command:true}
	make new window with properties {configuration:surfaceConfig}
end tell
```

Ghostty 1.3.1's scripting dictionary has **no `make` command** and **no `with properties`** terminology (its Standard Suite is trimmed to `count`/`exists`/`quit`); `surface configuration` is a **record-type**, not a makeable class; and windows are created via the custom `new window` command's **`with configuration`** parameter. The script therefore **fails to compile** (osascript `-2741`), exits non-zero in milliseconds without opening anything, and `mapGhosttyResult` maps a non-zero exit whose output contains neither `-1743` nor `-1712` to **`SpawnFailed`**. Every external window in every burst fails identically → `opened 0/N`.

**Why it shipped.** This is a **regression away from a researched-and-recorded API**, not a guess at an unknown one — earlier feasibility research had already resolved the correct `new window with configuration` form, and the implementation drifted to the invalid `make new … with properties` idiom without re-validation. The in-code comment claiming the template was "validated (Ghostty 1.3.1)" is false. The only test exercising real osascript (`TestManual_OpenWindow_OpensRealGhosttyWindow`) is `//go:build manual` — compiled into neither the unit nor integration lane — so no automatable lane could catch a wrong template, and the feature reached tagged release 0.9.1 without live validation.

**Two rider defects** surfaced by (not caused by) the template bug, both in scope:
- **Rider #1:** the per-window osascript failure detail logs at DEBUG only, so at production-default INFO the log records *that* windows failed but never *why*.
- **Rider #2:** the partial-failure banner suffix "— others left open" is static and renders even when `opened=0` (nothing was left open), producing misleading copy.

---

## Fix 1 — Correct the Ghostty AppleScript template

**File:** `internal/spawn/ghostty.go`

Replace the invalid two-statement `make new … with properties {…}` template (`ghosttyScriptTemplate`) with the single-statement, sdef-correct form that passes a `surface configuration` record literal directly to `new window`'s `with configuration` parameter:

```applescript
tell application "Ghostty"
	new window with configuration {command:"%s", wait after command:true}
end tell
```

Requirements:

- The single `%s` remains the `ghosttyEmbed`-escaped, space-joined composed argv, supplied as a `fmt.Sprintf` format argument (a `%` in the payload stays inert).
- The record literal carries exactly the two fields the sdef defines on `surface configuration`: `command` (text) and `wait after command` (boolean `true` — keeps the window up after its command exits, the normal-detach lifecycle for a spawned session).
- No `make`, no `with properties`, no intermediate `set surfaceConfig` variable.
- Correct the false "validated (Ghostty 1.3.1)" comment so it no longer claims validation the template never had; the comment should describe the actual sdef-correct `new window with configuration` form.
- Re-verify `ghosttyEmbed` escaping holds under the relocated `%s`: the payload now sits inside the record literal's double-quoted `command:"…"` string — the same double-quoted AppleScript string context as before — so the backslash-before-quote escape order is expected to be unchanged. Confirm, don't assume.

Everything downstream of the template is correct and stays unchanged: `ghosttyOpenScript` / `ghosttyOpenArgv` (script → `osascript -e` argv), the `osascriptRunner` exec seam, and `mapGhosttyResult`'s outcome mapping (`-1743`/`-1712` → PermissionRequired; other non-zero exit → SpawnFailed; clean exit → Success). Once the template compiles and opens a window, a clean exit maps to Success and the burst proceeds normally (acks land, trigger self-attaches).

---

## Fix 2 (Rider #1) — Surface per-window failure reason at WARN

**File:** `internal/spawn/logemit.go` (`LogWindowResults`). Spec amendment to the closed `spawn` log catalog.

**Problem.** `LogWindowResults` emits every external-window record — success *and* failure — at `DEBUG` (message `"external window"`, attrs `session`/`ack`/`detail`). At production-default INFO the batch summary `opened 0/N` is visible but the per-window `detail` (the osascript error text — the actual diagnosis) is not. The root cause could only be found by reproducing osascript outside portal.

**Change.** In the per-window loop, split by outcome:

- A window that **failed** (`!r.Confirmed()`, i.e. `r.Ack` is `AckTimeout` or `AckFailed`) **and** whose `r.Result.Outcome` is **not** `OutcomePermissionRequired` → emit at **`WARN`** with a distinct message **`"external window failed"`**, carrying the existing closed attrs `session`, `ack`, `detail`.
- Every other window — a **confirmed** window, or the **permission-required** window (whose detail is already carried by the dedicated `permission required — nothing self-attached` INFO event) → emit at **`DEBUG`** with the existing message `"external window"`, attrs unchanged.

Rationale for the exclusions:
- **Permission window excluded** so it does not double-report — `LogPermission` (INFO) is the single authority for the permission case, and the CLI's permission arm calls `LogWindowResults` before it.
- **Distinct message string** (`"external window failed"`) rather than reusing `"external window"` at a higher level, so the failure is greppable and the same message string never appears at two levels.

**Catalog amendment (spec-governed).** The closed `spawn` component gains **one** new message string, `external window failed`, at **WARN**. It introduces **no new attr keys** — `session`, `ack`, `detail` are all already in the closed `spawn` vocabulary. The INFO batch summary (`opened N/N`) and all other spawn events are unchanged; the WARN is additive (a total-failure batch now logs both `opened 0/N` at INFO and one `external window failed` WARN per non-permission failed window).

**Parity.** Both surfaces emit per-window records through this same helper (picker via `LogBatchSummary` → `LogWindowResults`; CLI via `logSpawnSummary`/its permission arm → `LogWindowResults`), so the WARN behaviour is identical across CLI and picker with no per-caller divergence beyond the pre-existing permission-arm asymmetry.

---

## Working Notes
