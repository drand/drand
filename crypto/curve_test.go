package crypto

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
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

//nolint:lll
func TestBLS12381CompatMockData(t *testing.T) {
	scheme := NewPedersenBLSChained()

	pubHex := "90bcf1bc6f710c23963bf402ffffa55c3dd3a9168c40d05b395d1e794797835eb494249095542e3b7e7405c8fbdb0908"
	previous := "628bdd13fbdf0eb52117514afc36af7310c3b72075780502f5f725deba2304e7"
	round := 1969
	sigExp, err := hex.DecodeString("b68afe4b92819ed4516a5894400cdf83ea4453c422c3b43f985087167bad044e6e11954bc4cf555905fd9968ea47ef2405a55f18afdf654c97ab9ea5c0f50921cb9288d70aa78b210191b313451c78f1c601bb2816a3d46d739a3d3b02858205")
	require.NoError(t, err)

	prev, err := hex.DecodeString(previous)
	fmt.Println(len(prev))
	require.NoError(t, err)

	h := sha256.New()
	_, _ = h.Write(prev)
	_ = binary.Write(h, binary.BigEndian, uint64(round))
	finalmsg := h.Sum(nil)

	pubb, err := hex.DecodeString(pubHex)
	require.NoError(t, err)

	pub := scheme.KeyGroup.Point()
	err = pub.UnmarshalBinary(pubb)
	require.NoError(t, err)

	require.NoError(t, scheme.ThresholdScheme.VerifyRecovered(pub, finalmsg, sigExp))
}
