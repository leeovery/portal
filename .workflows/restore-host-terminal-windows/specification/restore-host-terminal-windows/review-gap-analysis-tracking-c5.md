---
status: in-progress
created: 2026-07-11
cycle: 5
phase: Gap Analysis
topic: restore-host-terminal-windows
---

# Review Tracking: restore-host-terminal-windows - Gap Analysis

## Findings

### 1. Multi-select selection semantics in By-Tag mode (multi-row sessions) undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Multi-Select Mode (Granularity / Sticky selection) × Session grouping (By Tag = Pattern B)

**Details**:
The spec fixes marking as "per-session only" (§Multi-Select) and describes selection as sticky across regrouping. But in By Tag mode the codebase's grouping is Pattern B — "one item per (session, tag) pair," so a multi-tag session renders as **multiple rows** under multiple headings. The spec never reconciles per-session marking with multi-row rendering:

- When the user `m`-toggles one row of a multi-tag session, is the *session* marked (so all its rows across headings show the `●`), or only that one (session, tag) row?
- Does the `N selected` banner count that session once or once per rendered row?
- If a session is marked under one tag heading and the user regroups/filters, does the mark survive correctly given the session's identity spans multiple list items?

An implementer building the selection model (a set keyed by what — session identity or list-item identity?) must resolve this, and the visual (`●` on all instances vs one) is a concrete rendering decision. This is the most direct multi-select × grouping integration gap.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 2. Intra-config match precedence undefined when multiple `terminals.json` entries match one identity

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Config Schema (Structure / Precedence) × Adapter Contract (Resolution precedence)

**Details**:
The spec defines precedence *between layers* (config override → native adapter → unsupported) but not *within* config. A single detected identity can be matched by several config entries simultaneously: a friendly alias (`ghostty`), the exact raw bundle id (`com.mitchellh.ghostty`), a `.app` name, and a `*`-glob (`dev.warp.Warp-*`, or a bare `*` catch-all). The `*`-glob is explicitly a supported key form alongside specific keys, so overlapping matches are not hypothetical.

Which entry wins is unspecified — most-specific-wins, exact-before-glob, file order, or first-match? An implementer must invent an ordering, and different choices produce different behaviour (e.g. a user's `*` fallback silently shadowing their specific override, or vice-versa). Config resolution cannot be built without this rule.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 3. Config-recipe failure classification into the typed taxonomy unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Config Schema (Recipe execution contract) × Permissions & Error Quarantine (typed result taxonomy)

**Details**:
The Permissions section places all OS-specific error mapping (`-1712`/`-1743` → `permission-required`) "inside the terminal driver and nowhere else," and the built-in Go Ghostty adapter does this. But §Adapter Contract treats user-config entries as a second adapter implementation, and a config recipe is a generic argv/script runner — Portal cannot recognise AppleEvent codes in the output of an arbitrary `argv` (which may not even be `osascript`). The spec never states how a config recipe's outcome maps into the `permission-required` / `spawn-failed` taxonomy:

- Does Portal check the recipe's exit code (non-zero → `spawn-failed`), or does it always report "spawn accepted" and rely solely on the ack-timeout to classify failure?
- Can a config-terminal ever surface the `permission-required` guidance path, or is that native-adapter-only? (If config recipes can only ever time out or `spawn-failed`, that should be stated — it affects the within-burst "stops the burst" behaviour, which is keyed on `permission-required`.)

This is a genuine Config × Permissions integration seam an implementer must resolve to build the config-entry adapter.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 4. Env-injection set beyond PATH, and the TMUX-must-be-absent invariant, unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Spawn Architecture (Spawned-window environment) × Trigger-Context Matrix (spawned N−1 always out of tmux)

**Details**:
The composed command is `/usr/bin/env PATH=<picker's full PATH> [any other required vars] <exe> attach <session> --spawn-ack <batch>`. Two things are left open:

1. **`[any other required vars]` is a literal placeholder** — an implementer cannot enumerate what (if anything) beyond PATH must be injected. If the intent is "PATH only," say so; if extensible, name the set.
2. **The correctness of "spawned N−1 are always fresh host windows running `portal attach` out of tmux" (§Trigger-Context) depends on `TMUX` being absent from the spawned command's environment.** When the picker triggers from *inside* tmux, its own process has `TMUX` set. The composed command is currently safe only by construction (it names PATH explicitly and the host terminal's base env is bare), but the spec's "env-self-sufficient" framing plus the open-ended `[any other required vars]` invites an implementer to snapshot the picker's whole environment — which would leak `TMUX` and break inside-tmux spawns (the spawned window would take the switch-client path with no client). The invariant "inject the minimal set; never propagate `TMUX`" should be stated explicitly.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 5. `portal spawn` CLI reporting and exit-code semantics for the non-`--detect` path unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Spawn Architecture (`portal spawn` CLI behaviour) × Burst & Partial-Failure Contract × Testing Strategy

**Details**:
The CLI is a first-class surface and "the test seam." Its `--detect` output is specified (prints friendly name + bundle id, opens nothing) and the no-args usage error is specified. But the CLI mirrors "the identical pre-flight → sequential spawn → per-window ack → self-attach-last flow," and every user-facing outcome of that flow is specified only for the *picker* (in-TUI banners: pre-flight abort line, spawn-failure line, permission guidance, unsupported banner). The CLI has no TUI. Unspecified:

- What the CLI writes to stdout/stderr on pre-flight abort, partial spawn failure, `permission-required`, and unsupported/NULL terminal.
- Its exit codes for each outcome (success self-execs away; abort/failure stays — with what exit status?).

Because this is the automated test seam, exit codes and stderr are exactly what tests assert on, so their omission materially blocks building the CLI and its tests.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 6. Input handling during the async in-picker burst underspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Burst & Partial-Failure Contract (In-picker execution model)

**Details**:
The burst runs as an async `tea.Cmd` that keeps the TUI "responsive" with "cancellation points live." The spec defines cancel (`Ctrl-C`/`Esc`) behaviour mid-burst, but not what happens to *other* keys while the multi-second burst (up to ~`spawnAckTimeout` per window) is pending:

- Are marking (`m`), navigation, `Space` preview, `/`, `s`, and a second `Enter` accepted or frozen during the pending state?
- The completion handler mutates the selection (unmarks sessions whose windows opened, keeps failed ones marked). If the user also toggles marks concurrently, the final selection is a race between the handler and user input — the "retry re-opens only what's missing" guarantee depends on a well-defined selection state at completion.

An implementer must decide whether to input-lock the model (except cancel) during the pending burst. This is a design decision the spec leaves open.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 7. N≥2 Enter behaviour when detection is still in-flight (not yet cached) undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Terminal Identity & Detection (Detection lifecycle) × Unsupported-terminal behaviour (N≥2 Enter gate)

**Details**:
The new detection-lifecycle subsection makes detection **asynchronous** (runs on Sessions-page entry, resolves "tens of ms" later, banner appears when it resolves) and the N≥2 Enter gate "reads the cached identity." The spec covers two resolved states (a matched identity, and clean NULL/unsupported) plus transient error, but not the **in-flight** state: if a fast user enters multi-select, marks ≥2, and presses Enter before the async walk has resolved, the cached identity is neither a valid adapter nor a resolved NULL — it is simply not-yet-set. The gate has no defined behaviour for that (block/wait for resolution, treat as unsupported no-op, or proceed and fail). Rare given the keystrokes required, but it is a concrete unhandled state at the detection→spawn seam the async lifecycle introduces, and the implementer needs an explicit code path (distinguishing "in-flight" from "resolved NULL").

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 8. Notice-band single-slot arbiter precedence across all claimants not fully enumerated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Multi-Select Mode (Mode affordance / Filter sub-state) × Burst (in-burst feedback) × Detection (unsupported banner)

**Details**:
The single-slot notice-band now has many potential claimants: no-tags signpost, filter line, multi-select banner, in-burst pending affordance (`Opening n/N…`), pre-flight-abort / spawn-failure / permission-guidance error lines, and the unsupported-terminal banner. The spec specifies some transitions (filter time-shares by focus; multi-select banner "owns the slot while in mode"; errors "re-assert"), but not the full precedence. The notable unresolved pairing: on an **unsupported terminal**, entering multi-select mode — does the multi-select banner displace the unsupported banner (hiding the warning while the user marks, re-asserted only at the N≥2 Enter block)? A precedence/priority table across all claimants would remove the guesswork. Mostly polish since the primary flows are covered, but the unsupported-during-mode case is a real implicit decision.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 9. Ack-marker option name derivation from arbitrary session names unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Burst & Partial-Failure Contract (Ack channel) × Ack delivery & `portal attach` contract

**Details**:
The ack channel is the tmux server option `@portal-spawn-<batch>-<session>`, and the `<session>` component is a session name. Portal-created sessions are `{project}-{nanoid}`, but sessions are user-renameable (the `r` modal / external `rename-session`), so `<session>` may contain characters that are awkward or invalid inside a tmux user-option name (spaces, and the embedded `-` makes the `<batch>-<session>` boundary ambiguous if the name is ever parsed rather than reconstructed). The picker and the spawned `attach` both derive the marker name from the same `<session>` string (so they agree), but if that string yields an invalid option name, `set-option` fails → no marker → false ack-timeout → a spuriously "failed" window even though the attach succeeded. The spec should state an encoding/escaping rule (or restrict the marker key to a safe identity form, e.g. `@portal-id`).

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 10. `script` recipe execution mechanism (interpreter, exec bit, tilde) unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Config Schema (Recipe / Recipe execution contract)

**Details**:
For `script` recipes the spec says "a path to a file Portal executes" and "receives `{command}` as `$1`." The `$1` positional contract is clear, but the execution mechanism is not: direct exec (`exec.Command(scriptPath, command)` — requiring the file to be executable with a shebang) vs interpreted (`sh <scriptPath> <command>` — no exec bit needed but forcing POSIX sh). The example path is `~/.config/portal/terminals/myterm.sh`, implying `~` expansion, but tilde handling for the script path is also unstated. Wrong guesses cause a user's recipe to silently fail to launch. Small, but it is a user-authored escape-hatch surface where "it just doesn't work" is the failure mode.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 11. `opened` / `total` batch-summary count semantics ambiguous

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Observability & State Footprint (Attr keys) × Spawn Architecture (N vs N−1 split)

**Details**:
The enumerated attr keys include `opened` / `total` with the example `spawn: opened 11/14`. Given the N vs N−1 split (N−1 externally spawned + 1 self-attached trigger, and the trigger only self-attaches on all-confirm), the semantics are ambiguous: is `total` the marked N or the spawned N−1? Does `opened` count the self-attached trigger window (which, on the failure path, does not self-attach)? Because the attr set is presented as a closed, spec-governed contract, the count definition should be pinned so log summaries are consistent and assertable.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---
