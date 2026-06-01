package log

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// componentKey is the baseline attr key carrying the subsystem name. It is set
// per-package via For (root.With("component", ...)) and rendered by textHandler
// as a literal prefix before the colon — never as a key=value pair.
const componentKey = "component"

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

// Enabled reports whether a record at the given level should be handled. This is
// an ordinary level gate against the handler's slog.Leveler — the
// lifecycle-marker level-filter bypass is Phase 2.
func (h *textHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// Handle renders one line for r and writes it to the sink in a single
// unbuffered Write.
func (h *textHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder

	component := h.component(r)

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

	_, err := io.WriteString(h.w, b.String())
	return err
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
