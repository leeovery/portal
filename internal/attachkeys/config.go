package attachkeys

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// actionNames gives each action its name in prefs.json's keys object, in the
// order warnings and summaries name them.
var actionNames = []struct {
	action Action
	name   string
}{
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

// Warning names a binding that fell back to its action's default, and why.
type Warning struct {
	// Action fell back to its default. It is zero when the keys value as a
	// whole is unusable, and every action keeps its default.
	Action Action
	// Value is the binding as configured: a key quoted, any other value as
	// JSON.
	Value string
	// Reason completes a sentence whose subject is Value.
	Reason string
}

// String says what fell back to what, and why.
func (w Warning) String() string {
	if w.Action == 0 {
		return fmt.Sprintf("keys keep their defaults: %s %s", w.Value, w.Reason)
	}
	return fmt.Sprintf("%s falls back to %s: %s %s", nameOf(w.Action), defaultKeys[w.Action], w.Value, w.Reason)
}

// Parse reads prefs.json's keys object. A missing entry keeps its action's
// default, and an unknown one is ignored. An entry that doesn't parse, or
// binds a key the runner refuses, falls back to its action's default; so does
// each configured action with a key that sends another action's bytes. Every
// fallback returns a warning.
func Parse(raw json.RawMessage) (Bindings, []Warning) {
	b := Defaults()
	if len(raw) == 0 {
		return b, nil
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return b, []Warning{{Value: compactJSON(raw), Reason: "is not an object"}}
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
	return b, append(warnings, settleClashes(b, configured)...)
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

// claim is one action's hold on a key.
type claim struct {
	action Action
	key    Key
}

// settleClashes falls back each configured action whose key sends the same
// bytes as another action's, until no two actions share any. The defaults
// share none, so each round settles at least one action for good.
func settleClashes(b Bindings, configured map[Action]bool) []Warning {
	var warnings []Warning
	for {
		first, second, ok := findClash(b)
		if !ok {
			return warnings
		}
		for _, c := range [][2]claim{{first, second}, {second, first}} {
			mine, theirs := c[0], c[1]
			if !configured[mine.action] {
				continue
			}
			warnings = append(warnings, Warning{
				Action: mine.action,
				Value:  strconv.Quote(mine.key.String()),
				Reason: clashReason(mine.key, theirs),
			})
			b[mine.action] = []Key{defaultKeys[mine.action]}
			configured[mine.action] = false
		}
	}
}

// findClash finds two actions with keys that send the same bytes without the
// newer encoding, which can't tell them apart.
func findClash(b Bindings) (claim, claim, bool) {
	owners := map[string]claim{}
	for _, n := range actionNames {
		for _, k := range b[n.action] {
			seq := k.legacy()
			if owner, ok := owners[seq]; ok && owner.action != n.action {
				return owner, claim{action: n.action, key: k}, true
			}
			owners[seq] = claim{action: n.action, key: k}
		}
	}
	return claim{}, claim{}, false
}

func clashReason(mine Key, theirs claim) string {
	if mine == theirs.key {
		return "is also bound to " + nameOf(theirs.action)
	}
	return fmt.Sprintf("sends the same bytes as %q, bound to %s", theirs.key.String(), nameOf(theirs.action))
}

func compactJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}
