# Review Tracking: Lazy Resume On Attach - Input Review

## Findings

### 1. The one way this feature can fail silently leaves no trace anywhere

**Source**: `.workflows/lazy-resume-on-attach/discussion/lazy-resume-on-attach.md` — *When The Marker Cannot Be Written Or Cleared* (decision: "If the pending marker cannot be written, the helper does not paint: it fires the hook as an eager registration does, and the pane comes back as today's restore leaves it"). The degradation is decided; whether anything records it was never raised.
**Category**: Gap/Ambiguity
**Move**: decide
**Affects**: §8.2 (a pending pane is marked explicitly); sits beside the feature's other emission in §6.4

**Problem**:
When the write that protects a pane will not land, the pane gives up waiting and starts its process instead — and nothing records that it happened. On the boot where that bites, the user gets back exactly the behaviour this work exists to remove: sessions launching processes nobody asked for, and no panel on the pane to explain why. If the condition that refused one write refuses many, a whole reboot's worth of sessions comes back running. From outside, that install is indistinguishable from one deliberately set to resume everything eagerly — the pending count reads zero in both, the panes look the same, and there is nothing to grep. The user's only conclusion is that the feature does not work on their machine, with no way to find out it was a write that failed. Portal's log vocabulary is closed and spec-governed, so an implementer cannot add the missing breadcrumb on their own initiative — silence here is silence in the product.

**Proposal**:
State that the fall-through is recorded: one WARN from the helper as it fires the hook, naming the pane and the error that refused the marker, as one more event on the helper's existing hydrate catalog rather than a new component. What leaned: this is the only degradation the feature introduces that the user cannot see on the pane in front of them — a discard the store refuses and a freeze that will not lift both report on the panel — and the feature's other destructive path already takes the view that a consequential act costs nothing to record and is worth recovering. Alternatives that also fit the record: emit nothing, on the grounds that the pane lands exactly where eager would have put it, which the feature already accepts as its sanctioned degradation; or emit at INFO, treating a fall-through as ordinary rather than exceptional.

**Proposed Text**:
Add to §8.2, immediately after the paragraph beginning "**A pane that cannot be marked does not wait.**":

**The fall-through is recorded.** The helper emits one WARN as it fires the hook, naming the pane and the error that refused the marker — one more event on its existing hydrate catalog, not a new component. This is the only degradation the feature introduces that the user cannot read off the pane in front of them: a discard the store will not accept and a freeze that will not lift both report on the panel (§5.3). Without the line, an install that came back eager because a write failed is indistinguishable from one that is configured eager.

**Resolution**: Pending
**Notes**:

---

## Observations

- The discard confirmation's consequence line is the only user-facing string on the new surface whose wording is left to the Paper frame rather than stated; every other string on the panel and the row is given verbatim.
- The prefs key `resume_mode`, the pane option `@portal-resume-pending`, and the log tokens `discard` and `panel` are named by the specification and by no source — each is the name any implementer would reach for inside Portal's existing vocabularies.
- That every screen transition (the confirmation, the back-out, a redrawn report row) takes the same draw-then-hand-off handover is the specification's own generalisation of the resize rule; the sources state the handover for the first draw and for a resize only.
