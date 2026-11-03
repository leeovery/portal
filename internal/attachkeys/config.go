package attachkeys

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// actionName is an action's name in prefs.json's keys object.
type actionName struct {
	action Action
	name   string
}

// actionNames is in the order warnings and summaries name the actions.
var actionNames = []actionName{
	{Detach, "detach"},
	{DetachToShell, "detach_to_shell"},
	{Kill, "kill"},
}

func nameOf(a Action) string {
	for _, n := range actionNames {
		if n.action == a {
			return n.name
		}
	}
	return ""
}

var defaultKeys = map[Action]Key{
	Detach:        {mods: modAlt, char: 'd'},
	DetachToShell: {mods: modAlt | modShift, char: 'd'},
	Kill:          {mods: modAlt, char: 'q'},
}

// Bindings holds the keys bound to each action. An action bound to none is
// off.
type Bindings map[Action][]Key

// Defaults binds each action to its default key.
func Defaults() Bindings {
	b := Bindings{}
	for a, k := range defaultKeys {
		b[a] = []Key{k}
	}
	return b
}

// String names each action and its keys, or off.
func (b Bindings) String() string {
	parts := make([]string, 0, len(actionNames))
	for _, n := range actionNames {
		keys := "off"
		if bound := b[n.action]; len(bound) > 0 {
			names := make([]string, 0, len(bound))
			for _, k := range bound {
				names = append(names, k.String())
			}
			keys = strings.Join(names, ", ")
		}
		parts = append(parts, n.name+" "+keys)
	}
	return strings.Join(parts, "; ")
}

// Warning names something in the keys config the runner can't use, and why.
type Warning struct {
	// Action fell back to its default; zero for a warning about no one
	// action.
	Action Action
	// Value is what the warning is about: a key or an entry's name quoted,
	// any other value as JSON.
	Value string
	// Reason completes a sentence whose subject is Value. For a warning about
	// no one action, it also says what the runner does instead.
	Reason string
}

// String says what fell back to what, and why.
func (w Warning) String() string {
	if w.Action == 0 {
		return fmt.Sprintf("keys: %s %s", w.Value, w.Reason)
	}
	return fmt.Sprintf("%s falls back to %s: %s %s", nameOf(w.Action), defaultKeys[w.Action], w.Value, w.Reason)
}

// Parse reads prefs.json's keys object. A missing entry keeps its action's
// default, and an unknown one is ignored. An entry that doesn't parse, or
// binds a key the runner refuses, falls back to its action's default; so does
// each configured action with a key that sends another action's bytes. Every
// fallback and every unknown entry returns a warning.
func Parse(raw json.RawMessage) (Bindings, []Warning) {
	b := Defaults()
	if len(raw) == 0 {
		return b, nil
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return b, []Warning{{Value: compactJSON(raw), Reason: "is not an object, so every action keeps its default"}}
	}

	var warnings []Warning
	configured := map[Action]bool{}
	for _, n := range actionNames {
		value, ok := entries[n.name]
		if !ok {
			continue
		}
		keys, w := parseEntry(value)
		if w != nil {
			w.Action = n.action
			warnings = append(warnings, *w)
			continue
		}
		b[n.action] = keys
		// An entry naming just its default holds it against a clash, as a
		// missing entry does.
		configured[n.action] = !slices.Equal(keys, []Key{defaultKeys[n.action]})
	}
	warnings = append(warnings, settleClashes(b, configured)...)
	return b, append(warnings, unknownEntries(entries)...)
}

// unknownEntries names each entry that is no action's, in sorted order.
func unknownEntries(entries map[string]json.RawMessage) []Warning {
	var warnings []Warning
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		if !slices.ContainsFunc(actionNames, func(n actionName) bool { return n.name == name }) {
			warnings = append(warnings, Warning{Value: strconv.Quote(name), Reason: "is not an action, so it is ignored"})
		}
	}
	return warnings
}

// parseEntry reads one action's value: a key, or a list of keys. The warning
// it returns names no action.
func parseEntry(value json.RawMessage) ([]Key, *Warning) {
	texts, ok := keyTexts(value)
	if !ok {
		return nil, &Warning{Value: compactJSON(value), Reason: "is not a key or a list of keys"}
	}
	keys := []Key{}
	for _, text := range texts {
		k, err := bindable(text)
		if err != nil {
			return nil, &Warning{Value: strconv.Quote(text), Reason: err.Error()}
		}
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func keyTexts(value json.RawMessage) ([]string, bool) {
	var v any
	if err := json.Unmarshal(value, &v); err != nil {
		return nil, false
	}
	switch v := v.(type) {
	case string:
		return []string{v}, true
	case []any:
		texts := make([]string, 0, len(v))
		for _, e := range v {
			text, ok := e.(string)
			if !ok {
				return nil, false
			}
			texts = append(texts, text)
		}
		return texts, true
	}
	return nil, false
}

func bindable(text string) (Key, error) {
	k, err := parseKey(text)
	if err != nil {
		return Key{}, err
	}
	return k, refusal(k)
}

// settleClashes falls back each configured action bound to a key another
// action is bound to as well, until no two actions share one. The defaults
// share none, so each round settles at least one action for good.
func settleClashes(b Bindings, configured map[Action]bool) []Warning {
	var warnings []Warning
	for {
		k, first, second, ok := findClash(b)
		if !ok {
			return warnings
		}
		for _, pair := range [][2]Action{{first, second}, {second, first}} {
			mine, theirs := pair[0], pair[1]
			if !configured[mine] {
				continue
			}
			warnings = append(warnings, Warning{
				Action: mine,
				Value:  strconv.Quote(k.String()),
				Reason: "is also bound to " + nameOf(theirs),
			})
			b[mine] = []Key{defaultKeys[mine]}
			configured[mine] = false
		}
	}
}

// findClash finds a key two actions are bound to. Two bindable keys never
// send the same bytes, as refusal sees to and a test pins, so a key is the
// whole of what the matcher tells apart.
func findClash(b Bindings) (Key, Action, Action, bool) {
	owners := map[Key]Action{}
	for _, n := range actionNames {
		for _, k := range b[n.action] {
			if owner, ok := owners[k]; ok && owner != n.action {
				return k, owner, n.action, true
			}
			owners[k] = n.action
		}
	}
	return Key{}, 0, 0, false
}

func compactJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}
