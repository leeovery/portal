// White-box: the read step both decodes start from is unexported.
package prefs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/leeovery/portal/internal/themetest"
)

func TestReadBytes_SharedReadPolicy(t *testing.T) {
	t.Run("an absent file reports absent with no error", func(t *testing.T) {
		store := NewStore(filepath.Join(t.TempDir(), "nested", "prefs.json"))

		data, present, err := store.readBytes()
		if err != nil {
			t.Fatalf("readBytes error for an absent file = %v, want nil — absence is the normal first-run state", err)
		}
		if present {
			t.Error("readBytes reported an absent file as present")
		}
		if data != nil {
			t.Errorf("readBytes returned %q for an absent file, want no bytes", data)
		}
	})

	t.Run("a present file hands back its exact bytes", func(t *testing.T) {
		const content = `{"session_list_mode":"by-tag","theme":"nord"}`
		path := seedPrefsFile(t, content)

		data, present, err := NewStore(path).readBytes()
		if err != nil {
			t.Fatalf("unexpected readBytes error: %v", err)
		}
		if !present {
			t.Error("readBytes reported a present file as absent")
		}
		if got := string(data); got != content {
			t.Errorf("readBytes returned %q, want the file's bytes verbatim %q", got, content)
		}
	})

	t.Run("an unreadable file returns the OS error verbatim", func(t *testing.T) {
		path := seedPrefsFile(t, `{"theme":"nord"}`)
		_ = themetest.DenyRead(t, path)

		data, present, err := NewStore(path).readBytes()
		if !errors.Is(err, os.ErrPermission) {
			t.Fatalf("readBytes error = %v, want the OS permission error returned verbatim", err)
		}
		if present {
			t.Error("readBytes reported an unreadable file as present")
		}
		if data != nil {
			t.Errorf("readBytes returned %q alongside a read error, want no bytes", data)
		}
	})
}

// The raw keys field leaves prefsFile uncomparable with ==.
func isZeroRecord(f prefsFile) bool {
	return reflect.ValueOf(f).IsZero()
}

func TestDecodePaths_ShareTheReadAndSplitOnTheDecode(t *testing.T) {
	t.Run("an absent file is absent to both", func(t *testing.T) {
		store := NewStore(filepath.Join(t.TempDir(), "nested", "prefs.json"))

		tolerant, tolerantOK, tolerantErr := store.readFile()
		strict, strictOK, strictErr := store.readFileStrict()

		if tolerantErr != nil || strictErr != nil {
			t.Fatalf("absent-file errors: tolerant = %v, strict = %v, want nil from both", tolerantErr, strictErr)
		}
		if tolerantOK || strictOK {
			t.Errorf("absent-file presence: tolerant = %v, strict = %v, want false from both", tolerantOK, strictOK)
		}
		if !isZeroRecord(tolerant) || !isZeroRecord(strict) {
			t.Errorf("absent-file records: tolerant = %+v, strict = %+v, want the zero record from both", tolerant, strict)
		}
	})

	t.Run("an unreadable file fails both the same way", func(t *testing.T) {
		path := seedPrefsFile(t, `{"theme":"nord"}`)
		_ = themetest.DenyRead(t, path)
		store := NewStore(path)

		tolerant, tolerantOK, tolerantErr := store.readFile()
		strict, strictOK, strictErr := store.readFileStrict()

		if !errors.Is(tolerantErr, os.ErrPermission) || !errors.Is(strictErr, os.ErrPermission) {
			t.Fatalf("unreadable-file errors: tolerant = %v, strict = %v, want the OS permission error from both", tolerantErr, strictErr)
		}
		if tolerantOK || strictOK {
			t.Errorf("unreadable-file presence: tolerant = %v, strict = %v, want false from both", tolerantOK, strictOK)
		}
		if !isZeroRecord(tolerant) || !isZeroRecord(strict) {
			t.Errorf("unreadable-file records: tolerant = %+v, strict = %+v, want the zero record from both", tolerant, strict)
		}
	})

	t.Run("a corrupt file separates them", func(t *testing.T) {
		store := NewStore(seedPrefsFile(t, `{"session_list_mode":"flat",}`))

		tolerant, tolerantOK, tolerantErr := store.readFile()
		if tolerantErr != nil {
			t.Errorf("the tolerant decode returned %v for a corrupt file, want it absorbed", tolerantErr)
		}
		if tolerantOK || !isZeroRecord(tolerant) {
			t.Errorf("the tolerant decode returned (%+v, %v), want the zero record reported as not cleanly decoded", tolerant, tolerantOK)
		}

		_, strictOK, strictErr := store.readFileStrict()
		if _, ok := errors.AsType[*json.SyntaxError](strictErr); !ok {
			t.Errorf("the strict decode returned %v (%T) for a corrupt file, want the decoder's *json.SyntaxError — a writer must abort rather than merge", strictErr, strictErr)
		}
		if strictOK {
			t.Error("the strict decode reported a corrupt file as decoded")
		}
	})
}
