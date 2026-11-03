package runnerreg_test

import (
	"errors"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runnerreg"
)

func TestSwitchFrom(t *testing.T) {
	at := time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC)

	t.Run("it asks the runner on the session that last took input", func(t *testing.T) {
		sink := logtest.Install(t)
		r := runnerreg.New(stateDir(t))
		earlier, typed, elsewhere := inputAt(at), inputAt(at.Add(time.Second)), inputAt(at.Add(time.Hour))
		register(t, r, "api-x7Kd9a", earlier)
		register(t, r, "api-x7Kd9a", typed)
		register(t, r, "blog-c3d4", elsewhere)

		if err := r.SwitchFrom(t.Context(), "api-x7Kd9a", "docs-e5f6"); err != nil {
			t.Fatalf("SwitchFrom: %v", err)
		}

		for name, tc := range map[string]struct {
			h    *handler
			want []string
		}{
			"the runner typed in":                              {typed, []string{"docs-e5f6"}},
			"the runner on the session typed in less recently": {earlier, nil},
			"the runner on another session":                    {elsewhere, nil},
		} {
			if got, _ := tc.h.asked(); !slices.Equal(got, tc.want) {
				t.Errorf("%s was asked to switch to %q, want %q", name, got, tc.want)
			}
		}
		rec := sink.Records().Matching("attach", "switch").AtExactLevel(slog.LevelInfo).Only(t, "switch record")
		for key, want := range map[string]string{"session": "api-x7Kd9a", "target": "docs-e5f6", "result": "accepted"} {
			if got := rec.AttrString(t, key); got != want {
				t.Errorf("switch %s = %q, want %q", key, got, want)
			}
		}
		if got := rec.IntAttr(t, "target_pid"); got != int64(os.Getpid()) {
			t.Errorf("switch target_pid = %d, want the runner's %d", got, os.Getpid())
		}
	})

	t.Run("no runner on the session is ErrNoRunner", func(t *testing.T) {
		sink := logtest.Install(t)
		r := runnerreg.New(stateDir(t))
		elsewhere := inputAt(at)
		register(t, r, "blog-c3d4", elsewhere)

		if err := r.SwitchFrom(t.Context(), "api-x7Kd9a", "docs-e5f6"); !errors.Is(err, runnerreg.ErrNoRunner) {
			t.Errorf("SwitchFrom = %v, want ErrNoRunner", err)
		}
		if got, _ := elsewhere.asked(); len(got) != 0 {
			t.Errorf("the runner on another session was asked to switch to %q", got)
		}
		rec := sink.Records().Matching("attach", "switch").AtExactLevel(slog.LevelInfo).Only(t, "switch record")
		if got := rec.AttrString(t, "result"); got != "no runner" {
			t.Errorf("switch result = %q, want no runner", got)
		}
		if rec.HasAttr("target_pid") || rec.HasAttr("error") {
			t.Error("a switch with no runner names a runner or an error")
		}
	})

	t.Run("runners that have forwarded no input are no runner", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		idle := &handler{}
		register(t, r, "api-x7Kd9a", idle)

		if err := r.SwitchFrom(t.Context(), "api-x7Kd9a", "docs-e5f6"); !errors.Is(err, runnerreg.ErrNoRunner) {
			t.Errorf("SwitchFrom = %v, want ErrNoRunner", err)
		}
		if got, _ := idle.asked(); len(got) != 0 {
			t.Errorf("an idle runner was asked to switch to %q", got)
		}
	})

	t.Run("a refusal is returned, and logged as one", func(t *testing.T) {
		sink := logtest.Install(t)
		r := runnerreg.New(stateDir(t))
		register(t, r, "api-x7Kd9a", &handler{lastInput: at, refuse: errors.New("the runner is not attached")})

		if err := r.SwitchFrom(t.Context(), "api-x7Kd9a", "docs-e5f6"); !errors.Is(err, runnerreg.ErrRefused) {
			t.Errorf("SwitchFrom = %v, want ErrRefused", err)
		}
		rec := sink.Records().Matching("attach", "switch").AtExactLevel(slog.LevelInfo).Only(t, "switch record")
		if got := rec.AttrString(t, "result"); got != "refused" {
			t.Errorf("switch result = %q, want refused", got)
		}
		if err := rec.ErrorAttr(t, "error"); err == nil {
			t.Error("the refusal names no error")
		}
	})

	t.Run("a registry it can't read fails, and is logged as failing", func(t *testing.T) {
		sink := logtest.Install(t)
		blocker := stateDir(t) + "/file"
		if err := os.WriteFile(blocker, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		err := runnerreg.New(blocker).SwitchFrom(t.Context(), "api-x7Kd9a", "docs-e5f6")

		if err == nil || errors.Is(err, runnerreg.ErrNoRunner) || errors.Is(err, runnerreg.ErrRefused) {
			t.Errorf("SwitchFrom = %v, want the read's failure", err)
		}
		rec := sink.Records().Matching("attach", "switch").AtExactLevel(slog.LevelInfo).Only(t, "switch record")
		if got := rec.AttrString(t, "result"); got != "failed" {
			t.Errorf("switch result = %q, want failed", got)
		}
	})
}

func TestDetachToPicker(t *testing.T) {
	t.Run("it asks every runner on the session, and no other", func(t *testing.T) {
		sink := logtest.Install(t)
		r := runnerreg.New(stateDir(t))
		first, second, elsewhere, idle := inputAt(time.Now()), inputAt(time.Now()), inputAt(time.Now()), &handler{}
		register(t, r, "api-x7Kd9a", first)
		register(t, r, "api-x7Kd9a", second)
		register(t, r, "api-x7Kd9a", idle)
		register(t, r, "blog-c3d4", elsewhere)

		accepted, err := r.DetachToPicker(t.Context(), "api-x7Kd9a")

		if err != nil || accepted != 3 {
			t.Errorf("DetachToPicker = %d, %v; want all 3 on the session", accepted, err)
		}
		for name, tc := range map[string]struct {
			h    *handler
			want int
		}{"first": {first, 1}, "second": {second, 1}, "idle": {idle, 1}, "elsewhere": {elsewhere, 0}} {
			if _, got := tc.h.asked(); got != tc.want {
				t.Errorf("the %s runner was asked to detach %d times, want %d", name, got, tc.want)
			}
		}
		recs := sink.Records().Matching("attach", "detach to picker").AtExactLevel(slog.LevelInfo)
		if len(recs) != 3 {
			t.Fatalf("detach to picker records = %d, want one per runner asked", len(recs))
		}
		for _, rec := range recs {
			if rec.AttrString(t, "session") != "api-x7Kd9a" || rec.AttrString(t, "result") != "accepted" {
				t.Errorf("record = %+v, want an acceptance on api-x7Kd9a", rec)
			}
		}
	})

	t.Run("it counts only acceptances", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		register(t, r, "api-x7Kd9a", &handler{})
		register(t, r, "api-x7Kd9a", &handler{refuse: errors.New("the runner is not attached")})

		if accepted, err := r.DetachToPicker(t.Context(), "api-x7Kd9a"); err != nil || accepted != 1 {
			t.Errorf("DetachToPicker = %d, %v; want 1", accepted, err)
		}
	})

	t.Run("no runner on the session accepts none, and says so", func(t *testing.T) {
		sink := logtest.Install(t)
		r := runnerreg.New(stateDir(t))

		if accepted, err := r.DetachToPicker(t.Context(), "api-x7Kd9a"); err != nil || accepted != 0 {
			t.Errorf("DetachToPicker = %d, %v; want 0", accepted, err)
		}
		rec := sink.Records().Matching("attach", "detach to picker").AtExactLevel(slog.LevelInfo).Only(t, "detach to picker record")
		if got := rec.AttrString(t, "result"); got != "no runner" {
			t.Errorf("result = %q, want no runner", got)
		}
	})
}
