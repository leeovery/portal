package theme_test

import (
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const themePkg = "github.com/leeovery/portal/internal/theme"

const themesDirEnvVar = "PORTAL_THEMES_DIR"

// The loader resolves nothing and decides nothing about logging: the themes
// directory is resolved by cmd/config.go and arrives here as an injected
// value, which rules out internal/xdg in particular. The one edge the package
// does own is the event logger it emits through.
var themeMayImport = []string{"github.com/leeovery/portal/internal/log"}

func TestThemePackage_ResolvesNoPaths(t *testing.T) {
	t.Run("depends on no path-resolving package", func(t *testing.T) {
		for _, lane := range sourceguardtest.Lanes() {
			sourceguardtest.AssertDepsWithin(t, themePkg, themeMayImport, lane)
		}
	})

	t.Run("reads neither the themes env var nor the home directory", func(t *testing.T) {
		for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
			fset, file, path := source.Fset, source.File, source.Path

			ast.Inspect(file, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.BasicLit:
					if node.Kind != token.STRING {
						return true
					}
					value, uerr := strconv.Unquote(node.Value)
					if uerr != nil {
						return true
					}
					if strings.Contains(value, themesDirEnvVar) {
						t.Errorf("%s:%d carries the %s literal — the env var belongs to cmd/config.go's themesDirPath, and the directory arrives here as an injected value", path, fset.Position(node.Pos()).Line, themesDirEnvVar)
					}
				case *ast.SelectorExpr:
					pkg, ok := node.X.(*ast.Ident)
					if !ok {
						return true
					}
					if pkg.Name == "os" && node.Sel.Name == "UserHomeDir" {
						t.Errorf("%s:%d calls os.UserHomeDir — internal/theme resolves no paths; the themes directory arrives as an injected value", path, fset.Position(node.Pos()).Line)
					}
				}
				return true
			})
		}
	})
}

func TestThemePackage_DeclaresNoHexLiterals(t *testing.T) {
	hexLiteral := regexp.MustCompile(`#[0-9a-fA-F]{6}`)

	for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
		ast.Inspect(source.File, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}

			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if found := hexLiteral.FindString(value); found != "" {
				t.Errorf("%s:%d declares the hex literal %s — every colour value lives in a .theme file, never in Go", source.Path, source.Fset.Position(lit.Pos()).Line, found)
			}
			return true
		})
	}
}

func TestThemePackage_HasNoInitFunction(t *testing.T) {
	for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
		for _, decl := range source.File.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.Name == "init" {
					t.Errorf("%s:%d declares func init() — internal/theme does no work at load time; nothing walks the embedded set at init", source.Path, source.Fset.Position(d.Pos()).Line)
				}
			case *ast.GenDecl:
				if d.Tok == token.VAR {
					requireNoCallingInitialiser(t, source, d)
				}
			}
		}
	}
}

func requireNoCallingInitialiser(t *testing.T, source sourceguardtest.ParsedSource, decl *ast.GenDecl) {
	t.Helper()

	for _, spec := range decl.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		for _, expr := range value.Values {
			ast.Inspect(expr, func(n ast.Node) bool {
				call, isCall := n.(*ast.CallExpr)
				if !isCall {
					return true
				}
				t.Errorf("%s:%d initialises a package-level var by calling a function — internal/theme does no work at load time; nothing walks the embedded set at init", source.Path, source.Fset.Position(call.Pos()).Line)
				return false
			})
		}
	}
}
