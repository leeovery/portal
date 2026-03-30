AGENT: duplication
FINDINGS: none
SUMMARY: No significant duplication detected. The repeated t.Setenv line across 7 subtests is inherent to Go subtest isolation — each subtest requires its own t.TempDir() call, preventing extraction to a shared setup. Single-file implementation has no cross-file duplication to consolidate.
