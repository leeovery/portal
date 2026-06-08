TASK: Add `tags []string` field to Project record (session-tagging-and-grouping-1-1)

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA:
- The `Project` record carries a `tags []string` field
- A `projects.json` lacking the field decodes to nil/empty (zero-tag state) with no migration step
- Edge cases: missing tags field decodes to nil/empty (no migration), existing records round-trip unchanged, null vs [] in JSON

SPEC CONTEXT:
Spec "Tag Data Model & Persistence" (specification.md:42-49): Project gains `tags []string` in projects.json; missing field decodes to nil/empty, no migration required.

IMPLEMENTATION:
- Status: Implemented at internal/project/store.go:36-41 (`Tags []string \`json:"tags,omitempty"\`` line 40).
- `omitempty` keeps on-disk form clean; encoding/json decodes missing key, `null`, and `[]` all to zero-tag state. Upsert (store.go:119-141) never touches Tags, so existing tags survive the session-pipeline Upsert. No migration. No drift.

TESTS:
- Status: Adequate. store_test.go:79-233 TestTagsField — five subtests: no field → nil & len 0; tags:null → empty; tags:[] → empty; Save→Load round-trip preserved; Upsert preserves Tags while bumping Name + LastUsed.
- All edge cases covered; nil-vs-empty distinction pinned; real Store + tempdir, no mocking. Not over-tested.

CODE QUALITY: Project conventions followed; SOLID good; complexity low; idiomatic. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
