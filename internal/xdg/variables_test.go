package xdg_test

import (
	"go/ast"
	"go/token"
	"slices"
	"strconv"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/xdg"
)

func TestVariables(t *testing.T) {
	t.Run("it names the config base", func(t *testing.T) {
		if !slices.Contains(xdg.Variables(), "XDG_CONFIG_HOME") {
			t.Errorf("Variables() = %q, want XDG_CONFIG_HOME among them", xdg.Variables())
		}
	})

	// A file or directory declared without joining Variables would resolve, in
	// a process handed Variables' values, from wherever that process's own
	// environment points.
	t.Run("it names every config file's and directory's override this package declares", func(t *testing.T) {
		declared := declaredOverrides(t)
		if len(declared) == 0 {
			t.Fatal("found no ConfigFileID or ConfigDirID declarations — this guard has stopped looking at them")
		}
		for _, name := range declared {
			if !slices.Contains(xdg.Variables(), name) {
				t.Errorf("Variables() = %q, missing the declared override %s", xdg.Variables(), name)
			}
		}
	})
}

// declaredOverrides is the EnvVar of every ConfigFileID and ConfigDirID
// literal in the package's own sources.
func declaredOverrides(t *testing.T) []string {
	t.Helper()
	var names []string
	for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
		ast.Inspect(source.File, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if typ, ok := lit.Type.(*ast.Ident); !ok || (typ.Name != "ConfigFileID" && typ.Name != "ConfigDirID") {
				return true
			}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); !ok || key.Name != "EnvVar" {
					continue
				}
				if value, ok := kv.Value.(*ast.BasicLit); ok && value.Kind == token.STRING {
					name, err := strconv.Unquote(value.Value)
					if err != nil {
						t.Fatalf("%s: unquote %s: %v", source.Position(value.Pos()), value.Value, err)
					}
					names = append(names, name)
				}
			}
			return true
		})
	}
	return names
}
