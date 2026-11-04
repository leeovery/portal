package bootstrap

import (
	"context"
	"testing"
)

func TestProgressEmitterRidesTheContext(t *testing.T) {
	t.Run("a context carrying no emitter yields none", func(t *testing.T) {
		if progressEmitterFromContext(context.Background()) != nil {
			t.Error("an emitter came off a bare context")
		}
	})

	t.Run("the emitter set is the one read back", func(t *testing.T) {
		var got []StepEvent
		ctx := WithProgressEmitter(context.Background(), func(ev StepEvent) { got = append(got, ev) })

		progressEmitterFromContext(ctx)(StepEvent{Index: 3})

		if len(got) != 1 || got[0].Index != 3 {
			t.Errorf("events = %+v, want the one sent", got)
		}
	})
}
