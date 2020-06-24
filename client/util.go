package client

import (
	"bytes"
	"context"
	"fmt"
	"time"
)

// Verify validates each successive randomness value against the previous
// one, either from the chain genesis or starting at previously validated
// point in the chain. Validation of randomness provides a strogner threat
// model, by ensuring that even if the chain is compromised, the randomness
// generated folllows the expected protocol.
func Verify(ctx context.Context, c Client, previous Result) (current Result, err error) {
	start := uint64(1)
	if previous != nil {
		start = previous.Round()
	}

	expected := c.RoundAt(time.Now())
	var next Result
	for r := start; r < expected; r++ {
		next, err = c.Get(ctx, r)
		if err != nil {
			return
		}
		if r > 1 {
			if full, ok := next.(*RandomData); ok {
				if !bytes.Equal(full.PreviousSignature, previous.Signature()) {
					return nil, fmt.Errorf("signature inconsistant at round %d", r)
				}
			}
		}
		previous = next
	}
	current = next
	return
}
