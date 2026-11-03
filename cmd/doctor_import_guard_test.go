package cmd

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// doctorForbiddenImports are the hook packages doctor must not reach until
// hooks.json is keyed by session name: judged against session names, the
// staleness rule would reap every real pane-token key.
var doctorForbiddenImports = []string{
	"github.com/leeovery/portal/internal/hooks",
	"github.com/leeovery/portal/internal/hooksweep",
}

// doctorSources are the production sources named doctor*.go.
func doctorSources(sources []sourceguardtest.ParsedSource) []sourceguardtest.ParsedSource {
	return slices.DeleteFunc(slices.Clone(sources), func(s sourceguardtest.ParsedSource) bool {
		return !strings.HasPrefix(filepath.Base(s.Path), "doctor")
	})
}

// forbiddenDoctorImports reports each "<file>: <import>" a doctor source holds
// from doctorForbiddenImports.
func forbiddenDoctorImports(sources []sourceguardtest.ParsedSource) []string {
	var found []string
	for _, source := range sources {
		for _, spec := range source.File.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err == nil && slices.Contains(doctorForbiddenImports, path) {
				found = append(found, filepath.Base(source.Path)+": "+path)
			}
		}
	}
	return found
}

func TestDoctorImportsNoHookPackage(t *testing.T) {
	sources := doctorSources(sourceguardtest.ParsePackageSources(t, ".", false))
	if len(sources) == 0 {
		t.Fatal("no doctor*.go production source in the cmd package; the guard is scanning nothing")
	}

	t.Run("it finds no hook package imported by doctor", func(t *testing.T) {
		for _, found := range forbiddenDoctorImports(sources) {
			t.Errorf("%s; doctor must not reach the hook store or its sweep", found)
		}
	})

	t.Run("it fails the guard when a doctor source imports one", func(t *testing.T) {
		synthetic := parseSyntheticSource(t, `package cmd

import _ "github.com/leeovery/portal/internal/hooksweep"
`)
		if got, want := forbiddenDoctorImports(synthetic), []string{"synthetic.go: github.com/leeovery/portal/internal/hooksweep"}; !slices.Equal(got, want) {
			t.Errorf("forbiddenDoctorImports = %v, want %v; the rule would not catch the import", got, want)
		}
	})
}
