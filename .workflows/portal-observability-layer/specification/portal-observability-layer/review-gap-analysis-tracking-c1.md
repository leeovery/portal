---
status: in-progress
created: 2026-06-01
cycle: 1
phase: Gap Analysis
topic: portal-observability-layer
---

# Review Tracking: portal-observability-layer - Gap Analysis

## Findings

### 1. Recovered panic emits TWO terminal markers — contradicts "exactly one terminal marker" and the four-way classification

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Defensive invariants against log destruction § "Mechanical rule — `process: exit` and the `main` exit shape" (lines 515–560)

**Details**:
The `main` exit-shape code block (lines 517–529) shows, on a recovered panic, the recover handler emitting `log.For("process").Error("panic", ...)` and setting `code = 2`, after which `log.Close(code)` (line 527) unconditionally runs and — per its own comment — "emits process: exit code=N". So a recovered panic produces BOTH a `process: panic` line AND a `process: exit code=2` line.

This directly contradicts two adjacent claims:
- Line 532: "Exactly one terminal marker fires per run: `exit` on clean/error return, `panic` on a recovered panic. No double-emit, no defer-ordering ambiguity."
- The four-way classification table (lines 555–560), which treats `process: exit` and `process: panic` as mutually-exclusive terminal classes following a `process: start`.

This is load-bearing: the entire feature exists to make termination forensically unambiguous via these markers. An implementer copying the shown `main` shape gets a double-emit; an investigator applying the four-way table sees a `start` followed by both `panic` and `exit` with no rule covering that pairing. Two reasonable implementations result: (a) skip `Close` on the panic path so only `process: panic` fires, or (b) accept the pair and document `panic`-then-`exit` as a defined sequence. The spec must pin one.

**Proposed Addition**:
*(leave blank until discussed)*

**Resolution**: Pending
**Notes**: Priority: Critical — undermines the forensic terminal-marker guarantee that motivates the feature; spec contains a self-contradiction an implementer cannot resolve without guessing.

---

### 2. `process_role` value→invocation mapping is never specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Subsystem prefix taxonomy § "Mandatory baseline attrs" (line 200); The `internal/log` package § "Public API" (`Init` signature, line 56)

**Details**:
`process_role` is a closed 6-value space (`daemon` / `bootstrap` / `hydrate` / `hooks_cli` / `tui` / `clean`). `Init(stateDir, version, processRole)` takes it as a caller-supplied argument, and the coverage requirement (line 536) says `main.go` calls `Init` "before any other portal code that might log" — i.e. before Cobra parses argv. But the spec never defines how `main` computes which of the 6 role values applies to a given invocation.

Portal has more invocation surfaces than role values: `open`/`x`, `attach`, `bootstrap`, `state daemon`, `state hydrate`, `state signal-hydrate`, `hooks set`/`rm`, `clean`, `alias`, `config`, `version`, `init`. An implementer cannot determine without guessing, e.g.: does `portal attach` map to `tui` or its own role? `portal config`/`alias`/`version`/`init` map to which of the six? Is `portal state signal-hydrate` → `hydrate`? And because `Init` is called before argv is parsed, the mechanism for resolving the role at `Init` time (pre-parse argv inspection? a per-entry-point Init call? a default with later refinement?) is itself unspecified. This blocks task breakdown for the `main`/`Init` wiring and risks inconsistent `process_role` values across binary paths — the exact attr the spec calls "critical for multi-writer disambiguation."

**Proposed Addition**:
*(leave blank until discussed)*

**Resolution**: Pending
**Notes**: Priority: Important — forces the implementer to design the invocation→role mapping and the pre-parse resolution mechanism that the spec relies on for forensic disambiguation.

---

### 3. `process: panic` uses the `reason` attr, which is not in the Process attr group

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Subsystem prefix taxonomy § "Closed attr-key value space" (lines 187, 183); Defensive invariants § `main` exit shape (line 523)

**Details**:
The `process: panic` line is written as `log.For("process").Error("panic", "reason", r)` (line 523). `panic` is explicitly part of the `process` component's lifecycle/diagnostic event set (lines 156, 187, 550, 559). But the **Process** attr group (line 187, "set per `process:` lifecycle/diagnostic line — 7") enumerates `cmd`, `args`, `target`, `code`, `resolved`, `source`, `raw` — it does NOT include `reason`. `reason` instead lives in the **Lifecycle** group (line 183), scoped to saver/daemon lifecycle events.

Given the spec's emphatically closed, group-organized vocabulary and its "no ad-hoc invention at call-site time" rule (lines 228–232), this is an internal inconsistency: a `process:` line carries an attr the Process group does not sanction. Either `reason` should be listed in the Process group (and the "7" count reconciled), or the panic line should use a Process-group key. A reviewer applying the closed-vocabulary rule mechanically would flag the spec's own example as a violation.

**Proposed Addition**:
*(leave blank until discussed)*

**Resolution**: Pending
**Notes**: Priority: Important — closed-vocabulary contradiction in a normative code example; affects the per-group key counts the spec treats as authoritative. (Note: the 49-key total is fixed by scope; resolution likely reassigns/cross-lists `reason` rather than adding a new key — to confirm in discussion.)

---

### 4. Baseline-attr "Set where" attributes `version`/`process_role` to "Root logger construction" — contradicts per-record handler injection and the `Init` argument flow

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Subsystem prefix taxonomy § "Mandatory baseline attrs" table (lines 195–200); cross-refs The `internal/log` package (lines 56, 78–83)

**Details**:
The "Set where" column says `version` (line 199) and `process_role` (line 200) are set at "Root logger construction (`os.Getpid()`)" / "Root logger construction". But elsewhere the spec is explicit that:
- The root logger is constructed in `internal/log`'s package `init` (line 78), which runs *before* `Init` is ever called.
- `version` and `process_role` are arguments to `Init` (line 56) — they do not exist at package-`init` time.
- Baseline attrs are injected "**per-record** by the configured handler, NOT via `root.With(...)` at construction" (lines 83, 193) — and the configured handler is built by `Init`, not by package-`init` root construction.

So "Root logger construction" is the wrong attribution for all three non-`component` baselines; the values arrive via `Init` and are injected per-record by the configured handler. (`pid` is also nominally available at package-init but the same per-record-injection mechanism applies.) An implementer reading the table literally might try to bake `version`/`process_role` into the root logger at package-`init` construction — which is impossible (values unavailable) and contradicts the per-record-injection design that the spec specifically chose to avoid the package-init-children-miss-baselines footgun.

**Proposed Addition**:
*(leave blank until discussed)*

**Resolution**: Pending
**Notes**: Priority: Important — the "Set where" column misdescribes the injection point in a way that conflicts with the package's stated construction order and injection mechanism; could lead to an unimplementable or wrong wiring.

---

### 5. `signal timeout` line sets `took` to the string `"3s"`, contradicting `took`'s `time.Duration` type and the duration-rendering rule

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Hook-firing observability limit § "Failure-mode INFO lines" table (line 935); cross-refs Subsystem taxonomy (lines 171, 219–220)

**Details**:
The timeout failure-mode line is specified as `hookLogger.Info("signal timeout", "took", "3s")` (line 935) — a string literal. But:
- The attr catalog defines `took` as "duration (`time.Duration` rendered)" (line 171).
- The text-mode rendering rule says `time.Duration` values render via Go's default `String()` (line 220), and multi-word/string values are quoted (line 219).

A string `"3s"` would render as `took="3s"` (quoted, per the string rule), whereas every other `took` in the spec renders unquoted (`took=1.2s`, `took=2.1s`). This produces a typed-value inconsistency on one cataloged line that an implementer would copy verbatim. (The success line one row down correctly uses a real `took` variable: `"took", took`.)

**Proposed Addition**:
*(leave blank until discussed)*

**Resolution**: Pending
**Notes**: Priority: Minor — single normative example uses a string where the catalog mandates a `time.Duration`; cosmetic rendering drift but on the closed-vocabulary surface.

---

### 6. Canonical call-site example uses an `fd` attr that is not in the closed vocabulary

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Call-site logging pattern § "Canonical pattern" (line 313); cross-refs Subsystem taxonomy closed attr space (lines 160–189)

**Details**:
The canonical multi-step example logs `log.Debug("fifo opened", "fd", fd, "took", time.Since(start))` (line 313). `fd` is not among the 49 closed attr keys. The hydrate example carries an explicit disclaimer (line 333) that it "is not a literal transcription of the hydrate helper's log lines" and that subtopic catalogs govern real sites — which covers the *lines*, but the example still uses real vocabulary keys throughout (`path`, `pane_key`, `took`, `bytes`) and introduces `fd` alongside them.

Given the spec's hard rule that contributors "MAY NOT introduce new attr names ad hoc" and that "every contributor consults these lists" (lines 228–232), an illustrative example that uses a non-vocabulary attr key undercuts that rule and could lead a copying implementer to believe `fd` is sanctioned. Either add a one-line note that the example's `fd` is illustrative-only (not vocabulary), or replace it with a vocabulary key.

**Proposed Addition**:
*(leave blank until discussed)*

**Resolution**: Pending
**Notes**: Priority: Minor — the disclaimer arguably covers it, but the closed-vocabulary discipline is a central, strongly-worded contract, so an example violating it is worth a clarifying note.

---
