package theme_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

func TestRowOrder_ReservedNameSortsBySlugLabelsByFilename(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "nord.theme", themetest.Lines())
	assembler := theme.Assembler{Loader: theme.NewSilentLoader()}

	_, union := assembler.Open(dir, theme.RawKeys{})

	row := onlyRejectedRow(t, union, theme.ReasonReservedName)
	if got, want := row.SortKey(), "nord"; got != want {
		t.Errorf("reserved-name row SortKey() = %q, want the slug %q — the slug is what puts it beside the built-in it collides with", got, want)
	}
	if got, want := row.Label(), "nord.theme"; got != want {
		t.Errorf("reserved-name row Label() = %q, want the filename %q — a second row reading `nord` would not say which one is theirs", got, want)
	}
	if row.SortKey() == row.Label() {
		t.Errorf("reserved-name row SortKey() and Label() are both %q, want two separate values", row.SortKey())
	}
}

func TestRowOrder_BuiltinFirstOnTheGuaranteedTie(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "nord.theme", themetest.Lines())
	themetest.Write(t, dir, "nord-lee.theme", themetest.Lines())
	assembler := theme.Assembler{Loader: theme.NewSilentLoader()}

	_, union := assembler.Open(dir, theme.RawKeys{})

	collided := rowsWithSlug(union, "nord")
	if len(collided) != 2 {
		t.Fatalf("slug %q has %d rows, want the built-in and its reserved-name collider: %v", "nord", len(collided), rowLabels(union))
	}
	if collided[0].SortKey() != collided[1].SortKey() {
		t.Fatalf("the colliding rows sort by %q and %q, want one identical key — the tie is what the built-in-first rule exists to settle", collided[0].SortKey(), collided[1].SortKey())
	}
	if collided[0].Source != theme.SourceBuiltin {
		t.Errorf("the first %q row is a %v, want the built-in ahead of the file it collides with", "nord", collided[0].Source)
	}

	at := slices.Index(rowLabels(union), "nord")
	if got, want := slices.Index(rowLabels(union), "nord.theme"), at+1; got != want {
		t.Errorf("the reserved-name row is at index %d, want %d — directly beneath the built-in, with nothing able to fall between", got, want)
	}
	want := slices.Insert(theme.BuiltinSlugs(), builtinIndex(t, "nord")+1, "nord.theme", "nord-lee")
	if got := rowLabels(union); !slices.Equal(got, want) {
		t.Errorf("union labels = %v, want %v — the nearest neighbouring slug sorts after the pair, never between it", got, want)
	}
}

func TestRowOrder_BadNameSortsByFilename(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "Bad_Name.theme", themetest.Lines())
	assembler := theme.Assembler{Loader: theme.NewSilentLoader()}

	_, union := assembler.Open(dir, theme.RawKeys{})

	row := onlyRejectedRow(t, union, theme.ReasonBadName)
	if row.Slug != "" {
		t.Errorf("bad-name row slug = %q, want none — a bad name is rejected rather than normalised, so it yields no identity", row.Slug)
	}
	if got, want := row.SortKey(), "Bad_Name.theme"; got != want {
		t.Errorf("bad-name row SortKey() = %q, want the filename %q — it is the only thing the row has", got, want)
	}
	if got, want := row.Label(), "Bad_Name.theme"; got != want {
		t.Errorf("bad-name row Label() = %q, want the filename %q", got, want)
	}

	want := append([]string{"Bad_Name.theme"}, theme.BuiltinSlugs()...)
	if got := rowSortKeys(union); !slices.Equal(got, want) {
		t.Errorf("union sort keys = %v, want %v — the filename sorts among the slugs, not ahead of the list", got, want)
	}
}

func TestRowOrder_CharsetRejectedSortsByItself(t *testing.T) {
	const illegal = "../evil"
	assembler := theme.Assembler{Loader: theme.NewSilentLoader()}

	_, union := assembler.Open(t.TempDir(), theme.RawKeys{Theme: illegal})

	row := onlyPersistedRow(t, union)
	if row.Slug != "" || row.Filename != "" {
		t.Fatalf("charset-rejected row carries slug %q and filename %q, want neither — it is the row shape with nothing but the raw value", row.Slug, row.Filename)
	}
	if got, want := row.SortKey(), illegal; got != want {
		t.Errorf("charset-rejected row SortKey() = %q, want the persisted string %q", got, want)
	}
	if got, want := row.Label(), illegal; got != want {
		t.Errorf("charset-rejected row Label() = %q, want the persisted string %q", got, want)
	}

	want := append([]string{illegal}, theme.BuiltinSlugs()...)
	if got := rowSortKeys(union); !slices.Equal(got, want) {
		t.Errorf("union sort keys = %v, want %v — the string is placed by the same rule as every other key", got, want)
	}
}

func TestRowOrder_CaseInsensitiveThenByteWise(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		keys  theme.RawKeys
		want  []string
	}{
		{
			name:  "an uppercase filename sorts among the z's, not ahead of everything",
			files: []string{"Zed.theme", "aa-early.theme", "zz-late.theme"},
			want:  append(append([]string{"aa-early"}, theme.BuiltinSlugs()...), "Zed.theme", "zz-late"),
		},
		{
			name:  "keys equal but for case are settled byte-wise",
			files: []string{"zEd.theme"},
			keys:  theme.RawKeys{Theme: "Zed.theme"},
			want:  append(theme.BuiltinSlugs(), "Zed.theme", "zEd.theme"),
		},
	}

	assembler := theme.Assembler{Loader: theme.NewSilentLoader()}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, file := range tt.files {
				themetest.Write(t, dir, file, themetest.Lines())
			}

			enumeration, union := assembler.Open(dir, tt.keys)

			if got := rowSortKeys(union); !slices.Equal(got, tt.want) {
				t.Fatalf("union sort keys = %v, want %v", got, tt.want)
			}
			for again := range 5 {
				if got := rowSortKeys(assembler.Reassemble(enumeration, tt.keys)); !slices.Equal(got, tt.want) {
					t.Fatalf("re-derivation %d = %v, want the identical %v — the order must not vary by run", again, got, tt.want)
				}
			}
		})
	}
}

func TestRowOrder_TotalAndDeterministic(t *testing.T) {
	if got, want := theme.BuiltinSlugs(), []string{"gitlab-dark-grey", "horizon", "nord", "nord-wave", "tokyo-night", "tokyo-night-day"}; !slices.Equal(got, want) {
		t.Fatalf("the shipped built-ins are %v, want %v — the canonical sequence below states the WHOLE ordered union, so a new built-in belongs in it", got, want)
	}

	dir := t.TempDir()
	themetest.Write(t, dir, "nord.theme", themetest.Lines())
	themetest.Write(t, dir, "Bad_Name.theme", themetest.Lines())
	themetest.Write(t, dir, "zz-late.theme", themetest.Lines())
	themetest.WriteWithCanvas(t, dir, "aa-early.theme", "blue")
	keys := theme.RawKeys{Light: "ghost", Dark: "../evil"}
	assembler := theme.Assembler{Loader: theme.NewSilentLoader()}

	enumeration, union := assembler.Open(dir, keys)

	want := []string{"../evil", "aa-early", "Bad_Name.theme", "ghost", "gitlab-dark-grey", "horizon", "nord", "nord.theme", "nord-wave", "tokyo-night", "tokyo-night-day", "zz-late"}
	if got := rowLabels(union); !slices.Equal(got, want) {
		t.Fatalf("union labels = %v, want %v", got, want)
	}

	tests := []struct {
		name  string
		order []int
	}{
		{name: "the order the directory was read in", order: []int{0, 1, 2, 3}},
		{name: "reversed", order: []int{3, 2, 1, 0}},
		{name: "rotated", order: []int{1, 2, 3, 0}},
		{name: "the ends swapped", order: []int{3, 1, 2, 0}},
		{name: "the collision read first", order: []int{2, 0, 3, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shuffled := theme.Enumeration{
				Entries:     permuted(t, enumeration.Entries, tt.order),
				DirUnusable: enumeration.DirUnusable,
				DirPath:     enumeration.DirPath,
			}

			again := assembler.Reassemble(shuffled, keys)

			if got, want := rowFingerprints(again), rowFingerprints(union); !slices.Equal(got, want) {
				t.Errorf("re-derived union = %v, want the identical %v — the order must not carry any of the input's", got, want)
			}
		})
	}
}

func TestRowOrder_SortKeyAndLabelAreSeparateValues(t *testing.T) {
	tests := []struct {
		name     string
		filename string
	}{
		{name: "the filename a reserved-name file really has", filename: "nord.theme"},
		{name: "a filename that would sort to the head", filename: "AAA.theme"},
		{name: "a filename that would sort to the tail", filename: "zzz.theme"},
	}

	assembler := theme.Assembler{Loader: theme.NewSilentLoader()}
	wantAt := builtinIndex(t, "nord") + 1

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enumeration := theme.Enumeration{Entries: []theme.Entry{{
				Path:      "/themes/" + tt.filename,
				Filename:  tt.filename,
				Slug:      "nord",
				Rejection: &theme.Rejection{Reason: theme.ReasonReservedName},
			}}}

			union := assembler.Reassemble(enumeration, theme.RawKeys{})

			if got, want := union.Rows[wantAt].Label(), tt.filename; got != want {
				t.Errorf("the reserved-name row's label = %q, want the filename %q — the label follows the file", got, want)
			}
			if got, want := union.Rows[wantAt].SortKey(), "nord"; got != want {
				t.Errorf("the reserved-name row's sort key = %q, want the slug %q — the key does not follow the file", got, want)
			}
			if got := rowSortKeys(union); !slices.Equal(got, slices.Insert(theme.BuiltinSlugs(), wantAt, "nord")) {
				t.Errorf("union sort keys = %v, want the row still beneath its built-in — renaming the file must not move it", got)
			}
		})
	}
}

func TestRowOrder_UnionIsOrderedOnReturn(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "zz-late.theme", themetest.Lines())
	themetest.Write(t, dir, "aa-early.theme", themetest.Lines())
	assembler := theme.Assembler{Loader: theme.NewSilentLoader()}

	enumeration, opened := assembler.Open(dir, theme.RawKeys{Theme: "zzz-ghost"})

	want := append(append([]string{"aa-early"}, theme.BuiltinSlugs()...), "zz-late", "zzz-ghost")
	if got := rowSortKeys(opened); !slices.Equal(got, want) {
		t.Errorf("the opened union's sort keys = %v, want %v", got, want)
	}

	again := assembler.Reassemble(enumeration, theme.RawKeys{Theme: "../evil"})

	want = append(append([]string{"../evil", "aa-early"}, theme.BuiltinSlugs()...), "zz-late")
	if got := rowSortKeys(again); !slices.Equal(got, want) {
		t.Errorf("the re-derived union's sort keys = %v, want %v — a recompute is ordered too", got, want)
	}
}

func TestRowOrder_NoVariantConcept(t *testing.T) {
	light := theme.Theme{Canvas: theme.Token{Name: "canvas", Value: "#E1E2E7"}}
	dark := theme.Theme{Canvas: theme.Token{Name: "canvas", Value: "#0B0C14"}}

	tests := []struct {
		name        string
		early, late theme.Theme
	}{
		{name: "the earlier slug is dark", early: dark, late: light},
		{name: "the earlier slug is light", early: light, late: dark},
		{name: "neither row carries a palette at all", early: theme.Theme{}, late: theme.Theme{}},
	}

	assembler := theme.Assembler{Loader: theme.NewSilentLoader()}
	want := append(append([]string{"aa-early"}, theme.BuiltinSlugs()...), "zz-late")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enumeration := theme.Enumeration{Entries: []theme.Entry{
				{Path: "/themes/aa-early.theme", Filename: "aa-early.theme", Slug: "aa-early", Theme: tt.early},
				{Path: "/themes/zz-late.theme", Filename: "zz-late.theme", Slug: "zz-late", Theme: tt.late},
			}}

			union := assembler.Reassemble(enumeration, theme.RawKeys{})

			if got := rowSortKeys(union); !slices.Equal(got, want) {
				t.Errorf("union sort keys = %v, want %v — the palette is not an input to the ordering", got, want)
			}
		})
	}
}

func rowSortKeys(union theme.Union) []string {
	keys := make([]string, 0, len(union.Rows))
	for _, row := range union.Rows {
		keys = append(keys, row.SortKey())
	}
	return keys
}

func rowLabels(union theme.Union) []string {
	labels := make([]string, 0, len(union.Rows))
	for _, row := range union.Rows {
		labels = append(labels, row.Label())
	}
	return labels
}

func rowFingerprints(union theme.Union) []string {
	fingerprints := make([]string, 0, len(union.Rows))
	for _, row := range union.Rows {
		fingerprints = append(fingerprints, fmt.Sprintf("%s|%s|%d", row.SortKey(), row.Label(), row.Source))
	}
	return fingerprints
}

func onlyRejectedRow(t *testing.T, union theme.Union, reason theme.Reason) theme.Row {
	t.Helper()

	rows := []theme.Row{}
	for _, row := range union.Rows {
		if row.Rejection != nil && row.Rejection.Reason == reason {
			rows = append(rows, row)
		}
	}
	if len(rows) != 1 {
		t.Fatalf("union carries %d rows rejected %q, want exactly 1: %+v", len(rows), reason, rows)
	}
	return rows[0]
}

func builtinIndex(t *testing.T, slug string) int {
	t.Helper()

	slugs := theme.BuiltinSlugs()
	at := slices.Index(slugs, slug)
	if at < 0 {
		t.Fatalf("%q is not a built-in slug (%v) — the fixture has nothing to collide with", slug, slugs)
	}
	return at
}

func permuted(t *testing.T, entries []theme.Entry, order []int) []theme.Entry {
	t.Helper()

	if len(order) != len(entries) {
		t.Fatalf("the permutation covers %d of %d entries, want every one — a partial shuffle would drop rows rather than move them", len(order), len(entries))
	}
	shuffled := make([]theme.Entry, 0, len(order))
	for _, at := range order {
		shuffled = append(shuffled, entries[at])
	}
	return shuffled
}
