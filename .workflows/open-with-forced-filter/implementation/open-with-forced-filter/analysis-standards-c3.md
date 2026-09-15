AGENT: standards
FINDINGS: none
COMMENT_CORRECTIONS:
- internal/tui/model.go:550 — the doc claims every search form lands on a committed filter, which is false for the term-less form the option also carries (it lands focused and empty)
  OLD: // which lands as the committed sessions filter on the Sessions page.
  NEW: // which lands on the Sessions page as the committed sessions filter — or, for
       // an empty term, as that filter opened focused and empty.
SUMMARY: Full re-read of the specification against the implementation found no conformance drift: the sigil's recognition rule, its resolution outcomes by match count, the committed-vs-focused landing, the containment rule and its entry-point divergence, the matched-and-displayed home-abbreviated directory, the argv-composition refusals decided before any bootstrap, the picker classification and its teardown warning delivery, the completion shim across all three shells, and the help/README copy all match what was decided. One exported option's doc comment contradicts the term-less landing it also serves.
