---
status: in-progress
created: 2026-07-17
cycle: 2
phase: Gap Analysis
topic: CLI Verb Surface Redesign
---

# Review Tracking: CLI Verb Surface Redesign - Gap Analysis

## Findings

### 1. Trigger connects **last** in execution order — never disentangled from "trigger absorbs the first target"

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Multi-Target Burst Mechanics ("The trigger absorbs the first target"; "Burst exec-argv & mint responsibility"; "Atomic pre-flight & partial failure")

**Details**:
The spec pins two orderings that an implementer can easily conflate:

- **Target ordering** — "The trigger (invoking terminal) takes the first target in command-line order," and "The inside/outside-tmux split only selects the connector for the first-target surface (`switch-client` inside, `exec attach` outside)."
- **Execution ordering** — the sequence in which the N surfaces are actually opened.

The spec never states the execution ordering, but it is load-bearing: outside tmux the trigger's connector is `exec attach`, which **replaces the Portal process**. If the implementer reads "trigger takes the first target" as "connect the trigger first," the `exec` destroys the burster before the remaining N−1 windows are ever spawned — the burst silently opens only one surface. The correct behavior (spawn all N−1 windows first, then the trigger self-connects **last**) is only weakly implied by "the trigger's self-attach is skipped on failure" (which presupposes spawns already ran). The burst topic specifies exec-argv templates, per-window ack timeouts, and atomic pre-flight in detail, so the conspicuous absence of "trigger connects last / spawn the others first" is a genuine gap — the target the trigger *lands on* (first) and the point at which it *connects* (last) are two different orderings that must be stated separately.

**Proposed Addition**:
State the execution ordering explicitly in the burst mechanics: the N−1 non-trigger surfaces are spawned first; the trigger self-connects (`switch-client` / `exec attach`) **last**, after all spawns are issued and (per the partial-failure contract) their acks resolved — because the outside-tmux `exec attach` replaces the Portal process and would abort the burst if run before the spawns. Distinguish "trigger absorbs the first *target*" (which session it lands on) from "trigger connects *last*" (execution order).

**Resolution**: Approved
**Notes**: Auto-approved (single correct answer dictated by the outside-tmux `exec attach` process-replacement constraint; consistent with existing spawn "self-attach the Nth"). Logged to spec.

---

### 2. `doctor --fix` sweeps logs, but "logs" is not one of `doctor`'s diagnosed checks

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `doctor` — Diagnostics & Repair (check catalog; `--fix` repair list; "Exit-code contract")

**Details**:
`portal doctor --fix` is defined as performing "the low-stakes, reversible-by-reconstruction repairs **it diagnoses**" — an explicit "diagnose then repair the diagnosis" pairing. But the repair list is "prune stale hooks, prune stale projects, **sweep logs**," while the check catalog contains only: daemon alive; hooks registered without duplicates; `_portal-saver` up; state dir sane; `sessions.json` valid; no stale entries (dead-pane hooks, gone-dir projects); host terminal detected. There is **no "logs" check** in the catalog, yet `--fix` sweeps logs. So one of `--fix`'s three repairs is not tied to any diagnosed problem, contradicting the "repairs it diagnoses" framing.

An implementer cannot tell whether to (a) add a logs check to the catalog so `doctor` (no `--fix`) can report it and the exit-code contract can account for it, or (b) treat log-sweep as an unconditional side-action of `--fix` that is intentionally outside the diagnose→repair loop (and therefore never affects the "re-runs the diagnosis / exits 0 iff healthy post-repair" exit code). This also interacts with the exit-code contract: if log-sweep is not a check, a stale-log condition can never make `doctor` non-zero, which may or may not be intended.

**Proposed Addition**:
Reconcile the catalog and the `--fix` action list — either add an explicit log-retention check to the `doctor` catalog (so the sweep is a diagnosed repair like the others), or state that log-sweep is a deliberate unconditional maintenance side-action of `--fix` that is outside the diagnose→repair loop and does not participate in the exit-code contract. State which.

**Resolution**: Approved
**Notes**: Auto-approved with option (b) — log-sweep is an unconditional side-action outside the diagnose→repair loop, not in the exit-code contract. Faithful to the discussion's "log sweep is redundant" note (logs auto-rotate/retention-sweep, so there is no stale-logs health state). Logged to spec.

---

### 3. "Trigger's self-attach is skipped on failure" — the triggering condition and the outside-tmux consequence are underspecified for the CLI burst

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Multi-Target Burst Mechanics ("Atomic pre-flight & partial failure")

**Details**:
The partial-failure contract states "the trigger's self-attach is skipped on failure," but does not define **whose** failure gates the skip or **what the user experiences** afterward in the CLI:

- **Which failure?** The trigger's own first-target surface (an attach that vanished, or a local mint) is a distinct thing from a *spawned* N−1 window failing its ack. Reading literally, if any spawned window (e.g. the 3rd target) fails to ack, the trigger's self-attach to the *first* target is skipped — so `open api ~/a ~/b` would leave the user **not** attached to `api` merely because `~/b`'s window failed. That is surprising given the trigger's target succeeded and is independent of the failed window. It is unclear whether "on failure" means "any per-window failure in the burst" or "the trigger's own target failed."
- **Outside-tmux consequence.** Outside tmux the trigger self-attach is an `exec attach` (process replacement). "Skipping" it means the Portal process instead returns/exits — leaving the user back at the shell prompt with no surface attached. The spec never says this is the intended landing, nor whether an error is surfaced. (The picker's rationale for skipping — "failed surfaces stay marked so a retry re-opens exactly those" — is a *picker multi-select* concept with no equivalent in the CLI burst, so the contract does not transfer cleanly.)

**Proposed Addition**:
Define, for the CLI burst: (a) exactly which failure gates the skip — recommend the trigger self-connects whenever its **own** first-target surface resolved, independent of other windows' ack failures (its target is unrelated to theirs), with failed spawned windows reported but not blocking the trigger's landing; and (b) the outside-tmux consequence when the skip does apply (Portal returns to the shell without attaching; the failure is reported on stderr). Reconcile with the picker's "marked surfaces retry" note, which does not apply to the CLI.

**Resolution**: Pending
**Notes**:

---

### 4. `-f/--filter` picker page/mode is unspecified when no command is present

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Flags & Command Passthrough (`-f/--filter`); Grammar & Target Resolution ("Miss handling")

**Details**:
`-f/--filter <text>` "opens the picker pre-filled with `<text>`; skips resolution entirely," and it is the escape hatch the miss error points at (`nothing resolved for 'blog' — try -f blog`). The picker has distinct pages (Sessions / Projects), and the filter is a per-page query. The spec pins the page only for the **command** variant — "`-f <text> -e <cmd>` … filtered **Projects** picker" — but leaves the plain `-f <text>` (no command) case unspecified: does it land on the Sessions page, the Projects page, or something else, and is the pre-filter applied to session names, project directories, or both?

This is not cosmetic: the miss that sends a user to `-f` could have been an intended session *or* an intended directory, so which page they land on determines whether the escape hatch is useful. An implementer must currently guess (Sessions default? inherit the removed fallback's page?).

**Proposed Addition**:
State the page/mode plain `-f <text>` opens (recommend the default Sessions page, matching the removed implicit picker-with-filter fallback it replaces, with the user free to toggle to Projects via `x`), and that the pre-filter is applied to that page's filter. Keep the `-f … -e <cmd>` → Projects specialization as the stated exception.

**Resolution**: Approved
**Notes**: Auto-approved (faithful digest — Sessions is the removed fallback's default page; the `-e` → Projects exception was already decided). Logged to spec.

---

### 5. Atomic pre-flight abort — which failing target(s) are reported in a multi-target burst is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Grammar & Target Resolution ("Miss handling"); Multi-Target Burst Mechanics ("Atomic pre-flight & partial failure")

**Details**:
"Any target unresolvable ⇒ atomic abort: nothing opens, nothing is created," but the miss error is specified only in the singular form (`nothing resolved for 'blog' — try -f blog`). For a multi-target burst where two or more targets fail the read-only resolve, the spec does not say whether the abort reports the first unresolvable target, all of them, and what message form is used. The `-f`-based suggestion in the singular message also reads oddly in a multi-target context (`-f` is mutually exclusive with all targets, so it cannot re-run the surviving multi-target intent). An implementer would guess the aggregation and message shape.

**Proposed Addition**:
Specify multi-target abort reporting — e.g. report every unresolvable target (not just the first) so a single re-run fixes them all, and clarify whether the `-f` suggestion appears when there is more than one target (since `-f` cannot carry a multi-target intent).

**Resolution**: Approved
**Notes**: Auto-approved (completion — report every unresolvable target; `-f` hint only in the single-target case). Logged to spec.

---

### 6. A directory path containing glob metacharacters is only reachable via `-p` — not stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Grammar & Target Resolution ("Glob pre-check"); Multi-Target Burst Mechanics ("Glob targets")

**Details**:
The glob pre-check makes a bare token containing `*`, `?`, or `[…]` "session-domain by construction" and skips path/alias/zoxide entirely. A real on-disk directory whose name legitimately contains those characters (e.g. `~/tmp/foo[1]`, or a path a user quotes to protect from the shell) will therefore be routed to session-glob expansion, match zero live sessions, and hard-fail — never reaching path resolution. This in-scope edge (introduced by the redesign's own glob pre-check) has an escape hatch (`-p`, which pins path and skips the glob detection), but the spec never states that a glob-charactered path must be pinned with `-p`. An implementer/user would otherwise be surprised the path is unreachable as a bare target.

**Proposed Addition**:
Add a one-line note to the glob section: a directory path whose name contains glob metacharacters is unreachable as a bare positional (it is treated as a session glob); reach it with `-p <dir>`, which pins path and bypasses glob detection.

**Resolution**: Approved
**Notes**: Auto-approved (in-scope edge introduced by the redesign's own glob pre-check; escape hatch `-p` already exists). Logged to spec.

---

### 7. Single-string `-e` → multi-token `--` reconstruction for spawned mint windows is undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Multi-Target Burst Mechanics ("Burst exec-argv & mint responsibility", item 3); Flags & Command Passthrough ("Command passthrough")

**Details**:
Burst-argv item 3 bakes the command into each spawned mint window in the "multi-token passthrough form (which subsumes the single-string `-e` form)," e.g. `portal open --path <literal-dir> --ack <batch>:<token> -- <cmd> args…`. The word "subsumes" leaves the actual conversion undefined: if a user invokes `open ~/proj -e "npm run dev"` (a single `-e` string containing spaces), the parent must reconstruct that into the window's `--` form. Two conversions give different results and the spec picks neither:

- **Preserve as one token** → `-- "npm run dev"` (one passthrough arg); correct only if the receiving side runs the passthrough via a shell.
- **Word-split** → `-- npm run dev` (three tokens); changes the meaning if the string was meant to be shell-interpreted or contained quoting.

The trigger's local mint path (feeding the command to `CreateFromDir`/`QuickStart`) must produce the identical result to the spawned-window path, or the same command behaves differently on the trigger vs a spawned window. The reconstruction rule is load-bearing for that parity.

**Proposed Addition**:
Define the `-e`-string → `--`-argv reconstruction so the trigger's local mint and every spawned mint window receive a byte-identical command (e.g. state whether the single `-e` string is preserved as one passthrough token or word-split, and by what rule), guaranteeing the same command runs identically regardless of which surface a mint target lands on.

**Resolution**: Approved
**Notes**: Auto-approved with the parity-preserving rule — command carried as authored, no word-splitting (a single `-e "npm run dev"` string stays one unit), so trigger local-mint and spawned mint windows run byte-identical commands. Matches today's single-string `-e`. Logged to spec.

---
