package attachkeys

import "strconv"

// WrittenKeys is every key the grammar can write: each set of modifiers, none
// included, on each printable character and function key.
func WrittenKeys() []string {
	names := []string{"space"}
	for c := byte('!'); c <= '~'; c++ {
		names = append(names, string(c))
	}
	for n := 1; n <= 12; n++ {
		names = append(names, "f"+strconv.Itoa(n))
	}
	var texts []string
	for _, mods := range []string{"", "ctrl+", "alt+", "shift+", "ctrl+alt+", "ctrl+shift+", "alt+shift+", "ctrl+alt+shift+"} {
		for _, name := range names {
			texts = append(texts, mods+name)
		}
	}
	return texts
}
