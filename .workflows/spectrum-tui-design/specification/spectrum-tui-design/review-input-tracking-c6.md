---
status: in-progress
created: 2026-06-18
cycle: 6
phase: Input Review
topic: spectrum-tui-design
---

# Review Tracking: spectrum-tui-design - Input Review

## Findings

### 1. §2.9 token-table column headers still reference the pre-reversal canvas ("on black" / "on white")

**Source**: discussion "Terminal theming & canvas ownership → Decision — REVERSED (2026-06-18)" — "Contrast re-verification — DECIDED": *"the floor (§2.3) is now measured against the **exact owned canvas** (`#0b0c14` / `#e1e2e7`), not ≈black/≈white"* (lines 188-189); and "Canvas colour — revert to Tokyo Night" → *"pin a real light canvas (`#e1e2e7`), not 'white'"* (lines 169-177).
**Category**: Enhancement to existing topic
**Affects**: §2.9 (MV token table — the "Greys / text ramp" table header)

**Details**:
The reversal decided the contrast floor is re-verified against the **exact owned canvas** — dark variants vs `#0b0c14`, light variants vs `#e1e2e7` — explicitly **not** "≈black/≈white" / "not white." The spec absorbs this everywhere it states the rule in prose: §1 ("never an arbitrary terminal background"), §2.3 ("the dark variant on `#0b0c14` and the light variant on `#e1e2e7`"), and the §2.9 "Contrast re-verification" rule ("dark variants vs `#0b0c14`, light variants vs `#e1e2e7`").

But the §2.9 *token-table column headers themselves* were not updated: the "Greys / text ramp" table still labels its two ratio columns **"Dark (on black)"** and **"Light (on white)"** (line 106). Those columns hold the actual contrast ratios (e.g. `text.primary` `#C0CAF5` · 13.0), and after the reversal those ratios are computed against the owned canvas (`#0b0c14` / `#e1e2e7`), not pure black/white. The stale "on black" / "on white" header text is leftover pre-reversal wording the fold-in missed — it directly contradicts the decided "not ≈black/≈white" point and mislabels what the numbers were measured against. (The "Accents" and "Surfaces" tables below it use plain "Dark" / "Light" headers, so only the first table carries the stale label — also an internal inconsistency between the three tables.)

This is a faithful-fold-in miss (a decided source point — the canvas reference for the floor — not fully reflected), not new scope.

**Current**:
```
**Greys / text ramp**

| Token | Role | Dark (on black) | Light (on white) | Floor |
|---|---|---|---|---|
```

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---
