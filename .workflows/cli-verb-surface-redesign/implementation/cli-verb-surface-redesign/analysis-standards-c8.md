AGENT: standards
FINDINGS:
- FINDING: doctor checkStatus enum places the healthy value at iota 0 (zero-value reads as "pass")
  SEVERITY: low
  FILES: cmd/doctor.go:38-50
  DESCRIPTION: The `checkStatus` enum introduced by the doctor command declares
    `checkPass checkStatus = iota` — the healthy outcome sits at the zero value.
    The golang-naming skill explicitly calls this out as a convention to avoid
    ("Enum zero values: Always place an explicit Unknown/Invalid sentinel at iota
    position 0 ... if [0] maps to a real state like StatusReady, code can behave
    as if a status was deliberately chosen when it wasn't"). For a health
    diagnostic this is the most dangerous default: a zero-value `checkResult{}`
    silently classifies as pass, and `doctorUnhealthy` (which drives the
    scriptable exit code, spec § Exit-code contract) counts only `checkFail`, so a
    forgotten status assignment would mask a failure as green rather than surface
    it. Practical impact is contained today because every `checkResult` in
    cmd/doctor.go is constructed with an explicit status field, so no zero-value
    leaks in the current code — this is a defensive/convention finding, not an
    active bug.
  RECOMMENDATION: Introduce an explicit sentinel at iota 0 (e.g. `checkUnknown`)
    and shift `checkPass`/`checkFail`/`checkInfo`/`checkNotEvaluable` up by one, so
    an uninitialised `checkResult` can never read as a passing health check.
    `checkMarker`/`doctorUnhealthy` already have a default arm, so the added
    sentinel needs no new call-site handling. Alternatively, if the zero-value
    placement is deliberate, add an inline comment stating the exemption (the
    skill permits ignoring a rule with a documenting comment) — the
    internal/spawn `SurfaceKind` enum already takes that documented-exemption
    route ("a zero Surface is an attach").
SUMMARY: The cli-verb-surface-redesign implementation is strongly conformant to
  the specification: the single public `open` verb with its precedence chain,
  domain pins, glob routing, multi-target burst (trigger-first net-N, atomic
  read-only pre-flight, leave-what-opened, mint-scoped command parity, hidden
  --ack), pinned-domain hard-fail contract, `doctor`/`--fix` catalog and
  exit-code + down-server-guard contract, runtime-only `uninstall`, unchanged
  `kill`, the `hooks`→`hook` rename with the silent back-compat alias, the fully
  hidden `state` namespace, bootstrap exemptions, tab-completion slots, and the
  spec-governed `resolve` log component (no call-site component/attr invention;
  the closed spawn attr-key set is respected) are all in place, and the retired
  attach/spawn/clean/state-status/state-cleanup surfaces are cleanly removed. The
  only drift found is one low-severity Go-convention deviation in the new doctor
  status enum.
