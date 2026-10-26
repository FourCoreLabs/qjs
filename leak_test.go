package qjs_test

import (
	"strings"
	"testing"

	"github.com/FourCoreLabs/qjs"
	"github.com/stretchr/testify/require"
)

// newCappedRuntime returns a runtime whose linear memory is capped at pages
// (64 KiB each). The cap lives in the process-wide runtime config, so it is
// restored (and the config rebuilt) when the test ends.
func newCappedRuntime(t *testing.T, pages uint32) *qjs.Runtime {
	t.Helper()

	prev := qjs.MaxMemoryPages
	qjs.MaxMemoryPages = pages
	rt, err := qjs.New(qjs.Option{NoHostFS: true, DisableBuildCache: true})
	require.NoError(t, err)

	t.Cleanup(func() {
		rt.Close()
		qjs.MaxMemoryPages = prev
		restored, err := qjs.New(qjs.Option{NoHostFS: true, DisableBuildCache: true})
		require.NoError(t, err)
		restored.Close()
	})

	return rt
}

// TestCStringHandlesAreReleased repeats calls that copy a ~1 MiB Go string
// into WASM memory far more times than the 32 MiB cap could hold if any copy
// were leaked.
func TestCStringHandlesAreReleased(t *testing.T) {
	const iterations = 100

	payload := strings.Repeat("x", 1<<20)
	jsonText := `{"data":"` + payload + `"}`

	t.Run("ParseJSON", func(t *testing.T) {
		ctx := newCappedRuntime(t, 512).Context()
		for i := range iterations {
			require.NotPanics(t, func() {
				obj := ctx.ParseJSON(jsonText)
				defer obj.Free()
				require.True(t, obj.IsObject(), "iteration %d", i)
			}, "iteration %d", i)
		}
	})

	t.Run("NewString", func(t *testing.T) {
		ctx := newCappedRuntime(t, 512).Context()
		for i := range iterations {
			require.NotPanics(t, func() {
				s := ctx.NewString(payload)
				defer s.Free()
				require.True(t, s.IsString(), "iteration %d", i)
			}, "iteration %d", i)
		}
	})

	t.Run("EvalCode", func(t *testing.T) {
		ctx := newCappedRuntime(t, 512).Context()
		code := `"` + payload + `".length`
		for i := range iterations {
			require.NotPanics(t, func() {
				val, err := ctx.Eval("big.js", qjs.Code(code))
				require.NoError(t, err, "iteration %d", i)
				defer val.Free()
				require.Equal(t, int32(1<<20), val.Int32(), "iteration %d", i)
			}, "iteration %d", i)
		}
	})
}
