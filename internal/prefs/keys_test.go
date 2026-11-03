package prefs_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/themetest"
)

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
		got, err := prefs.NewStore(writePrefsFile(t, `{"theme":"nord"}`)).LoadKeys()
		if err != nil || got != nil {
			t.Errorf("LoadKeys = (%s, %v); want (nil, nil)", got, err)
		}
	})

	t.Run("it returns nothing for an absent file", func(t *testing.T) {
		got, err := prefs.NewStore(t.TempDir() + "/prefs.json").LoadKeys()
		if err != nil || got != nil {
			t.Errorf("LoadKeys = (%s, %v); want (nil, nil)", got, err)
		}
	})

	t.Run("it still reads the keys beside a wrong-typed field", func(t *testing.T) {
		got, err := prefs.NewStore(writePrefsFile(t, `{"theme":42,"keys":{"kill":"alt+x"}}`)).LoadKeys()
		if err != nil || string(got) != `{"kill":"alt+x"}` {
			t.Errorf("LoadKeys = (%s, %v); want the keys object and no error", got, err)
		}
	})

	t.Run("it reports a prefs.json that does not decode, saying how", func(t *testing.T) {
		for _, tc := range []struct {
			content string
			want    error
		}{
			{`{"keys":{"detach":"alt+x"}`, prefs.ErrInvalidJSON},
			{`not json`, prefs.ErrInvalidJSON},
			{`[]`, prefs.ErrNotObject},
			{`5`, prefs.ErrNotObject},
			{`"x"`, prefs.ErrNotObject},
		} {
			got, err := prefs.NewStore(writePrefsFile(t, tc.content)).LoadKeys()
			if !errors.Is(err, tc.want) || got != nil {
				t.Errorf("%s: LoadKeys = (%s, %v); want (nil, an error wrapping %q)", tc.content, got, err, tc.want)
			}
		}
	})

	t.Run("it returns a read error, which is neither", func(t *testing.T) {
		path := writePrefsFile(t, `{"keys":{}}`)
		if err := themetest.DenyRead(t, path); err == nil {
			t.Skip("prefs.json stayed readable at mode 0000 (running as root?)")
		}
		_, err := prefs.NewStore(path).LoadKeys()
		if err == nil || errors.Is(err, prefs.ErrInvalidJSON) || errors.Is(err, prefs.ErrNotObject) {
			t.Errorf("LoadKeys error = %v; want the read error", err)
		}
	})
}

// keysValue returns the keys value in a prefs.json, compacted: what the file
// says, whatever its whitespace.
func keysValue(t *testing.T, data []byte) string {
	t.Helper()
	var f map[string]json.RawMessage
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("prefs file is not valid JSON (%q): %v", data, err)
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, f["keys"]); err != nil {
		t.Fatalf("compact keys %q: %v", f["keys"], err)
	}
	return buf.String()
}

func TestKeysSurviveOtherWrites(t *testing.T) {
	t.Run("an invalid value and an unknown entry keep their meaning through a theme write", func(t *testing.T) {
		// Written by hand: compacted, entries unsorted, a number with a
		// trailing zero, and the characters an HTML-escaping encoder rewrites.
		const content = `{"theme":"dracula","keys":{"kill":"alt+<&>","unknown":{"nested":[1, 2.50]},"detach":42}}`
		path := writePrefsFile(t, content)
		if err := prefs.NewStore(path).SaveTheme("nord"); err != nil {
			t.Fatalf("SaveTheme error = %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read prefs file: %v", err)
		}
		if got, want := keysValue(t, got), keysValue(t, []byte(content)); got != want {
			t.Errorf("keys after a theme write = %s; want %s", got, want)
		}
		assertPrefsValue(t, decodePrefsFile(t, path), "theme", "nord")
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
