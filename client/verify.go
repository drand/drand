package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/drand/drand/common"
	chain2 "github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/client"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/crypto"
)

type verifyingClient struct {
	// Client is the wrapped client. calls to `get` and `watch` return results proxied from this client's fetch
	client.Client

	// indirectClient is used to fetch other rounds of randomness needed for verification.
	// it is separated so that it can provide a cache or shared pool that the direct client may not.
	indirectClient client.Client

	pointOfTrust client.Result
	potLk        sync.Mutex
	strict       bool

	scheme *crypto.Scheme
	log    log.Logger
}

// newVerifyingClient wraps a client to perform `chain.Verify` on emitted results.
func newVerifyingClient(c client.Client, previousResult client.Result, strict bool, sch *crypto.Scheme) client.Client {
	return &verifyingClient{
		Client:         c,
		indirectClient: c,
		pointOfTrust:   previousResult,
		strict:         strict,
		scheme:         sch,
		log:            log.DefaultLogger(),
	}
}

// SetLog configures the client log output.
func (v *verifyingClient) SetLog(l log.Logger) {
	v.log = l.Named("verifyingClient")
}

// Get returns a requested round of randomness
func (v *verifyingClient) Get(ctx context.Context, round uint64) (client.Result, error) {
	info, err := v.indirectClient.Info(ctx)
	if err != nil {
		return nil, err
	}
	r, err := v.Client.Get(ctx, round)
	if err != nil {
		return nil, err
	}
	rd := asRandomData(r)
	if err := v.verify(ctx, info, rd); err != nil {
		return nil, err
	}
	return rd, nil
}

// Watch returns new randomness as it becomes available.
func (v *verifyingClient) Watch(ctx context.Context) <-chan client.Result {
	outCh := make(chan client.Result, 1)

	info, err := v.indirectClient.Info(ctx)
	if err != nil {
		v.log.Errorw("", "verifying_client", "could not get info", "err", err)
		close(outCh)
		return outCh
	}

	inCh := v.Client.Watch(ctx)
	go func() {
		defer close(outCh)
		for r := range inCh {
			if err := v.verify(ctx, info, asRandomData(r)); err != nil {
				v.log.Warnw("", "verifying_client", "skipping invalid watch round", "round", r.Round(), "err", err)
				continue
			}
			outCh <- r
		}
	}()
	return outCh
}

type resultWithPreviousSignature interface {
	PreviousSignature() []byte
}

func asRandomData(r client.Result) *RandomData {
	rd, ok := r.(*RandomData)
	if ok {
		return rd
	}
	rd = &RandomData{
		Rnd:    r.Round(),
		Random: r.Randomness(),
		Sig:    r.Signature(),
	}
	if rp, ok := r.(resultWithPreviousSignature); ok {
		rd.PreviousSignature = rp.PreviousSignature()
	}

	return rd
}

func (v *verifyingClient) getTrustedPreviousSignature(ctx context.Context, round uint64) ([]byte, error) {
	info, err := v.indirectClient.Info(ctx)
	if err != nil {
		v.log.Errorw("", "drand_client", "could not get info to verify round 1", "err", err)
		return []byte{}, fmt.Errorf("could not get info: %w", err)
	}

	if round == 1 {
		return info.GenesisSeed, nil
	}

	trustRound := uint64(1)
	var trustPrevSig []byte

	v.potLk.Lock()
	if v.pointOfTrust == nil || v.pointOfTrust.Round() > round {
		// slow path
		v.potLk.Unlock()
		trustPrevSig, err = v.getTrustedPreviousSignature(ctx, 1)
		if err != nil {
			return nil, err
		}
	} else {
		trustRound = v.pointOfTrust.Round()
		trustPrevSig = v.pointOfTrust.Signature()
		v.potLk.Unlock()
	}
	initialTrustRound := trustRound

	var next client.Result
	for trustRound < round-1 {
		trustRound++
		v.log.Warnw("", "verifying_client", "loading round to verify", "round", trustRound)
		next, err = v.indirectClient.Get(ctx, trustRound)
		if err != nil {
			return []byte{}, fmt.Errorf("could not get round %d: %w", trustRound, err)
		}
		b := &common.Beacon{
			PreviousSig: trustPrevSig,
			Round:       trustRound,
			Signature:   next.Signature(),
		}

		ipk := info.PublicKey.Clone()

		err = v.scheme.VerifyBeacon(b, ipk)
		if err != nil {
			v.log.Warnw("", "verifying_client", "failed to verify value", "b", b, "err", err)
			return []byte{}, fmt.Errorf("verifying beacon: %w", err)
		}
		trustPrevSig = next.Signature()
	}
	if trustRound == round-1 && trustRound > initialTrustRound {
		v.potLk.Lock()
		v.pointOfTrust = next
		v.potLk.Unlock()
	}

	if trustRound != round-1 {
		return []byte{}, fmt.Errorf("unexpected trust round %d", trustRound)
	}
	return trustPrevSig, nil
}

func (v *verifyingClient) verify(ctx context.Context, info *chain2.Info, r *RandomData) (err error) {
	fetchPrevSignature := v.strict // only useful for chained schemes
	ps := r.PreviousSignature

	if fetchPrevSignature {
		ps, err = v.getTrustedPreviousSignature(ctx, r.Round())
		if err != nil {
			return
		}
	}

	b := &common.Beacon{
		PreviousSig: ps, // for unchained schemes, this is not used in the VerifyBeacon function and can be nil
		Round:       r.Round(),
		Signature:   r.Signature(),
	}

	ipk := info.PublicKey.Clone()

	err = v.scheme.VerifyBeacon(b, ipk)
	if err != nil {
		return fmt.Errorf("verification of %v failed: %w", b, err)
	}

	r.Random = crypto.RandomnessFromSignature(r.Sig)
	return nil
}

// String returns the name of this client.
func (v *verifyingClient) String() string {
	return fmt.Sprintf("%s.(+verifier)", v.Client)
}
