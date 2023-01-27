package crypto

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBLS12381Compatv112(t *testing.T) {
	scheme := NewPedersenBLSChained()
	privHex := "643d6c704505385387a20d98aba19664e3ee81c600d21a0da910cc87f5dc4ab3"
	privBuff, err := hex.DecodeString(privHex)
	require.NoError(t, err)
	msgHex := "7061737320746865207369676e6174757265"
	msg, err := hex.DecodeString(msgHex)
	require.NoError(t, err)
	sigExp, err := hex.DecodeString("9940ca447bab3bab393c3a07866349343630437167eae" +
		"ab063ef1e47acedc51e85c513121cf319a8832c3d136d7f36490fa7241194b403a3bbbba9e7d5" +
		"e73c9a86f67a9585c6fe077cd6576b2f76560efbab3550d9d5124242c728e3a7ef6989")

	require.NoError(t, err)

	priv := scheme.KeyGroup.Scalar()
	require.NoError(t, priv.UnmarshalBinary(privBuff))
	pub := scheme.KeyGroup.Point().Mul(priv, nil)
	sig, err := scheme.AuthScheme.Sign(priv, msg)
	require.NoError(t, err)
	require.NoError(t, scheme.AuthScheme.Verify(pub, msg, sig))
	require.Equal(t, sig, sigExp)
}
