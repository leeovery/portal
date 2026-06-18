---
status: complete
created: 2026-06-18
cycle: 2
phase: Gap Analysis
topic: Spectrum TUI Design
---

> **All 3 findings resolved (cycle 2).** 1: authoritative Sessions footer membership (§3.4/§11.1). 2: no-tags signpost `x → e` path (§11.3 + mock). 3: expanded by user into a full **modal anatomy unification** — header title (red if destructive), `◉ EDIT MODE`-only right-corner, dismiss key in footer as `esc <verb>`, kill/delete `y kill · esc cancel` (dropped `n`); applied to §8.1/§8.2/§8.3/§8.4/§8.6/§12.1 + mocks (kill title red + footer, rename/edit header words removed). Also captured (separately): mid-tone background open item → §2.6/§16.6 routing to follow-up discussion.

# Review Tracking: Spectrum TUI Design - Gap Analysis

## Findings

### 1. Sessions footer key-membership is undefined — §11.1 "reduces to `n`" presumes `n` is in a footer that §3.4 never lists it in

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §3.4 (Footer), §11.1 (Empty states), cross-checked against §12.1 (Sessions keymap)

**Details**:
§3.4 says the Sessions footer "Shows only the **core** keys for the page" and gives one example, prefixed "e.g.":
`↑↓ navigate · ⏎ attach · / filter · ␣ preview · s switch view · x projects` + right-aligned `? help`.
That example omits `n` (new-in-cwd), `r` (rename) and `k` (kill) — all three of which §12.1 lists as live Sessions bindings.

§11.1 (Empty sessions) then states the footer "reduces to the still-relevant keys (`n` / `x` / `/` / `?`)." The verb "reduces" presumes those keys are a subset of the normal footer — but `n` is **not** in §3.4's example footer. So either:
- the normal Sessions footer *does* include `n` (and the §3.4 example is incomplete/wrong), or
- the empty-state footer *adds* `n` (contradicting "reduces").

There is no "core keys" definition anywhere except the §3.4 "e.g." example, so an implementer cannot determine the authoritative membership of the Sessions footer — specifically whether `n`/`r`/`k` belong in it. Because §13.4 treats the §3.4 footer as load-bearing ("`s switch view` lives in the footer"), and because the empty-state derivation literally subtracts from it, the footer's exact key set needs to be unambiguous. This is a planning-readiness gap: the footer string is a concrete render target that two sections describe inconsistently.

Note: this is not a leftover `p`/`s` stray reference (the cycle-1 keymap simplification is clean — `x` = Sessions⟷Projects, `s` = Sessions view-cycle, both correctly reflected in §3.4/§6.3/§12.2/§13.2). The gap is the pre-existing-but-now-exposed question of whether `n`/`r`/`k` appear in the condensed Sessions footer, which the §11.1 "reduces to `n`" wording forces into the open.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**: Priority: Important

---

### 2. No-tags signpost (§11.3) instructs `(e)` while it renders on the Sessions page, where `e` is unbound

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §11.3 (No-tags signpost), §5.3 (By Tag), cross-checked against §12.1 (Sessions vs Projects keymaps), §6.3

**Details**:
The "No tags yet" signpost reads: `No tags yet — add tags in the project editor (e) …` (§11.3). Per §5.3 this signpost appears on the **Sessions by-Tag view** — i.e. while the user is on the **Sessions page**.

But `e` (edit) is a **Projects-page** binding only (§12.1 Projects list; §6.3 Projects footer; tags "managed only in the projects edit modal" §5.5). The Sessions page has no `e` binding (§12.1 Sessions list). So a user reading "add tags in the project editor (e)" on the Sessions by-Tag view who presses `e` gets nothing — they must first press `x` to switch to Projects, then `e`.

After the cycle-1 keymap simplification (which made `x` the Sessions⟷Projects toggle and dropped the old `p` alias), the path to the editor from this signpost is "press `x`, then `e`", but the hint names only `(e)`. This is a small navigational ambiguity: the on-screen instruction references a key that is inert on the page the instruction is shown on. The trailing `…` suggests the hint is truncated, so the resolution may simply be to confirm the intended full hint text (e.g. "press `x` for projects, then `e`").

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**: Priority: Minor

---

### 3. Cancel-key footer wording drifts between destructive modals (`esc to cancel`) and other dismiss footers (`esc cancel` / `esc close` / `esc clear`)

**Source**: Specification analysis
**Affects**: §8.3 (Kill), §8.6 (Delete project), §8.4 (Rename), §11.4 (Command-pending), §7.1 (Filter), §8.2 (Edit-modal footers)
**Category**: Gap/Ambiguity

**Details**:
Footer dismiss-key labels use inconsistent phrasing across modals/states:
- Kill (§8.3): `... · esc to cancel`
- Delete project (§8.6): `... · esc to cancel`
- Rename (§8.4): `↵ rename · esc cancel`
- Command-pending (§11.4): `⏎ run here · n run in cwd · esc cancel`
- Filter list-active (§7.1): `... · esc clear filter`; input-active: `... · esc clear`
- Edit-modal (§8.2): `... · esc close`

The two new/cycle-1-touched destructive modals (kill restyle §8.3 and the new delete-project modal §8.6) both standardised on `esc to cancel`, while the rename modal (§8.4) and command-pending banner (§11.4) use the terser `esc cancel`. These are user-visible footer strings and concrete render targets; the mixed `esc to cancel` vs `esc cancel` reads as an unintended inconsistency rather than a deliberate distinction (the differing *verbs* — cancel/close/clear — are meaningfully distinct and are fine; only the `to`-insertion varies arbitrarily). Pinning one form avoids an implementer guessing which spelling to render.

**Current**:
- §8.3 Kill footer: `y kill · n cancel · esc to cancel`
- §8.6 Delete project footer: `y delete · n cancel · esc to cancel`
- §8.4 Rename footer: `↵ rename · esc cancel`
- §11.4 Command-pending footer: `⏎ run here · n run in cwd · esc cancel`

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**: Priority: Minor
