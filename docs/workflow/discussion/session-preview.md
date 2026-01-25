---
topic: session-preview
status: in-progress
date: 2026-01-25
---

# Discussion: Session Preview in TUI

## Context

ZW's TUI shows a list of Zellij sessions. The current spec shows minimal info per session: name, attached status, and tab count (e.g., `2 tabs`).

The enhancement proposal (from Zesh comparison research) is to show **tab names** when a session is selected, giving users more context about what's in a session without attaching to it.

### References

- [ZW Specification](../specification/zw.md) - Current spec (lines 119-127)
- [Zellij Multi-Directory Discussion](zellij-multi-directory.md) - Prior discussion on session info display (lines 200-243)

### Prior Decisions

From `zellij-multi-directory.md`:
- Decision: "Minimal with optional expansion"
- Tab names queryable via `zellij --session <name> action query-tab-names`
- Discussed showing `2 tabs: "Claude Code", "Tests"` inline

### Current Spec

```
SESSIONS

  > cx-03          ● attached
    api-work       2 tabs
    client-proj
```

Shows tab count but not tab names.

## Questions

- [ ] When should the preview be shown?
- [ ] What information should be displayed?
- [ ] How should it be visually presented?
- [ ] Are there performance considerations?

---

*Each question above gets its own section below. Check off as concluded.*

---

## When should the preview be shown?

### Context

Need to decide the trigger for showing expanded session info. This affects both UX and performance (each preview requires a Zellij query).

### Options Considered

**Option A: Always show tab names inline**
```
SESSIONS
  > cx-03          ● attached    main, tests, docs
    api-work       2 tabs        api, worker
```
- Pros: All info visible at once, no interaction needed
- Cons: Cluttered on small screens (mobile use case), performance cost for all sessions

**Option B: Show on selection (highlighted row only)**
```
SESSIONS
  > cx-03          ● attached
    ├─ main
    ├─ tests
    └─ docs
    api-work       2 tabs
```
- Pros: Clean default view, detail when needed, single query
- Cons: Info hidden until you navigate to it

**Option C: Show on keypress (e.g., `p` for preview)**
- Pros: User controls when to see detail, no extra queries by default
- Cons: Hidden feature, extra interaction step

