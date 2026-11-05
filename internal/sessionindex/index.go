package sessionindex

import (
	"errors"
	"log/slog"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrLabelUnprintable refuses a label that is not valid UTF-8 or holds a
// control, format or line/paragraph-separator character: a newline, tab,
// escape or bidi override would break or disguise the picker row and the
// footers that print it. Its text is the refusal every surface shows.
var ErrLabelUnprintable = errors.New("label refused: it holds a control or formatting character, such as a newline, tab or escape")

// ErrRemoved refuses a label for a session whose entry the same Update
// removed: an entry made again there would be a replace no claim judged.
var ErrRemoved = errors.New("the session's entry was removed in this update")

type change struct {
	op      string
	session string
	value   string
	// detail says what the change dropped or displaced, empty when nothing.
	detail string
	// level is the change's log level once written: INFO, its zero value,
	// unless the change is one the saver makes on most polls.
	level slog.Level
}

// Lookup returns a copy of the entry named name as stored, whatever creation
// it records; surfaces that show a live session read through LookupLive.
// Changes go through Index's own methods, which record them for Update to log.
func (idx *Index) Lookup(name string) (Entry, bool) {
	i := idx.indexOf(name)
	if i < 0 {
		return Entry{}, false
	}
	return idx.sessions[i], true
}

// SetLabel sets the session's label, trimmed of surrounding whitespace. An
// empty label, or one equal to the name, clears it back to the default. A
// session with no entry gets a bare one only when it is given a label;
// clearing one with no entry changes nothing. A label still holding an
// unprintable character once trimmed is refused with ErrLabelUnprintable and
// changes nothing.
func (idx *Index) SetLabel(name, label string) error {
	label = strings.TrimSpace(label)
	if !printable(label) {
		return ErrLabelUnprintable
	}
	if label == name {
		label = ""
	}
	i := idx.indexOf(name)
	if i < 0 {
		if label == "" {
			return nil
		}
		var made bool
		if i, made = idx.add(Entry{Name: name}); !made {
			return ErrRemoved
		}
	}
	if idx.sessions[i].Label == label {
		return nil
	}
	idx.sessions[i].Label = label
	if label == "" {
		idx.changes = append(idx.changes, change{op: "clear-label", session: name})
	} else {
		idx.changes = append(idx.changes, change{op: "set-label", session: name, value: label})
	}
	return nil
}

// The zero-width joiner is format-class but builds joined emoji, so it stays.
func printable(label string) bool {
	return utf8.ValidString(label) && !strings.ContainsFunc(label, func(r rune) bool {
		return r != '\u200d' && (unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp))
	})
}

// SetScrollback names the file, relative to the state directory, holding the
// session's saved scrollback. It changes only an entry the session has.
func (idx *Index) SetScrollback(name, file string) {
	idx.setField(name, "set-scrollback", file, slog.LevelInfo, func(e *Entry) *string { return &e.ScrollbackFile })
}

// RecordCwd records the session's working directory. It changes only an
// entry the session has, and logs at DEBUG: the saver records it on most
// polls.
func (idx *Index) RecordCwd(name, cwd string) {
	idx.setField(name, "record-cwd", cwd, slog.LevelDebug, func(e *Entry) *string { return &e.Cwd })
}

// setField sets the field of name's entry to value, recording op when that
// changes it.
func (idx *Index) setField(name, op, value string, level slog.Level, field func(*Entry) *string) {
	i := idx.indexOf(name)
	if i < 0 {
		return
	}
	if f := field(&idx.sessions[i]); *f != value {
		*f = value
		idx.changes = append(idx.changes, change{op: op, session: name, value: value, level: level})
	}
}

// Owe marks the session's entry owed a restore, reporting whether that
// changed it. It changes only an entry the session has. Its caller is
// restore, which a guard holds to.
func (idx *Index) Owe(name string) bool {
	i := idx.indexOf(name)
	return i >= 0 && idx.owe(i, "")
}

// owe marks the entry at i owed a restore, recording why when given, and
// reports whether that changed it.
func (idx *Index) owe(i int, why string) bool {
	if idx.sessions[i].Owed {
		return false
	}
	idx.sessions[i].Owed = true
	idx.changes = append(idx.changes, change{op: "set-owed", session: idx.sessions[i].Name, detail: why})
	return true
}

// repay clears the owed mark of the entry at i, recording why, and reports
// whether it had one.
func (idx *Index) repay(i int, why string) bool {
	if !idx.sessions[i].Owed {
		return false
	}
	idx.sessions[i].Owed = false
	idx.changes = append(idx.changes, change{op: "clear-owed", session: idx.sessions[i].Name, detail: why})
	return true
}

// Remove drops the session's entry, reporting whether there was one. No
// method makes an entry under the name again in the same Update. Its callers
// are kill and the saver's drop of a session that ended, which a guard holds
// to.
func (idx *Index) Remove(name string) bool {
	i := idx.indexOf(name)
	if i < 0 {
		return false
	}
	idx.sessions = slices.Delete(idx.sessions, i, i+1)
	if idx.removed == nil {
		idx.removed = map[string]bool{}
	}
	idx.removed[name] = true
	idx.changes = append(idx.changes, change{op: "rm", session: name})
	return true
}

// add appends e, an entry made afresh rather than read, and returns its
// position, or reports it made none for a name removed in this Update.
func (idx *Index) add(e Entry) (int, bool) {
	if idx.removed[e.Name] {
		return -1, false
	}
	idx.markFresh(e.Name)
	idx.sessions = append(idx.sessions, e)
	return len(idx.sessions) - 1, true
}

// markFresh records that name's entry was made afresh rather than read, so
// Update reports an entry it read under that name as dropped.
func (idx *Index) markFresh(name string) {
	if idx.fresh == nil {
		idx.fresh = map[string]bool{}
	}
	idx.fresh[name] = true
}

func (idx *Index) indexOf(name string) int {
	return slices.IndexFunc(idx.sessions, func(e Entry) bool { return e.Name == name })
}
