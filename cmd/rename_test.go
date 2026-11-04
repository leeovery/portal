package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/tui"
)

// The live session the rename suite runs inside.
var (
	renameCreated = time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	renameDir     = "/work/dotfiles"
	renamePID     = 4242
)

// stageRename stages a backend running this process inside the session
// currentSessionName, created at renameCreated in renameDir, in a state
// directory of the test's own, and returns the directory.
func stageRename(t *testing.T) (*backendtest.Fake, string) {
	t.Helper()
	fake := inSession(t, "", backend.Session{Name: currentSessionName, Dir: renameDir, Created: renameCreated, PID: renamePID})
	fake.SetCurrent(currentSessionName)
	return fake, os.Getenv("PORTAL_STATE_DIR")
}

// labelLive gives the live session's own entry a label, as an earlier rename
// would have.
func labelLive(t *testing.T, dir, label string) {
	t.Helper()
	if _, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
		idx.Claim(currentSessionName, renameDir, renameCreated, renamePID, claimRule(dir))
		return idx.SetLabel(currentSessionName, label)
	}); err != nil {
		t.Fatalf("label %s as %q: %v", currentSessionName, label, err)
	}
}

// entryOf is name's index entry, the zero Entry for none.
func entryOf(t *testing.T, dir, name string) sessionindex.Entry {
	t.Helper()
	idx := readIndex(t, dir)
	e, _ := idx.Lookup(name)
	return e
}

// assertLabel holds name's index entry to want, "" for no entry.
func assertLabel(t *testing.T, dir, name, want string) {
	t.Helper()
	if e := entryOf(t, dir, name); e.Label != want {
		t.Errorf("label of %s = %q, want %q: %+v", name, e.Label, want, e)
	}
}

func TestRenameCommand(t *testing.T) {
	t.Run("it sets the current session's label from its arguments joined with spaces", func(t *testing.T) {
		_, dir := stageRename(t)

		if _, _, err := runRootCmd(t, "rename", "dotfiles", "·", "user", "search"); err != nil {
			t.Fatalf("rename: %v", err)
		}

		assertLabel(t, dir, currentSessionName, "dotfiles · user search")
	})

	t.Run("it records the live session's created time, and no dir, on a session with no entry", func(t *testing.T) {
		fake, dir := stageRename(t)

		if _, _, err := runRootCmd(t, "rename", "api work"); err != nil {
			t.Fatalf("rename: %v", err)
		}

		want := sessionindex.Entry{Name: currentSessionName, Label: "api work", CreatedAt: renameCreated, PID: renamePID}
		if e := entryOf(t, dir, currentSessionName); e != want {
			t.Errorf("entry = %+v, want %+v", e, want)
		}
		if calls := fake.CallsTo(backendtest.OpList); len(calls) != 0 {
			t.Errorf("backend List calls = %d, want none: rename reads its one session", len(calls))
		}
		if calls := fake.CallsTo(backendtest.OpSession); len(calls) != 1 || calls[0].Name != currentSessionName {
			t.Errorf("backend Session calls = %+v, want one for %s", calls, currentSessionName)
		}
	})

	// Grouping prefers the recorded dir and matches a project by exact key, so
	// a session sitting below its project's root would move out of its group
	// if rename froze that cwd as its dir.
	t.Run("it leaves the picker grouping a session in a subdirectory as it did before", func(t *testing.T) {
		subdir := renameDir + "/cmd"
		fake := inSession(t, "", backend.Session{Name: currentSessionName, Dir: subdir, Created: renameCreated, Answered: true})
		fake.SetCurrent(currentSessionName)
		before := shownRows(t, fake)

		if _, _, err := runRootCmd(t, "rename", "api work"); err != nil {
			t.Fatalf("rename: %v", err)
		}

		after := shownRows(t, fake)
		if len(before) != 1 || len(after) != 1 {
			t.Fatalf("rows before = %+v, after = %+v; want one each", before, after)
		}
		before[0].Label = "api work"
		if after[0] != before[0] {
			t.Errorf("row after rename = %+v, want %+v: only the label changes, so the picker groups it by its cwd %s as before", after[0], before[0], subdir)
		}
	})

	t.Run("it keeps the entry of the same creation, scrollback and all", func(t *testing.T) {
		stageRename(t)
		dir := seedSessionsJSON(t, `{"version": 2, "saved_at": "2026-10-31T00:00:00Z", "sessions": [`+
			`{"name": "`+currentSessionName+`", "label": "old", "dir": "`+renameDir+`", "created_at": "2026-10-30T09:00:00Z", "scrollback_file": "scrollback/`+currentSessionName+`.bin"}]}`)

		if _, _, err := runRootCmd(t, "rename", "new"); err != nil {
			t.Fatalf("rename: %v", err)
		}

		want := sessionindex.Entry{Name: currentSessionName, Label: "new", Dir: renameDir, CreatedAt: renameCreated, PID: renamePID, ScrollbackFile: "scrollback/" + currentSessionName + ".bin"}
		if e := entryOf(t, dir, currentSessionName); e != want {
			t.Errorf("entry = %+v, want %+v", e, want)
		}
	})

	t.Run("it drops an earlier session's label and scrollback after the name was recreated", func(t *testing.T) {
		stageRename(t)
		dir := seedSessionsJSON(t, `{"version": 2, "saved_at": "2026-10-29T00:00:00Z", "sessions": [`+
			`{"name": "`+currentSessionName+`", "label": "old", "dir": "/work/old", "created_at": "2026-10-28T09:00:00Z", "scrollback_file": "scrollback/`+currentSessionName+`.bin"}]}`)

		if _, _, err := runRootCmd(t, "rename", "new"); err != nil {
			t.Fatalf("rename: %v", err)
		}

		want := sessionindex.Entry{Name: currentSessionName, Label: "new", CreatedAt: renameCreated, PID: renamePID}
		if e := entryOf(t, dir, currentSessionName); e != want {
			t.Errorf("entry = %+v, want %+v", e, want)
		}
	})

	t.Run("it refuses in one line when the current session does not answer, writing nothing", func(t *testing.T) {
		fake, dir := stageRename(t)
		fake.StopAnswering(currentSessionName)

		_, _, err := runRootCmd(t, "rename", "api work")

		want := "session " + currentSessionName + " is not answering; its label is unchanged"
		if err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
		if entries := dirListing(t, dir); len(entries) != 0 {
			t.Errorf("state directory = %v, want nothing written", entries)
		}
	})

	t.Run("it refuses a session that answers with no creation, writing nothing, so no later session inherits the label", func(t *testing.T) {
		fake := inSession(t, "", backend.Session{Name: currentSessionName, Dir: renameDir, PID: renamePID})
		fake.SetCurrent(currentSessionName)
		dir := os.Getenv("PORTAL_STATE_DIR")

		_, _, err := runRootCmd(t, "rename", "api work")

		want := "session " + currentSessionName + " did not report when it was created; its label is unchanged"
		if err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
		if entries := dirListing(t, dir); len(entries) != 0 {
			t.Errorf("state directory = %v, want nothing written", entries)
		}
	})

	t.Run("it says the current session refuses connections in kill's words, writing nothing", func(t *testing.T) {
		fake, dir := stageRename(t)
		fake.Fail(backendtest.OpSession, fmt.Errorf("session %q: %w", currentSessionName, zmx.ErrRefusing))

		_, _, err := runRootCmd(t, "rename", "api work")

		want := "session " + currentSessionName + " refuses connections: " + stillHeld
		if err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
		if entries := dirListing(t, dir); len(entries) != 0 {
			t.Errorf("state directory = %v, want nothing written", entries)
		}
	})

	t.Run("it says the current session is gone in kill's words, writing nothing", func(t *testing.T) {
		fake, dir := stageRename(t)
		fake.Remove(currentSessionName)

		_, _, err := runRootCmd(t, "rename", "api work")

		want := "No session found: " + currentSessionName
		if err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
		if entries := dirListing(t, dir); len(entries) != 0 {
			t.Errorf("state directory = %v, want nothing written", entries)
		}
	})

	t.Run("it takes one quoted argument as the whole label", func(t *testing.T) {
		_, dir := stageRename(t)

		if _, _, err := runRootCmd(t, "rename", "api  work"); err != nil {
			t.Fatalf("rename: %v", err)
		}

		assertLabel(t, dir, currentSessionName, "api  work")
	})

	t.Run("it takes a label starting with - after --, as its help shows", func(t *testing.T) {
		_, dir := stageRename(t)

		if _, _, err := runRootCmd(t, "rename", "--", "-wip"); err != nil {
			t.Fatalf("rename -- -wip: %v", err)
		}
		assertLabel(t, dir, currentSessionName, "-wip")

		stdout, _, err := runRootCmd(t, "rename", "--help")
		if err != nil {
			t.Fatalf("rename --help: %v", err)
		}
		if !strings.Contains(stdout.String(), "portal rename -- -wip") {
			t.Errorf("help = %q, want it to show portal rename -- -wip", stdout)
		}
	})

	t.Run("it leaves other sessions' labels alone", func(t *testing.T) {
		_, dir := stageRename(t)
		storedLabel(t, dir, "web-Qm3pLz", "web · checkout")

		if _, _, err := runRootCmd(t, "rename", "dotfiles"); err != nil {
			t.Fatalf("rename: %v", err)
		}

		assertLabel(t, dir, "web-Qm3pLz", "web · checkout")
		assertLabel(t, dir, currentSessionName, "dotfiles")
	})

	t.Run("it resets the label to the name given no text", func(t *testing.T) {
		for _, args := range [][]string{{"rename"}, {"rename", ""}, {"rename", "  "}, {"rename", currentSessionName}} {
			_, dir := stageRename(t)
			labelLive(t, dir, "dotfiles · user search")

			if _, _, err := runRootCmd(t, args...); err != nil {
				t.Fatalf("%q: %v", args, err)
			}

			assertLabel(t, dir, currentSessionName, "")
			stdout, _, err := runRootCmd(t, "current")
			if err != nil || stdout.String() != currentSessionName+"\n" {
				t.Errorf("after %q, current = %q, %v; want the name", args, stdout, err)
			}
		}
	})

	t.Run("it refuses an unprintable label and changes nothing", func(t *testing.T) {
		_, dir := stageRename(t)
		labelLive(t, dir, "dotfiles")

		_, _, err := runRootCmd(t, "rename", "api\nwork")

		want := "label refused: it holds a control or formatting character, such as a newline, tab or escape"
		if err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
		assertLabel(t, dir, currentSessionName, "dotfiles")
	})

	t.Run("it fails in one line outside a session and writes nothing", func(t *testing.T) {
		fake, dir := stageRename(t)
		fake.SetCurrent("")

		_, _, err := runRootCmd(t, "rename", "api work")

		if !errors.Is(err, errNotInSession) {
			t.Errorf("error = %v, want %v", err, errNotInSession)
		}
		if entries := dirListing(t, dir); len(entries) != 0 {
			t.Errorf("state directory = %v, want nothing written", entries)
		}
	})

	t.Run("it stops on a version 1 index and leaves it as it was", func(t *testing.T) {
		stageRename(t)
		dir := seedSessionsJSON(t, legacyIndexBody)

		_, _, err := runRootCmd(t, "rename", "api work")

		if !errors.Is(err, sessionindex.ErrLegacyIndex) {
			t.Errorf("error = %v, want the version 1 stop", err)
		}
		if got := string(readFileBytes(t, sessionindex.Path(dir))); got != legacyIndexBody {
			t.Errorf("sessions.json = %q, want it untouched", got)
		}
	})

	t.Run("it is exempt from the zmx binary lookup", func(t *testing.T) {
		if !bootstrapExempt["rename"] {
			t.Error(`bootstrapExempt["rename"] = false; want true`)
		}
	})
}

func TestPickerLabeller(t *testing.T) {
	label := func(fake *backendtest.Fake, text string) error {
		return pickerLabeller{ctx: context.Background(), b: fake}.SetLabel(currentSessionName, text)
	}

	t.Run("it claims the entry for the live creation, then labels it, via the tui", func(t *testing.T) {
		sink := logtest.Install(t)
		fake, _ := stageRename(t)
		dir := seedSessionsJSON(t, `{"version": 2, "saved_at": "2026-10-29T00:00:00Z", "sessions": [`+
			`{"name": "`+currentSessionName+`", "label": "old", "dir": "/work/old", "created_at": "2026-10-28T09:00:00Z"}]}`)

		if err := label(fake, "api work"); err != nil {
			t.Fatalf("SetLabel: %v", err)
		}

		want := sessionindex.Entry{Name: currentSessionName, Label: "api work", CreatedAt: renameCreated, PID: renamePID}
		if e := entryOf(t, dir, currentSessionName); e != want {
			t.Errorf("entry = %+v, want %+v", e, want)
		}
		for _, op := range []string{"replace", "set-label"} {
			r := sink.Records().Matching("sessions", op).Only(t, op+" record")
			if got := r.AttrString(t, "via"); got != "tui" {
				t.Errorf("%s via = %q, want tui", op, got)
			}
		}
	})

	t.Run("it resets the label to the name given empty text or the name", func(t *testing.T) {
		for _, text := range []string{"", "  ", currentSessionName} {
			fake, dir := stageRename(t)
			labelLive(t, dir, "api work")

			if err := label(fake, text); err != nil {
				t.Fatalf("SetLabel(%q): %v", text, err)
			}

			assertLabel(t, dir, currentSessionName, "")
		}
	})

	t.Run("it refuses a session that does not answer, writing nothing", func(t *testing.T) {
		fake, dir := stageRename(t)
		fake.StopAnswering(currentSessionName)

		err := label(fake, "api work")

		want := "session " + currentSessionName + " is not answering; its label is unchanged"
		if err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
		if entries := dirListing(t, dir); len(entries) != 0 {
			t.Errorf("state directory = %v, want nothing written", entries)
		}
	})

	t.Run("it words a session gone since the listing as kill does, writing nothing", func(t *testing.T) {
		fake, dir := stageRename(t)
		fake.Remove(currentSessionName)

		err := label(fake, "api work")

		want := "No session found: " + currentSessionName
		if err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
		if entries := dirListing(t, dir); len(entries) != 0 {
			t.Errorf("state directory = %v, want nothing written", entries)
		}
	})

	t.Run("it names the spelling a session is held under, as kill does", func(t *testing.T) {
		fake, _ := stageRename(t)
		other := strings.ToUpper(currentSessionName)

		err := pickerLabeller{ctx: context.Background(), b: fake}.SetLabel(other, "api work")

		want := "No session found: " + other + "; did you mean " + currentSessionName + "?"
		if err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
	})

	t.Run("it refuses an unprintable label and changes nothing", func(t *testing.T) {
		fake, dir := stageRename(t)
		labelLive(t, dir, "dotfiles")

		err := label(fake, "api\u200bwork")

		want := "label refused: it holds a control or formatting character, such as a newline, tab or escape"
		if err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
		assertLabel(t, dir, currentSessionName, "dotfiles")
	})
}

// shownRows is the picker's rows over b.
func shownRows(t *testing.T, b backend.Backend) []tui.Session {
	t.Helper()
	rows, err := listShownSessions(t.Context(), b)
	if err != nil {
		t.Fatalf("list shown sessions: %v", err)
	}
	return rows
}
