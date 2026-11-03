package attachkeys_test

import (
	"encoding/json"
	"slices"
	"strconv"
	"testing"

	"github.com/leeovery/portal/internal/attachkeys"
)

const defaultsSummary = "detach alt+d; detach_to_shell alt+shift+d; kill alt+q"

// parse reads a keys object and returns its bindings' summary and warnings.
func parse(t *testing.T, raw string) (string, []attachkeys.Warning) {
	t.Helper()
	var msg json.RawMessage
	if raw != "" {
		msg = json.RawMessage(raw)
	}
	b, warnings := attachkeys.Parse(msg)
	return b.String(), warnings
}

func assertClean(t *testing.T, raw, want string) {
	t.Helper()
	got, warnings := parse(t, raw)
	if len(warnings) != 0 {
		t.Errorf("Parse(%s) warnings = %v; want none", raw, warnings)
	}
	if got != want {
		t.Errorf("Parse(%s) = %q; want %q", raw, got, want)
	}
}

func assertWarnings(t *testing.T, raw, wantSummary string, want ...string) {
	t.Helper()
	got, warnings := parse(t, raw)
	var lines []string
	for _, w := range warnings {
		lines = append(lines, w.String())
	}
	if !slices.Equal(lines, want) {
		t.Errorf("Parse(%s) warnings =\n  %q\nwant\n  %q", raw, lines, want)
	}
	if got != wantSummary {
		t.Errorf("Parse(%s) = %q; want %q", raw, got, wantSummary)
	}
}

func TestParse_Shape(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{"no keys object keeps every default", "", defaultsSummary},
		{"a null keys object keeps every default", "null", defaultsSummary},
		{"an empty object keeps every default", "{}", defaultsSummary},
		{"a missing entry keeps its default", `{"kill":"alt+x"}`, "detach alt+d; detach_to_shell alt+shift+d; kill alt+x"},
		{"a value may be one key", `{"detach":"ctrl+o"}`, "detach ctrl+o; detach_to_shell alt+shift+d; kill alt+q"},
		{"a value may be a list of keys", `{"detach":["alt+x","ctrl+o","alt+f5"]}`, "detach alt+x, ctrl+o, alt+f5; detach_to_shell alt+shift+d; kill alt+q"},
		{"an empty list turns the action off", `{"kill":[]}`, "detach alt+d; detach_to_shell alt+shift+d; kill off"},
		{"a key repeated in one list counts once", `{"detach":["alt+x","ALT+X"]}`, "detach alt+x; detach_to_shell alt+shift+d; kill alt+q"},
		{"rebinding a default to its own action is no clash", `{"detach":"alt+d","kill":"alt+x"}`, "detach alt+d; detach_to_shell alt+shift+d; kill alt+x"},
		{"two actions may swap their defaults", `{"detach":"alt+q","kill":"alt+d"}`, "detach alt+q; detach_to_shell alt+shift+d; kill alt+d"},
		{"an off action leaves its default free", `{"kill":[],"detach":"alt+q"}`, "detach alt+q; detach_to_shell alt+shift+d; kill off"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertClean(t, tc.raw, tc.want) })
	}

	t.Run("unknown entries are ignored, each named in sorted order", func(t *testing.T) {
		assertWarnings(t, `{"extra":{"a":[1]},"detatch":"nonsense","detach":"alt+x"}`,
			"detach alt+x; detach_to_shell alt+shift+d; kill alt+q",
			`keys: "detatch" is not an action, so it is ignored`,
			`keys: "extra" is not an action, so it is ignored`)
	})
}

func TestParse_KeyGrammar(t *testing.T) {
	cases := []struct {
		name, key, want string
	}{
		{"alt and a letter", "alt+x", "alt+x"},
		{"ctrl and a letter", "ctrl+o", "ctrl+o"},
		{"ctrl, alt and a letter", "ctrl+alt+o", "ctrl+alt+o"},
		{"alt, shift and a letter", "alt+shift+k", "alt+shift+k"},
		{"modifiers in any order", "shift+alt+k", "alt+shift+k"},
		{"case-insensitive modifiers and key", "CTRL+Alt+O", "ctrl+alt+o"},
		{"an uppercase letter is the letter, not shift", "alt+K", "alt+k"},
		{"a digit", "alt+1", "alt+1"},
		{"a punctuation mark", "alt+,", "alt+,"},
		{"a backtick", "alt+`", "alt+`"},
		{"a minus", "alt+-", "alt+-"},
		{"an equals sign", "alt+=", "alt+="},
		{"a semicolon", "alt+;", "alt+;"},
		{"an apostrophe", "alt+'", "alt+'"},
		{"a full stop", "alt+.", "alt+."},
		{"a slash", "alt+/", "alt+/"},
		{"ctrl and a punctuation mark with a C0 byte", "ctrl+]", "ctrl+]"},
		{"ctrl and a slash, a C0 byte", "ctrl+/", "ctrl+/"},
		{"ctrl and a digit with a C0 byte", "ctrl+5", "ctrl+5"},
		{"a function key", "ctrl+f1", "ctrl+f1"},
		{"the last function key", "alt+F12", "alt+f12"},
		{"shift on a function key", "ctrl+shift+f5", "ctrl+shift+f5"},
		{"alt, shift and N, which no terminal sends as SS2", "alt+shift+n", "alt+shift+n"},
		{"lowercase o, p and x, which begin nothing", "alt+o", "alt+o"},
		{"ctrl, alt and backslash, which is not zmx's", "ctrl+alt+\\", "ctrl+alt+\\"},
		{"ctrl, alt and g, which ends no reply", "ctrl+alt+g", "ctrl+alt+g"},
		{"surrounding whitespace is trimmed", " alt+x\t", "alt+x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"kill":` + mustJSON(t, tc.key) + `}`
			assertClean(t, raw, "detach alt+d; detach_to_shell alt+shift+d; kill "+tc.want)
		})
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %v: %v", v, err)
	}
	return string(data)
}

// assertKillFallsBack binds key to kill and expects it to fall back to alt+q
// for reason.
func assertKillFallsBack(t *testing.T, key, reason string) {
	t.Helper()
	assertWarnings(t, `{"kill":`+mustJSON(t, key)+`}`, defaultsSummary,
		"kill falls back to alt+q: "+strconv.Quote(key)+" "+reason)
}

func TestParse_UnparsableKeysFallBack(t *testing.T) {
	cases := []struct {
		key, reason string
	}{
		{"", "has no key"},
		{"alt+", "has no key"},
		{"x", "needs ctrl or alt"},
		{"shift+x", "needs ctrl or alt"},
		{"shift+f5", "needs ctrl or alt"},
		{"+", "needs ctrl or alt"},
		{"cmd+x", `has an unknown modifier "cmd"`},
		{"alt++x", `has an unknown modifier ""`},
		{"alt+alt+x", "names alt twice"},
		{"alt+space", `has an unknown key "space"`},
		{"alt+tab", `has an unknown key "tab"`},
		{"alt+f0", `has an unknown key "f0"`},
		{"alt+f13", `has an unknown key "f13"`},
		{"alt+f01", `has an unknown key "f01"`},
		{"alt+é", `has an unknown key "é"`},
		{"alt+ x", `has an unknown key " x"`},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) { assertKillFallsBack(t, tc.key, tc.reason) })
	}
}

// Every refusal follows from what the key sends without the newer encoding,
// but the layout ones.
func TestParse_RefusedKeysFallBack(t *testing.T) {
	const (
		layout    = "puts shift on a digit or punctuation mark, whose bytes depend on the keyboard layout"
		usShifted = "is typed with shift on a US layout, so its bytes depend on the keyboard layout"
	)
	for _, mark := range `!@#$%^&*()_+{}|:"<>?~` {
		t.Run("alt+"+string(mark)+" needs shift on a US layout", func(t *testing.T) {
			assertKillFallsBack(t, "alt+"+string(mark), usShifted)
		})
	}
	cases := []struct {
		name, key, reason string
	}{
		{"ctrl+shift+letter is ctrl+letter", "ctrl+shift+d", `sends the same bytes as "ctrl+d"`},
		{"ctrl+alt+shift+letter is ctrl+alt+letter", "ctrl+alt+shift+d", `sends the same bytes as "ctrl+alt+d"`},
		{"ctrl+\\ is zmx's", "ctrl+\\", "is zmx's own detach key"},
		{"ctrl+4 sends zmx's byte", "ctrl+4", `sends the same bytes as "ctrl+\", zmx's own detach key`},
		{"shift with a digit", "alt+shift+1", layout},
		{"shift with a punctuation mark", "alt+shift+/", layout},
		{"shift with a digit beside ctrl", "ctrl+shift+5", layout},
		{"ctrl+i is tab", "ctrl+i", "sends the same bytes as tab"},
		{"ctrl+m is enter", "ctrl+m", "sends the same bytes as enter"},
		{"ctrl+j is a linefeed", "ctrl+j", "sends the same bytes as linefeed"},
		{"ctrl+h is backspace", "ctrl+h", "sends the same bytes as backspace"},
		{"ctrl+8 is DEL, backspace", "ctrl+8", "sends the same bytes as backspace"},
		{"ctrl+? needs shift on a US layout", "ctrl+?", usShifted},
		{"ctrl+@ needs shift on a US layout", "ctrl+@", usShifted},
		{"ctrl+[ is ESC", "ctrl+[", "sends the same bytes as escape"},
		{"ctrl+3 is ESC", "ctrl+3", "sends the same bytes as escape"},
		{"alt+ctrl+i is alt+tab", "ctrl+alt+i", "sends the same bytes as alt+tab"},
		{"alt+ctrl+m is alt+enter", "ctrl+alt+m", "sends the same bytes as alt+enter"},
		{"alt+ctrl+h is alt+backspace", "ctrl+alt+h", "sends the same bytes as alt+backspace"},
		{"alt+ctrl+[ is alt+escape", "ctrl+alt+[", "sends the same bytes as alt+escape"},
		{"ctrl+2 is NUL, ctrl+space", "ctrl+2", "sends the same bytes as ctrl+space"},
		{"alt+ctrl+2 is alt+ctrl+space", "ctrl+alt+2", "sends the same bytes as alt+ctrl+space"},
		{"ctrl+g is BEL, which ends a reply", "ctrl+g", "sends BEL, which ends a terminal's reply"},
		{"alt+\\ is ST, which ends a reply", "alt+\\", "sends ST, which ends a terminal's reply"},
		{"a refused key's whitespace is trimmed", " ctrl+i ", "sends the same bytes as tab"},
		{"ctrl on a digit with no C0 byte is lost", "ctrl+1", `sends the same bytes as "1"`},
		{"ctrl on a punctuation mark with no C0 byte is lost", "ctrl+,", `sends the same bytes as ","`},
		{"ctrl lost beside alt", "ctrl+alt+,", `sends the same bytes as "alt+,"`},
		{"alt+[ begins CSI", "alt+[", "is the start of a CSI sequence"},
		{"alt+] begins OSC", "alt+]", "is the start of an OSC string"},
		{"alt+shift+o begins SS3", "alt+shift+o", "is the start of an SS3 sequence"},
		{"alt+shift+p begins DCS", "alt+shift+p", "is the start of a DCS string"},
		{"alt+shift+x begins SOS", "alt+shift+x", "is the start of an SOS string"},
		{"f3 with a modifier is a cursor position report", "ctrl+f3", "sends the same bytes as a cursor position report"},
		{"f3 with alt as well", "alt+f3", "sends the same bytes as a cursor position report"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertKillFallsBack(t, tc.key, tc.reason) })
	}
}

func TestParse_MalformedValuesFallBack(t *testing.T) {
	const reason = "is not a key or a list of keys"
	for _, value := range []string{`42`, `true`, `null`, `{"key":"alt+x"}`, `["alt+x",1]`, `[["alt+x"]]`} {
		t.Run(value, func(t *testing.T) {
			assertWarnings(t, `{"detach": `+value+`}`, defaultsSummary, "detach falls back to alt+d: "+value+" "+reason)
		})
	}

	t.Run("a keys value that is not an object keeps every default with one warning", func(t *testing.T) {
		for _, raw := range []string{`42`, `"alt+d"`, `["alt+d"]`} {
			assertWarnings(t, raw, defaultsSummary, "keys: "+raw+" is not an object, so every action keeps its default")
		}
	})

	t.Run("a list with one bad key falls back whole", func(t *testing.T) {
		assertWarnings(t, `{"detach":["alt+x","ctrl+i"]}`, defaultsSummary,
			`detach falls back to alt+d: "ctrl+i" sends the same bytes as tab`)
	})

	t.Run("a warning carries its parts", func(t *testing.T) {
		_, warnings := parse(t, `{"detach_to_shell":"ctrl+i"}`)
		want := []attachkeys.Warning{{Action: attachkeys.DetachToShell, Value: `"ctrl+i"`, Reason: "sends the same bytes as tab"}}
		if !slices.Equal(warnings, want) {
			t.Errorf("warnings = %+v; want %+v", warnings, want)
		}
	})
}

func TestParse_KeysBoundToTwoActionsFallBack(t *testing.T) {
	t.Run("a key taken from another action's default falls back", func(t *testing.T) {
		assertWarnings(t, `{"detach":"alt+q"}`, defaultsSummary,
			`detach falls back to alt+d: "alt+q" is also bound to kill`)
	})

	t.Run("a key two actions configure falls back in both", func(t *testing.T) {
		assertWarnings(t, `{"detach":"alt+x","kill":["ctrl+o","ALT+X"]}`, defaultsSummary,
			`detach falls back to alt+d: "alt+x" is also bound to kill`,
			`kill falls back to alt+q: "alt+x" is also bound to detach`)
	})

	t.Run("two keys sending the same bytes clash", func(t *testing.T) {
		assertWarnings(t, `{"detach":"ctrl+5","kill":"ctrl+]"}`, defaultsSummary,
			`detach falls back to alt+d: "ctrl+5" sends the same bytes as "ctrl+]", bound to kill`,
			`kill falls back to alt+q: "ctrl+]" sends the same bytes as "ctrl+5", bound to detach`)
	})

	t.Run("an explicit default keeps its key against a configured clash", func(t *testing.T) {
		assertWarnings(t, `{"detach":"alt+d","kill":"alt+d"}`, defaultsSummary,
			`kill falls back to alt+q: "alt+d" is also bound to detach`)
	})

	t.Run("a fallback that clashes in turn falls back the next", func(t *testing.T) {
		assertWarnings(t, `{"detach":"alt+shift+d","detach_to_shell":"alt+q"}`, defaultsSummary,
			`detach_to_shell falls back to alt+shift+d: "alt+q" is also bound to kill`,
			`detach falls back to alt+d: "alt+shift+d" is also bound to detach_to_shell`)
	})

	t.Run("a refused key's default can push out the action that took it", func(t *testing.T) {
		assertWarnings(t, `{"detach":"alt+q","kill":"ctrl+i"}`, defaultsSummary,
			`kill falls back to alt+q: "ctrl+i" sends the same bytes as tab`,
			`detach falls back to alt+d: "alt+q" is also bound to kill`)
	})
}

func TestDefaults(t *testing.T) {
	if got := attachkeys.Defaults().String(); got != defaultsSummary {
		t.Errorf("Defaults() = %q; want %q", got, defaultsSummary)
	}
}
