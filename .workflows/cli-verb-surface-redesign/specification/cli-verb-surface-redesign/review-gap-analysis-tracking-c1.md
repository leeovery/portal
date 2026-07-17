---
status: in-progress
created: 2026-07-17
cycle: 1
phase: Gap Analysis
topic: CLI Verb Surface Redesign
---

# Review Tracking: CLI Verb Surface Redesign - Gap Analysis

## Findings

### 1. `os.Args` ordering-parse contract is underspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Multi-Target Burst Mechanics ("The trigger absorbs the first target"); `portal open` — Flags & Command Passthrough

**Details**:
The trigger-absorb rule hinges on "command-line order (left-to-right as typed — positionals and pins interleaved; the implementation reads `os.Args` rather than cobra's split positional/flag buckets to preserve true order)." This names an approach but leaves the parsing contract undefined, and a correct implementation depends entirely on that contract:

- How are pin flags paired to their values when scanning raw `os.Args`? Both `-s api` (space form) and `-s=api` / `--session=api` (equals form) must be recognized so the value is attributed to the right target *and* not mistaken for a positional target.
- Combined short flags (e.g. `-sf`) — are they allowed at all? If yes, how are they ordered?
- `--` terminates flag parsing and hands the rest to command passthrough; the scanner must stop treating tokens as targets past `--`.
- `-e <cmd>` and its value are not targets and must be excluded from the ordered target list (`open -e claude ~/new` → the sole target is `~/new`; `claude` is the command).
- How does the raw-`os.Args` order scan reconcile with cobra, which still needs to validate flags, enforce `-f` mutual exclusion, and reject unknowns? The spec implies two parsers (cobra for validation/values + a raw scan for ordering) but never states how they agree.

Without a defined argv-ordering contract, the implementer must design a mini argv parser from scratch and guess the flag/value/terminator handling — a load-bearing decision because it determines which target the trigger absorbs and whether the target set is even assembled correctly.

**Proposed Addition**:
Added an "Argv parsing contract (target ordering)" subsection: cobra owns validation/binding/mutual-exclusion; a raw `os.Args` scan recovers order under a fixed contract (both `-s api` and `-s=api` value forms recognized and attributed to the pin; `-e <cmd>` value excluded from targets; `--` terminates parsing; value pins written separately, no bundling; ordered list = positionals + pin-values in os.Args order; trigger takes the first).

**Resolution**: Approved
**Notes**: Auto-approved (routine completeness — standard argv parsing rules, no new design fork). Logged to spec.

---

### 2. Command passthrough (`-e` / `--`) is not wired into the burst window argv (or the trigger's local mint)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Multi-Target Burst Mechanics ("Burst exec-argv & mint responsibility"); `portal open` — Flags & Command Passthrough ("Command passthrough")

**Details**:
The command-passthrough section establishes that "the command is baked only into mint targets' invocations" and "The command runs in every minted target," including across a multi-target burst (`x ~/Code/skill* -- claude` = N new sessions each running claude, in N windows). But the "Burst exec-argv" section's per-surface argv templates omit the command entirely:

- Mint window argv is given as `portal open --path <literal-dir> --ack <batch>:<token>` — with no slot for `-e <cmd>` / `-- <cmd> args…`.

So an implementer wiring the burst has no spec for *where* the baked command rides the spawned window's argv (append `-e <cmd>` before `--ack`? append `-- <cmd> args`?), nor for how the multi-arg `--` form (which captures multiple tokens) is reconstructed into a spawned window argv versus the single-token `-e` form. The trigger surface has the parallel gap: when the first target is a mint target carrying a command, the trigger mints locally and connects (no spawned window) — the command must feed `CreateFromDir` / `QuickStart` on that local path, which the passthrough section covers semantically but the burst section does not tie to the trigger's connect path. The intersection of command-passthrough and burst mechanics is where the two sections meet and neither fully specifies it.

**Proposed Addition**:
Added burst-argv item 3: command rides mint windows only, appended as `-- <cmd> args…` after `--ack` (multi-token form subsumes single-string `-e`); attach windows never carry it; the trigger, when it's a mint target with a command, mints locally and feeds `CreateFromDir`/`QuickStart` — same path as a spawned mint window.

**Resolution**: Approved
**Notes**: Auto-approved (routine completeness — mechanical wiring of an already-decided rule). Logged to spec.

---

### 3. `doctor` exit-code / re-diagnosis contract is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `doctor` — Diagnostics & Repair

**Details**:
`doctor` is positioned as following the `brew doctor` / `flutter doctor` idiom and is a read-only health report with a defined check catalog. But the spec never states the process contract that scripts and humans depend on:

- Does `portal doctor` exit non-zero when it finds problems (the `brew doctor` convention), or always exit zero and rely on printed text? This is the difference between `doctor` being scriptable ("run it in a health check and branch on exit status") and being human-only.
- Does `doctor --fix` re-run the diagnosis after applying repairs, and what is its exit code — zero if repairs succeeded, non-zero if something remains unhealthy/unfixable?
- With `doctor` bootstrap-exempt and a server down, several catalog checks (daemon alive, `_portal-saver` up, hooks registered) will fail; the spec says a down server is "reported honestly" but does not say whether that constitutes an unhealthy (non-zero) result or just an informational line.

The concrete per-check probe is explicitly delegated to planning, which is fine — but the exit-code semantics are a spec-level behavioral contract, not a probe detail, and an implementer would have to guess it.

**Proposed Addition**:
Added an "Exit-code contract" subsection to `doctor`: exits 0 iff all checks pass, non-zero (1) on any problem (scriptable gate); down server counts as unhealthy → non-zero but is reported honestly/distinctly ("runtime not running — run `portal open`" vs. corruption); `--fix` re-runs the diagnosis after repairing and exits 0 iff healthy post-repair, non-zero if anything remains unhealthy/unfixable.

**Resolution**: Approved
**Notes**: Surfaced as a genuine undiscussed behavioral decision. User approved the recommendation (scriptable non-zero-on-problems, per the brew doctor idiom + the redesign's scriptability value). Logged to spec.

---

### 4. `resolve` log line: level, miss case, and pinned-flag emission are undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Grammar & Target Resolution ("Wrong-guess feedback — tmux is the receipt")

**Details**:
The spec locks the new `resolve` component and its attr keys (`target`, `domain`, `resolved_path`) but leaves several behavioral details that an implementer must pin:

- **Level unspecified.** The closed taxonomy has four levels with production default INFO. A per-invocation resolution decision line at INFO would appear on every `open` at the default level; at DEBUG it would be silent by default. The stated purpose ("reconstruct a confusing guess from `portal.log`") only works if the level is chosen deliberately — this is a real decision, not a detail.
- **Miss case.** When resolution hard-fails (`open blog` resolves to nothing), is a `resolve` line emitted? A hard miss is exactly the "confusing outcome" the line exists to explain, yet the `domain` attr vocabulary (session / path / alias / zoxide) has no miss/none value.
- **Pinned flags.** Do explicit pins (`-s`, `-p`, `-z`, `-a`) emit a `resolve` line, or only bare targets going through the guessing chain? Pins aren't guesses, but the attr `domain` covers all four domains, leaving it ambiguous.
- **Multi-target bursts.** Is one `resolve` line emitted per target in a burst? The spec phrases it in the singular ("`open` logs its resolution decision").

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 5. Trigger-absorb "or absent" branch is ambiguous / potentially misleading

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Multi-Target Burst Mechanics ("The trigger absorbs the first target")

**Details**:
The bullet reads: "If it's elsewhere in the set, or absent → the terminal moves to the first target, and the current session (if named) simply gets its own window like any other target."

This lumps two distinct branches under one consequence, and the consequence is only correct for one of them:

- **Current session is elsewhere in the set** (a non-first target) → correct: it gets its own window because it *is* a target.
- **Current session is absent from the set** (e.g. sitting in `foo`, running `open api web`) → the current session is NOT a target and must NOT get a window; the terminal just switches to the first target and `foo` is left as a detached session with no surface.

As written, the sentence appears to state that in *both* branches "the current session gets its own window," which would wrongly imply spawning a window for a non-target current session. Given "No current-session detection, no special-casing," the intended behavior is clearly the latter, but the grammar could mislead an implementer into special-casing the current session.

**Proposed Addition**:
Split the ambiguous "elsewhere, or absent" bullet into two: current session elsewhere-in-set → gets its own window because it is a target; current session absent-from-set → left as a detached session with no surface, NOT given a window. Reinforced "no current-session detection — it gets a window only when it appears in the target set."

**Resolution**: Approved
**Notes**: Auto-approved (clarity fix consistent with the already-decided "no current-session detection" rule; no new decision). Logged to spec.

---

### 6. Glob-detection step is absent from the "Target resolution precedence" section

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Grammar & Target Resolution ("Target resolution precedence"); Multi-Target Burst Mechanics ("Glob targets")

**Details**:
"Target resolution precedence" presents the resolution algorithm as a single fixed chain: exact session name → path → alias → zoxide, first match wins. But a separate section ("Glob targets") establishes that a bare target containing glob metacharacters (`*`, `?`, `[…]`) is "session-domain by construction" and "skip[s] path/alias/zoxide entirely." That is effectively a pre-step of the resolution algorithm (detect glob → session-glob domain; else run the precedence chain), yet it is not mentioned or cross-referenced in the precedence section itself. An implementer building the resolver from the precedence section alone would run glob targets through path/alias/zoxide, which contradicts the glob section. The full per-positional algorithm should be stated in one place so the ordering (glob detection relative to the precedence chain) is unambiguous.

**Proposed Addition**:
Restructured "Target resolution precedence" into two steps: (1) glob pre-check — glob metacharacters ⇒ session-domain, expand against live names, skip the chain, zero matches ⇒ hard fail; (2) otherwise the precedence chain session → path → alias → zoxide.

**Resolution**: Approved
**Notes**: Auto-approved (consistency fix — the glob decision already exists in Glob Targets; this states the full per-positional algorithm in one place). Logged to spec.

---

### 7. `uninstall` behavior when nothing is running (idempotency) and the `_portal-bootstrap` anchor are unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `uninstall` — Runtime-Only Teardown

**Details**:
`uninstall` is bootstrap-exempt ("would EnsureServer / RegisterHooks … and then immediately tear all of it down") so it observes raw state and starts nothing. Two in-scope edge cases are not covered:

- **Already-clean / no server.** If there is no running tmux server, or no `_portal-saver` daemon, or no registered hooks (nothing to remove), what does `uninstall` do — a graceful no-op, and does it still print the "runtime removed / config untouched" completion message, or a different "nothing to remove" message? Since it is meant to be safe and recoverable, idempotency is expected, but the output/behavior in the nothing-to-remove case is undefined.
- **`_portal-bootstrap` anchor.** The teardown "removes only Portal's tmux-server footprint: kills the `_portal-saver` daemon and unregisters the global tmux hooks." It does not say whether the load-bearing `_portal-bootstrap` anchor session (and any live user sessions) are left in place. Presumably left (consistent with "touches no sessions"), but stating it removes ambiguity about how complete "removes Portal's tmux-server footprint" is.

**Proposed Addition**:
Added two bullets to `uninstall`: (1) idempotent / nothing-to-remove — graceful no-op that still prints the completion message, never errors on already-clean state; (2) leaves all sessions in place — user sessions and the load-bearing `_portal-bootstrap` anchor are left running; "removes the tmux footprint" = daemon + global hooks only, not sessions.

**Resolution**: Approved
**Notes**: Auto-approved (in-scope edge cases; defaults consistent with "touches no sessions" and CLAUDE.md's "`_portal-bootstrap` never killed by production code"). Logged to spec.

---

### 8. "Never falls back to the picker" guarantee is stated only for `--session`/`--path`, not `-z`/`-a`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Flags & Command Passthrough ("Pinned-domain contract"); `attach` — Retired ("Spawned-window contract")

**Details**:
The "Pinned-domain contract — never falls back to the picker" section names only `--session` and `--path` when asserting the hard-fail / no-TUI-popup guarantee ("a spawned window or script must never pop a TUI"). The domain-pinning flags table already specifies hard-fail-on-miss for `-z` (no zoxide match / not installed) and `-a` (unknown key), which implies no picker fallback — but the explicit "never pop a TUI" guarantee, which is the load-bearing property for scripts and spawned windows, is not extended to `-z`/`-a` in prose. For consistency and to avoid an implementer wiring a picker fallback onto a `-z`/`-a` miss, the guarantee should be stated for all four domain pins (any explicit pin hard-fails and never pops the picker).

**Proposed Addition**:
Rewrote the pinned-domain contract to cover all four pins: "Every domain pin (`-s`, `-p`, `-z`, `-a`) hard-fails on unresolvable and never falls back to the TUI picker … Only bare positionals run the guessing chain; only `-f` opens the picker."

**Resolution**: Approved
**Notes**: Auto-approved (consistency fix — extends the load-bearing "never pop a TUI" guarantee already implied by the hard-fail table to `-z`/`-a`). Logged to spec.

---
