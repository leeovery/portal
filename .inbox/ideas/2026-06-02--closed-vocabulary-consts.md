# Compiler-check the closed component / op / via / reason vocabularies

The feature's whole premise is a *closed* taxonomy (15 components, closed `op`/`via`/`reason` value spaces) so `grep "<component>:" portal.log` reconstructs any subsystem. Today these are enforced only by per-site string literals plus convention — `main.go` hardcodes `"process"`, two packages independently write `log.For("signal")`, and stores pass free-string `op`/`via`. A typo (`log.For("siganl")`) or an off-vocabulary `op` would silently mis-route a line and defeat the grep-reconstruction guarantee that is the entire point of the layer; the existing drift-tripwire test only covers `process_role`, not component/attr-key typos.

Recommendation: export `const`s from `internal/log` (e.g. `log.ComponentProcess`, `log.ComponentSignal`, …) and typed value sets for the closed `op`/`via`/`reason` enums, and migrate the call sites onto them so the closed vocabulary is compiler-checked.

Scope guidance (from review): scope this to the **15 component names + the closed value enums (`op`/`via`/`reason`)** — NOT all 49 attr keys. Bare-string attr keys are idiomatic slog, and consts there cost more readability than they buy. Pure addition + mechanical call-site swap, low behavioural risk. The implementation analysis cycles deliberately deferred this as cross-cutting; it was rated the highest-value remaining idea and the one worth doing as its own focused change.

Source: review of portal-observability-layer/portal-observability-layer
