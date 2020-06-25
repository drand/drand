package client

import (
	"context"
	"fmt"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
)

// newVerifyingClient wraps a client to perform `chain.Verify` on emitted results.
func newVerifyingClient(c Client, previousResult Result, strict bool) Client {
	return &verifyingClient{
		Client:         c,
		indirectClient: c,
		pointOfTrust:   previousResult,
		strict:         strict,
	}
}

type verifyingClient struct {
	// Client is the wrapped client. calls to `get` and `watch` return results proxied from this client's fetch
	Client

	// indirectClient is used to fetch other rounds of randomness needed for verification.
	// it is separated so that it can provide a cache or shared pool that the direct client may not.
	indirectClient Client

	pointOfTrust Result
	strict       bool

	log log.Logger
}

// SetLog configures the client log output.
func (v *verifyingClient) SetLog(l log.Logger) {
	v.log = l
}

// Get returns a requested round of randomness
func (v *verifyingClient) Get(ctx context.Context, round uint64) (Result, error) {
	info, err := v.Client.Info(ctx)
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
func (v *verifyingClient) Watch(ctx context.Context) <-chan Result {
	outCh := make(chan Result, 1)

	info, err := v.Client.Info(ctx)
	if err != nil {
		v.log.Error("drand_client", "could not get info", "err", err)
		close(outCh)
		return outCh
	}

	inCh := v.Client.Watch(ctx)
	go func() {
		defer close(outCh)
		for r := range inCh {
			if err := v.verify(ctx, info, asRandomData(r)); err != nil {
				continue
			}
			outCh <- r
		}
	}()
	return outCh
}

func asRandomData(r Result) *RandomData {
	rd, ok := r.(*RandomData)
	if ok {
		return rd
	}
	rd = &RandomData{
		Rnd:    r.Round(),
		Random: r.Randomness(),
		Sig:    r.Signature(),
	}
	return rd
}

func (v *verifyingClient) getTrustedPreviousSignature(ctx context.Context, round uint64) ([]byte, error) {
	info, err := v.Client.Info(ctx)
	if err != nil {
		v.log.Error("drand_client", "could not get info to verify round 1", "err", err)
		return []byte{}, fmt.Errorf("could not get info: %w", err)
	}

	if round == 1 {
		return info.GroupHash, nil
	}

	trustRound := v.pointOfTrust.Round()
	trustPrevSig := v.pointOfTrust.Signature()
	b := chain.Beacon{}
	if v.pointOfTrust.Round() > round {
		// slow path
		trustRound = 1
		trustPrevSig, err = v.getTrustedPreviousSignature(ctx, 1)
		if err != nil {
			return nil, err
		}
	}

	for trustRound < round-1 {
		trustRound++
		next, err := v.indirectClient.Get(ctx, trustRound)
		if err != nil {
			return []byte{}, fmt.Errorf("could not get round %d: %w", trustRound, err)
		}
		b.PreviousSig = trustPrevSig
		b.Round = trustRound
		b.Signature = next.Signature()
		if err := chain.VerifyBeacon(info.PublicKey, &b); err != nil {
			v.log.Warn("drand_client", "failed to verify value", "err", err)
			return []byte{}, fmt.Errorf("verifying beacon: %w", err)
		}
		trustPrevSig = next.Signature()
		if trustRound == round-1 && trustRound > v.pointOfTrust.Round() {
			v.pointOfTrust = next
		}
	}

	if trustRound != round-1 {
		return []byte{}, fmt.Errorf("unexpected trust round %d", trustRound)
	}
	return trustPrevSig, nil
}

func (v *verifyingClient) verify(ctx context.Context, info *chain.Info, r *RandomData) (err error) {
	ps := r.PreviousSignature
	if v.strict || r.PreviousSignature == nil {
		ps, err = v.getTrustedPreviousSignature(ctx, r.Round())
		if err != nil {
			return
		}
	}

	b := chain.Beacon{
		PreviousSig: ps,
		Round:       r.Round(),
		Signature:   r.Signature(),
	}

	if err = chain.VerifyBeacon(info.PublicKey, &b); err != nil {
		return
	}

	r.Random = chain.RandomnessFromSignature(r.Sig)
	return nil
}
