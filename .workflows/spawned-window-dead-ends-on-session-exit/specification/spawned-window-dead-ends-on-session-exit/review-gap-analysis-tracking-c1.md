---
status: in-progress
created: 2026-07-21
cycle: 1
phase: Gap Analysis
topic: spawned-window-dead-ends-on-session-exit
---

# Review Tracking: spawned-window-dead-ends-on-session-exit - Gap Analysis

## Findings

### 1. Quoting/escaping mechanism for the `bash -lc` wrapper is unspecified — and the illustrative wrapper form is not literally achievable

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: The Fix — Ghostty-Adapter-Scoped Shell Fallback; Constraints ("Quoting must nest correctly"); In scope; Testing Requirements (unit coverage); Acceptance Criteria #2 and #7

**Details**:
The spec repeatedly presents the fix as the literal string:

```
bash -lc '<composed open argv>; exec "$SHELL" -il'
```

and Acceptance Criterion #2 asserts the adapter's window command *is* exactly this form. But `<composed open argv>` is defined as the argv "rendered exactly as it is today" and "already POSIX-single-quoted per element." The existing renderer quotes **every** element in POSIX single quotes and space-joins them, so the composed argv string is a sequence of single-quoted tokens (e.g. `'/usr/bin/env' '-u' 'TMUX' … '--ack' '<b>:<t>'`). That string cannot be nested verbatim inside an outer single-quoted `bash -lc '…'` — the first inner `'` terminates the outer quote.

The spec flags "quoting must nest correctly" as a constraint and requires a unit test for it, but never specifies the mechanism the implementer must use to make the three layers (inner per-element single quotes → the `bash -lc '…'` single-quote layer → the osascript `command:"…"` double-quote/backslash escape) compose. This is the single most error-prone part of the change, and it is left to implementer design. It also means AC #2 and the in-scope/Testing illustrations, read literally, describe a form that a correct implementation would NOT byte-match (a correct implementation must escape or re-quote the inner single quotes, e.g. via the standard `'\''` close-escape-reopen idiom the existing shell-quote helper already emits). Without a specified scheme, the unit test's expected value is also undefined — the implementer would author both the implementation and the "correct" expectation, defeating the regression intent.

An implementer taking the illustration at face value (naive concatenation) would emit a broken/corrupted command that fails at Ghostty launch or silently mis-runs — reintroducing exactly the class of failure this fix exists to remove.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 2. Fix scope is stated and illustrated only for attach surfaces; mint surfaces (`--path <dir>` and the mint `-- <command…>` passthrough) are never mentioned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: The Fix (all `<composed open argv>` examples); Acceptance Criteria #3 ("same `open --session <name> --ack …` argv")

**Details**:
Every illustration and acceptance criterion frames the composed argv as the attach form `open --session <name> --ack <batch>:<token>`. The burst also spawns **mint** surfaces, whose composed argv is `open --path <dir> --ack <batch>:<token>` and may additionally carry a `-- <command…>` passthrough. Because the wrap lives at the adapter and is argv-agnostic, mint surfaces should be covered automatically — but the spec never states this, so it is left implicit whether a mint window (and its post-command session exit) is in scope for the fallback shell. This matters both for completeness of the acceptance criteria and because a mint's `-- <command…>` passthrough element (single-quoted, possibly containing spaces or quotes) is another payload that must survive the wrapper's quote nesting from Finding #1. An implementer should not have to infer that mint surfaces get the same treatment.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---

### 3. Degenerate fallback behaviour undefined: empty/unset `$SHELL` or a failing `exec "$SHELL"`, combined with dropping `wait after command`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: The Fix (`exec "$SHELL" -il`); Resulting shell; Scope & Non-Goals (dropping `wait after command`); Acceptance Criteria #1

**Details**:
The fix's core promise — "the window stays visible AND usable" — rests entirely on `exec "$SHELL" -il` succeeding and the exec'd shell staying alive. The spec asserts `$SHELL` propagates (validated in one environment) but defines no behaviour if `$SHELL` is empty/unset in the inner `bash -lc --noprofile --norc` context (`exec "" -il` fails; the inner bash then exits with nothing left to run). Simultaneously the spec drops `wait after command` entirely (AC #2), so in this degenerate case Ghostty's default window-close behaviour governs — and that default is never stated. The net result of the failure path (silent close vs. a reappearing "press any key" dead-end) is therefore unspecified. This is an in-scope edge case because the fallback shell is the entire subject of the fix; a one-line statement of intended behaviour (or an explicit "`$SHELL` is guaranteed set by `login`, no fallback needed") would close it.

**Proposed Addition**:
_Leave blank until discussed._

**Resolution**: Pending
**Notes**:

---
