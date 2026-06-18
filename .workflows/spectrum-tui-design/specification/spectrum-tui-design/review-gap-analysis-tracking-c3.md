---
status: complete
created: 2026-06-18
cycle: 3
phase: Gap Analysis
topic: spectrum-tui-design
---

> **Both findings resolved (cycle 3).** 1: help modal reconciled to §8.1 anatomy via a stated exception (header dismiss `esc close`, no "to") — §8.1/§8.5 + mock. 2: kill/delete parity notes aligned to `y`/`Esc` (dropped stray `n`, reworded from "parity" to "keymap changed") — §8.3/§8.6. Also during this turn: kill modal mock given a proper header row (structural §8.1 conformance), and §15.5 added (Paper-reference comparison mechanism).

# Review Tracking: Spectrum TUI Design - Gap Analysis

## Findings

### 1. Help modal (§8.5) violates the §8.1 shared modal anatomy — dismiss key in header, banned "esc *to*" wording, no footer

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §8.1 (Modal framing — shared anatomy), §8.5 (`?` help modal)

**Priority**: Important

**Details**:
The cycle-2 §8.1 unification asserts a rule for **"Every modal"**: anatomy is a **header row** (title left; right-corner **empty except `◉ EDIT MODE`**) over the body over a **contextual footer**, and "The **dismiss key always lives in the footer** (never the header) as `esc <verb>` … the verbs differ by semantics, **never the wording** (no 'esc *to* cancel')."

The `?` help modal (§8.5, line 330) does not conform to this shared anatomy on three points:
1. **Dismiss key in the header, not the footer.** §8.5 puts a **right-aligned `esc to close` in the header** (`header '? Keybindings' … right-aligned 'esc to close'`). §8.1 mandates the dismiss key live in the footer, never the header. The §8.1 header right-corner is also meant to be empty except `◉ EDIT MODE` — help's right-corner carries `esc to close`.
2. **Banned wording.** §8.5 uses `esc **to** close` — exactly the "esc *to* …" construction §8.1 explicitly forbids ("no 'esc *to* cancel'"). This is a stray pre-unification leftover.
3. **No contextual footer.** §8.5 describes no footer at all; §8.1 says every modal has "the body over a contextual footer."

Additionally, §8.1's `esc <verb>` enumeration (`esc cancel` kill/delete/rename; `esc close` edit navigate/chip; `esc discard` edit-in-place) **omits a verb for help** — so even after relocating the dismiss key to a footer, the verb to use for help is unspecified.

The help modal is unambiguously a modal under §8 (titled in §8, mocked as `Sessions — Help Modal (?)`, §15), and §8.1 says "Every modal," so there is no exemption. This is a NEW contradiction created when the cycle-2 §8.1 shared-anatomy block was added without reconciling §8.5. An implementer following §8.1 would build a footer-dismiss help modal; one following §8.5 would build a header-dismiss one — and the §8.1 verb table gives no help verb either way.

**Current**:
§8.1: "The **dismiss key always lives in the footer** (never the header) as `esc <verb>` — `esc cancel` (kill / delete / rename), `esc close` (edit navigate / chip), `esc discard` (edit-in-place); the verbs differ by semantics, never the wording (no 'esc *to* cancel')."

§8.5: "header `? Keybindings` (`text.primary`), right-aligned `esc to close` (`text.detail`)."

**Proposed Addition**:
*(leave blank until discussed)* — reconcile §8.5 to the §8.1 anatomy: move dismiss to a contextual footer with §8.1-style wording (e.g. `esc close`), drop the header `esc to close`, and add help's verb to the §8.1 enumeration; OR, if help is a deliberate exception (no contextual footer, header-corner dismiss), state that exception explicitly in §8.1 and within the §8.1 "Every modal" rule so the two sections agree.

**Resolution**: Pending
**Notes**:

---

### 2. Kill/Delete parity-note "`y`/`n`/`Esc`" contradicts the authoritative `y`/`Esc` keymap (stray `n`)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: §8.3 (Kill confirm modal — reskin-status note), §8.6 (Delete project confirm modal — reskin-status note), §8.1 (modal anatomy), §12.1 (per-screen keymaps — Modals)

**Priority**: Important

**Details**:
The reskin-status note blocks for the two destructive modals still describe the confirm flow as **`y`/`n`/`Esc`**, while every authoritative surface in the cycle-2 edits dropped `n`:
- §8.3 note (line 318): "The confirm flow (`y`/`n`/`Esc`) is unchanged (parity)" — but the body footer is `y kill · esc cancel` and "Keys: `y` (confirm) / `Esc` (cancel)" (line 320).
- §8.6 note (line 333): "The confirm flow (`y`/`n`/`Esc`) is unchanged (parity)" — but the body footer is `y delete · esc cancel` and "Keys: `y` (confirm) / `Esc` (cancel)" (line 335).
- §12.1 modals (line 444): "kill `y`/`Esc` · delete-project `y`/`Esc`".

So the authoritative keymap (§12.1), the modal footers, and the modal "Keys:" lines all say **`y`/`Esc`** — `n` is gone — while the two parity notes still carry the old **`y`/`n`/`Esc`** and simultaneously assert that flow is "unchanged (parity)."

This is a direct contradiction and leaves the `n` binding ambiguous: is `n` still bound as a cancel alias (per "the confirm flow … is unchanged") but merely undisplayed, or was it dropped (per §12.1 / footer / keys line)? An implementer cannot tell whether to wire `n`→cancel. The footers showing `esc cancel` and §12.1 listing only `y`/`Esc` indicate `n` was intentionally dropped, which makes the parity notes' `y`/`n`/`Esc` a stray leftover and also makes "unchanged (parity)" mildly inaccurate (a key was removed). These are the "stray `n` leftover" class flagged for this cycle.

**Current**:
§8.3 note: "> **Logic preserved; rendering changed.** The confirm flow (`y`/`n`/`Esc`) is unchanged (parity); it inherits the new blank-screen rendering (§8.1) and the MV restyle."

§8.6 note: "> **Logic preserved; rendering changed.** The confirm flow (`y`/`n`/`Esc`) is unchanged (parity); it inherits the blank-screen rendering (§8.1) + MV restyle."

**Proposed Addition**:
*(leave blank until discussed)* — align the two parity notes with the authoritative `y`/`Esc` keymap: drop `n` from the note (and, if `n` was a real cancel key today, note that the cancel-on-`n` alias is intentionally removed rather than "unchanged"), OR, if `n` is meant to remain bound as an undisplayed cancel alias, add it back to §12.1 and the modal "Keys:" lines so all surfaces agree.

**Resolution**: Pending
**Notes**:

---
