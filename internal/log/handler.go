package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// stderrFallback is the best-effort last-resort destination for a serialized
// record when the primary sink write fails (open/reopen failure or a mid-record
// write error — disk-full / EACCES / ENOSPC). It is a package var rather than a
// handler field so the policy is single-sourced and tests can capture the
// fallback output by swapping it (restoring via t.Cleanup). Production always
// uses os.Stderr.
var stderrFallback io.Writer = os.Stderr

// componentKey is the baseline attr key carrying the subsystem name. It is set
// per-package via For (root.With("component", ...)) and rendered by textHandler
// as a literal prefix before the colon — never as a key=value pair.
const componentKey = "component"

// processComponent is the component name carried by the portal-binary lifecycle
// markers. A record bypasses the level filter only when its component is this
// value AND its message is in lifecycleBypassMsgs.
const processComponent = "process"

// lifecycleBypassMsgs is the CLOSED process-lifecycle message set from the spec
// (§ "Defensive invariants against log destruction" → "Lifecycle markers bypass
// the level filter"). A record whose component is "process" and whose message is
// one of these writes through the configured level gate UNCONDITIONALLY — these
// are forensic tripwires (and, for "log-level resolved", a test anchor) that must
// appear even at PORTAL_LOG_LEVEL=warn/error. They remain semantically INFO; the
// bypass is the mechanism, not a level change. Adding to this set requires a spec
// amendment.
var lifecycleBypassMsgs = map[string]bool{
	"start":              true,
	"exit":               true,
	"exec":               true,
	"panic":              true,
	"log-level resolved": true,
}

// textHandler is the configured tail/grep-default slog.Handler. It renders one
// line per record in the form:
//
//	<RFC3339Nano> <LEVEL> <component>: <msg> <contextual attrs> pid=… version=… process_role=…
//
// The component attr (whether arriving via the record or via accumulated
// WithAttrs from For) is rendered as the literal prefix immediately before the
// colon and omitted from the key=value list. The three remaining baselines
// (pid/version/process_role) are injected per-record — NOT via root.With — so a
// logger cached before this handler was constructed still carries them.
//
// This is the SIMPLE single-writer variant: each Handle performs one unbuffered
// Write to w. The rotating *os.File wiring and retention sweeps are Phase 2.
//
// JSON-mode seam: the spec also defines a JSON rendering (standard
// slog.NewJSONHandler with component as an ordinary field). Phase 1 ships
// text-mode only; the selection mechanism between text and JSON is intentionally
// left unspecified here — no selection env var is invented. A future task wires
// the JSON handler behind the same swap indirection.
type textHandler struct {
	w     io.Writer
	level slog.Leveler

	// Baselines captured at construction (from Init) and injected per-record.
	pid         int
	version     string
	processRole string

	// attrs is the accumulated WithAttrs chain (sticky .With context), each attr
	// already carrying any dotted group prefix in force when it was added.
	attrs []prefixedAttr
	// groupPrefix is the dotted prefix accumulated from WithGroup calls, applied
	// to every attr added or recorded while the group(s) are open.
	groupPrefix string
}

// prefixedAttr is a slog.Attr paired with the dotted group prefix that was in
// force when it was accumulated via WithAttrs.
type prefixedAttr struct {
	prefix string
	attr   slog.Attr
}

// newTextHandler constructs the configured text-mode handler. w is the
// single-writer sink; level gates Enabled; pid/version/processRole are the
// baselines injected per-record.
func newTextHandler(w io.Writer, level slog.Leveler, pid int, version, processRole string) slog.Handler {
	return &textHandler{
		w:           w,
		level:       level,
		pid:         pid,
		version:     version,
		processRole: processRole,
	}
}

// Enabled is a COARSE INFO-floor pre-gate, not the authoritative level filter —
// Handle is (see Handle's doc). slog's Logger.Info calls Enabled(ctx, LevelInfo)
// first and skips Handle entirely when it returns false, so Enabled MUST admit
// INFO even when the handler is configured at WARN/ERROR; otherwise a lifecycle
// INFO marker would be dropped before Handle ever sees its component+msg and
// could apply the bypass. It admits anything at the INFO floor OR at/above the
// configured level: at WARN this admits INFO+, at DEBUG it admits DEBUG+. The
// authoritative drop for non-lifecycle INFO happens in Handle.
func (h *textHandler) Enabled(_ context.Context, level slog.Level) bool {
	floor := min(h.level.Level(), slog.LevelInfo)
	return level >= floor
}

// Handle renders one line for r and writes it to the sink in a single
// unbuffered Write. The write path is BEST-EFFORT: logging owns no control flow,
// so an open/reopen failure or a mid-record write error (disk-full / EACCES /
// ENOSPC) is swallowed here — the serialized record is attempted once on the
// stderr fallback and Handle ALWAYS returns nil. It never propagates an error to
// the slog caller and never panics on any I/O failure. (slog ignores a handler's
// returned error in practice; the explicit nil makes the "logging never crashes
// portal" contract unambiguous.)
func (h *textHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder

	component := h.component(r)

	// The HANDLER — not Enabled — is the authoritative level-filter. Enabled is
	// only a coarse INFO-floor pre-gate (so lifecycle INFO is never skipped before
	// Handle sees the record's component+msg), which means non-lifecycle INFO
	// records can reach Handle even when configured at WARN; Handle drops them
	// here (negligible cost). A process-component record whose msg is in the closed
	// lifecycle set ALWAYS writes — it bypasses the level gate unconditionally.
	bypass := component == processComponent && lifecycleBypassMsgs[r.Message]
	if !bypass && r.Level < h.level.Level() {
		return nil
	}

	b.WriteString(r.Time.Format(time.RFC3339Nano))
	b.WriteByte(' ')
	b.WriteString(r.Level.String())
	b.WriteByte(' ')
	b.WriteString(component)
	b.WriteString(": ")
	b.WriteString(r.Message)

	// Contextual attrs: accumulated WithAttrs (sticky context) first, then the
	// record's own attrs, both in declaration order, component excluded.
	for _, pa := range h.attrs {
		if pa.prefix == "" && pa.attr.Key == componentKey {
			continue
		}
		writeAttr(&b, pa.prefix, pa.attr)
	}
	r.Attrs(func(a slog.Attr) bool {
		if h.groupPrefix == "" && a.Key == componentKey {
			return true
		}
		writeAttr(&b, h.groupPrefix, a)
		return true
	})

	// Baselines last, injected per-record (component already rendered as prefix).
	b.WriteString(" pid=")
	b.WriteString(strconv.Itoa(h.pid))
	b.WriteString(" version=")
	b.WriteString(quoteIfMultiWord(h.version))
	b.WriteString(" process_role=")
	b.WriteString(quoteIfMultiWord(h.processRole))

	b.WriteByte('\n')

	h.bestEffortWrite(b.String())
	return nil
}

// bestEffortWrite performs the single unbuffered write of the serialized record
// to the sink. On any sink error — an open/reopen failure surfaced by the sink,
// or a mid-record write(2) failure — it drops the record from the primary sink
// and attempts the line ONCE on the stderr fallback. It never returns an error
// and never panics: every failure mode is swallowed here so Handle's "always
// returns nil" contract holds. The stderr fallback write is itself best-effort —
// if stderr is also gone, there is nowhere left to go and the record is dropped.
func (h *textHandler) bestEffortWrite(line string) {
	if _, err := io.WriteString(h.w, line); err != nil {
		_, _ = fmt.Fprint(stderrFallback, line)
	}
}

// component resolves the subsystem prefix from the record's attrs first, then
// the accumulated WithAttrs chain (where For delivers it). It is found
// regardless of which path supplied it.
func (h *textHandler) component(r slog.Record) string {
	var found string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == componentKey {
			found = a.Value.Resolve().String()
			return false
		}
		return true
	})
	if found != "" {
		return found
	}
	for _, pa := range h.attrs {
		if pa.prefix == "" && pa.attr.Key == componentKey {
			return pa.attr.Value.Resolve().String()
		}
	}
	return found
}

// WithAttrs accumulates attrs under the current group prefix, returning a derived
// handler. The component attr is carried through unchanged so component resolves
// regardless of whether it arrives here or on the record.
func (h *textHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	clone := h.clone()
	for _, a := range attrs {
		clone.attrs = append(clone.attrs, prefixedAttr{prefix: h.groupPrefix, attr: a})
	}
	return clone
}

// WithGroup opens a named group: attrs added or recorded while it is open render
// with a dotted prefix, mirroring the JSON handler's nesting. An empty name is a
// no-op per the slog.Handler contract.
func (h *textHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := h.clone()
	clone.groupPrefix = h.groupPrefix + name + "."
	return clone
}

// clone returns a shallow copy with an independent attrs slice so derived
// handlers do not alias the parent's accumulated chain.
func (h *textHandler) clone() *textHandler {
	attrs := make([]prefixedAttr, len(h.attrs))
	copy(attrs, h.attrs)
	c := *h
	c.attrs = attrs
	return &c
}

// writeAttr renders a single attr as a space-prefixed key=value pair, flattening
// groups to dotted keys and applying value-formatting rules.
func writeAttr(b *strings.Builder, prefix string, a slog.Attr) {
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		group := v.Group()
		if len(group) == 0 {
			return
		}
		// An empty group key is inlined (no dotted segment) per slog semantics.
		nested := prefix
		if a.Key != "" {
			nested = prefix + a.Key + "."
		}
		for _, ga := range group {
			writeAttr(b, nested, ga)
		}
		return
	}
	if a.Key == "" {
		return
	}
	b.WriteByte(' ')
	b.WriteString(prefix)
	b.WriteString(a.Key)
	b.WriteByte('=')
	b.WriteString(formatValue(v))
}

// formatValue renders a resolved slog.Value: durations via String(), strings
// quoted when multi-word, everything else via the standard slog string form.
func formatValue(v slog.Value) string {
	if v.Kind() == slog.KindDuration {
		return v.Duration().String()
	}
	if v.Kind() == slog.KindString {
		return quoteIfMultiWord(v.String())
	}
	return v.String()
}

// quoteIfMultiWord wraps s in double quotes when it contains any whitespace;
// single-token values are returned unquoted.
func quoteIfMultiWord(s string) string {
	if strings.ContainsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }) {
		return `"` + s + `"`
	}
	return s
}
