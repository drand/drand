package lp2p

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// SwarmBind ensures a persistent connection between host and peers by periodically connecting to them.
func SwarmBind(ctx context.Context, h host.Host, bindAddrs []ma.Multiaddr, period time.Duration) {
	go func() {
		for {
			t := time.NewTimer(time.Second * 5)
			select {
			case <-t.C:
				for _, ma := range bindAddrs {
					ai, err := peer.AddrInfoFromP2pAddr(ma)
					if err != nil {
						fmt.Println("invalid p2p address", err)
						continue
					}
					err = h.Connect(ctx, *ai)
					if err != nil {
						fmt.Printf("failed to connect to %v\n", ma)
						continue
					}
				}
			case <-ctx.Done():
				t.Stop()
				return
			}
		}
	}()
}
