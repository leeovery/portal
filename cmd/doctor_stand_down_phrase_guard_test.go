package cmd

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// forEachValueSpec visits every const and var spec the sources declare.
func forEachValueSpec(sources []sourceguardtest.ParsedSource, visit func(*ast.GenDecl, *token.FileSet, *ast.ValueSpec)) {
	for _, source := range sources {
		for _, decl := range source.File.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				if value, ok := spec.(*ast.ValueSpec); ok {
					visit(gen, source.Fset, value)
				}
			}
		}
	}
}

// parseSyntheticSource parses source authored by the test, so a rule's own
// failure mode is exercised against a declaration the package does not hold.
func parseSyntheticSource(t *testing.T, source string) []sourceguardtest.ParsedSource {
	t.Helper()

	path := filepath.Join(t.TempDir(), "synthetic.go")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write synthetic source: %v", err)
	}
	return sourceguardtest.ParseSources(t, []string{path})
}

// phraseConstSuffix is the convention naming a const as one of the stand-down
// phrases, so the guard below reads the declarations rather than a list it
// would have to be told to grow.
const phraseConstSuffix = "StandDownPhrase"

// phraseRespelling is one production string literal that spells a phrase a
// const already declares — the second home the guard exists to find.
type phraseRespelling struct {
	Const   string
	Literal string
}

// standDownPhraseConsts returns every declared stand-down phrase by the name
// that declares it.
func standDownPhraseConsts(t *testing.T, sources []sourceguardtest.ParsedSource) map[string]string {
	t.Helper()

	consts := map[string]string{}
	forEachValueSpec(sources, func(gen *ast.GenDecl, fset *token.FileSet, value *ast.ValueSpec) {
		if gen.Tok != token.CONST {
			return
		}
		for i, name := range value.Names {
			if !strings.HasSuffix(name.Name, phraseConstSuffix) {
				continue
			}
			consts[name.Name] = stringLiteralValue(t, fset, value.Values[i])
		}
	})
	return consts
}

// standDownPhraseRespellings reports the string literals that spell a declared
// phrase somewhere other than its declaration. Containment rather than equality
// is the rule: a surface that words a phrase and then adds to it — the
// not-evaluable suffix, a prefix — has written the phrase a second time as
// surely as one that repeats it whole.
func standDownPhraseRespellings(sources []sourceguardtest.ParsedSource, consts map[string]string) []phraseRespelling {
	declarations := map[ast.Expr]bool{}
	forEachValueSpec(sources, func(gen *ast.GenDecl, _ *token.FileSet, value *ast.ValueSpec) {
		if gen.Tok != token.CONST {
			return
		}
		for i, name := range value.Names {
			if strings.HasSuffix(name.Name, phraseConstSuffix) {
				declarations[value.Values[i]] = true
			}
		}
	})

	var found []phraseRespelling
	for _, source := range sources {
		ast.Inspect(source.File, func(node ast.Node) bool {
			basic, ok := node.(*ast.BasicLit)
			if !ok || basic.Kind != token.STRING || declarations[ast.Expr(basic)] {
				return true
			}
			literal, err := strconv.Unquote(basic.Value)
			if err != nil {
				return true
			}
			for name, phrase := range consts {
				if strings.Contains(literal, phrase) {
					found = append(found, phraseRespelling{Const: name, Literal: literal})
				}
			}
			return true
		})
	}
	slices.SortFunc(found, func(a, b phraseRespelling) int { return strings.Compare(a.Literal, b.Literal) })
	return found
}

func stringLiteralValue(t *testing.T, fset *token.FileSet, expr ast.Expr) string {
	t.Helper()

	basic, ok := expr.(*ast.BasicLit)
	if !ok || basic.Kind != token.STRING {
		t.Fatalf("%s: stand-down phrase const is not a string literal", fset.Position(expr.Pos()))
	}
	value, err := strconv.Unquote(basic.Value)
	if err != nil {
		t.Fatalf("%s: stand-down phrase const %s is not a quoted string", fset.Position(basic.Pos()), basic.Value)
	}
	return value
}

// A phrase is declared once and every surface that prints it composes from the
// declaration. A second spelling agrees with the first only until one of them
// is re-worded, and nothing at runtime can tell the two apart — so the rule is
// over the source: no production literal may spell a phrase a const declares.
func TestStandDownPhrasesAreSpelledOnlyInTheirDeclaredHome(t *testing.T) {
	sources := sourceguardtest.ParsePackageSources(t, ".", false)
	consts := standDownPhraseConsts(t, sources)
	if len(consts) == 0 {
		t.Fatalf("no const named *%s in the cmd package; the guard is scanning nothing", phraseConstSuffix)
	}

	t.Run("it finds no phrase spelled outside its declaration", func(t *testing.T) {
		for _, respelling := range standDownPhraseRespellings(sources, consts) {
			t.Errorf("the literal %q spells the phrase %s already declares; compose it from the const instead",
				respelling.Literal, respelling.Const)
		}
	})

	t.Run("it fails the copy guard when a phrase is spelled outside its declared home", func(t *testing.T) {
		synthetic := parseSyntheticSource(t, `package cmd

const lockedStandDownPhrase = "hooks.json is locked"

var elsewhere = map[string]string{"lock": "hooks.json is locked (not evaluable)"}
`)
		want := []phraseRespelling{{Const: "lockedStandDownPhrase", Literal: "hooks.json is locked (not evaluable)"}}
		got := standDownPhraseRespellings(synthetic, standDownPhraseConsts(t, synthetic))
		if !slices.Equal(got, want) {
			t.Errorf("standDownPhraseRespellings = %v, want %v; the rule would not catch the second spelling", got, want)
		}
	})
}
