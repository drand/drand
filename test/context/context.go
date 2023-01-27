package context

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/crypto"
)

// PrevSignatureMatersOnContext checks if the previous signature matters or not for future operations
// based on the used schema.
// If it matters, then it's also set on the passed context.
func PrevSignatureMatersOnContext(t *testing.T, ctx context.Context) (context.Context, *crypto.Scheme, bool) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	prevMatters := true
	if sch.Name == crypto.UnchainedSchemeID ||
		sch.Name == crypto.ShortSigSchemeID {
		prevMatters = false
	}

	if prevMatters {
		ctx = chain.SetPreviousRequiredOnContext(ctx)
	}

	return ctx, sch, prevMatters
}
