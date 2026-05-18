---
status: complete
created: 2026-05-18
cycle: 1
phase: Plan Integrity Review
topic: Preview Visual Distinction
---

# Review Tracking: Preview Visual Distinction - Integrity

## Findings

### 1. Task 1-4 contains a non-pass/fail acceptance criterion ("verified by inspection")

**Severity**: Important
**Plan Reference**: preview-visual-distinction-1-4 (`tick-c90448`) — Acceptance Criteria / Tests
**Category**: Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The final Tests entry on task 1-4 reads `'composeChromeLineParts and composeChromeLine share tier-selection logic via a private helper' (structural assertion — single tier-selection entry point exists; verified by inspection or by a test that mutates a fixture and confirms both functions reflect the change consistently)`. "Verified by inspection" is not pass/fail and the "mutates a fixture" alternative is described loosely. The intent (no drift between the two functions) is already covered by the stronger byte-for-byte equality test `left + chrome + right == composeChromeLine(...)` listed immediately above it. The vague structural test should be removed; the byte-for-byte equality across the full threshold set is the actual guarantee against drift.

**Current**:
```
- 'composeChromeLineParts left plus chrome plus right equals composeChromeLine at every cascade threshold'
- 'composeChromeLineParts returns empty chrome at tier 4 widths'
- 'composeChromeLineParts and composeChromeLine share tier-selection logic via a private helper' (structural assertion — single tier-selection entry point exists; verified by inspection or by a test that mutates a fixture and confirms both functions reflect the change consistently)
```

**Proposed**:
```
- 'composeChromeLineParts left plus chrome plus right equals composeChromeLine at every cascade threshold'
- 'composeChromeLineParts returns empty chrome at tier 4 widths'
- 'composeChromeLineParts chrome region width matches composeChromeLine chrome region at every cascade threshold' (drives both functions over the same width set and asserts lipgloss.Width(chrome) matches the chrome span extracted from composeChromeLine's output by length subtraction — a behavioural drift guard that replaces the by-inspection structural assertion)
```

**Resolution**: Fixed
**Notes**:

---

### 2. Task 1-8 acceptance for "all four edges coloured" is too weak

**Severity**: Important
**Plan Reference**: preview-visual-distinction-1-8 (`tick-5f158b`) — Acceptance Criteria
**Category**: Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The acceptance criterion "All four edges are styled with previewBorderColor — verified by asserting the rendered output contains `'\x1b['` style codes (lipgloss emits these unconditionally when a foreground is set)" only proves *something somewhere* is styled. Phase acceptance requires "all four edges are coloured via previewBorderColor". A more specific assertion is needed — at minimum that the rendered top row contains the colour SGR around the corner glyphs and that the body's bordered output contains the colour SGR around the bottom corners. The current bullet would pass even if only one edge were coloured.

**Current**:
```
- All four edges are styled with previewBorderColor — verified by asserting the rendered output contains '\x1b[' style codes (lipgloss emits these unconditionally when a foreground is set).
```

**Proposed**:
```
- All four edges are styled with previewBorderColor — verified by locating each of the four rounded corner glyphs ('╭','╮','╰','╯') in the rendered output and asserting an SGR colour sequence appears within the byte run immediately preceding each corner. The hex codes of previewBorderColor.Light ('3B5577') and previewBorderColor.Dark ('7B95BD') need not be literal-byte asserted (lipgloss may translate via terminal-profile mapping); a non-empty foreground SGR ('\x1b[38;…m') prefix on each corner is sufficient. The test must inspect all four corners individually so a one-edge regression cannot pass.
```

**Resolution**: Fixed
**Notes**:

---

### 3. Task 1-8 "no explicit foreground SGR on chrome" test description is self-contradictory

**Severity**: Important
**Plan Reference**: preview-visual-distinction-1-8 (`tick-5f158b`) — Do section, second-to-last bullet
**Category**: Acceptance Criteria Quality / Task Self-Containment
**Change Type**: update-task

**Details**:
The Do bullet describing the chrome-foreground assertion is contradictory: it says to "Strip ANSI from the top row, locate the chrome substring, and verify the raw byte sequence preceding the chrome substring ends with a reset" — but if you strip ANSI you no longer have raw byte sequences to inspect. The next sentence then says "split the styled top row at the chrome substring" (no strip), which is the actually-workable approach. An implementer would have to guess which form is correct. Tighten the description to a single coherent procedure.

**Current**:
```
    - Asserts that the chrome content region of the rendered top row is not preceded by a foreground-colour SGR. Strip ANSI from the top row, locate the chrome substring, and verify the raw byte sequence preceding the chrome substring ends with a reset (or contains no Foreground SGR for the chrome span). Concrete assertion: split the styled top row at the chrome substring; assert the prefix and suffix carry colour SGR codes but the chrome span itself does not.
```

**Proposed**:
```
    - Asserts that the chrome content region of the rendered top row is not wrapped in a foreground-colour SGR. Procedure: take the raw (un-stripped) top row, locate the chrome substring by its plain-text content (e.g. 'Window 1 of 1 · Pane 1 of 1 · win: nvim-editor' at width 80), then split the raw top row at that substring into (prefix, chromeBytes, suffix). Assert: (a) prefix contains a foreground SGR sequence ('\x1b[38;'); (b) suffix contains a foreground SGR sequence ('\x1b[38;'); (c) chromeBytes contains no '\x1b[38;' sequence (no explicit foreground inside the chrome region). If the chrome region happens to end with an SGR reset that's fine — the load-bearing assertion is that the chrome span carries no foreground SGR of its own.
```

**Resolution**: Fixed
**Notes**:

---

### 4. Task 1-7 leaves View() as a stub — interim degradation not captured in plan-level acceptance

**Severity**: Minor
**Plan Reference**: preview-visual-distinction-1-7 (`tick-674c91`) — Do / Acceptance Criteria / Outcome
**Category**: Scope and Granularity / Vertical Slicing
**Change Type**: update-task

**Details**:
Task 1-7's plan deliberately stubs `View()` to return `m.viewport.View()` so the codebase still compiles between 1-7 and 1-8. This is a pragmatic refactor choice but means task 1-7 in isolation does not deliver verifiable user-visible value — the rendered preview loses chrome between these two commits. The Acceptance Criteria already acknowledges this with "go test ./internal/tui/... either passes or fails only on tests that 1-8 will rewrite", which is unusually loose. This is acceptable for a tightly-coupled refactor pair but the task should make explicit that 1-7 + 1-8 form an atomic logical pair and identify which existing tests are expected to break temporarily, so a reviewer of the intermediate commit isn't surprised.

**Current**:
```
  - go test ./internal/tui/... either passes or fails only on tests that 1-8 will rewrite; the resize test added in this task passes.
```

**Proposed**:
```
  - go test ./internal/tui/... passes for the resize test added in this task, the task 1-2 / 1-3 / 1-4 / 1-5 / 1-6 tests already in place, and the layout/precedence/scroll tests updated in 1-2. Tests that exercise the old chromeLine() method (pagepreview_chrome_test.go, pagepreview_chrome_enterattach_test.go) may need updates per Do; if any pre-existing test asserts on a full-frame rendered View() output, list it in the commit body as 'expected to fail until task 1-8' rather than rewriting it here.
  - The task 1-7 + 1-8 pair is an atomic refactor: between these two commits the rendered preview shows no chrome. Reviewers landing 1-7 in isolation should expect a temporary regression of the preview chrome until 1-8 lands.
```

**Resolution**: Fixed
**Notes**:

---

### 5. Phase acceptance bullet about hardcoded glyphs duplicates task 1-4 acceptance but is silent on bottom border

**Severity**: Minor
**Plan Reference**: Phase 1 acceptance criterion: "The manually-composed top-edge corner and edge glyphs are sourced from lipgloss.RoundedBorder() (not hardcoded); border parts are wrapped in lipgloss.NewStyle().Foreground(previewBorderColor).Render(…); chrome content renders with no explicit foreground."
**Category**: Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The phase acceptance specifies sourcing for the *top-edge* corner glyphs but is silent on the bottom-edge corners (╰ ╯). Task 1-8 uses `lipgloss.NewStyle().Border(lipgloss.RoundedBorder(), false, true, true, true)` for the body, so the bottom corners come from lipgloss too — but no acceptance criterion ties the bottom corners back to RoundedBorder explicitly. Adding a phase-level acceptance line covering this completes the picture and matches the spec § Border style.

**Current**:
```
- [ ] The manually-composed top-edge corner and edge glyphs are sourced from `lipgloss.RoundedBorder()` (not hardcoded); border parts are wrapped in `lipgloss.NewStyle().Foreground(previewBorderColor).Render(…)`; chrome content renders with no explicit foreground.
```

**Proposed**:
```
- [ ] The manually-composed top-edge corner and edge glyphs are sourced from `lipgloss.RoundedBorder()` (not hardcoded); border parts are wrapped in `lipgloss.NewStyle().Foreground(previewBorderColor).Render(…)`; chrome content renders with no explicit foreground.
- [ ] The bottom border (left edge, right edge, bottom edge, and corners ╰ ╯) is rendered via a single `lipgloss.NewStyle().Border(lipgloss.RoundedBorder(), false, true, true, true).BorderForeground(previewBorderColor)` body-wrap so all four corners derive from the same `lipgloss.RoundedBorder()` source and the same `previewBorderColor`.
```

**Resolution**: Fixed
**Notes**:

---

### 6. Task 1-1 outcome inconsistency between "two constants" and the var declaration

**Severity**: Minor
**Plan Reference**: preview-visual-distinction-1-1 (`tick-073f38`) — Solution / Outcome
**Category**: Task Template Compliance
**Change Type**: update-task

**Details**:
The Solution says "Introduce two package-level string constants and one package-level lipgloss.AdaptiveColor variable" (correct). The Outcome opens with "internal/tui/pagepreview.go declares verboseKeymap, compactKeymap, and previewBorderColor (only)" (correct) — but the test description "A test pins both keymap constants to literal bytes" omits the previewBorderColor verification, while the Acceptance Criteria does include a previewBorderColor existence check. The phrasing is internally consistent but a literal-value test for previewBorderColor's Light/Dark hex codes would strengthen the constant-pin guarantee and matches the spec § Border colour byte-pinning intent.

**Current**:
```
  Tests:
  - 'verboseKeymap exact byte content matches spec'
  - 'compactKeymap exact byte content matches spec'
  - 'compactKeymap is single-space separated with no interpuncts'
```

**Proposed**:
```
  Tests:
  - 'verboseKeymap exact byte content matches spec'
  - 'compactKeymap exact byte content matches spec'
  - 'compactKeymap is single-space separated with no interpuncts'
  - 'previewBorderColor Light hex equals 3B5577 and Dark hex equals 7B95BD'
```

**Resolution**: Fixed
**Notes**:

---

### 7. Task 1-3 algorithm description has an ambiguous edge for `budget == 1`

**Severity**: Minor
**Plan Reference**: preview-visual-distinction-1-3 (`tick-6fc894`) — Do / Acceptance Criteria
**Category**: Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The algorithm step "Otherwise iterate runes, accumulating cells. Stop when adding the next rune would exceed budget − 1. Append '…' and return." combined with the boundary test "Budget == 1, non-empty s that does not fit whole: returns '…'" produces an output of width 1 (just the ellipsis with no preceding content). That matches the test expectation, but the acceptance criterion `runewidth.StringWidth(got) <= budget` would also be satisfied by returning `""` for budget == 1. The task should pin which result is canonical so an implementer doesn't choose `""` and break the test. The intent (judging by the test row) is `'…'`. Add an explicit Do bullet for this.

**Current**:
```
  - Algorithm:
    - If budget <= 0 return "".
    - Measure full string first: if runewidth.StringWidth(s) <= budget return s unchanged (no truncation, no ellipsis).
    - Otherwise iterate runes, accumulating cells. Stop when adding the next rune would exceed budget − 1. Append '…' and return.
```

**Proposed**:
```
  - Algorithm:
    - If budget <= 0 return "".
    - If s == "" return "".
    - Measure full string first: if runewidth.StringWidth(s) <= budget return s unchanged (no truncation, no ellipsis).
    - Otherwise iterate runes, accumulating cells. Stop when adding the next rune would exceed budget − 1 (reserving one cell for the ellipsis). Append '…' and return.
    - For budget == 1 with non-empty s that does not fit whole: the loop adds zero runes (every rune would exceed budget − 1 == 0), then '…' is appended — output is '…' (width 1). This is the canonical result; do not collapse to "".
```

**Resolution**: Fixed
**Notes**:

---
