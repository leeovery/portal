---
status: in-progress
created: 2026-07-17
cycle: 3
phase: Gap Analysis
topic: CLI Verb Surface Redesign
---

# Review Tracking: CLI Verb Surface Redesign - Gap Analysis

## Findings

### 1. `doctor`'s "host terminal detected + supported" check — its participation in the exit-code contract is undefined (an unsupported/remote terminal is not a health problem)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `doctor` — Diagnostics & Repair (check catalog; "Exit-code contract"; "Host-terminal detection folded in")

**Details**:
The exit-code contract is stated as a hard binary: "`portal doctor` exits **0 iff every check passes; non-zero (1) if any check reports a problem**." The authoritative check catalog lists, as its last entry, "host terminal detected + supported." Taken literally, a host terminal that resolves to **unsupported** is a check that "reports a problem" ⇒ non-zero exit.

But the host-detection subsection frames that same result as *informational*: `doctor` "prints a line such as `host terminal: Ghostty (supported)` / `unsupported (remote session)`." An unsupported terminal (a remote/mosh session, or a terminal Portal has no adapter for) is a legitimate **environmental** state, not a Portal-health defect — Portal itself is fully healthy there (single-target `open` works; only the multi-window burst is unavailable). If that state fails the exit code, `portal doctor && …` becomes a false-negative gate on every remote box.

This is the exact class of question cycle 2 resolved for the log-sweep (an action that is *not* a diagnosed health state was explicitly carved out of the exit-code contract), but the host-terminal check was never given the same ruling. It is also distinct from the already-decided down-server case (cycle 1), which is a genuine runtime-health failure. Every *other* catalog entry (daemon, hooks, saver, state dir, sessions.json, stale entries) is a real health state that should count; the host-terminal check is the sole entry whose exit-code participation is ambiguous, and an implementer wiring the exit code must pick one behavior with no spec guidance. It also propagates to `doctor --fix`, whose re-diagnosis exits "non-zero if anything remains unhealthy or **unfixable**" — an unsupported terminal is unfixable, so the same ambiguity decides whether `--fix` can ever return 0 on a remote host.

**Proposed Addition**:
State whether the "host terminal detected + supported" catalog entry participates in the exit-code contract. Recommend it is an **informational line only** (outside the pass/fail set), consistent with the "prints a line" framing and analogous to the log-sweep carve-out — so an unsupported/remote terminal is reported honestly but never makes `doctor` (or `doctor --fix`) non-zero; only genuine runtime-health failures (daemon/hooks/saver/state/stale) drive the exit code. (Alternatively, if it must count, state that explicitly and reconcile it with the remote-session use case.)

**Resolution**: Pending
**Notes**:

---

### 2. The `resolve` log line's behavior for **glob** targets is undefined (globs are bare positionals but are deterministic, and one glob token expands to K targets)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Grammar & Target Resolution ("Wrong-guess feedback — tmux is the receipt", the `resolve` line's behavior); Multi-Target Burst Mechanics ("Glob targets")

**Details**:
The `resolve` component's behavior was pinned precisely in cycle 1: **INFO level**, **bare positionals only** ("explicit pins … are deterministic and self-documenting … so they emit no `resolve` line; the component stays focused on guesses"), **emitted on a miss** (`domain = miss`), and **one line per resolved bare target**. That pinning was framed entirely around the guessing chain (session → path → alias → zoxide) and never addresses the **glob** target form, which the spec treats as a first-class bare-positional case that *skips* the chain ("session-domain by construction").

A glob target sits in the exact blind spot between two of the pinned rules:
- By the **"bare positionals only"** rule, a bare glob is a bare positional ⇒ it should emit a `resolve` line.
- By the **"stays focused on guesses"** rationale, a glob is fully *deterministic* (expand against the finite live-session set — no guessing) ⇒ it looks like it should emit nothing, like the equally-deterministic pins.

The two framings give opposite answers for the glob case, and the spec picks neither. Compounding it, a single glob token **expands to K targets** ("Expansion produces K targets that join the target list"), so "one line per resolved bare target" is itself ambiguous for globs: one line for the glob token, or K lines (one per expanded session), and with what `domain`/`resolved_path` values (the vocabulary — session/path/alias/zoxide/miss — has no glob notion; presumably `domain = session` per expanded hit). Because the line is INFO (present in production logs by default), the choice materially changes production log content and volume for every glob invocation, and an implementer has no spec basis to decide.

**Proposed Addition**:
Extend the `resolve` line's behavior list to cover glob targets explicitly: state whether a bare glob emits `resolve` line(s) at all (recommend **no** line — a glob is deterministic like a pin, consistent with "focused on guesses" — or, if lines are wanted, one **per expanded session hit** with `domain = session` and the session name in `resolved_path`), and reconcile the wording so "bare positionals only" and "focused on guesses" no longer conflict for the glob case.

**Resolution**: Pending
**Notes**:

---

### 3. Command passthrough — whether `-e` and `--` may co-occur (and which wins) is undefined

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Flags & Command Passthrough ("Command passthrough (`-e` / `--`)"); Multi-Target Burst Mechanics ("Argv parsing contract")

**Details**:
The command-passthrough feature is presented as two interchangeable spellings under one heading, "Command passthrough (`-e` / `--`)": `open -e <cmd>` and `open <target> -- <cmd> args…`. The argv-parsing contract handles each independently — "`-e <cmd>` and its value are not targets and are excluded from the ordered target list" and "`--` terminates flag/target parsing; every token after `--` is command-passthrough args" — but the spec never states what happens when **both** appear in one invocation, e.g. `open ~/new -e claude -- npm run dev`.

Under the argv contract, `-e claude` is excluded from targets *and* everything after `--` is command-passthrough args, so both are simultaneously "the command" with no defined precedence or exclusion. An implementer must choose among: usage error, `-e` wins, `--` wins, or (worst) both run. For a redesign whose premise is a precise, legible argv grammar with a single command feeding `CreateFromDir`/`QuickStart`, this is a small but real hole — and the command-parity guarantee ("byte-identical command to every mint surface") presupposes a single unambiguous command string.

**Proposed Addition**:
State the `-e` / `--` coexistence rule — recommend treating simultaneous `-e` and `--` as a **usage error** (they are two spellings of the same single command; specifying both is ambiguous), or, if a precedence is preferred, state which form wins — so the "one command per mint surface" and command-parity guarantees rest on an unambiguous source.

**Resolution**: Pending
**Notes**:

---
