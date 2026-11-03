package prefs_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/themetest"
)

// keysPrefs is a prefs.json as Portal writes it, theme aside, whose keys
// object holds a value the runner can't parse, an unknown entry, entries out
// of sorted order, a number written with a trailing zero, and the characters
// an HTML-escaping encoder rewrites.
const keysPrefs = `{
  "theme": "%s",
  "keys": {
    "kill": "alt+<&>",
    "unknown": {
      "nested": [
        1,
        2.50
      ]
    },
    "detach": 42
  }
}
`

func TestLoadKeys(t *testing.T) {
	t.Run("it returns the keys object verbatim", func(t *testing.T) {
		store := prefs.NewStore(writePrefsFile(t, `{"keys":{"detach":42, "zz":[1]}}`))
		got, err := store.LoadKeys()
		if err != nil {
			t.Fatalf("LoadKeys error = %v; want nil", err)
		}
		if want := `{"detach":42, "zz":[1]}`; string(got) != want {
			t.Errorf("LoadKeys = %s; want %s", got, want)
		}
	})

	t.Run("it returns nothing when prefs.json sets no keys", func(t *testing.T) {
		for name, content := range map[string]string{
			"no keys":         `{"theme":"nord"}`,
			"corrupt file":    `{"keys":{"detach":"alt+x"}`,
			"top-level array": `[]`,
		} {
			got, err := prefs.NewStore(writePrefsFile(t, content)).LoadKeys()
			if err != nil || got != nil {
				t.Errorf("%s: LoadKeys = (%s, %v); want (nil, nil)", name, got, err)
			}
		}
	})

	t.Run("it returns nothing for an absent file", func(t *testing.T) {
		got, err := prefs.NewStore(t.TempDir() + "/prefs.json").LoadKeys()
		if err != nil || got != nil {
			t.Errorf("LoadKeys = (%s, %v); want (nil, nil)", got, err)
		}
	})

	t.Run("it returns a read error", func(t *testing.T) {
		path := writePrefsFile(t, `{"keys":{}}`)
		if err := themetest.DenyRead(t, path); err == nil {
			t.Skip("prefs.json stayed readable at mode 0000 (running as root?)")
		}
		if _, err := prefs.NewStore(path).LoadKeys(); err == nil {
			t.Error("LoadKeys error = nil; want the read error")
		}
	})
}

func TestKeysSurviveOtherWrites(t *testing.T) {
	t.Run("an invalid value and an unknown entry survive a theme write byte for byte", func(t *testing.T) {
		path := writePrefsFile(t, fmt.Sprintf(keysPrefs, "dracula"))
		if err := prefs.NewStore(path).SaveTheme("nord"); err != nil {
			t.Fatalf("SaveTheme error = %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read prefs file: %v", err)
		}
		if want := fmt.Sprintf(keysPrefs, "nord"); string(got) != want {
			t.Errorf("prefs.json after a theme write =\n%s\nwant\n%s", got, want)
		}
	})

	t.Run("no keys object is written when there was none", func(t *testing.T) {
		path := writePrefsFile(t, `{"theme":"dracula"}`)
		if err := prefs.NewStore(path).SaveTheme("nord"); err != nil {
			t.Fatalf("SaveTheme error = %v", err)
		}
		if _, ok := decodePrefsFile(t, path)["keys"]; ok {
			t.Error("a theme write added a keys object")
		}
	})
}
