package client

import (
	"bytes"
	"context"
	"time"

	"github.com/drand/drand/crypto"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/protobuf/drand"
)

func randomnessValidator(info *chain.Info, cache client.Cache, c *Client) pubsub.ValidatorEx {
	return func(ctx context.Context, p peer.ID, m *pubsub.Message) pubsub.ValidationResult {
		rand := &drand.PublicRandResponse{}
		err := proto.Unmarshal(m.Data, rand)
		if err != nil {
			return pubsub.ValidationReject
		}

		if info == nil {
			c.log.Warnw("", "gossip validator", "Not validating received randomness due to lack of trust root.")
			return pubsub.ValidationAccept
		}

		// Unwilling to relay beacons in the future.
		if time.Unix(chain.TimeOfRound(info.Period, info.GenesisTime, rand.GetRound()), 0).After(time.Now()) {
			return pubsub.ValidationReject
		}

		if cache != nil {
			if current := cache.TryGet(rand.GetRound()); current != nil {
				currentFull, ok := current.(*client.RandomData)
				if !ok {
					// Note: this shouldn't happen in practice, but if we have a
					// degraded cache entry we can't validate the full byte
					// sequence.
					if bytes.Equal(rand.GetSignature(), current.Signature()) {
						return pubsub.ValidationIgnore
					}
					return pubsub.ValidationReject
				}
				if current.Round() == rand.GetRound() &&
					bytes.Equal(current.Randomness(), rand.GetRandomness()) &&
					bytes.Equal(current.Signature(), rand.GetSignature()) &&
					bytes.Equal(currentFull.PreviousSignature, rand.GetPreviousSignature()) {
					return pubsub.ValidationIgnore
				}
				return pubsub.ValidationReject
			}
		}
		scheme, err := crypto.SchemeFromName(info.Scheme)
		if err != nil {
			return pubsub.ValidationReject
		}

		err = scheme.VerifyBeacon(rand, info.PublicKey)
		if err != nil {
			return pubsub.ValidationReject
		}
		return pubsub.ValidationAccept
	}
}
