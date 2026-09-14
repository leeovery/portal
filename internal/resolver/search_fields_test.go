package resolver_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/resolver"
)

func TestSearchFields(t *testing.T) {
	t.Run("it returns the name alone when no directory is recorded", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())

		got := resolver.SearchFields("api-work", "")

		if want := []string{"api-work"}; !reflect.DeepEqual(got, want) {
			t.Errorf("SearchFields(%q, %q) = %q, want %q", "api-work", "", got, want)
		}
	})

	t.Run("it returns the name and the home-abbreviated directory, name first", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		dir := filepath.Join(home, "Code", "portal")

		got := resolver.SearchFields("api-work", dir)

		if want := []string{"api-work", "~/Code/portal"}; !reflect.DeepEqual(got, want) {
			t.Errorf("SearchFields(%q, %q) = %q, want %q", "api-work", dir, got, want)
		}
	})

	t.Run("a term spanning the name and the directory is not a containment match", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())

		fields := resolver.SearchFields("api", "/opt/tools")
		spanning := "i /o"

		if !strings.Contains(strings.Join(fields, " "), spanning) {
			t.Fatalf("precondition: %q must span the joined fields %q", spanning, fields)
		}
		if resolver.MatchesSearchTerm(spanning, "api", "/opt/tools") {
			t.Errorf("MatchesSearchTerm(%q, …) = true, want false: the fields are tested separately, never joined", spanning)
		}
	})
}
