package tui

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/theme"
)

// slugs records every commit whatever its shape; the rest separate the two
// commit keys, and slotCommits pairs a slot's slug with its half.
type fakeThemePersister struct {
	slugs       []string
	constants   []string
	slots       []theme.Member
	slotCommits []slotCommit
	err         error
}

type slotCommit struct {
	slug   string
	member theme.Member
}

func (f *fakeThemePersister) CommitTheme(slug string) error {
	f.slugs = append(f.slugs, slug)
	f.constants = append(f.constants, slug)
	return f.err
}

func (f *fakeThemePersister) CommitThemeSlot(slug string, member theme.Member) error {
	f.slugs = append(f.slugs, slug)
	f.slots = append(f.slots, member)
	f.slotCommits = append(f.slotCommits, slotCommit{slug: slug, member: member})
	return f.err
}

func TestBuild_NilThemePersisterIsTolerated(t *testing.T) {
	t.Run("a nil dep leaves the seam unwired", func(t *testing.T) {
		m := Build(Deps{Lister: fakeLister{}})

		if m.themeState.persister != nil {
			t.Errorf("themePersister = %#v, want nil — a nil Deps.ThemePersister must apply no option", m.themeState.persister)
		}
	})

	t.Run("the model built that way drives without panicking", func(t *testing.T) {
		var model tea.Model = Build(Deps{Lister: fakeLister{}})

		model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		model, _ = model.Update(SessionsMsg{Sessions: []Session{{Name: "dev"}}})
		model, _ = model.Update(ProjectsLoadedMsg{})
		for _, key := range []tea.KeyPressMsg{
			{Code: 's', Text: "s"},
			{Code: 'x', Text: "x"},
			{Code: '?', Text: "?"},
		} {
			model, _ = model.Update(key)
		}

		if content := model.(Model).View().Content; content == "" {
			t.Error("the nil-persister model painted nothing; the no-panic assertion would be vacuous")
		}
	})

	t.Run("an injected persister is the one the model holds", func(t *testing.T) {
		persister := &fakeThemePersister{}

		m := Build(Deps{Lister: fakeLister{}, ThemePersister: persister})

		if m.themeState.persister != persister {
			t.Fatalf("themePersister = %#v, want the injected recorder", m.themeState.persister)
		}
		if err := m.themeState.persister.CommitThemeSlot("nord", theme.MemberDark); err != nil {
			t.Fatalf("CommitThemeSlot: %v", err)
		}
		if len(persister.slugs) != 1 || persister.slugs[0] != "nord" {
			t.Errorf("recorded slugs = %v, want [nord]", persister.slugs)
		}
		if len(persister.slots) != 1 || persister.slots[0] != theme.MemberDark {
			t.Errorf("recorded slots = %v, want [MemberDark]", persister.slots)
		}
	})
}

// Boxed through `any` because a direct `var _ ThemePersister =
// (*prefs.Store)(nil)` cannot express a negative — it would fail to compile.
func TestPrefsStore_DoesNotSatisfyThemePersister(t *testing.T) {
	var store any = (*prefs.Store)(nil)

	if _, satisfied := store.(ThemePersister); satisfied {
		t.Error("*prefs.Store satisfies ThemePersister; the seam's method names must differ from the store's savers so the logging site cannot be bypassed")
	}
	if _, satisfied := store.(ModePersister); !satisfied {
		t.Error("*prefs.Store no longer satisfies ModePersister; the negative assertion above is vacuous without this control")
	}
}

func TestCommitFailed_SingleEmissionSite(t *testing.T) {
	for name, file := range parsePackageFilesByName(t) {
		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && isThemeComponentBinding(call) {
				t.Errorf(`%s binds the "theme" log component; the model holds the persister seam and emits nothing — cmd owns the component's records here`, name)
			}
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if strings.Contains(value, commitFailedEvent) {
				t.Errorf("%s carries the %q message; it is single-sited on cmd's theme persister, which also returns the error for the panel to report", name, commitFailedEvent)
			}
			return true
		})
	}
}

// Deliberately a test-side constant: production in this package must not carry
// this string at all, so importing one would defeat the guard above.
const commitFailedEvent = "commit failed"

func isThemeComponentBinding(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "For" || len(call.Args) != 1 {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "log" {
		return false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	return ok && lit.Kind == token.STRING && lit.Value == `"theme"`
}
