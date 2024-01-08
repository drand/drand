package context

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/chain"
)

// PrevSignatureMattersOnContext checks if the previous signature matters or not for future operations
// based on the used schema.
// If it matters, then it's also set on the passed context.
func PrevSignatureMattersOnContext(t *testing.T, ctx context.Context) (context.Context, *crypto.Scheme, bool) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	prevMatters := sch.Name == crypto.DefaultSchemeID

	if prevMatters {
		ctx = chain.SetPreviousRequiredOnContext(ctx)
	}

	return ctx, sch, prevMatters
}
