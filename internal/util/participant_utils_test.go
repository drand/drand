package util

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/crypto"
	drand "github.com/drand/drand/v2/protobuf/dkg"
	proto "github.com/drand/drand/v2/protobuf/drand"
)

func testScheme(t *testing.T) *crypto.Scheme {
	t.Helper()
	sch, err := crypto.GetSchemeByID(crypto.DefaultSchemeID)
	require.NoError(t, err)
	return sch
}

// validParticipant produces a participant backed by a real, valid public key
// for the given scheme so that pkToPoint/ToNode succeed.
func validParticipant(t *testing.T, addr string, sch *crypto.Scheme) *drand.Participant {
	t.Helper()
	kp, err := key.NewKeyPair(addr, sch)
	require.NoError(t, err)
	p, err := PublicKeyAsParticipant(kp.Public)
	require.NoError(t, err)
	return p
}

func TestContains(t *testing.T) {
	a := &drand.Participant{Address: "a"}
	b := &drand.Participant{Address: "b"}
	t.Run("nil haystack", func(t *testing.T) {
		assert.False(t, Contains(nil, a))
	})
	t.Run("present", func(t *testing.T) {
		assert.True(t, Contains([]*drand.Participant{a, b}, b))
	})
	t.Run("absent", func(t *testing.T) {
		assert.False(t, Contains([]*drand.Participant{a}, b))
	})
}

func TestContainsAll(t *testing.T) {
	a := &drand.Participant{Address: "a"}
	b := &drand.Participant{Address: "b"}
	c := &drand.Participant{Address: "c"}
	t.Run("all present", func(t *testing.T) {
		assert.True(t, ContainsAll([]*drand.Participant{a, b, c}, []*drand.Participant{a, c}))
	})
	t.Run("missing one", func(t *testing.T) {
		assert.False(t, ContainsAll([]*drand.Participant{a, b}, []*drand.Participant{a, c}))
	})
	t.Run("empty needles", func(t *testing.T) {
		assert.True(t, ContainsAll([]*drand.Participant{a}, nil))
	})
}

func TestEqualParticipant(t *testing.T) {
	base := &drand.Participant{Address: "a", Key: []byte("k"), Signature: []byte("s")}
	t.Run("equal", func(t *testing.T) {
		other := &drand.Participant{Address: "a", Key: []byte("k"), Signature: []byte("s")}
		assert.True(t, EqualParticipant(base, other))
	})
	t.Run("different address", func(t *testing.T) {
		other := &drand.Participant{Address: "b", Key: []byte("k"), Signature: []byte("s")}
		assert.False(t, EqualParticipant(base, other))
	})
	t.Run("different key", func(t *testing.T) {
		other := &drand.Participant{Address: "a", Key: []byte("x"), Signature: []byte("s")}
		assert.False(t, EqualParticipant(base, other))
	})
	t.Run("different signature", func(t *testing.T) {
		other := &drand.Participant{Address: "a", Key: []byte("k"), Signature: []byte("x")}
		assert.False(t, EqualParticipant(base, other))
	})
	t.Run("nil handled by getters", func(t *testing.T) {
		assert.True(t, EqualParticipant(nil, nil))
		assert.False(t, EqualParticipant(base, nil))
	})
}

func TestNonEmpty(t *testing.T) {
	assert.False(t, NonEmpty(nil))
	assert.False(t, NonEmpty(&drand.Participant{}))
	assert.True(t, NonEmpty(&drand.Participant{Address: "a"}))
}

func TestPublicKeyAsParticipant(t *testing.T) {
	sch := testScheme(t)
	kp, err := key.NewKeyPair("127.0.0.1:8000", sch)
	require.NoError(t, err)

	p, err := PublicKeyAsParticipant(kp.Public)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:8000", p.Address)
	assert.NotEmpty(t, p.Key)

	expectedKey, err := kp.Public.Key.MarshalBinary()
	require.NoError(t, err)
	assert.Equal(t, expectedKey, p.Key)
}

func TestToParticipant(t *testing.T) {
	node := &proto.Node{
		Public: &proto.Identity{
			Address:   "1.2.3.4:80",
			Key:       []byte("key"),
			Signature: []byte("sig"),
		},
	}
	p := ToParticipant(node)
	assert.Equal(t, "1.2.3.4:80", p.Address)
	assert.Equal(t, []byte("key"), p.Key)
	assert.Equal(t, []byte("sig"), p.Signature)
}

func TestToPeer(t *testing.T) {
	p := &drand.Participant{Address: "5.6.7.8:90"}
	peer := ToPeer(p)
	assert.Equal(t, "5.6.7.8:90", peer.Address())
}

func TestToNode(t *testing.T) {
	sch := testScheme(t)
	t.Run("valid key", func(t *testing.T) {
		p := validParticipant(t, "127.0.0.1:1234", sch)
		node, err := ToNode(3, p, sch)
		require.NoError(t, err)
		assert.Equal(t, uint32(3), node.Index)
		assert.NotNil(t, node.Public)
	})
	t.Run("invalid key returns ErrInvalidKeyScheme", func(t *testing.T) {
		p := &drand.Participant{Address: "a", Key: []byte("not-a-point")}
		_, err := ToNode(0, p, sch)
		assert.ErrorIs(t, err, key.ErrInvalidKeyScheme)
	})
}

func TestToKeyNode(t *testing.T) {
	sch := testScheme(t)
	t.Run("valid key", func(t *testing.T) {
		p := validParticipant(t, "127.0.0.1:5678", sch)
		node, err := ToKeyNode(7, p, sch)
		require.NoError(t, err)
		assert.Equal(t, uint32(7), node.Index)
		require.NotNil(t, node.Identity)
		assert.Equal(t, "127.0.0.1:5678", node.Identity.Addr)
		assert.Equal(t, sch, node.Identity.Scheme)
	})
	t.Run("invalid key returns ErrInvalidKeyScheme", func(t *testing.T) {
		p := &drand.Participant{Address: "a", Key: []byte("not-a-point")}
		_, err := ToKeyNode(0, p, sch)
		assert.ErrorIs(t, err, key.ErrInvalidKeyScheme)
	})
}

func TestSortedByPublicKey(t *testing.T) {
	a := &drand.Participant{Address: "a", Key: []byte{0x03}}
	b := &drand.Participant{Address: "b", Key: []byte{0x01}}
	c := &drand.Participant{Address: "c", Key: []byte{0x02}}
	out := SortedByPublicKey([]*drand.Participant{a, b, c})
	assert.Equal(t, []*drand.Participant{b, c, a}, out)
}

func TestTryMapEach(t *testing.T) {
	in := []*drand.Participant{
		{Address: "a"},
		{Address: "b"},
		{Address: "c"},
	}
	t.Run("happy path maps with index", func(t *testing.T) {
		out, err := TryMapEach(in, func(index int, p *drand.Participant) (string, error) {
			return fmt.Sprintf("%d:%s", index, p.Address), nil
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"0:a", "1:b", "2:c"}, out)
	})
	t.Run("propagates error and returns nil", func(t *testing.T) {
		boom := fmt.Errorf("boom")
		out, err := TryMapEach(in, func(index int, _ *drand.Participant) (string, error) {
			if index == 1 {
				return "", boom
			}
			return "ok", nil
		})
		assert.ErrorIs(t, err, boom)
		assert.Nil(t, out)
	})
	t.Run("empty input returns empty slice", func(t *testing.T) {
		out, err := TryMapEach([]*drand.Participant{}, func(int, *drand.Participant) (int, error) {
			return 0, nil
		})
		require.NoError(t, err)
		assert.Empty(t, out)
	})
}
