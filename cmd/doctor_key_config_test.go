package cmd

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/themetest"
)

const keyConfigCheckName = "key config"

// defaultKeyConfigLine is the key-config line of an install that sets no keys.
const defaultKeyConfigLine = "  ✓ key config: detach alt+d; detach_to_shell alt+shift+d; kill alt+q"

// doctorKeyConfigReport runs doctor over a healthy install whose prefs.json
// holds content, and returns the rendered key-config line and the summary.
func doctorKeyConfigReport(t *testing.T, content string) (line, summary string) {
	t.Helper()
	setPrefsFile(t, content)
	outBuf, _, err := runDoctorWith(t, healthyDoctorDeps(t))
	if err != nil {
		t.Fatalf("Execute err = %v; want nil — the key config never fails doctor", err)
	}
	lines := reportLines(outBuf.String())
	for _, l := range lines {
		if strings.Contains(l, " "+keyConfigCheckName+": ") {
			return l, lines[len(lines)-1]
		}
	}
	t.Fatalf("no key-config line in report:\n%s", outBuf.String())
	return "", ""
}

func TestDoctorKeyConfig(t *testing.T) {
	t.Run("it passes with the defaults when prefs.json sets no keys", func(t *testing.T) {
		for _, content := range []string{"", `{"theme":"nord"}`, `{"keys":null}`} {
			if line, _ := doctorKeyConfigReport(t, content); line != defaultKeyConfigLine {
				t.Errorf("prefs.json %q: key-config line = %q; want %q", content, line, defaultKeyConfigLine)
			}
		}
	})

	t.Run("it says so when prefs.json does not decode, and how", func(t *testing.T) {
		const notJSON, notObject = "not valid JSON", "not a JSON object"
		for _, tc := range []struct{ content, problem string }{
			{`{"keys":{"detach":"alt+d"`, notJSON},
			{`not json`, notJSON},
			{`[]`, notObject},
			{`5`, notObject},
			{`"x"`, notObject},
		} {
			line, summary := doctorKeyConfigReport(t, tc.content)
			want := "  " + checkMarker(checkInfo) + " key config: prefs.json is " + tc.problem + "; keys keep their defaults"
			if line != want {
				t.Errorf("prefs.json %q: key-config line = %q; want %q", tc.content, line, want)
			}
			if want := "  9 checks passed"; summary != want {
				t.Errorf("prefs.json %q: summary = %q; want %q", tc.content, summary, want)
			}
		}
	})

	t.Run("it names an entry that is no action's", func(t *testing.T) {
		line, _ := doctorKeyConfigReport(t, `{"keys":{"detatch":"alt+x"}}`)
		want := "  " + checkMarker(checkInfo) + ` key config: keys: "detatch" is not an action, so it is ignored`
		if line != want {
			t.Errorf("key-config line = %q; want %q", line, want)
		}
	})

	t.Run("it passes a config whose every binding parses, naming the keys in effect", func(t *testing.T) {
		line, summary := doctorKeyConfigReport(t, `{"keys":{"detach":["ALT+x","ctrl+o"],"kill":[]}}`)
		if want := "  ✓ key config: detach alt+x, ctrl+o; detach_to_shell alt+shift+d; kill off"; line != want {
			t.Errorf("key-config line = %q; want %q", line, want)
		}
		if want := "  10 checks passed"; summary != want {
			t.Errorf("summary = %q; want %q, the passing key config counted", summary, want)
		}
	})

	t.Run("it warns, naming each fallback and why, without failing doctor", func(t *testing.T) {
		line, summary := doctorKeyConfigReport(t, `{"keys":{"detach":"ctrl+i","kill":"alt+d"}}`)
		want := "  " + checkMarker(checkInfo) + " key config: " +
			`detach falls back to alt+d: "ctrl+i" sends the same bytes as tab; ` +
			`kill falls back to alt+q: "alt+d" is also bound to detach`
		if line != want {
			t.Errorf("key-config line = %q; want %q", line, want)
		}
		if want := "  9 checks passed"; summary != want {
			t.Errorf("summary = %q; want %q, a warning counted as neither a pass nor a failure", summary, want)
		}
	})

	t.Run("it warns once when the keys value is not an object", func(t *testing.T) {
		line, _ := doctorKeyConfigReport(t, `{"keys":["alt+d"]}`)
		want := "  " + checkMarker(checkInfo) + ` key config: keys: ["alt+d"] is not an object, so every action keeps its default`
		if line != want {
			t.Errorf("key-config line = %q; want %q", line, want)
		}
	})

	t.Run("it stands down when prefs.json can't be read", func(t *testing.T) {
		path := setPrefsFile(t, `{"keys":{"detach":"alt+x"}}`)
		if err := themetest.DenyRead(t, path); err == nil {
			t.Skip("prefs.json stayed readable at mode 0000 (running as root?)")
		}
		for name, store := range map[string]*prefs.Store{"no store": nil, "unreadable file": prefs.NewStore(path)} {
			want := checkResult{name: keyConfigCheckName, status: checkNotEvaluable, detail: prefsReadStandDownPhrase}
			if got := checkKeyConfig(store); got != want {
				t.Errorf("%s: checkKeyConfig = %+v; want %+v", name, got, want)
			}
		}
	})
}
