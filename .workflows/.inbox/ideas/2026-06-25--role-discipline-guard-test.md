# Mechanical guard test for the §2.9 colour role-discipline rule

The §2.9 token vocabulary carries a reservation rule beyond "no literal hex": `state.green` is for live/positive signals only (attached marker, Sessions count, Projects label, `✓` done-tick, success flash — never chips or decoration), `state.red` is destructive-only, and `text.faint` is decorative-only (must never carry functional text).

The "no literal hex at call sites" half of §2.9 is already locked executably by the glob-based colour-literal guard (`colour_literal_guard_test.go`). The *role-discipline* half is currently enforced only by reviewer audit plus per-call-site comments — there is no test that fails if, say, a future change renders a chip in `state.green` or puts functional text in `text.faint`.

Add a mechanical assertion in `theme/theme_test.go` (or a new guard test) that locks the reservation the way the hex guard locks the literal-hex rule. The tricky part — and why this is a thinking item, not a ready quick-fix — is expressing "live/positive-only" / "destructive-only" / "decorative-only" as a checkable property: it likely needs either a small annotation of which render call-sites are allowed to reference each reserved token, or an AST/string-scan heuristic over the render files. Worth a little design before writing.

Source: review of spectrum-tui-design/spectrum-tui-design (report 1-3).
