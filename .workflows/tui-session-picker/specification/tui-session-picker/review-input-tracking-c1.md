---
status: complete
created: 2026-02-27
cycle: 1
phase: Input Review
topic: tui-session-picker
---

# Review Tracking: TUI Session Picker - Input Review

## Findings

### 1. `n` key visibility on Projects page help bar

**Source**: Discussion — "How should the `n` key auto-execute behavior work?" + "How should page switching and keybindings work?"
**Category**: Gap/Ambiguity
**Affects**: Sessions Page, Projects Page, Command-Pending Mode

**Details**:
The `n` key section says it "Works from both pages" and "Works in command-pending mode." However, `[n] new here` only appears in the Sessions page help bar. The Projects page help bar and command-pending help bar don't include it. This is ambiguous — is `n` intentionally undocumented on the Projects page (like `x` for toggle), or should it appear in both help bars?

**Proposed Addition**:
Update all three help bars: Sessions adds `[n] new in cwd` (replacing `new here`), Projects adds `[n] new in cwd`, command-pending adds `[n] new in cwd`.

**Resolution**: Approved
**Notes**: User chose "new in cwd" label over "new here". Applied to all three help bars for consistency.

---

### 2. `Esc` behavior conflict in command-pending mode

**Source**: Discussion — "What happens to command-pending mode?" + "How should filter work?"
**Category**: Gap/Ambiguity
**Affects**: Command-Pending Mode, Filter & Initial Filter

**Details**:
The command-pending section says "`q`/`Esc` — cancels entirely (exits TUI without creating a session)." However, `bubbles/list` uses `Esc` to clear an active filter. If someone is filtering in command-pending mode and presses `Esc`, should it clear the filter (bubbles/list behavior) or exit the TUI (command-pending behavior)? These conflict. In normal mode this isn't an issue because `Esc` isn't mapped to quit — `q` handles that, and `Esc` is left to `bubbles/list` for filter management.

**Proposed Addition**:
Clarify `Esc` in command-pending: when filter is active, `Esc` first clears filter (bubbles/list consumes it); second `Esc` exits. No conditional check needed — bubbles/list Update chain handles this naturally.

**Resolution**: Approved
**Notes**: User pointed out bubbles/list consumes Esc when filtering — no explicit check needed, it falls through to outer handler when not filtering.
