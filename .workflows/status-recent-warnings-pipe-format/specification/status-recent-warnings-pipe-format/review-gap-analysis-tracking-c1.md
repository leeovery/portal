---
status: in-progress
created: 2026-06-09
cycle: 1
phase: Gap Analysis
topic: status-recent-warnings-pipe-format
---

# Review Tracking: Status Recent Warnings Pipe Format - Gap Analysis

## Findings

### 1. Empty-component parse rule is underspecified against the real writer output

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Solution Design → §1 (Parsing rules, Component bullet)

**Details**:
The spec states the empty-component case yields `Component == ""` and still `ok==true`, but the parsing rule as written ("Component = the run after the level token up to the first `:`, trailing `:` removed") does not match what the writer actually emits, and leaves the implementer to guess the whitespace handling.

The writer (`handler.go` lines 149-153) emits, for an empty component: `<level>` + a single space + `` (empty component) + `": "` + `<msg>`. So the rendered line is literally:

```
2026-06-09T12:00:00Z WARN : some message pid=…
```

There is a space *before* the colon. Under the spec's rule, "the run after the level token up to the first `:`" is therefore a whitespace-only (or empty) run. The spec never says whether that run is trimmed to `""` before the equality check, nor whether leading whitespace after the level token is skipped before the component run begins. An implementer could reasonably:
- capture `" "` (a single space) as the component → `Component == " "`, not `""` (violates the stated outcome), or
- trim and get `""` (matches the stated outcome).

The non-empty case has the same latent issue: writer emits `<level> <component>: ` (single space between level and component), so the component run must skip exactly one leading space. The rule should state the whitespace contract explicitly (e.g. "trim surrounding whitespace from the component run; an all-whitespace or empty run yields `Component == \"\"`").

**Proposed Addition**:
Replace the Component bullet in Solution Design §1 with:
- **Component** = the text between the level token and the first `:` in the line, with surrounding whitespace trimmed. (The writer emits one space after the level token, and — for an empty component — a space before the colon.) An all-whitespace or empty run yields `Component == ""` with `ok == true`. Component names carry no spaces or colons, so the first `:` reliably ends the component.

**Resolution**: Pending
**Notes**:

---

### 2. Minimum-token / degenerate-line handling for `ParseLogLine` is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Solution Design → §1 (ParseLogLine contract & parsing rules)

**Details**:
The spec defines `ok=false` for "wrong shape / unparseable timestamp" but never enumerates the shape failures beyond the timestamp. Several real inputs are not addressed, forcing the implementer to invent the contract:

- **Empty line** (`""`) — `scanRecentWarnings` will encounter blank lines (e.g. trailing newline, or any future blank separator). First token parse fails → presumably `ok=false`, but unstated.
- **Line with only a timestamp, or timestamp + level but no `:`** — e.g. `2026-… WARN` with no colon anywhere. The "run up to the first `:`" has no terminator. Does the parser return `ok=false` (no colon found), or treat the rest as component? Unspecified.
- **Fewer than two whitespace tokens** — "second whitespace-delimited token" presupposes at least two tokens exist; the failure path when only one (or zero) token is present is implied but not stated.

Acceptance criterion 7 and the testing section require malformed lines to "silently skip ... and not abort the scan," and the retained malformed-line test must be "re-expressed as a line that does not match the new layout" — but the spec does not define precisely which shapes are "does not match," so the implementer cannot know which exact line(s) to use as the fixture or what `ParseLogLine` must return for each. Recommend enumerating the `ok=false` triggers: no colon present, fewer than two tokens, unparseable timestamp.

**Proposed Addition**:
Add to the `ParseLogLine` contract in Solution Design §1:

`ParseLogLine` returns `ok == false` for any line that does not match the layout — specifically when **any** of:
- the line contains no `:` (no component delimiter), or
- the line has fewer than two whitespace-delimited tokens, or
- the first token does not parse as an RFC3339Nano timestamp.

An empty line (`""`) falls under these (no tokens / no colon) → `ok == false`. These are exactly the shapes the malformed-line test treats as "does not match the new layout."

**Resolution**: Pending
**Notes**:

---

### 3. Message rule does not address messages that themselves contain a colon

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Solution Design → §1 (Component & Message parsing rules)

**Details**:
The Component rule keys off "the first `:`". Real messages contain colons — the existing test fixtures use `flush failed: disk full` and the codebase's catalog messages can include colons. For a line `2026-… WARN daemon: flush failed: disk full pid=…`, the *first* colon (after `daemon`) is the component delimiter and the second colon is inside the message. The intended parse is `Component="daemon"`, `Message="flush failed: disk full"`, which the "first `:`" rule does produce — but the spec never states this explicitly, and the parenthetical "(component names carry no spaces or colons)" only constrains the component, not the message. An implementer reading the Message rule ("the text after `<component>: `") could be unsure whether a colon in the message interferes. A one-line note that the message may contain colons (only the first colon delimits the component) removes the doubt and pins the test expectation for a colon-bearing message.

**Proposed Addition**:
Add a note under the Message bullet in Solution Design §1:

A message may itself contain `:`. Only the **first** `:` in the line (immediately after the component token) delimits the component; any later colons belong to the message. E.g. `… WARN daemon: flush failed: disk full pid=…` parses as `Component="daemon"`, `Message="flush failed: disk full"`.

**Resolution**: Pending
**Notes**:

---

### 4. Quoted multi-word values (notably `version=`) interact with the message-boundary regex — unaddressed

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Solution Design → §1 (Message boundary rule)

**Details**:
The writer quotes any attr value (and the `version` baseline) that contains whitespace: `quoteIfMultiWord` (handler.go lines 175, 300-305) emits e.g. `version="3.6 beta"` or `reason="some message"`. The Message-boundary rule stops at "the first whitespace-delimited token of the form `key=value` (matching `^[A-Za-z_][A-Za-z0-9_.]*=`)." Two interactions are unspecified:

1. A quoted value containing spaces (`version="3.6 beta"`) is split by whitespace tokenisation into `version="3.6` and `beta"`. The first sub-token still matches the regex (`version=`), so the boundary is found correctly — but the spec's reliance on "whitespace-delimited token" tokenisation in the presence of quoted-with-space values is not called out, and an implementer who tries to respect quoting could over-engineer it. Worth stating that the simple whitespace split is sufficient because the boundary token (`pid=`/`version=`/`process_role=` or any attr key) always starts a fresh whitespace-delimited token.
2. A *contextual attr* whose quoted string value contains a `key=value`-shaped substring (e.g. `note="x=1 done"`) — the `x=1` sub-token would also match the regex. This is an extension of the existing "Documented assumption" (which only covers the *message* not containing a `key=value` token), but the assumption does not cover *attr values* containing such tokens. Since the boundary is the *first* matching token, and the genuine first attr key always precedes any value content, the truncation still lands at or before the correct point — but this should be reconciled with the documented-assumption bullet so the two are not in tension.

**Proposed Addition**:
Add a clarification under the Message-boundary rule in Solution Design §1:

A plain whitespace split is sufficient to find the boundary even when an attr value is quoted and contains spaces (`version="3.6 beta"`): the boundary token (`pid=`/`version=`/`process_role=` or any contextual attr key) always begins a fresh whitespace-delimited token, so the first regex match lands at the first real attr regardless of quoting. The boundary keys off the *first* matching token and a genuine attr key always precedes any value content, so a `key=value`-shaped substring inside a quoted value can never shift the boundary earlier than the first attr. (The documented assumption below concerns only the message text.)

**Resolution**: Pending
**Notes**:

---

### 5. `LastWarning` summary recomposition is redundant with parsed fields but the composition source is ambiguous for empty component

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Solution Design → §2 (LastWarning composition); Acceptance criteria 2

**Details**:
The reader composes `LastWarning` as `<LEVEL> <component>: <msg>` (e.g. `WARN daemon: tick complete`). For the empty-component case (Finding 1), the composed string would be `WARN : tick complete` (with a space before the colon) or `WARN: tick complete` depending on whether the implementer interpolates the empty `Component` directly into `"%s %s: %s"`. The displayed output to the user differs between these two (`Recent warnings: 1 (last: WARN : msg)` vs `... WARN: msg`). The spec gives the format string by example only for the non-empty case and does not define the empty-component rendering. Recommend pinning the exact composition (format string and empty-component behaviour) so the displayed line is deterministic and testable.

**Proposed Addition**:
Pin the `LastWarning` composition in Solution Design §2:

`LastWarning` composition: when `Component != ""`, render `"<LEVEL> <component>: <msg>"` (e.g. `WARN daemon: tick complete`); when `Component == ""`, render `"<LEVEL>: <msg>"` (e.g. `WARN: tick complete`) — no stray space before the colon. The displayed status line is therefore deterministic for both cases.

**Resolution**: Pending
**Notes**:

---

### 6. `LastWarning` value mismatch between two existing cmd-layer tests and the new trimmed format is acknowledged for removal but the replacement assertion shape is not pinned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria & Testing → "Retained coverage" and "Anti-false-green"

**Details**:
The two `cmd/state_status_test.go` fixtures (lines 182-203 and 243-259) assert the *full raw line* including the timestamp inside `Recent warnings: 1 (last: <full-line>)`. The spec mandates removing the pipe-format fixtures, and the new `LastWarning` is the trimmed `<LEVEL> <component>: <msg>`. The spec's acceptance criterion 2 states the rendered line is `Recent warnings: N (last: <LEVEL> <component>: <msg>)`, which is good — but it does not state how the cmd-layer test should source its log line now that hand-authored fixtures are banned. The "producer-coupled end-to-end regression test" requirement is described at the `CollectStatus` level (internal/state); it is unclear whether the cmd-layer rendering assertion (the `runStateStatus` / stdout-contains tests) must *also* drive the real writer, or whether those cmd tests are retired/folded into the new producer-coupled test. An implementer needs to know whether `TestStateStatusRecentWarningsLastLineSuffixWhenNonZero` and `TestStateStatusExitNonZeroWhenRecentWarningsPresent` are deleted, rewritten to use the real writer, or kept with a writer-sourced line. Recommend stating which existing cmd tests are removed vs. migrated and where the producer-coupled assertion lives (state layer only, or both layers).

**Proposed Addition**:
Add to the Testing requirements:

The producer-coupled end-to-end regression test lives at the `CollectStatus` (state) layer. The two `cmd/state_status_test.go` pipe-format fixtures are **migrated, not deleted**: their assertions are retained (the rendered `Recent warnings: N (last: …)` suffix and the non-zero exit when warnings are present), but each must source its log line from the real `internal/log` writer rather than a hand-authored string, and the asserted `(last: …)` suffix updates to the trimmed `<LEVEL> <component>: <msg>` form. No cmd-layer test may construct a log line from an independently-defined format string.

**Resolution**: Pending
**Notes**:

---

### 7. `StatusReport.RecentWarnings` / `LastWarning` doc comment update specified, but the `scanRecentWarnings` and `logEntryQualifies` doc comments are not

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Solution Design → §2

**Details**:
The spec explicitly directs updating the `StatusReport.LastWarning` doc comment. It does not mention the now-stale doc comments on `scanRecentWarnings` ("Malformed lines (wrong field count, unparseable timestamp ...)" — "wrong field count" is a pipe-format concept that disappears) and `logEntryQualifies` (whose entire body and doc reference the removed `logFieldSeparator`/`expectedLogFieldCount`). The spec says to remove the two constants but does not state the fate of `logEntryQualifies` itself — is it deleted (its logic absorbed into `scanRecentWarnings` calling `log.ParseLogLine` + level/cutoff checks), or rewritten? Leaving this implicit means an implementer must decide the function boundary. Minor, but it affects how the "parse each line once via `log.ParseLogLine`" instruction is realised (one function vs two) and which doc comments to refresh.

**Proposed Addition**:
Add to Solution Design §2:

After the change, no doc comment or constant in `status.go` may reference the removed pipe format. Specifically: refresh `scanRecentWarnings`'s doc comment to drop "wrong field count" (a pipe-format concept). `logEntryQualifies` may be folded into the parse-once flow or rewritten to operate on the parsed `LogLine` — the function boundary is the implementer's choice — but its body and doc comment must no longer reference `logFieldSeparator` / `expectedLogFieldCount`.

**Resolution**: Pending
**Notes**:

---

### 8. Last-wins ordering relies on file line order being timestamp-ascending — assumption not stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Solution Design → §2 (last-wins); Acceptance criterion 2

**Details**:
The reader preserves "last-wins" by overwriting `LastWarning` on each qualifying line as it scans top-to-bottom. This makes "most recent" == "last in file" only if `portal.log` lines are written in timestamp-ascending order. The writer appends sequentially, so this holds in practice, but the spec frames `LastWarning` as "the most recent qualifying entry" (a timestamp claim) while the mechanism is purely positional (last line wins, regardless of its timestamp). Within scope this is unchanged behaviour and likely fine, but the spec should state the assumption explicitly (last-wins == last-in-file == most-recent because the writer appends in chronological order) so the producer-coupled test does not accidentally encode a non-chronological fixture and the "most recent" wording is not mistaken for a timestamp-max selection. Minor / clarification.

**Proposed Addition**:
Add to Solution Design §2:

Last-wins is positional: the reader overwrites `LastWarning` on each qualifying line top-to-bottom, so "most recent" means "last qualifying line in the file." This equals chronological-most-recent because the writer only ever appends, in chronological order. The producer-coupled test must therefore write its fixtures in append (chronological) order so "last in file" and "most-recent timestamp" coincide.

**Resolution**: Pending
**Notes**:

---
