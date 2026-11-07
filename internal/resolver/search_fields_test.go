package resolver_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/resolver"
)

const searchHome = "/home/user"

func TestSearchFields(t *testing.T) {
	t.Run("it returns the name alone when no directory is recorded", func(t *testing.T) {
		got := resolver.SearchFields(searchHome, resolver.SearchSubject{Name: "api-work"})

		if want := []string{"api-work"}; !reflect.DeepEqual(got, want) {
			t.Errorf("SearchFields = %q, want %q", got, want)
		}
	})

	t.Run("it returns the name and the home-abbreviated directory, name first", func(t *testing.T) {
		dir := searchHome + "/code/portal"

		got := resolver.SearchFields(searchHome, resolver.SearchSubject{Name: "api-work", Dir: dir})

		if want := []string{"api-work", "~/code/portal"}; !reflect.DeepEqual(got, want) {
			t.Errorf("SearchFields = %q, want %q", got, want)
		}
	})

	t.Run("it puts a label first, before the name and the directory", func(t *testing.T) {
		got := resolver.SearchFields(searchHome, resolver.SearchSubject{Label: "api work", Name: "api-x7Kd9a", Dir: searchHome + "/code/api"})

		if want := []string{"api work", "api-x7Kd9a", "~/code/api"}; !reflect.DeepEqual(got, want) {
			t.Errorf("SearchFields = %q, want %q", got, want)
		}
	})

	t.Run("it does not repeat a label that is the name", func(t *testing.T) {
		got := resolver.SearchFields(searchHome, resolver.SearchSubject{Label: "api-x7Kd9a", Name: "api-x7Kd9a"})

		if want := []string{"api-x7Kd9a"}; !reflect.DeepEqual(got, want) {
			t.Errorf("SearchFields = %q, want %q", got, want)
		}
	})

	t.Run("a term spanning the name and the directory is not a containment match", func(t *testing.T) {
		fields := resolver.SearchFields(searchHome, resolver.SearchSubject{Name: "api", Dir: "/opt/tools"})
		spanning := "i /o"

		if !strings.Contains(strings.Join(fields, " "), spanning) {
			t.Fatalf("precondition: %q must span the joined fields %q", spanning, fields)
		}
		if resolver.MatchesSearchTerm(searchHome, spanning, resolver.SearchSubject{Name: "api", Dir: "/opt/tools"}) {
			t.Errorf("MatchesSearchTerm(%q, %q, …) = true, want false: the fields are tested separately, never joined", searchHome, spanning)
		}
	})
}

func TestSearchReadsNoEnvironment(t *testing.T) {
	t.Setenv("HOME", "/elsewhere")

	if got, want := resolver.SearchFields(searchHome, resolver.SearchSubject{Name: "api", Dir: "/home/user/code/api"}), []string{"api", "~/code/api"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SearchFields(%q, api, /home/user/code/api) = %v, want %v", searchHome, got, want)
	}
	if !resolver.MatchesSearchTerm(searchHome, "~/code", resolver.SearchSubject{Name: "api", Dir: "/home/user/code/api"}) {
		t.Errorf("MatchesSearchTerm(%q, ~/code, …) = false, want the directory abbreviated against the home given to match", searchHome)
	}
}
