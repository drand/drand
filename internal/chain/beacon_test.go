package chain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreviousRequiredOnContext(t *testing.T) {
	ctx := context.Background()

	// A plain context should not have the flag set.
	require.False(t, PreviousRequiredFromContext(ctx))

	// Once set, the flag must be retrievable.
	ctx = SetPreviousRequiredOnContext(ctx)
	require.True(t, PreviousRequiredFromContext(ctx))

	// Children of the context inherit the flag.
	child := context.WithValue(ctx, struct{}{}, "noise")
	require.True(t, PreviousRequiredFromContext(child))
}

func TestPreviousRequiredFromContextEmpty(t *testing.T) {
	// TODO context is also a fresh context with no flag.
	require.False(t, PreviousRequiredFromContext(context.TODO()))
}
