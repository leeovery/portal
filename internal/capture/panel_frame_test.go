package capture_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const panelBorder = "│"

const panelCursorBar = "▌"

const panelFixtureNamePrefix = "theme-panel-"

type panelLine struct {
	visible string
	raw     string
}

func (l panelLine) fields() (fields []string, cursor bool) {
	fields = strings.Fields(l.visible)
	if len(fields) > 0 && fields[0] == panelCursorBar {
		return fields[1:], true
	}
	return fields, false
}

func panelLines(t *testing.T, frame string) []panelLine {
	t.Helper()

	var lines []panelLine
	for line := range strings.SplitSeq(frame, "\n") {
		_, panel, onPanel := strings.Cut(line, panelBorder)
		if !onPanel {
			continue
		}
		lines = append(lines, panelLine{visible: strings.TrimRight(ansi.Strip(panel), " "), raw: panel})
	}
	if len(lines) == 0 {
		t.Fatalf("no frame line carries the panel's left border %q, so the panel did not render:\n%s", panelBorder, ansi.Strip(frame))
	}
	return lines
}

func panelText(lines []panelLine) string {
	visible := make([]string, 0, len(lines))
	for _, line := range lines {
		visible = append(visible, line.visible)
	}
	return strings.Join(visible, "\n")
}

func panelLineIndex(t *testing.T, lines []panelLine, want string) int {
	t.Helper()

	found := -1
	for i, line := range lines {
		if !strings.Contains(line.visible, want) {
			continue
		}
		if found >= 0 {
			t.Fatalf("%q renders on panel lines %d AND %d; the assertions below locate blocks by position, which two matches make meaningless", want, found, i)
		}
		found = i
	}
	if found < 0 {
		t.Fatalf("no panel line carries %q:\n%s", want, panelText(lines))
	}
	return found
}

func uniquePanelLine(t *testing.T, frame, subject string, matches func(line panelLine) bool) (index int, raw string) {
	t.Helper()

	found := -1
	for i, line := range panelLines(t, frame) {
		if !matches(line) {
			continue
		}
		if found >= 0 {
			t.Fatalf("%s renders on panel lines %d AND %d; every row must render on exactly one delegate line:\n%s", subject, found, i, ansi.Strip(frame))
		}
		found, raw = i, line.raw
	}
	if found < 0 {
		t.Fatalf("no panel line carries %s:\n%s", subject, ansi.Strip(frame))
	}
	return found, raw
}

func panelLineWith(t *testing.T, frame, want string) (index int, raw string) {
	t.Helper()
	return uniquePanelLine(t, frame, want, func(line panelLine) bool {
		return strings.Contains(line.visible, want)
	})
}

func panelRowLine(t *testing.T, frame, label string) (index int, raw string) {
	t.Helper()
	return uniquePanelLine(t, frame, "the row "+label, func(line panelLine) bool {
		fields, _ := line.fields()
		return len(fields) > 0 && fields[0] == label
	})
}

func panelSlugRow(t *testing.T, frame, slug string) panelRow {
	t.Helper()

	var exact, truncated []panelRow
	for _, line := range panelLines(t, frame) {
		fields, cursor := line.fields()
		if len(fields) == 0 {
			continue
		}
		row := panelRow{cursor: cursor, label: fields[0], badge: strings.Join(fields[1:], " ")}
		switch {
		case fields[0] == slug:
			exact = append(exact, row)
		case labelTruncates(fields[0], slug):
			truncated = append(truncated, row)
		}
	}

	found := exact
	if len(found) == 0 {
		found = truncated
	}
	if len(found) != 1 {
		t.Fatalf("the row %s renders on %d panel lines, want exactly 1 — every row must render on exactly one delegate line:\n%s", slug, len(found), ansi.Strip(frame))
	}
	return found[0]
}

func labelTruncates(label, slug string) bool {
	cut, ok := strings.CutSuffix(label, panelEllipsis)
	return ok && cut != "" && strings.HasPrefix(slug, cut)
}

const panelEllipsis = "…"

type panelRow struct {
	cursor bool
	label  string
	badge  string
}

func panelRows(t *testing.T, frame string) map[string]panelRow {
	t.Helper()

	rows := make(map[string]panelRow)
	for _, line := range panelLines(t, frame) {
		fields, cursor := line.fields()
		if len(fields) == 0 {
			continue
		}
		rows[fields[0]] = panelRow{cursor: cursor, label: fields[0], badge: strings.Join(fields[1:], " ")}
	}
	if len(rows) == 0 {
		t.Fatalf("no panel rows were parsed out of the frame; the slide-over did not render:\n%s", ansi.Strip(frame))
	}
	return rows
}
