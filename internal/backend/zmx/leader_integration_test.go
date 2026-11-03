//go:build integration

package zmx_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/zmxtest"
)

// daReply is a terminal's answer to a primary device attributes query: a
// reply the terminal sends on its own, not a key the user typed.
const daReply = "\x1b[?62;22c"

// awaitLeader waits for a client to lead the session, and returns its
// tracked environment.
func awaitLeader(t *testing.T, b *zmx.Backend, session string) backend.ClientEnv {
	t.Helper()
	var env backend.ClientEnv
	if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
		var err error
		env, err = b.LeaderEnv(t.Context(), session)
		return err == nil && len(env) > 0
	}) {
		t.Fatalf("no client came to lead %s", session)
	}
	return env
}

// awaitLeading waits for the client Track marked with PORTAL_ATTACH=token to
// lead the session.
func awaitLeading(t *testing.T, b *zmx.Backend, session, token string) {
	t.Helper()
	var env backend.ClientEnv
	if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
		env, _ = b.LeaderEnv(t.Context(), session)
		got, _ := env.Lookup("PORTAL_ATTACH")
		return got == token
	}) {
		t.Fatalf("the client marked %s never led %s; the leader tracks %+v", token, session, env)
	}
}

// marked attaches a client to the session, marked by Track with
// PORTAL_ATTACH=token.
func marked(t *testing.T, b *zmx.Backend, session, token string) *zmxtest.Client {
	t.Helper()
	attach, err := b.AttachCommand(t.Context(), session)
	if err != nil {
		t.Fatalf("AttachCommand(%q) = %v", session, err)
	}
	return zmxtest.Run(t, b.Track(attach, "PORTAL_ATTACH", token))
}

func TestLeaderAgainstZmx(t *testing.T) {
	t.Run("EnvGet names the client whose terminal typed last", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "work", t.TempDir(), "sleep", "1000")
		first := marked(t, b, "work", "first1")
		awaitLeading(t, b, "work", "first1")
		second := marked(t, b, "work", "second")

		second.Type(t, "x")
		awaitLeading(t, b, "work", "second")
		first.Type(t, "y")
		awaitLeading(t, b, "work", "first1")
	})

	t.Run("a terminal's reply to a query from a client that is not typing does not make it the leader", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "work", t.TempDir(), "sleep", "1000")
		marked(t, b, "work", "typing")
		awaitLeading(t, b, "work", "typing")
		passive := marked(t, b, "work", "passiv")

		passive.Type(t, daReply)
		// ctrl+\ in a read of its own detaches the passive client. zmx takes
		// its client's messages in order, so once it has gone its reply was
		// read first; had the reply made it the leader, its going would leave
		// no leader at all.
		passive.AwaitRead(t)
		passive.Type(t, "\x1c")
		if !passive.Exited(settle) {
			t.Fatal("the passive client never detached")
		}
		env := awaitLeader(t, b, "work")
		if got, _ := env.Lookup("PORTAL_ATTACH"); got != "typing" {
			t.Errorf("the leader tracks %+v, want the typing client still leading", env)
		}
	})

	t.Run("a client whose environment names nothing to track is tracked for zmx's default list, as Portal copies it", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "work", t.TempDir(), "sleep", "1000")
		attach, err := b.AttachCommand(t.Context(), "work")
		if err != nil {
			t.Fatalf("AttachCommand = %v", err)
		}
		// Stripped, so a list the developer exports never reaches the client,
		// however the test's environment is made.
		attach.Env = slices.DeleteFunc(slices.Clone(attach.Env), func(kv string) bool { return strings.HasPrefix(kv, "ZMX_TRACK_ENV=") })
		zmxtest.Run(t, attach)

		var names []string
		for _, v := range awaitLeader(t, b, "work") {
			names = append(names, v.Name)
		}
		if want := strings.Split(zmx.DefaultTracked, ","); !slices.Equal(names, want) {
			t.Errorf("zmx tracks %q for the client, want Portal's copy of its default list, %q", names, want)
		}
	})
}
