package client

import (
	"bytes"
	"context"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/protobuf/drand"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"google.golang.org/protobuf/proto"
)

func randomnessValidator(info *chain.Info, cache client.Cache, c *Client) pubsub.ValidatorEx {
	return func(ctx context.Context, p peer.ID, m *pubsub.Message) pubsub.ValidationResult {
		var rand drand.PublicRandResponse
		err := proto.Unmarshal(m.Data, &rand)
		if err != nil {
			return pubsub.ValidationReject
		}

		if info == nil {
			c.log.Warn("gossip validator", "Not validating received randomness due to lack of trust root.")
			return pubsub.ValidationAccept
		}

		b := chain.Beacon{
			Round:       rand.GetRound(),
			Signature:   rand.GetSignature(),
			PreviousSig: rand.GetPreviousSignature(),
		}

		// Unwilling to relay beacons in the future.
		if time.Unix(chain.TimeOfRound(info.Period, info.GenesisTime, b.Round), 0).After(time.Now()) {
			return pubsub.ValidationReject
		}

		if cache != nil {
			if current := cache.TryGet(rand.GetRound()); current != nil {
				currentFull, ok := current.(*client.RandomData)
				if !ok {
					// Note: this shouldn't happen in practice, but if we have a
					// degraded cache entry we can't validate the full byte
					// sequence.
					if bytes.Equal(b.Signature, current.Signature()) {
						return pubsub.ValidationIgnore
					}
					return pubsub.ValidationReject
				}
				curB := chain.Beacon{
					Round:       current.Round(),
					Signature:   current.Signature(),
					PreviousSig: currentFull.PreviousSignature,
				}
				if b.Equal(&curB) {
					return pubsub.ValidationIgnore
				}
				return pubsub.ValidationReject
			}
		}

		if err := chain.VerifyBeacon(info.PublicKey, &b); err != nil {
			return pubsub.ValidationReject
		}
		return pubsub.ValidationAccept
	}
}
