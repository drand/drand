package lp2p

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
)

const (
	dnsResolveTimeout = 10 * time.Second
)

// resolveAddresses resolves addresses parallelly
func resolveAddresses(ctx context.Context, addrs []ma.Multiaddr, resolver *madns.Resolver) ([]peer.AddrInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, dnsResolveTimeout)
	defer cancel()

	if resolver == nil {
		resolver = madns.DefaultResolver
	}

	var maddrs []ma.Multiaddr
	var wg sync.WaitGroup
	resolveErrC := make(chan error, len(addrs))

	maddrC := make(chan ma.Multiaddr)

	for _, addr := range addrs {
		// check whether address ends in `ipfs/Qm...`
		if _, last := ma.SplitLast(addr); last.Protocol().Code == ma.P_IPFS {
			maddrs = append(maddrs, addr)
			continue
		}
		wg.Add(1)
		go func(maddr ma.Multiaddr) {
			defer wg.Done()
			raddrs, err := resolver.Resolve(ctx, maddr)
			if err != nil {
				resolveErrC <- fmt.Errorf("failed to resolve \"%s\": %w)", maddr, err)
				return
			}
			// filter out addresses that still doesn't end in `ipfs/Qm...`
			found := 0
			for _, raddr := range raddrs {
				if _, last := ma.SplitLast(raddr); last != nil && last.Protocol().Code == ma.P_P2P {
					maddrC <- raddr
					found++
				}
			}
			if found == 0 {
				resolveErrC <- fmt.Errorf("found no ipfs peers at %s", maddr)
			}
		}(addr)
	}
	go func() {
		wg.Wait()
		close(maddrC)
	}()

	for maddr := range maddrC {
		maddrs = append(maddrs, maddr)
	}

	select {
	case err := <-resolveErrC:
		return nil, err
	default:
	}
	return peer.AddrInfosFromP2pAddrs(maddrs...)
}
