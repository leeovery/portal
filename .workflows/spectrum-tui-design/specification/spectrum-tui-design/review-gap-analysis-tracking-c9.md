---
status: complete
created: 2026-06-18
cycle: 9
phase: Gap Analysis
topic: spectrum-tui-design
---

# Review Tracking: spectrum-tui-design - Gap Analysis

## Findings

### 1. `state.green` reservation/role prose understates green's actual five uses

**Source**: Specification analysis (§2.1, §2.9, §3.2, §6.1, §10.3, §11.2)
**Category**: Gap/Ambiguity
**Affects**: §2.1 (line 56), §2.9 token table + Rules (lines 128, 146); cross-checked against §3.2, §6.1, §10.3, §11.2

**Details**:
`state.green` is now used in **five** distinct functional places across the spec:
1. `● attached` marker (§4.1 line 202)
2. Sessions section-header **count** (§3.2 line 172 — "`state.green` for the Sessions count")
3. **Projects** section-header **label** (§3.2 line 171 / §6.1 line 254)
4. `✓ done` loading tick (§10.3 line 402)
5. success flash (§11.2 line 442)

The reservation/role prose does not match this:
- **§2.1 line 56** states green is "reserved for **the attached state only** — never reused for chips or decoration." This is flatly contradicted by uses 2–5. A colour-by-role implementer reading §2.1 first could treat the green Projects header / Sessions count / done-tick / success flash as rule violations.
- **§2.9 Rules line 146** says "`state.green` is **attached-only** (+ success flash)" — acknowledges use 5 but omits uses 2, 3, 4.
- **§2.9 token-table role column line 128** lists only "`● attached`, `✓` done, success flash" — it enumerates uses 1, 4, 5 but **omits the Sessions count (use 2) and the Projects label (use 3)**. Since §2.9 is the closed-vocabulary source of truth ("every renderer references a token"), the omission means the table doesn't document two real call-sites that need the token.

This is a cumulative consequence of the canvas-reversal + cycle 6–8 reconciliation: "+ success flash" was bolted onto §2.9's prose, but the broader "attached-only" reservation (§2.1) and the §2.9 table's role list were never harmonised with green's full usage set. The §3.2/§6.1 green-for-Projects-header and green-Sessions-count are existing, mocked design decisions — not being re-opened; the issue is purely that the role/reservation prose is internally inconsistent with the usage sections it governs.

The genuine "don't decorate with green" intent (chips, arbitrary decoration) is clear and correct — but the literal wording "attached state only" / the incomplete table role list creates a real contradiction an implementer must resolve by guessing which statement wins.

**Proposed Addition**:
(a) §2.1 green role: "reserved for **live / positive** signals — the attached marker, Sessions count, Projects label, `✓` done-tick, success flash — never chips or decoration."
(b) §2.9 accents-table role cell: "`● attached`, Sessions count, Projects label, `✓` done, success flash".
(c) §2.9 Rules: "`state.green` carries **live / positive** signals (attached marker, Sessions count, Projects label, `✓` done-tick, success flash) — **never** chips or decoration; ...".

**Resolution**: Approved
**Notes**: Harmonised the green role/reservation prose with its five actual call-sites; the §2.9 closed-vocabulary table now documents the Sessions count + Projects label sites it omitted. Pre-existing inconsistency, exposed by the success-flash work; "never chips/decoration" intent preserved.

---

### 2. §11 notice-colour convention omits the green success band (third notice colour)

**Source**: Specification analysis (§11, §11.2)
**Category**: Gap/Ambiguity
**Affects**: §11 shared-convention paragraph (line 433) and single-slot rule (line 435); §11.2 (line 442)

**Details**:
The §11 "Shared convention — left-bar accent notices" paragraph (line 433) presents itself as the authoritative colour map for inline notices: "Inline notices use a `▌` left-bar accent line: **`accent.orange`** = transient / warning, **`accent.violet`** = mode / info." It enumerates **two** notice-band colours.

§11.2 (line 442) then introduces a **third** notice-band colour: the success flash uses **`state.green`** with a `✓` glyph. The success flash is a transient band that occupies the same single notice slot (§11 line 435 explicitly governs "a transient flash (§11.2)" under the single-slot rule).

Two spots are left under-counted:
- The §11 colour map (line 433) does not list the green success variant, so the paragraph that claims to map notice colours is incomplete.
- The §11 single-slot mutual-exclusion sentence (line 435) — "Orange (warning) and violet (info) never display at once — the transient flash wins while shown" — names only orange and violet and omits green, even though green-success is itself the transient flash that "wins."

§11.2 does fully describe the green success band, and the single-slot rule clearly covers it by reference, so an implementer would not be blocked — but the convention paragraph and the exclusion sentence read as a complete two-colour enumeration that the green success case silently violates. Minor: a one-clause addition to line 433 (and/or generalising line 435 to "the transient flash wins over any persistent band") closes it.

**Proposed Addition**:
(a) §11 convention map: "... **`accent.orange`** = transient / warning, **`state.green`** = transient / success, **`accent.violet`** = mode / info."
(b) §11 single-slot rule: "A persistent (violet info) band and a transient flash (orange warning or green success) never display at once — the transient flash wins while shown."

**Resolution**: Approved
**Notes**: §11 convention now enumerates all three notice colours and the exclusion sentence generalises to persistent-vs-transient.

---
